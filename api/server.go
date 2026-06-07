package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/xjwm5685-ui/turbodrop/discovery"
	"github.com/xjwm5685-ui/turbodrop/models"
	"github.com/xjwm5685-ui/turbodrop/transfer"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return isAllowedOrigin(r.Header.Get("Origin"))
	},
}

const uploadDir = "./uploads"
const (
	maxJSONBodyBytes   int64 = 1 << 20
	maxUploadBodyBytes int64 = 1 << 30
)

// allowedWebHost 当前配置的 web 主机地址，用于 CORS/Origin 校验
var allowedWebHost string

// Server API 服务器
type Server struct {
	hub              *Hub
	historyStore     *HistoryStore
	configStore      *ConfigStore
	router           *mux.Router
	httpServer       *http.Server
	pinReceiver      *discovery.PINReceiver
	currentTransfer  atomic.Value // 当前传输状态
	configMutex      sync.RWMutex
	receiveMutex     sync.Mutex
	activeReceive    *receiveSession
	backgroundTasks  sync.WaitGroup
	webuiFS          fs.FS
	config           AppConfig
	listenAddr       string
	deviceName       string
	quicPort         int
	baseCtx          context.Context
	baseCancel       context.CancelFunc
}

type receiveSession struct {
	cancel       context.CancelFunc
	pinReceiver  *discovery.PINReceiver
	fileReceiver *transfer.QUICFileReceiver
}

// DeviceInfo 设备信息
type DeviceInfo struct {
	Name     string `json:"name"`
	IP       string `json:"ip"`
	Platform string `json:"platform"`
	QUICPort int    `json:"quic_port"`
}

// ReceiveRequest 接收请求
type ReceiveRequest struct {
	DeviceName string `json:"device_name,omitempty"`
}

// ReceiveResponse 接收响应
type ReceiveResponse struct {
	Success bool   `json:"success"`
	PIN     string `json:"pin"`
	Message string `json:"message"`
}

// SendRequest 发送请求
type SendRequest struct {
	PIN      string `json:"pin"`
	FilePath string `json:"filepath"`
	FileName string `json:"filename,omitempty"`
	Files    []SendFileItem `json:"files,omitempty"`
}

// SendResponse 发送响应
type SendResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// SendFileItem 表示队列中的一个发送文件
type SendFileItem struct {
	FilePath string `json:"filepath"`
	FileName string `json:"filename,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

// UploadResponse 上传响应
type UploadResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	FilePath     string `json:"filepath"`
	OriginalName string `json:"original_name"`
	Size         int64  `json:"size"`
}

// ConfigUpdateResponse 配置更新响应
type ConfigUpdateResponse struct {
	Success         bool      `json:"success"`
	Message         string    `json:"message"`
	Config          AppConfig `json:"config"`
	RequiresRestart bool      `json:"requires_restart"`
}

// NewServer 创建 API 服务器
func NewServer(listenAddr string, deviceName string, quicPort int) *Server {
	cfg := DefaultAppConfig()
	host, port := splitHostPortOrDefault(listenAddr, cfg.WebHost, cfg.WebPort)
	cfg.WebHost = host
	cfg.WebPort = port
	cfg.DeviceName = deviceName
	cfg.QUICPort = quicPort
	return NewServerWithConfig(cfg)
}

// NewServerWithConfig 使用配置创建 API 服务器
func NewServerWithConfig(cfg AppConfig) *Server {
	hub := NewHub()
	go hub.Run()
	baseCtx, baseCancel := context.WithCancel(context.Background())

	_ = transfer.SetRuntimeConfig(transfer.RuntimeConfig{
		ChunkSizeBytes:       int64(cfg.ChunkSizeMB) * 1024 * 1024,
		MaxConcurrentStreams: cfg.MaxConcurrentStreams,
	})

	s := &Server{
		hub:          hub,
		historyStore: NewHistoryStore(historyFilePath),
		configStore:  NewConfigStore(configFilePath),
		router:       mux.NewRouter(),
		config:       cfg,
		listenAddr:   cfg.ListenAddr(),
		deviceName:   cfg.DeviceName,
		quicPort:     cfg.QUICPort,
		baseCtx:      baseCtx,
		baseCancel:   baseCancel,
	}

	s.setupRoutes()
	allowedWebHost = cfg.WebHost
	return s
}

// SetWebUIFS 设置嵌入的 Web UI 文件系统
func (s *Server) SetWebUIFS(filesystem fs.FS) {
	s.webuiFS = filesystem
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	// API 路由
	api := s.router.PathPrefix("/api/v1").Subrouter()
	
	api.HandleFunc("/info", s.handleGetInfo).Methods("GET")
	api.HandleFunc("/config", s.handleGetConfig).Methods("GET")
	api.HandleFunc("/config", s.handleUpdateConfig).Methods("PUT")
	api.HandleFunc("/config/select-save-dir", s.handleSelectSaveDir).Methods("POST")
	api.HandleFunc("/history", s.handleGetHistory).Methods("GET")
	api.HandleFunc("/receive", s.handleReceive).Methods("POST")
	api.HandleFunc("/upload", s.handleUpload).Methods("POST")
	api.HandleFunc("/send", s.handleSend).Methods("POST")
	api.HandleFunc("/ws", s.handleWebSocket)

	// 静态文件
	if s.webuiFS != nil {
		s.router.PathPrefix("/").Handler(http.FileServer(http.FS(s.webuiFS)))
	} else {
		s.router.PathPrefix("/").Handler(http.FileServer(http.Dir("./webui")))
	}

	// 添加 CORS 支持
	s.router.Use(corsMiddleware)
}

// corsMiddleware CORS 中间件
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			if origin != "" && !isAllowedOrigin(origin) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Start 启动服务器
func (s *Server) Start() error {
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return fmt.Errorf("创建上传目录失败: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}

	cfg := s.getConfig()
	fmt.Printf("🌐 API 服务器启动: http://%s\n", cfg.ListenAddr())
	fmt.Printf("📡 WebSocket 端点: ws://%s/api/v1/ws\n", cfg.ListenAddr())
	fmt.Printf("🎨 Web 控制台: http://%s/dashboard.html\n\n", cfg.ListenAddr())
	if err := os.MkdirAll(cfg.SaveDir, 0755); err != nil {
		return fmt.Errorf("创建默认保存目录失败: %w", err)
	}

	s.httpServer = &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.baseCancel()
	s.stopReceiveSession()

	var errs []error
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, err)
		}
	}

	done := make(chan struct{})
	go func() {
		s.backgroundTasks.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		errs = append(errs, ctx.Err())
	}

	return errors.Join(errs...)
}

// handleGetInfo 获取设备信息
func (s *Server) handleGetInfo(w http.ResponseWriter, r *http.Request) {
	localIP, _ := discovery.GetLocalIP()
	cfg := s.getConfig()

	info := DeviceInfo{
		Name:     cfg.DeviceName,
		IP:       localIP,
		Platform: runtime.GOOS,
		QUICPort: cfg.QUICPort,
	}

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"config":  s.getConfig(),
	})
}

func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req AppConfig
	if err := decodeJSONBody(w, r, &req, maxJSONBodyBytes); err != nil {
		respondError(w, http.StatusBadRequest, "无效的配置请求")
		return
	}

	oldConfig := s.getConfig()
	if err := req.Validate(); err != nil {
		respondError(w, http.StatusBadRequest, "配置校验失败: "+err.Error())
		return
	}
	if err := os.MkdirAll(req.SaveDir, 0755); err != nil {
		respondError(w, http.StatusBadRequest, "创建默认保存目录失败: "+err.Error())
		return
	}

	if err := s.configStore.Save(req); err != nil {
		respondError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
		return
	}

	if err := transfer.SetRuntimeConfig(transfer.RuntimeConfig{
		ChunkSizeBytes:       int64(req.ChunkSizeMB) * 1024 * 1024,
		MaxConcurrentStreams: req.MaxConcurrentStreams,
	}); err != nil {
		respondError(w, http.StatusBadRequest, "应用传输配置失败: "+err.Error())
		return
	}

	s.setConfig(req)
	requiresRestart := oldConfig.WebHost != req.WebHost || oldConfig.WebPort != req.WebPort
	respondJSON(w, http.StatusOK, ConfigUpdateResponse{
		Success:         true,
		Message:         "配置已保存",
		Config:          req,
		RequiresRestart: requiresRestart,
	})
}

// handleGetHistory 获取持久化传输历史
func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	entries, err := s.historyStore.List()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "读取传输历史失败: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"items":   entries,
	})
}

// handleUpload 接收浏览器上传的文件并保存到本地临时目录
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBodyBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		respondError(w, http.StatusBadRequest, "解析上传请求失败: "+err.Error())
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "未找到上传文件")
		return
	}
	defer file.Close()

	if header.Size > maxUploadBodyBytes {
		respondError(w, http.StatusRequestEntityTooLarge, "上传文件过大")
		return
	}

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		respondError(w, http.StatusInternalServerError, "创建上传目录失败: "+err.Error())
		return
	}

	safeName := sanitizeUploadFileName(header.Filename)
	tempFile, err := os.CreateTemp(uploadDir, "upload-*"+filepath.Ext(safeName))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "创建临时文件失败: "+err.Error())
		return
	}
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, file); err != nil {
		os.Remove(tempFile.Name())
		respondError(w, http.StatusInternalServerError, "保存上传文件失败: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, UploadResponse{
		Success:      true,
		Message:      "文件上传成功",
		FilePath:     tempFile.Name(),
		OriginalName: safeName,
		Size:         header.Size,
	})
}

// handleReceive 启动接收模式
func (s *Server) handleReceive(w http.ResponseWriter, r *http.Request) {
	var req ReceiveRequest
	if err := decodeJSONBody(w, r, &req, maxJSONBodyBytes); err != nil {
		respondError(w, http.StatusBadRequest, "无效的请求")
		return
	}

	deviceName := s.deviceName
	cfg := s.getConfig()
	deviceName = cfg.DeviceName
	if req.DeviceName != "" {
		deviceName = req.DeviceName
	}

	tlsConfig, certFingerprint, err := transfer.GenerateEphemeralTLSConfig()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "创建 TLS 会话失败: "+err.Error())
		return
	}

	// 创建接收端
	receiver, err := discovery.NewPINReceiver(deviceName, cfg.QUICPort, certFingerprint)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "创建接收端失败: "+err.Error())
		return
	}
	fileReceiver := transfer.NewQUICFileReceiver(cfg.QUICPort, cfg.SaveDir, tlsConfig, receiver.GetAuthToken())
	sessionCtx, cancel := context.WithCancel(s.baseCtx)
	session := &receiveSession{
		cancel:       cancel,
		pinReceiver:  receiver,
		fileReceiver: fileReceiver,
	}

	pin := receiver.GetPIN()
	s.pinReceiver = receiver
	s.replaceReceiveSession(session)

	// 启动接收端（后台）
	s.backgroundTasks.Add(1)
	go func() {
		defer s.backgroundTasks.Done()
		if err := receiver.Start(); err != nil && sessionCtx.Err() == nil {
			s.hub.BroadcastError(err)
		}
	}()

	// 启动 QUIC 文件接收（后台）
	s.backgroundTasks.Add(1)
	go func() {
		defer s.backgroundTasks.Done()
		time.Sleep(500 * time.Millisecond) // 等待 PIN 监听启动
		if sessionCtx.Err() != nil {
			return
		}

		s.hub.BroadcastLog("等待发送端连接...")
		if err := fileReceiver.Start(sessionCtx); err != nil && sessionCtx.Err() == nil {
			s.hub.BroadcastError(err)
		}
	}()

	// 广播 PIN 码
	s.hub.Broadcast(EventPINGenerated, map[string]string{
		"pin":     pin,
		"message": "接收端已就绪",
	})

	respondJSON(w, http.StatusOK, ReceiveResponse{
		Success: true,
		PIN:     pin,
		Message: "接收端已启动",
	})
}

// handleSend 启动发送模式
func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	var req SendRequest
	if err := decodeJSONBody(w, r, &req, maxJSONBodyBytes); err != nil {
		respondError(w, http.StatusBadRequest, "无效的请求")
		return
	}

	if req.PIN == "" || len(req.PIN) != 6 {
		respondError(w, http.StatusBadRequest, "PIN 码必须是 6 位数字")
		return
	}

	items, err := s.normalizeSendItems(req)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 后台执行发送
	s.backgroundTasks.Add(1)
	go func() {
		defer s.backgroundTasks.Done()
		taskCtx, cancel := context.WithCancel(s.baseCtx)
		defer cancel()

		s.hub.BroadcastLog(fmt.Sprintf("开始扫描，PIN: %s", req.PIN))

		// Phase 1: 设备发现
		scanner, err := discovery.NewPINScannerWithContext(taskCtx)
		if err != nil {
			s.hub.BroadcastError(fmt.Errorf("创建扫描器失败: %w", err))
			return
		}
		defer scanner.Stop()

		device, err := scanner.Scan(req.PIN)
		if err != nil {
			s.hub.BroadcastError(fmt.Errorf("设备发现失败: %w", err))
			return
		}

		// 广播设备发现成功
		s.hub.Broadcast(EventDeviceFound, map[string]interface{}{
			"device_name": device.Name,
			"device_ip":   device.IP,
			"device_port": device.QUICPort,
		})

		s.hub.Broadcast(EventQueueStatus, map[string]interface{}{
			"total":     len(items),
			"completed": 0,
			"status":    "started",
		})

		completed := 0
		for index, item := range items {
			if err := s.sendSingleFile(taskCtx, req.PIN, item, *device); err != nil {
				s.hub.BroadcastError(fmt.Errorf("文件传输失败: %w", err))
			} else {
				completed++
			}

			s.hub.Broadcast(EventQueueStatus, map[string]interface{}{
				"total":     len(items),
				"completed": completed,
				"current":   index + 1,
				"status":    "running",
			})
		}

		s.hub.Broadcast(EventQueueStatus, map[string]interface{}{
			"total":     len(items),
			"completed": completed,
			"status":    "done",
		})
	}()

	respondJSON(w, http.StatusOK, SendResponse{
		Success: true,
		Message: fmt.Sprintf("队列已启动，共 %d 个文件", len(items)),
	})
}

// monitorProgress 监控传输进度（简化版）
func (s *Server) monitorProgress(ctx context.Context, sender *transfer.QUICFileSender) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastBytes := int64(0)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		transferred := sender.GetBytesTransferred()
		total := sender.GetTotalBytes()

		if total == 0 {
			continue
		}

		delta := transferred - lastBytes
		speedMBps := float64(delta) / (1024 * 1024)
		percent := float64(transferred) / float64(total) * 100

		s.hub.BroadcastProgress(ProgressData{
			FileName:    "transferring...",
			SpeedMBps:   speedMBps,
			Percent:     percent,
			Transferred: transferred,
			Total:       total,
		})

		lastBytes = transferred

		if transferred >= total {
			return
		}
	}
}

// handleWebSocket WebSocket 连接处理
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("WebSocket 升级失败: %v\n", err)
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
		hub:  s.hub,
	}

	s.hub.register <- client

	go client.writePump()
	go client.readPump()

	// 发送欢迎消息
	s.hub.Broadcast(EventLog, map[string]string{
		"message": "WebSocket 连接已建立",
	})
}

// 辅助函数

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func (s *Server) getConfig() AppConfig {
	s.configMutex.RLock()
	defer s.configMutex.RUnlock()
	return s.config.Clone()
}

func (s *Server) setConfig(cfg AppConfig) {
	s.configMutex.Lock()
	defer s.configMutex.Unlock()
	s.config = cfg
	s.listenAddr = cfg.ListenAddr()
	s.deviceName = cfg.DeviceName
	s.quicPort = cfg.QUICPort
	allowedWebHost = cfg.WebHost
}

func (s *Server) normalizeSendItems(req SendRequest) ([]SendFileItem, error) {
	items := make([]SendFileItem, 0, len(req.Files)+1)

	if len(req.Files) > 0 {
		items = append(items, req.Files...)
	}

	if req.FilePath != "" {
		items = append(items, SendFileItem{
			FilePath: req.FilePath,
			FileName: req.FileName,
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("至少需要一个文件")
	}

	for i := range items {
		if items[i].FilePath == "" {
			return nil, fmt.Errorf("第 %d 个文件路径为空", i+1)
		}
		if items[i].FileName == "" {
			items[i].FileName = filepath.Base(items[i].FilePath)
		}

		info, err := os.Stat(items[i].FilePath)
		if err != nil {
			return nil, fmt.Errorf("文件不存在或不可读取: %s", items[i].FilePath)
		}
		if items[i].Size == 0 {
			items[i].Size = info.Size()
		}
	}

	return items, nil
}

func (s *Server) sendSingleFile(ctx context.Context, pin string, item SendFileItem, device models.Device) error {
	shouldCleanupUpload := isManagedUploadPath(item.FilePath)
	if shouldCleanupUpload {
		defer os.Remove(item.FilePath)
	}

	s.hub.BroadcastLog(fmt.Sprintf("开始传输文件: %s", item.FileName))
	s.hub.Broadcast(EventTransferStart, map[string]interface{}{
		"filename": item.FileName,
		"size":     item.Size,
	})

	fileSender := transfer.NewQUICFileSender(item.FilePath, device.IP, device.QUICPort)
	fileSender.SetDisplayName(item.FileName)
	fileSender.SetExpectedCertFingerprint(device.CertFingerprint)
	fileSender.SetAuthToken(device.AuthToken)

	progressCtx, cancelProgress := context.WithCancel(ctx)
	defer cancelProgress()
	go s.monitorProgress(progressCtx, fileSender)

	startedAt := time.Now()
	if err := fileSender.Send(ctx); err != nil {
		_ = s.historyStore.Add(TransferHistoryEntry{
			FileName:    item.FileName,
			FilePath:    item.FilePath,
			Status:      "failed",
			Message:     err.Error(),
			Size:        item.Size,
			PIN:         pin,
			DeviceName:  device.Name,
			DeviceIP:    device.IP,
			StartedAt:   startedAt.Format(time.RFC3339),
			CompletedAt: time.Now().Format(time.RFC3339),
		})
		return err
	}

	completedAt := time.Now()
	entry := TransferHistoryEntry{
		FileName:    item.FileName,
		FilePath:    item.FilePath,
		Status:      "success",
		Message:     "文件传输成功",
		Size:        item.Size,
		PIN:         pin,
		DeviceName:  device.Name,
		DeviceIP:    device.IP,
		StartedAt:   startedAt.Format(time.RFC3339),
		CompletedAt: completedAt.Format(time.RFC3339),
	}
	if err := s.historyStore.Add(entry); err != nil {
		s.hub.BroadcastLog("写入传输历史失败: " + err.Error())
	}

	s.hub.Broadcast(EventTransferDone, map[string]interface{}{
		"filename":     item.FileName,
		"message":      "文件传输成功",
		"size":         item.Size,
		"status":       "success",
		"completed_at": entry.CompletedAt,
	})

	return nil
}

func sanitizeUploadFileName(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "." || base == "" {
		return "uploaded-file"
	}

	replacer := strings.NewReplacer(
		"\\", "_",
		"/", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(base)
}

func isManagedUploadPath(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	absUploadDir, err := filepath.Abs(uploadDir)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(absUploadDir, absPath)
	if err != nil {
		return false
	}

	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func splitHostPortOrDefault(addr string, defaultHost string, defaultPort int) (string, int) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return defaultHost, defaultPort
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return host, defaultPort
	}
	return host, portNum
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst interface{}, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func isAllowedOrigin(origin string) bool {
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		// 允许配置的 web_host
		if allowedWebHost != "" && host == strings.ToLower(allowedWebHost) {
			return true
		}
		return false
	}
}

func (s *Server) replaceReceiveSession(session *receiveSession) {
	s.receiveMutex.Lock()
	old := s.activeReceive
	s.activeReceive = session
	s.receiveMutex.Unlock()

	if old != nil {
		old.cancel()
		if old.pinReceiver != nil {
			old.pinReceiver.Stop()
		}
		if old.fileReceiver != nil {
			_ = old.fileReceiver.Stop()
		}
	}
}

func (s *Server) stopReceiveSession() {
	s.receiveMutex.Lock()
	session := s.activeReceive
	s.activeReceive = nil
	s.receiveMutex.Unlock()

	if session == nil {
		return
	}
	session.cancel()
	if session.pinReceiver != nil {
		session.pinReceiver.Stop()
	}
	if session.fileReceiver != nil {
		_ = session.fileReceiver.Stop()
	}
}
