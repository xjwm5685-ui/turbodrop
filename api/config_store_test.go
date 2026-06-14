package api

import (
	"path/filepath"
	"testing"
)

func TestConfigStoreSaveLoad(t *testing.T) {
	store := NewConfigStore(filepath.Join(t.TempDir(), "app_config.json"))
	cfg := AppConfig{
		WebHost:              "127.0.0.1",
		WebPort:              49080,
		DeviceName:           "TurboDrop Test",
		QUICPort:             9101,
		MaxConcurrentStreams: 8,
		ChunkSizeMB:          2,
		SaveDir:              "./downloads",
	}

	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save() 失败: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() 失败: %v", err)
	}

	if loaded != cfg {
		t.Fatalf("Load() = %+v, want %+v", loaded, cfg)
	}
}

func TestAppConfigValidate(t *testing.T) {
	cfg := DefaultAppConfig()
	if cfg.WebHost != "0.0.0.0" {
		t.Fatalf("DefaultAppConfig().WebHost = %q, want 0.0.0.0", cfg.WebHost)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("默认配置应可通过校验: %v", err)
	}

	cfg = DefaultAppConfig()
	cfg.WebPort = 0
	if err := cfg.Validate(); err == nil {
		t.Fatalf("无效端口应返回错误")
	}

	cfg = DefaultAppConfig()
	cfg.MaxConcurrentStreams = 999
	if err := cfg.Validate(); err == nil {
		t.Fatalf("无效并发流数应返回错误")
	}

	cfg = DefaultAppConfig()
	cfg.SaveDir = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("空保存目录应返回错误")
	}
}
