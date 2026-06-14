package api

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

// EventType 事件类型
type EventType string

const (
	EventDeviceInfo    EventType = "device_info"    // 设备信息
	EventPINGenerated  EventType = "pin_generated"  // PIN 码生成
	EventDeviceFound   EventType = "device_found"   // 设备发现成功
	EventTransferStart EventType = "transfer_start" // 传输开始
	EventProgress      EventType = "progress"       // 传输进度
	EventTransferDone  EventType = "transfer_done"  // 传输完成
	EventQueueStatus   EventType = "queue_status"   // 队列状态
	EventSharedFiles   EventType = "shared_files"   // 共享文件列表变化
	EventError         EventType = "error"          // 错误
	EventLog           EventType = "log"            // 日志消息
)

// WSMessage WebSocket 消息结构
type WSMessage struct {
	Type EventType   `json:"type"`
	Data interface{} `json:"data"`
}

// ProgressData 进度数据
type ProgressData struct {
	FileName    string  `json:"filename"`
	SpeedMBps   float64 `json:"speed_mbps"`
	Percent     float64 `json:"percent"`
	Transferred int64   `json:"transferred"`
	Total       int64   `json:"total"`
}

// Client WebSocket 客户端
type Client struct {
	conn *websocket.Conn
	send chan []byte
	hub  *Hub
}

// Hub WebSocket 连接管理器
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mutex      sync.RWMutex
	stop       chan struct{}
	stopOnce   sync.Once
}

// NewHub 创建新的 Hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		stop:       make(chan struct{}),
	}
}

// Run 启动 Hub
func (h *Hub) Run() {
	for {
		select {
		case <-h.stop:
			// 清理所有客户端连接后退出
			h.mutex.Lock()
			for client := range h.clients {
				delete(h.clients, client)
				close(client.send)
			}
			h.mutex.Unlock()
			return
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			h.mutex.Unlock()

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mutex.Unlock()

		case message := <-h.broadcast:
			var staleClients []*Client
			h.mutex.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					staleClients = append(staleClients, client)
				}
			}
			h.mutex.RUnlock()

			if len(staleClients) == 0 {
				continue
			}

			h.mutex.Lock()
			for _, client := range staleClients {
				if _, ok := h.clients[client]; ok {
					delete(h.clients, client)
					close(client.send)
				}
			}
			h.mutex.Unlock()
		}
	}
}

// Stop 停止 Hub，关闭后所有 goroutine 会安全退出
func (h *Hub) Stop() {
	h.stopOnce.Do(func() {
		close(h.stop)
	})
}

// Close 是 Stop 的别名，便于调用方使用统一命名
func (h *Hub) Close() {
	h.Stop()
}

// Broadcast 广播消息
func (h *Hub) Broadcast(eventType EventType, data interface{}) {
	msg := WSMessage{
		Type: eventType,
		Data: data,
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.broadcast <- jsonData
}

// BroadcastLog 广播日志消息
func (h *Hub) BroadcastLog(message string) {
	h.Broadcast(EventLog, map[string]string{"message": message})
}

// BroadcastError 广播错误消息
func (h *Hub) BroadcastError(err error) {
	h.Broadcast(EventError, map[string]string{"error": err.Error()})
}

// BroadcastProgress 广播进度更新
func (h *Hub) BroadcastProgress(data ProgressData) {
	h.Broadcast(EventProgress, data)
}

// readPump 读取客户端消息
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// writePump 向客户端发送消息
func (c *Client) writePump() {
	defer c.conn.Close()

	for message := range c.send {
		err := c.conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			break
		}
	}
}
