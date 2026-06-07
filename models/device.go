package models

import "time"

// Device 表示局域网中的一台设备
type Device struct {
	ID              string    `json:"id"`               // 唯一设备标识 (UUID)
	Name            string    `json:"name"`             // 设备名称
	IP              string    `json:"ip"`               // IP 地址
	QUICPort        int       `json:"quic_port"`        // QUIC 传输端口
	Platform        string    `json:"platform"`         // 平台信息 (Windows/macOS/Linux/Android/iOS)
	CertFingerprint string    `json:"cert_fingerprint"` // QUIC 证书指纹（SHA-256）
	DiscoveryAt     time.Time `json:"discovery_at"`     // 发现时间
}

// PINProbe 表示 PIN 码探测包结构
type PINProbe struct {
	Type      string `json:"type"`       // 握手消息类型: hello/proof
	PINHash   string `json:"pin_hash"`   // PIN 码的 BLAKE3 哈希
	DeviceID  string `json:"device_id"`  // 发送方设备 ID
	Timestamp int64  `json:"timestamp"`  // 时间戳，防止重放攻击
}

// PINResponse 表示接收端对探测包的响应
type PINResponse struct {
	Type        string `json:"type"`         // 握手响应类型: challenge/result
	Device      Device `json:"device"`       // 设备完整信息
	Success     bool   `json:"success"`      // 是否匹配成功
	SessionSalt string `json:"session_salt"` // 会话盐值（用于计算一次性 PIN 哈希）
	Timestamp   int64  `json:"timestamp"`    // 响应时间戳
}
