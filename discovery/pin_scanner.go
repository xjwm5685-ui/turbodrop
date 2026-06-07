package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/xjwm5685-ui/turbodrop/models"
)

const (
	// UDPPort PIN 码发现的默认 UDP 端口
	UDPPort = 8899
	// ScanTimeout 子网扫描的默认超时时间
	ScanTimeout = 5 * time.Second
	probeTypeHello = "hello"
	probeTypeProof = "proof"
	responseTypeChallenge = "challenge"
	responseTypeResult    = "result"
)

// PINReceiver 接收端：生成 PIN 码并监听 UDP 连接
type PINReceiver struct {
	device      models.Device
	pin         string
	sessionSalt string
	listener    *net.UDPConn
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewPINReceiver 创建一个新的接收端实例
func NewPINReceiver(deviceName string, quicPort int, certFingerprint string) (*PINReceiver, error) {
	localIP, err := GetLocalIP()
	if err != nil {
		return nil, fmt.Errorf("获取本机 IP 失败: %w", err)
	}

	pin := GeneratePIN()
	sessionSalt := GenerateDeviceID()

	device := models.Device{
		ID:              GenerateDeviceID(),
		Name:            deviceName,
		IP:              localIP,
		QUICPort:        quicPort,
		Platform:        runtime.GOOS,
		CertFingerprint: certFingerprint,
		DiscoveryAt:     time.Now(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &PINReceiver{
		device:      device,
		pin:         pin,
		sessionSalt: sessionSalt,
		ctx:         ctx,
		cancel:      cancel,
	}, nil
}

// GetPIN 获取生成的 PIN 码
func (r *PINReceiver) GetPIN() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pin
}

// Start 启动接收端监听器
func (r *PINReceiver) Start() error {
	addr := &net.UDPAddr{
		Port: UDPPort,
		IP:   net.ParseIP("0.0.0.0"),
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("监听 UDP 端口失败: %w", err)
	}

	r.listener = conn
	fmt.Printf("✅ 接收端已启动，PIN 码: %s\n", r.pin)
	fmt.Printf("📡 监听端口: %d\n", UDPPort)
	fmt.Printf("🔍 等待探测包...\n\n")

	go r.handleIncoming()
	return nil
}

// handleIncoming 处理传入的 UDP 探测包
func (r *PINReceiver) handleIncoming() {
	buffer := make([]byte, 4096)

	for {
		select {
		case <-r.ctx.Done():
			return
		default:
			// 设置读取超时，避免阻塞
			r.listener.SetReadDeadline(time.Now().Add(1 * time.Second))
			
			n, remoteAddr, err := r.listener.ReadFromUDP(buffer)
			if err != nil {
				// 超时错误不打印，继续等待
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				fmt.Printf("❌ 读取 UDP 数据失败: %v\n", err)
				continue
			}

			// 解析探测包
			var probe models.PINProbe
			if err := json.Unmarshal(buffer[:n], &probe); err != nil {
				fmt.Printf("⚠️  解析探测包失败: %v\n", err)
				continue
			}

			switch probe.Type {
			case probeTypeHello:
				response := models.PINResponse{
					Type:        responseTypeChallenge,
					SessionSalt: r.sessionSalt,
					Timestamp:   time.Now().Unix(),
				}
				if err := r.writeResponse(remoteAddr, response); err != nil {
					fmt.Printf("❌ 发送会话盐失败: %v\n", err)
				}

			case probeTypeProof:
				if probe.PINHash != HashPIN(r.pin, r.sessionSalt) {
					fmt.Printf("⚠️  收到错误的 PIN 哈希，忽略\n")
					continue
				}

				fmt.Printf("✨ PIN 码匹配成功！来自: %s\n", remoteAddr.IP.String())
				response := models.PINResponse{
					Type:      responseTypeResult,
					Device:    r.device,
					Success:   true,
					Timestamp: time.Now().Unix(),
				}
				if err := r.writeResponse(remoteAddr, response); err != nil {
					fmt.Printf("❌ 发送响应失败: %v\n", err)
					continue
				}

				fmt.Printf("📤 已向 %s 发送设备信息\n", remoteAddr.IP.String())
			}
		}
	}
}

func (r *PINReceiver) writeResponse(remoteAddr *net.UDPAddr, response models.PINResponse) error {
	responseData, err := json.Marshal(response)
	if err != nil {
		return err
	}

	_, err = r.listener.WriteToUDP(responseData, remoteAddr)
	return err
}

// Stop 停止接收端
func (r *PINReceiver) Stop() {
	r.cancel()
	if r.listener != nil {
		r.listener.Close()
	}
	fmt.Println("🛑 接收端已停止")
}

// PINScanner 请求端：通过 PIN 码进行子网并发扫描
type PINScanner struct {
	deviceID   string
	localIP    string
	ctx        context.Context
	cancel     context.CancelFunc
	foundChan  chan models.Device
	workerPool chan struct{} // 信号量，控制并发数
}

// NewPINScanner 创建一个新的扫描端实例
func NewPINScanner() (*PINScanner, error) {
	return NewPINScannerWithContext(context.Background())
}

// NewPINScannerWithContext 创建一个绑定父上下文的新扫描端实例
func NewPINScannerWithContext(parent context.Context) (*PINScanner, error) {
	localIP, err := GetLocalIP()
	if err != nil {
		return nil, fmt.Errorf("获取本机 IP 失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(parent, ScanTimeout)

	return &PINScanner{
		deviceID:   GenerateDeviceID(),
		localIP:    localIP,
		ctx:        ctx,
		cancel:     cancel,
		foundChan:  make(chan models.Device, 10),
		workerPool: make(chan struct{}, 50), // 限制最大 50 个并发协程
	}, nil
}

// Scan 开始扫描子网，寻找匹配 PIN 码的设备
func (s *PINScanner) Scan(pin string) (*models.Device, error) {
	// 获取子网内所有 IP
	ips, err := GetSubnetIPs(s.localIP)
	if err != nil {
		return nil, fmt.Errorf("获取子网 IP 列表失败: %w", err)
	}

	fmt.Printf("🔍 开始扫描子网，共 %d 个 IP...\n", len(ips))
	fmt.Printf("⚡ 使用并发协程池，最大并发数: 50\n\n")

	var wg sync.WaitGroup
	
	// 为每个 IP 启动一个扫描协程
	for _, ip := range ips {
		wg.Add(1)
		
		go func(targetIP string) {
			defer wg.Done()

			// 获取信号量（控制并发数）
			s.workerPool <- struct{}{}
			defer func() { <-s.workerPool }()

			// 检查上下文是否已取消
			select {
			case <-s.ctx.Done():
				return
			default:
			}

			// 发送探测包
			s.probeIP(targetIP, pin)
		}(ip)
	}

	// 等待所有协程完成或超时
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case device := <-s.foundChan:
		s.cancel() // 找到设备后立即取消其他协程
		return &device, nil
	case <-done:
		return nil, fmt.Errorf("扫描完成，未找到匹配的设备")
	case <-s.ctx.Done():
		return nil, fmt.Errorf("扫描超时")
	}
}

// probeIP 向指定 IP 发送 UDP 探测包
func (s *PINScanner) probeIP(targetIP string, pin string) {
	// 创建 UDP 连接
	conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:%d", targetIP, UDPPort), 500*time.Millisecond)
	if err != nil {
		return // 连接失败，静默跳过
	}
	defer conn.Close()

	// 构造探测包
	probe := models.PINProbe{
		Type:      probeTypeHello,
		DeviceID:  s.deviceID,
		Timestamp: time.Now().Unix(),
	}

	probeData, err := json.Marshal(probe)
	if err != nil {
		return
	}

	// 发送探测包
	conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	_, err = conn.Write(probeData)
	if err != nil {
		return
	}

	// 等待响应
	buffer := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, err := conn.Read(buffer)
	if err != nil {
		return // 无响应或超时，静默跳过
	}

	// 解析响应
	var response models.PINResponse
	if err := json.Unmarshal(buffer[:n], &response); err != nil {
		return
	}

	if response.Type != responseTypeChallenge || response.SessionSalt == "" {
		return
	}

	proof := models.PINProbe{
		Type:      probeTypeProof,
		PINHash:   HashPIN(pin, response.SessionSalt),
		DeviceID:  s.deviceID,
		Timestamp: time.Now().Unix(),
	}
	proofData, err := json.Marshal(proof)
	if err != nil {
		return
	}
	conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := conn.Write(proofData); err != nil {
		return
	}

	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, err = conn.Read(buffer)
	if err != nil {
		return
	}

	if err := json.Unmarshal(buffer[:n], &response); err != nil {
		return
	}

	if response.Type == responseTypeResult && response.Success {
		fmt.Printf("🎉 找到设备！\n")
		fmt.Printf("   名称: %s\n", response.Device.Name)
		fmt.Printf("   IP: %s\n", response.Device.IP)
		fmt.Printf("   平台: %s\n", response.Device.Platform)
		fmt.Printf("   QUIC 端口: %d\n\n", response.Device.QUICPort)

		// 将找到的设备发送到通道
		select {
		case s.foundChan <- response.Device:
		case <-s.ctx.Done():
		}
	}
}

// Stop 停止扫描
func (s *PINScanner) Stop() {
	s.cancel()
}
