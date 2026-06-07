package api

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/xjwm5685-ui/turbodrop/transfer"
)

const configFilePath = "./data/app_config.json"

// AppConfig 表示 TurboDrop 的可持久化配置
type AppConfig struct {
	WebHost              string `json:"web_host"`
	WebPort              int    `json:"web_port"`
	DeviceName           string `json:"device_name"`
	QUICPort             int    `json:"quic_port"`
	MaxConcurrentStreams int    `json:"max_concurrent_streams"`
	ChunkSizeMB          int    `json:"chunk_size_mb"`
	SaveDir              string `json:"save_dir"`
}

// ConfigStore 负责配置持久化
type ConfigStore struct {
	filePath string
	mutex    sync.RWMutex
}

func DefaultAppConfig() AppConfig {
	runtimeCfg := transfer.GetRuntimeConfig()
	return AppConfig{
		WebHost:              "localhost",
		WebPort:              48080,
		DeviceName:           "TurboDrop Device",
		QUICPort:             9001,
		MaxConcurrentStreams: runtimeCfg.MaxConcurrentStreams,
		ChunkSizeMB:          int(runtimeCfg.ChunkSizeBytes / (1024 * 1024)),
		SaveDir:              "./received_files",
	}
}

func (c AppConfig) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.WebHost, c.WebPort)
}

func (c AppConfig) Clone() AppConfig {
	return c
}

func (c AppConfig) Validate() error {
	if net.ParseIP(c.WebHost) == nil && c.WebHost != "localhost" {
		return fmt.Errorf("Web 主机地址无效")
	}
	if c.WebPort <= 0 || c.WebPort > 65535 {
		return fmt.Errorf("Web 端口无效")
	}
	if c.QUICPort <= 0 || c.QUICPort > 65535 {
		return fmt.Errorf("QUIC 端口无效")
	}
	if c.DeviceName == "" {
		return fmt.Errorf("设备名称不能为空")
	}
	if c.MaxConcurrentStreams < 1 || c.MaxConcurrentStreams > 128 {
		return fmt.Errorf("并发流数量必须在 1-128 之间")
	}
	if c.ChunkSizeMB < 1 || c.ChunkSizeMB > 64 {
		return fmt.Errorf("块大小必须在 1-64 MB 之间")
	}
	if c.SaveDir == "" {
		return fmt.Errorf("默认保存位置不能为空")
	}
	return nil
}

func NewConfigStore(filePath string) *ConfigStore {
	return &ConfigStore{filePath: filePath}
}

func LoadOrCreateConfig() (AppConfig, error) {
	store := NewConfigStore(configFilePath)
	cfg, err := store.Load()
	if err == nil {
		return cfg, nil
	}
	if !os.IsNotExist(err) {
		return AppConfig{}, err
	}

	cfg = DefaultAppConfig()
	if err := store.Save(cfg); err != nil {
		return AppConfig{}, err
	}
	return cfg, nil
}

func (s *ConfigStore) Load() (AppConfig, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return AppConfig{}, err
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return AppConfig{}, fmt.Errorf("解析配置失败: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return AppConfig{}, err
	}
	return cfg, nil
}

func (s *ConfigStore) Save(cfg AppConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.filePath), 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	tempFile := s.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("写入配置临时文件失败: %w", err)
	}

	if err := os.Rename(tempFile, s.filePath); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	return nil
}
