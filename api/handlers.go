package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/xjwm5685-ui/turbodrop/discovery"
	"github.com/xjwm5685-ui/turbodrop/models"
	"github.com/xjwm5685-ui/turbodrop/transfer"
)

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
	result, ok := s.saveMultipartFile(w, r, uploadDir, true)
	if !ok {
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// handleInboxUpload 接收局域网浏览器上传的文件并保存到这台电脑的默认保存目录
func (s *Server) handleInboxUpload(w http.ResponseWriter, r *http.Request) {
	cfg := s.getConfig()
	result, ok := s.saveMultipartFile(w, r, cfg.SaveDir, false)
	if !ok {
		return
	}

	now := time.Now().Format(time.RFC3339)
	entry := TransferHistoryEntry{
		FileName:    result.OriginalName,
		FilePath:    result.FilePath,
		Status:      "success",
		Message:     "浏览器上传到这台电脑",
		Size:        result.Size,
		DeviceName:  clientHost(r),
		StartedAt:   now,
		CompletedAt: now,
	}
	if err := s.historyStore.Add(entry); err != nil {
		s.hub.BroadcastLog("写入浏览器上传历史失败: " + err.Error())
	}

	s.hub.BroadcastLog(fmt.Sprintf("收到浏览器上传: %s", result.OriginalName))
	s.hub.Broadcast(EventTransferDone, map[string]interface{}{
		"filename":     result.OriginalName,
		"message":      "浏览器上传到这台电脑",
		"size":         result.Size,
		"status":       "success",
		"completed_at": now,
	})

	result.Message = "已上传到这台电脑"
	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleShareUpload(w http.ResponseWriter, r *http.Request) {
	result, ok := s.saveMultipartFile(w, r, sharedUploadDir, false)
	if !ok {
		return
	}

	downloadURL := "/api/v1/shared/download/" + url.PathEscape(filepath.Base(result.FilePath))
	s.hub.BroadcastLog(fmt.Sprintf("已共享给局域网下载: %s", result.OriginalName))
	s.hub.Broadcast(EventSharedFiles, map[string]interface{}{
		"name":         result.OriginalName,
		"size":         result.Size,
		"download_url": downloadURL,
	})

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"message":      "文件已加入局域网共享列表",
		"filename":     result.OriginalName,
		"size":         result.Size,
		"download_url": downloadURL,
	})
}

func (s *Server) handleListSharedFiles(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(sharedUploadDir)
	if err != nil {
		if os.IsNotExist(err) {
			respondJSON(w, http.StatusOK, map[string]interface{}{"success": true, "items": []SharedFileInfo{}})
			return
		}
		respondError(w, http.StatusInternalServerError, "读取共享文件失败: "+err.Error())
		return
	}

	items := make([]SharedFileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		name := info.Name()
		items = append(items, SharedFileInfo{
			Name:        name,
			Size:        info.Size(),
			ModifiedAt:  info.ModTime().Format(time.RFC3339),
			DownloadURL: "/api/v1/shared/download/" + url.PathEscape(name),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ModifiedAt > items[j].ModifiedAt
	})

	respondJSON(w, http.StatusOK, map[string]interface{}{"success": true, "items": items})
}

func (s *Server) handleDownloadSharedFile(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(mux.Vars(r)["name"])
	if name == "." || name == "" {
		respondError(w, http.StatusBadRequest, "文件名无效")
		return
	}

	path := filepath.Join(sharedUploadDir, name)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		respondError(w, http.StatusNotFound, "共享文件不存在")
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	http.ServeFile(w, r, path)
}

func (s *Server) saveMultipartFile(w http.ResponseWriter, r *http.Request, targetDir string, useTempName bool) (UploadResponse, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBodyBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		respondError(w, http.StatusBadRequest, "解析上传请求失败: "+err.Error())
		return UploadResponse{}, false
	}
	defer func() {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "未找到上传文件")
		return UploadResponse{}, false
	}
	defer file.Close()

	if header.Size > maxUploadBodyBytes {
		respondError(w, http.StatusRequestEntityTooLarge, "上传文件过大")
		return UploadResponse{}, false
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		respondError(w, http.StatusInternalServerError, "创建上传目录失败: "+err.Error())
		return UploadResponse{}, false
	}

	safeName := sanitizeUploadFileName(header.Filename)
	var savedFile *os.File
	var savedPath string
	if useTempName {
		savedFile, err = os.CreateTemp(targetDir, "upload-*"+filepath.Ext(safeName))
		if err == nil {
			savedPath = savedFile.Name()
		}
	} else {
		savedFile, savedPath, err = createUniqueFile(targetDir, safeName)
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "创建保存文件失败: "+err.Error())
		return UploadResponse{}, false
	}
	defer savedFile.Close()

	if _, err := io.Copy(savedFile, file); err != nil {
		os.Remove(savedPath)
		respondError(w, http.StatusInternalServerError, "保存上传文件失败: "+err.Error())
		return UploadResponse{}, false
	}

	return UploadResponse{
		Success:      true,
		Message:      "文件上传成功",
		FilePath:     savedPath,
		OriginalName: safeName,
		Size:         header.Size,
	}, true
}

func createUniqueFile(dir string, name string) (*os.File, string, error) {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" {
		base = "file"
	}

	for i := 0; i < 1000; i++ {
		candidateName := name
		if i > 0 {
			candidateName = fmt.Sprintf("%s-%d%s", base, i, ext)
		}

		path := filepath.Join(dir, candidateName)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err == nil {
			return file, path, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
	}

	return nil, "", fmt.Errorf("无法生成唯一文件名")
}

func clientHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// handleReceive 启动接收模式
func (s *Server) handleReceive(w http.ResponseWriter, r *http.Request) {
	var req ReceiveRequest
	if err := decodeJSONBody(w, r, &req, maxJSONBodyBytes); err != nil {
		respondError(w, http.StatusBadRequest, "无效的请求")
		return
	}

	cfg := s.getConfig()
	deviceName := cfg.DeviceName
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

	// 如果后续流程出错，确保停止接收端
	cleanupStarted := false
	defer func() {
		if !cleanupStarted {
			s.stopReceiveSession()
		}
	}()

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

	cleanupStarted = true

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

	if !isLoopbackRemote(r.RemoteAddr) && sendRequestUsesComputerPath(req) {
		respondError(w, http.StatusForbidden, "局域网页面不能发送电脑本地路径，请上传浏览器文件或在电脑本机操作")
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

func sendRequestUsesComputerPath(req SendRequest) bool {
	if req.FilePath != "" && !isManagedUploadPath(req.FilePath) {
		return true
	}

	for _, item := range req.Files {
		if item.FilePath != "" && !isManagedUploadPath(item.FilePath) {
			return true
		}
	}

	return false
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
	conn, err := s.upgrader.Upgrade(w, r, nil)
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
