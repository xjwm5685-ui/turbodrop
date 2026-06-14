package transfer

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/xjwm5685-ui/turbodrop/models"
	"github.com/zeebo/blake3"
)

// QUICFileReceiver QUIC 文件接收器
type QUICFileReceiver struct {
	listenAddr        string
	saveDir           string
	tlsConfig         *tls.Config
	authToken         string          // PIN 握手认证令牌
	listener          *quic.Listener
	bytesReceived     atomic.Int64
	totalBytes        int64
	metadata          models.FileMetadata
	file              *os.File
	startTime         time.Time
	writeMutex        sync.Mutex      // 保护文件写入
	stateManager      *StateManager   // 状态管理器
	resuming          bool            // 是否正在断点续传
	certFingerprint   string          // 实际使用的证书指纹
	streamAcceptTimeout time.Duration // 每次 AcceptUniStream 的超时时间
}

// defaultStreamAcceptTimeout 单个流等待超时
const defaultStreamAcceptTimeout = 30 * time.Second

// NewQUICFileReceiver 创建文件接收器
// listenPort 传 0 表示由系统分配随机可用端口
func NewQUICFileReceiver(listenPort int, saveDir string, tlsConfig *tls.Config, authToken string) *QUICFileReceiver {
	return &QUICFileReceiver{
		listenAddr:          fmt.Sprintf("0.0.0.0:%d", listenPort),
		saveDir:             saveDir,
		tlsConfig:           tlsConfig,
		authToken:           authToken,
		streamAcceptTimeout: defaultStreamAcceptTimeout,
	}
}

// GetListenAddr 返回实际监听的地址（端口可能由系统分配）
func (r *QUICFileReceiver) GetListenAddr() string {
	if r.listener != nil {
		return r.listener.Addr().String()
	}
	return r.listenAddr
}

// GetCertFingerprint 返回当前使用的 TLS 证书指纹
func (r *QUICFileReceiver) GetCertFingerprint() string {
	return r.certFingerprint
}

// Start 启动接收器监听
func (r *QUICFileReceiver) Start(ctx context.Context) error {
	tlsConfig := r.tlsConfig
	if tlsConfig == nil {
		var err error
		var fp string
		tlsConfig, fp, err = GenerateEphemeralTLSConfig()
		if err != nil {
			return fmt.Errorf("生成 TLS 证书失败: %w", err)
		}
		r.certFingerprint = fp
	}

	// 启动 QUIC 监听器（支持传 0 让系统分配端口）
	listener, err := quic.ListenAddr(r.listenAddr, tlsConfig, &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("QUIC 监听失败: %w", err)
	}
	r.listener = listener
	r.listenAddr = listener.Addr().String()

	fmt.Printf("📡 QUIC 接收端已启动，监听 %s\n", r.listenAddr)
	fmt.Println("⏳ 等待发送端连接...")
	fmt.Println()

	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("接受连接失败: %w", err)
		}

		fmt.Println("✅ 发送端已连接")
		fmt.Println()

		r.resetSessionState()
		if err := r.handleConnection(ctx, conn); err != nil {
			fmt.Printf("⚠️  单次传输失败: %v\n", err)
		}

		fmt.Println("⏳ 等待下一次发送端连接...")
		fmt.Println()
	}
}

func (r *QUICFileReceiver) resetSessionState() {
	r.bytesReceived.Store(0)
	r.totalBytes = 0
	r.metadata = models.FileMetadata{}
	r.file = nil
	r.startTime = time.Time{}
	r.stateManager = nil
	r.resuming = false
}

// handleConnection 处理单个 QUIC 连接
func (r *QUICFileReceiver) handleConnection(ctx context.Context, conn quic.Connection) error {
	defer conn.CloseWithError(0, "transfer complete")

	// 1. 接收元数据（控制流）
	if err := r.receiveMetadata(ctx, conn); err != nil {
		return fmt.Errorf("接收元数据失败: %w", err)
	}

	// 2. 创建目标文件并预分配空间
	if err := r.prepareFile(); err != nil {
		return fmt.Errorf("准备文件失败: %w", err)
	}
	defer r.file.Close()

	// 3. 启动进度监控
	progressCtx, progressCancel := context.WithCancel(ctx)
	defer progressCancel()
	go r.monitorProgress(progressCtx)

	// 4. 并发接收所有 Chunks
	r.startTime = time.Now()
	if err := r.receiveChunksConcurrently(ctx, conn); err != nil {
		return fmt.Errorf("接收文件块失败: %w", err)
	}

	elapsed := time.Since(r.startTime)
	avgSpeed := float64(r.totalBytes) / elapsed.Seconds() / (1024 * 1024)

	fmt.Printf("\n✅ 接收完成！\n")
	fmt.Printf("⏱️  耗时: %.2f 秒\n", elapsed.Seconds())
	fmt.Printf("⚡ 平均速度: %.2f MB/s\n\n", avgSpeed)

	// 5. 验证 BLAKE3 哈希
	if err := r.verifyHash(); err != nil {
		return fmt.Errorf("哈希验证失败: %w", err)
	}

	// 6. 删除状态文件
	if err := r.stateManager.DeleteState(); err != nil {
		fmt.Printf("⚠️  删除状态文件失败: %v\n", err)
	}

	fmt.Println("🎉 文件传输成功且完整性校验通过！")

	return nil
}

// receiveMetadata 接收文件元数据并协商断点续传
func (r *QUICFileReceiver) receiveMetadata(ctx context.Context, conn quic.Connection) error {
	// 接受控制流
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()

	// 读取 JSON 元数据
	decoder := json.NewDecoder(stream)
	if err := decoder.Decode(&r.metadata); err != nil {
		return err
	}

	// S-1: 验证认证令牌
	if r.authToken != "" && r.metadata.AuthToken != r.authToken {
		return fmt.Errorf("认证失败：发送端令牌不匹配")
	}

	// S-2: 验证元数据边界
	if err := validateMetadata(&r.metadata); err != nil {
		return fmt.Errorf("元数据校验失败: %w", err)
	}

	r.totalBytes = r.metadata.FileSize

	fmt.Println("📥 收到文件元数据:")
	fmt.Printf("   文件名: %s\n", r.metadata.FileName)
	fmt.Printf("   大小: %.2f MB\n", float64(r.totalBytes)/(1024*1024))
	fmt.Printf("   总块数: %d\n", r.metadata.TotalChunks)
	fmt.Printf("   哈希: %s\n\n", r.metadata.Blake3Hash)

	// 检查是否存在未完成的传输
	filePath, err := r.resolveTargetFilePath(r.metadata.FileName)
	if err != nil {
		return err
	}
	
	var resumeReq models.ResumeRequest

	if CheckExists(filePath) {
		fmt.Println("🔄 发现历史传输记录！")
		
		// 加载状态
		sm, err := LoadState(filePath)
		if err != nil {
			fmt.Printf("⚠️  加载状态失败: %v，将重新开始传输\n", err)
			resumeReq = models.ResumeRequest{
				Resume:  false,
				Message: "状态文件损坏，重新传输",
			}
		} else {
			// 验证元数据是否匹配
			if sm.metadata.Blake3Hash != r.metadata.Blake3Hash ||
			   sm.metadata.FileSize != r.metadata.FileSize {
				fmt.Println("⚠️  文件已变更，将重新开始传输")
				os.Remove(filePath + ".turbo")
				resumeReq = models.ResumeRequest{
					Resume:  false,
					Message: "文件已变更",
				}
			} else {
				r.stateManager = sm
				r.resuming = true
				
				completed := sm.GetCompletedCount()
				percentage := sm.GetCompletionPercentage()
				
				fmt.Printf("   已完成: %d/%d 块 (%.1f%%)\n", 
					completed, r.metadata.TotalChunks, percentage)
				fmt.Println("   将跳过已完成的块，继续传输")
				fmt.Println()
				
				// 初始化已接收字节数（按实际完成的块大小求和）
				r.bytesReceived.Store(sm.GetCompletedBytes(r.metadata.FileSize, r.metadata.ChunkSize))

				resumeReq = models.ResumeRequest{
					Resume:       true,
					CompletedMap: sm.GetBitsetBytes(),
					Message:      fmt.Sprintf("续传，已完成 %.1f%%", percentage),
				}
			}
		}
	} else {
		resumeReq = models.ResumeRequest{
			Resume:  false,
			Message: "新传输",
		}
	}

	// 发送续传请求/ACK
	encoder := json.NewEncoder(stream)
	if err := encoder.Encode(resumeReq); err != nil {
		return err
	}

	if resumeReq.Resume {
		fmt.Println("✅ 断点续传协商完成")
		fmt.Println()
	} else {
		fmt.Println("✅ 接收端已就绪")
		fmt.Println()
	}

	return nil
}

// prepareFile 创建文件并预分配空间（或打开已存在的文件）
func (r *QUICFileReceiver) prepareFile() error {
	filePath, err := r.resolveTargetFilePath(r.metadata.FileName)
	if err != nil {
		return err
	}
	
	var file *os.File

	if r.resuming {
		// 断点续传：打开已存在的文件
		file, err = os.OpenFile(filePath, os.O_RDWR, 0644)
		if err != nil {
			return fmt.Errorf("打开已存在文件失败: %w", err)
		}
		fmt.Printf("📝 打开已存在文件: %s\n", filePath)
	} else {
		// 新传输：创建文件
		file, err = os.Create(filePath)
		if err != nil {
			return err
		}

		// 预分配文件空间
		if err := file.Truncate(r.metadata.FileSize); err != nil {
			file.Close()
			return err
		}

		// 创建状态管理器
		r.stateManager = NewStateManager(filePath, r.metadata)
		if err := r.stateManager.SaveState(); err != nil {
			file.Close()
			return fmt.Errorf("创建状态文件失败: %w", err)
		}

		fmt.Printf("📝 文件已创建: %s\n", filePath)
		fmt.Printf("💾 预分配空间: %.2f MB\n", float64(r.metadata.FileSize)/(1024*1024))
	}

	r.file = file
	fmt.Println()

	return nil
}

// receiveChunksConcurrently 并发接收所有文件块
func (r *QUICFileReceiver) receiveChunksConcurrently(ctx context.Context, conn quic.Connection) error {
	expectedStreams := r.metadata.TotalChunks
	if r.stateManager != nil {
		expectedStreams = r.metadata.TotalChunks - r.stateManager.GetCompletedCount()
	}
	if expectedStreams <= 0 {
		// 确保最终状态已落盘（空文件场景）
		if r.stateManager != nil {
			_ = r.stateManager.Flush()
		}
		return nil
	}

	var wg sync.WaitGroup
	errChan := make(chan error, expectedStreams)

	fmt.Println("⚡ 开始并发接收数据流...")
	fmt.Println()

	// 接收所有单向流，每次 Accept 有独立超时
	for i := 0; i < expectedStreams; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			acceptCtx, acceptCancel := context.WithTimeout(ctx, r.streamAcceptTimeout)
			defer acceptCancel()

			stream, err := conn.AcceptUniStream(acceptCtx)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					errChan <- fmt.Errorf("接收超时或发送端断开: %w", err)
				} else {
					errChan <- err
				}
				return
			}

			if err := r.receiveChunk(stream); err != nil {
				errChan <- err
			}
		}()
	}

	// 等待所有协程完成
	wg.Wait()
	close(errChan)

	// 最终刷新一次状态文件（确保最后一批 NoSave 的块落盘）
	if r.stateManager != nil {
		_ = r.stateManager.Flush()
	}

	// 检查错误
	var errs []error
	for err := range errChan {
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// receiveChunk 接收单个文件块
func (r *QUICFileReceiver) receiveChunk(stream quic.ReceiveStream) error {
	// 读取 8 字节的偏移量头部
	header := make([]byte, 8)
	if _, err := io.ReadFull(stream, header); err != nil {
		return err
	}
	offset := int64(binary.BigEndian.Uint64(header))

	// 边界校验：offset 必须在文件范围内（0 字节文件允许 offset=0）
	if r.metadata.FileSize > 0 && (offset < 0 || offset >= r.metadata.FileSize) {
		io.Copy(io.Discard, stream)
		return fmt.Errorf("偏移量越界: offset=%d, fileSize=%d", offset, r.metadata.FileSize)
	}

	// 计算块索引
	chunkIndex := int(offset / r.metadata.ChunkSize)

	// 如果该块已完成，跳过（防止重复接收）
	if r.stateManager.IsChunkComplete(chunkIndex) {
		// 仍然需要读取数据，但不写入
		io.Copy(io.Discard, stream)
		return nil
	}

	// 限制读取大小：不超过 ChunkSize 且不超过文件剩余空间
	maxRead := r.metadata.ChunkSize
	remaining := r.metadata.FileSize - offset
	if maxRead > remaining {
		maxRead = remaining
	}

	// 使用 LimitReader 防止单块读入超大内存
	data, err := io.ReadAll(io.LimitReader(stream, maxRead))
	if err != nil {
		return err
	}

	if int64(len(data)) > maxRead {
		return fmt.Errorf("数据块超大: got %d bytes, max %d", len(data), maxRead)
	}

	// 并发安全地写入文件
	r.writeMutex.Lock()
	_, err = r.file.WriteAt(data, offset)
	r.writeMutex.Unlock()

	if err != nil {
		return err
	}

	// 标记该块为已完成，但不每次写盘；每 10 个块或传输结束时统一 Flush
	r.stateManager.MarkChunkCompleteNoSave(chunkIndex)
	if (chunkIndex+1)%10 == 0 {
		if err := r.stateManager.Flush(); err != nil {
			fmt.Printf("⚠️  保存状态失败: %v\n", err)
		}
	}

	// 更新已接收字节数
	r.bytesReceived.Add(int64(len(data)))

	return nil
}

// monitorProgress 监控接收进度
func (r *QUICFileReceiver) monitorProgress(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastBytes := int64(0)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			received := r.bytesReceived.Load()
			percentage := 100.0
			if r.totalBytes > 0 {
				percentage = float64(received) / float64(r.totalBytes) * 100
			}

			// 计算瞬时速度
			delta := received - lastBytes
			speedMBps := float64(delta) / (1024 * 1024)
			lastBytes = received

			fmt.Printf("\r📊 进度: %.1f%% | 速度: %.2f MB/s | 已接收: %.2f/%.2f MB    ",
				percentage,
				speedMBps,
				float64(received)/(1024*1024),
				float64(r.totalBytes)/(1024*1024))
		}
	}
}

// verifyHash 验证文件 BLAKE3 哈希
func (r *QUICFileReceiver) verifyHash() error {
	fmt.Println("🔐 验证文件完整性...")

	if _, err := r.file.Seek(0, 0); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}
	hasher := blake3.New()
	if _, err := io.Copy(hasher, r.file); err != nil {
		return err
	}

	actualHash := fmt.Sprintf("%x", hasher.Sum(nil))

	fmt.Printf("   预期哈希: %s\n", r.metadata.Blake3Hash)
	fmt.Printf("   实际哈希: %s\n", actualHash)

	if actualHash != r.metadata.Blake3Hash {
		return fmt.Errorf("哈希不匹配！文件可能损坏")
	}

	fmt.Println("   ✅ 哈希验证通过")
	fmt.Println()
	return nil
}

// Stop 停止接收器
func (r *QUICFileReceiver) Stop() error {
	if r.listener != nil {
		return r.listener.Close()
	}
	return nil
}

// validateMetadata 校验文件元数据边界，防止恶意客户端触发 panic 或 OOM
func validateMetadata(m *models.FileMetadata) error {
	if m.ChunkSize <= 0 {
		return fmt.Errorf("ChunkSize 必须大于 0，收到: %d", m.ChunkSize)
	}
	if m.TotalChunks <= 0 {
		return fmt.Errorf("TotalChunks 必须大于 0，收到: %d", m.TotalChunks)
	}
	if m.FileSize < 0 {
		return fmt.Errorf("FileSize 不能为负数，收到: %d", m.FileSize)
	}

	// 0 字节文件允许 TotalChunks == 1
	if m.FileSize == 0 {
		if m.TotalChunks != 1 {
			return fmt.Errorf("0 字节文件的 TotalChunks 必须为 1，收到: %d", m.TotalChunks)
		}
		return nil
	}

	// 检查 TotalChunks 与 FileSize/ChunkSize 的一致性（允许 ±1 的余数差异）
	expectedChunks := int((m.FileSize + m.ChunkSize - 1) / m.ChunkSize)
	if m.TotalChunks > expectedChunks+1 {
		return fmt.Errorf("TotalChunks (%d) 与文件大小不匹配，预期约 %d", m.TotalChunks, expectedChunks)
	}
	if m.TotalChunks > 1<<24 { // 16M chunks max
		return fmt.Errorf("TotalChunks 过大: %d (上限 %d)", m.TotalChunks, 1<<24)
	}
	return nil
}

func (r *QUICFileReceiver) resolveTargetFilePath(fileName string) (string, error) {
	baseName := filepath.Base(filepath.Clean(fileName))
	if baseName == "." || baseName == string(filepath.Separator) || baseName == "" {
		return "", fmt.Errorf("非法文件名")
	}

	rootAbs, err := filepath.Abs(r.saveDir)
	if err != nil {
		return "", fmt.Errorf("解析保存目录失败: %w", err)
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, baseName))
	if err != nil {
		return "", fmt.Errorf("解析目标文件路径失败: %w", err)
	}

	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", fmt.Errorf("校验目标文件路径失败: %w", err)
	}
	if rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator) {
		return "", fmt.Errorf("目标文件路径超出保存目录")
	}
	return targetAbs, nil
}
