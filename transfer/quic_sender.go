package transfer

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/xjwm5685-ui/turbodrop/models"
	"github.com/zeebo/blake3"
)

// QUICFileSender QUIC 文件发送器
type QUICFileSender struct {
	filePath         string
	displayName      string
	expectedCertFP   string
	targetAddr       string
	chunkSize        int64
	maxStreams       int
	conn             quic.Connection
	bytesTransferred atomic.Int64
	totalBytes       int64
	startTime        time.Time
	stateManager     *StateManager  // 接收端的状态（用于跳过已完成的块）
	simulateFailure  bool           // 是否模拟失败（测试用）
	failureAt        float64        // 在多少百分比时失败
}

// GetBytesTransferred 获取已传输字节数（用于 API 监控）
func (s *QUICFileSender) GetBytesTransferred() int64 {
	return s.bytesTransferred.Load()
}

// GetTotalBytes 获取总字节数（用于 API 监控）
func (s *QUICFileSender) GetTotalBytes() int64 {
	return s.totalBytes
}

// NewQUICFileSender 创建文件发送器
func NewQUICFileSender(filePath string, targetIP string, targetPort int) *QUICFileSender {
	cfg := GetRuntimeConfig()
	return &QUICFileSender{
		filePath:        filePath,
		displayName:     "",
		targetAddr:      fmt.Sprintf("%s:%d", targetIP, targetPort),
		chunkSize:       cfg.ChunkSizeBytes,
		maxStreams:      cfg.MaxConcurrentStreams,
		simulateFailure: false,
		failureAt:       0.5, // 默认50%时失败
	}
}

// SetDisplayName 设置发送给接收端时使用的文件名
func (s *QUICFileSender) SetDisplayName(name string) {
	s.displayName = name
}

func (s *QUICFileSender) SetExpectedCertFingerprint(fingerprint string) {
	s.expectedCertFP = fingerprint
}

// EnableFailureSimulation 启用失败模拟（测试用）
func (s *QUICFileSender) EnableFailureSimulation(percentage float64) {
	s.simulateFailure = true
	s.failureAt = percentage
	fmt.Printf("⚠️  [测试模式] 将在 %.0f%% 时模拟传输中断\n\n", percentage*100)
}

// Send 发送文件
func (s *QUICFileSender) Send(ctx context.Context) error {
	fmt.Println("🚀 启动 QUIC 文件传输...")

	// 1. 打开文件并计算哈希
	file, err := os.Open(s.filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %w", err)
	}
	s.totalBytes = fileInfo.Size()

	displayName := fileInfo.Name()
	if s.displayName != "" {
		displayName = s.displayName
	}

	fmt.Printf("📄 文件: %s\n", displayName)
	fmt.Printf("📊 大小: %.2f MB\n", float64(s.totalBytes)/(1024*1024))

	// 计算 BLAKE3 哈希
	fmt.Println("🔐 计算文件哈希...")
	hash, err := s.calculateBlake3Hash(file)
	if err != nil {
		return fmt.Errorf("计算哈希失败: %w", err)
	}
	fmt.Printf("✅ BLAKE3: %s\n\n", hash)

	// 2. 建立 QUIC 连接
	fmt.Printf("🔗 连接到 %s...\n", s.targetAddr)
	tlsConfig := GenerateClientTLSConfig(s.expectedCertFP)
	
	conn, err := quic.DialAddr(ctx, s.targetAddr, tlsConfig, &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("QUIC 连接失败: %w", err)
	}
	s.conn = conn
	defer conn.CloseWithError(0, "transfer complete")

	fmt.Println("✅ QUIC 连接建立成功")
	fmt.Println()

	// 3. 发送文件元数据（控制流）
	totalChunks := int((s.totalBytes + s.chunkSize - 1) / s.chunkSize)
	metadata := models.FileMetadata{
		FileName:    displayName,
		FileSize:    s.totalBytes,
		ChunkSize:   s.chunkSize,
		TotalChunks: totalChunks,
		Blake3Hash:  hash,
	}

	if err := s.sendMetadata(ctx, metadata); err != nil {
		return fmt.Errorf("发送元数据失败: %w", err)
	}

	// 4. 启动进度监控
	progressCtx, progressCancel := context.WithCancel(ctx)
	defer progressCancel()
	go s.monitorProgress(progressCtx)

	// 5. 并发发送所有 Chunks
	s.startTime = time.Now()
	if err := s.sendChunksConcurrently(ctx, file, totalChunks); err != nil {
		return fmt.Errorf("发送文件块失败: %w", err)
	}

	elapsed := time.Since(s.startTime)
	avgSpeed := float64(s.totalBytes) / elapsed.Seconds() / (1024 * 1024)

	fmt.Printf("\n✅ 传输完成！\n")
	fmt.Printf("⏱️  耗时: %.2f 秒\n", elapsed.Seconds())
	fmt.Printf("⚡ 平均速度: %.2f MB/s\n", avgSpeed)

	return nil
}

// sendMetadata 发送文件元数据并接收续传信息
func (s *QUICFileSender) sendMetadata(ctx context.Context, metadata models.FileMetadata) error {
	stream, err := s.conn.OpenStreamSync(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()

	// 发送 JSON 元数据
	encoder := json.NewEncoder(stream)
	if err := encoder.Encode(metadata); err != nil {
		return err
	}

	fmt.Println("📤 元数据已发送")
	fmt.Printf("   总块数: %d\n", metadata.TotalChunks)
	fmt.Printf("   块大小: %.2f MB\n\n", float64(s.chunkSize)/(1024*1024))

	// 接收续传请求
	var resumeReq models.ResumeRequest
	decoder := json.NewDecoder(stream)
	if err := decoder.Decode(&resumeReq); err != nil {
		return err
	}

	if resumeReq.Resume {
		fmt.Println("🔄 接收端请求断点续传！")
		fmt.Printf("   消息: %s\n", resumeReq.Message)

		// 创建状态管理器并加载 Bitset
		s.stateManager = NewStateManager("", metadata)
		if err := s.stateManager.SetBitsetFromBytes(resumeReq.CompletedMap); err != nil {
			return fmt.Errorf("解析续传位图失败: %w", err)
		}

		completed := s.stateManager.GetCompletedCount()
		percentage := s.stateManager.GetCompletionPercentage()
		missing := metadata.TotalChunks - completed

		fmt.Printf("   已完成: %d/%d 块 (%.1f%%)\n", completed, metadata.TotalChunks, percentage)
		fmt.Printf("   待传输: %d 块\n", missing)
		fmt.Println("   将跳过已完成的块")
		fmt.Println()

		// 初始化已传输字节数
		s.bytesTransferred.Store(int64(completed) * metadata.ChunkSize)
	} else {
		fmt.Println("✅ 接收端已就绪（新传输）")
		fmt.Println()
	}

	return nil
}

// sendChunksConcurrently 并发发送所有文件块
func (s *QUICFileSender) sendChunksConcurrently(ctx context.Context, file *os.File, totalChunks int) error {
	// 创建协程池（信号量）
	semaphore := make(chan struct{}, s.maxStreams)
	var wg sync.WaitGroup
	errChan := make(chan error, totalChunks)

	// 统计跳过的块数
	skippedCount := 0
	if s.stateManager != nil {
		skippedCount = s.stateManager.GetCompletedCount()
	}

	if skippedCount > 0 {
		fmt.Printf("⏭️  跳过 %d 个已完成的块\n", skippedCount)
	}
	fmt.Printf("⚡ 开启 %d 个并发流传输...\n\n", s.maxStreams)

	for chunkIndex := 0; chunkIndex < totalChunks; chunkIndex++ {
		// 检查是否已完成（断点续传）
		if s.stateManager != nil && s.stateManager.IsChunkComplete(chunkIndex) {
			continue // 跳过已完成的块
		}

		wg.Add(1)
		semaphore <- struct{}{} // 获取信号量

		go func(index int) {
			defer wg.Done()
			defer func() { <-semaphore }() // 释放信号量

			// 模拟失败（测试用）
			if s.simulateFailure {
				transferred := s.bytesTransferred.Load()
				percentage := float64(transferred) / float64(s.totalBytes)
				if percentage >= s.failureAt {
					errChan <- fmt.Errorf("模拟传输中断于 %.0f%%", percentage*100)
					return
				}
			}

			if err := s.sendChunk(ctx, file, index); err != nil {
				errChan <- fmt.Errorf("块 %d 发送失败: %w", index, err)
			}
		}(chunkIndex)
	}

	// 等待所有协程完成
	wg.Wait()
	close(errChan)

	// 检查错误
	var errs []error
	for err := range errChan {
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// sendChunk 发送单个文件块
func (s *QUICFileSender) sendChunk(ctx context.Context, file *os.File, chunkIndex int) error {
	// 打开单向流
	stream, err := s.conn.OpenUniStreamSync(ctx)
	if err != nil {
		return err
	}

	// 计算偏移量和读取大小
	offset := int64(chunkIndex) * s.chunkSize
	size := s.chunkSize
	if offset+size > s.totalBytes {
		size = s.totalBytes - offset
	}

	// 写入 8 字节的偏移量头部
	header := make([]byte, 8)
	binary.BigEndian.PutUint64(header, uint64(offset))
	if _, err := stream.Write(header); err != nil {
		return err
	}

	// 读取文件块（使用 ReadAt 保证并发安全）
	buffer := make([]byte, size)
	n, err := file.ReadAt(buffer, offset)
	if err != nil && err != io.EOF {
		return err
	}

	// 写入文件块数据
	if _, err := stream.Write(buffer[:n]); err != nil {
		return err
	}

	if err := stream.Close(); err != nil {
		return err
	}

	// 更新已传输字节数
	s.bytesTransferred.Add(int64(n))

	return nil
}

// monitorProgress 监控传输进度
func (s *QUICFileSender) monitorProgress(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastBytes := int64(0)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			transferred := s.bytesTransferred.Load()
			percentage := float64(transferred) / float64(s.totalBytes) * 100
			
			// 计算瞬时速度
			delta := transferred - lastBytes
			speedMBps := float64(delta) / (1024 * 1024)
			lastBytes = transferred

			fmt.Printf("\r📊 进度: %.1f%% | 速度: %.2f MB/s | 已传输: %.2f/%.2f MB    ",
				percentage,
				speedMBps,
				float64(transferred)/(1024*1024),
				float64(s.totalBytes)/(1024*1024))
		}
	}
}

// calculateBlake3Hash 计算文件的 BLAKE3 哈希
func (s *QUICFileSender) calculateBlake3Hash(file *os.File) (string, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return "", fmt.Errorf("重置文件指针失败: %w", err)
	}
	hasher := blake3.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return "", fmt.Errorf("重置文件指针失败: %w", err)
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}
