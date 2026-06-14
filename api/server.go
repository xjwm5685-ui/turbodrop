package api

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/xjwm5685-ui/turbodrop/discovery"
	"github.com/xjwm5685-ui/turbodrop/transfer"
)

const uploadDir = "./uploads"
const sharedUploadDir = "./uploads/shared"
const (
	maxJSONBodyBytes   int64 = 1 << 20
	maxUploadBodyBytes int64 = 1 << 30
)

// Server API 服务器
type Server struct {
	hub             *Hub
	historyStore    *HistoryStore
	configStore     *ConfigStore
	router          *mux.Router
	httpServer      *http.Server
	pinReceiver     *discovery.PINReceiver
	currentTransfer atomic.Value // 当前传输状态
	configMutex     sync.RWMutex
	receiveMutex    sync.Mutex
	activeReceive   *receiveSession
	backgroundTasks sync.WaitGroup
	webuiFS         fs.FS
	config          AppConfig
	listenAddr      string
	deviceName      string
	quicPort        int
	allowedWebHost  string
	baseCtx         context.Context
	baseCancel      context.CancelFunc
	upgrader        websocket.Upgrader
}

// receiveSession 表示一次接收模式的会话
type receiveSession struct {
	cancel       context.CancelFunc
	pinReceiver  *discovery.PINReceiver
	fileReceiver *transfer.QUICFileReceiver
}

type DeviceInfo struct {
	Name     string `json:"name"`
	IP       string `json:"ip"`
	Platform string `json:"platform"`
	QUICPort int    `json:"quic_port"`
}

type ReceiveRequest struct {
	DeviceName string `json:"device_name,omitempty"`
}

type ReceiveResponse struct {
	Success bool   `json:"success"`
	PIN     string `json:"pin"`
	Message string `json:"message"`
}

type SendRequest struct {
	PIN      string         `json:"pin"`
	FilePath string         `json:"filepath"`
	FileName string         `json:"filename,omitempty"`
	Files    []SendFileItem `json:"files,omitempty"`
}

type SendResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type SendFileItem struct {
	FilePath string `json:"filepath"`
	FileName string `json:"filename,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

type UploadResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	FilePath     string `json:"filepath"`
	OriginalName string `json:"original_name"`
	Size         int64  `json:"size"`
}

type SharedFileInfo struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ModifiedAt  string `json:"modified_at"`
	DownloadURL string `json:"download_url"`
}

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
		hub:            hub,
		historyStore:   NewHistoryStore(historyFilePath),
		configStore:    NewConfigStore(configFilePath),
		router:         mux.NewRouter(),
		config:         cfg,
		listenAddr:     cfg.ListenAddr(),
		deviceName:     cfg.DeviceName,
		quicPort:       cfg.QUICPort,
		allowedWebHost: cfg.WebHost,
		baseCtx:        baseCtx,
		baseCancel:     baseCancel,
	}

	// 每个 Server 实例拥有独立的 WebSocket upgrader 与 Origin 校验策略
	s.upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return s.isOriginAllowed(r.Header.Get("Origin"))
		},
	}

	s.setupRoutes()
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
	api.HandleFunc("/config", s.localOnly(s.handleGetConfig)).Methods("GET")
	api.HandleFunc("/config", s.localOnly(s.handleUpdateConfig)).Methods("PUT")
	api.HandleFunc("/config/select-save-dir", s.localOnly(s.handleSelectSaveDir)).Methods("POST")
	api.HandleFunc("/history", s.handleGetHistory).Methods("GET")
	api.HandleFunc("/inbox/upload", s.handleInboxUpload).Methods("POST")
	api.HandleFunc("/shared", s.handleListSharedFiles).Methods("GET")
	api.HandleFunc("/shared/upload", s.localOnly(s.handleShareUpload)).Methods("POST")
	api.HandleFunc("/shared/download/{name}", s.handleDownloadSharedFile).Methods("GET")
	api.HandleFunc("/receive", s.handleReceive).Methods("POST")
	api.HandleFunc("/upload", s.handleUpload).Methods("POST")
	api.HandleFunc("/send", s.handleSend).Methods("POST")
	api.HandleFunc("/ws", s.handleWebSocket)

	// 静态文件。SetWebUIFS 在 NewServerWithConfig 之后调用，因此这里运行时选择文件系统。
	s.router.PathPrefix("/").HandlerFunc(s.handleWebUI)

	// 添加 CORS 支持
	s.router.Use(s.corsMiddleware)
}

func (s *Server) handleWebUI(w http.ResponseWriter, r *http.Request) {
	if s.webuiFS != nil {
		http.FileServer(http.FS(s.webuiFS)).ServeHTTP(w, r)
		return
	}

	http.FileServer(http.Dir("./webui")).ServeHTTP(w, r)
}

// corsMiddleware CORS 中间件
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if s.isOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			if origin != "" && !s.isOriginAllowed(origin) {
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
	if err := os.MkdirAll(sharedUploadDir, 0755); err != nil {
		return fmt.Errorf("创建共享目录失败: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}

	cfg := s.getConfig()
	s.printStartupEndpoints(cfg)
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

func (s *Server) printStartupEndpoints(cfg AppConfig) {
	fmt.Printf("🌐 API 服务器监听: %s\n", cfg.ListenAddr())

	hosts := []string{cfg.WebHost}
	if isWildcardWebHost(cfg.WebHost) {
		hosts = []string{"localhost"}
		if localIP, err := discovery.GetLocalIP(); err == nil && localIP != "" {
			hosts = append(hosts, localIP)
		}
	}

	for index, host := range hosts {
		label := "🎨 Web 控制台"
		if isWildcardWebHost(cfg.WebHost) {
			if index == 0 {
				label = "🏠 本机访问"
			} else {
				label = "📱 局域网访问"
			}
		}
		fmt.Printf("%s: http://%s:%d/dashboard.html\n", label, host, cfg.WebPort)
	}

	fmt.Printf("📡 WebSocket 端点: ws://%s:%d/api/v1/ws\n\n", hosts[0], cfg.WebPort)
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.hub.Stop()
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
	s.allowedWebHost = cfg.WebHost
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
