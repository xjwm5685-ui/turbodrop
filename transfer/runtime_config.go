package transfer

import (
	"fmt"
	"sync"
)

const (
	defaultChunkSizeBytes      int64 = 4 * 1024 * 1024
	defaultMaxConcurrentStreams      = 16
)

// RuntimeConfig 表示发送端运行时传输参数
type RuntimeConfig struct {
	ChunkSizeBytes       int64
	MaxConcurrentStreams int
}

var runtimeConfigState = struct {
	mutex sync.RWMutex
	cfg   RuntimeConfig
}{
	cfg: RuntimeConfig{
		ChunkSizeBytes:       defaultChunkSizeBytes,
		MaxConcurrentStreams: defaultMaxConcurrentStreams,
	},
}

func GetRuntimeConfig() RuntimeConfig {
	runtimeConfigState.mutex.RLock()
	defer runtimeConfigState.mutex.RUnlock()
	return runtimeConfigState.cfg
}

func SetRuntimeConfig(cfg RuntimeConfig) error {
	if cfg.ChunkSizeBytes < 1*1024*1024 || cfg.ChunkSizeBytes > 64*1024*1024 {
		return fmt.Errorf("块大小必须在 1MB 到 64MB 之间")
	}
	if cfg.MaxConcurrentStreams < 1 || cfg.MaxConcurrentStreams > 128 {
		return fmt.Errorf("并发流数量必须在 1 到 128 之间")
	}

	runtimeConfigState.mutex.Lock()
	defer runtimeConfigState.mutex.Unlock()
	runtimeConfigState.cfg = cfg
	return nil
}
