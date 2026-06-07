package transfer

import "testing"

func TestSetAndGetRuntimeConfig(t *testing.T) {
	original := GetRuntimeConfig()
	defer func() {
		_ = SetRuntimeConfig(original)
	}()

	newCfg := RuntimeConfig{
		ChunkSizeBytes:       2 * 1024 * 1024,
		MaxConcurrentStreams: 8,
	}

	if err := SetRuntimeConfig(newCfg); err != nil {
		t.Fatalf("SetRuntimeConfig() 失败: %v", err)
	}

	got := GetRuntimeConfig()
	if got != newCfg {
		t.Fatalf("GetRuntimeConfig() = %+v, want %+v", got, newCfg)
	}
}

func TestSetRuntimeConfigRejectsInvalidValues(t *testing.T) {
	if err := SetRuntimeConfig(RuntimeConfig{
		ChunkSizeBytes:       512 * 1024,
		MaxConcurrentStreams: 8,
	}); err == nil {
		t.Fatalf("非法块大小应返回错误")
	}

	if err := SetRuntimeConfig(RuntimeConfig{
		ChunkSizeBytes:       2 * 1024 * 1024,
		MaxConcurrentStreams: 0,
	}); err == nil {
		t.Fatalf("非法并发流数应返回错误")
	}
}
