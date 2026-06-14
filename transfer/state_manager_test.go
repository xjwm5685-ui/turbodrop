package transfer

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/xjwm5685-ui/turbodrop/models"
)

func TestStateManagerSaveLoadAndDelete(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "sample.bin")
	metadata := models.FileMetadata{
		FileName:    "sample.bin",
		FileSize:    1024,
		ChunkSize:   256,
		TotalChunks: 4,
		Blake3Hash:  "abc123",
	}

	sm := NewStateManager(filePath, metadata)
	if err := sm.SaveState(); err != nil {
		t.Fatalf("SaveState() 失败: %v", err)
	}

	if !CheckExists(filePath) {
		t.Fatalf("SaveState() 后应存在状态文件")
	}

	if err := sm.MarkChunkComplete(1); err != nil {
		t.Fatalf("MarkChunkComplete(1) 失败: %v", err)
	}
	if err := sm.MarkChunkComplete(3); err != nil {
		t.Fatalf("MarkChunkComplete(3) 失败: %v", err)
	}

	loaded, err := LoadState(filePath)
	if err != nil {
		t.Fatalf("LoadState() 失败: %v", err)
	}

	if got := loaded.GetCompletedCount(); got != 2 {
		t.Fatalf("GetCompletedCount() = %d, want 2", got)
	}

	if !loaded.IsChunkComplete(1) || !loaded.IsChunkComplete(3) {
		t.Fatalf("LoadState() 后应保留已完成块状态")
	}

	missing := loaded.GetMissingChunks()
	wantMissing := []int{0, 2}
	if !reflect.DeepEqual(missing, wantMissing) {
		t.Fatalf("GetMissingChunks() = %v, want %v", missing, wantMissing)
	}

	if err := loaded.DeleteState(); err != nil {
		t.Fatalf("DeleteState() 失败: %v", err)
	}

	if CheckExists(filePath) {
		t.Fatalf("DeleteState() 后状态文件应被删除")
	}
}

func TestStateManagerBitsetRoundTrip(t *testing.T) {
	metadata := models.FileMetadata{
		FileName:    "roundtrip.bin",
		FileSize:    2048,
		ChunkSize:   512,
		TotalChunks: 5,
		Blake3Hash:  "hash",
	}

	source := NewStateManager("roundtrip.bin", metadata)
	if err := source.MarkChunkComplete(0); err != nil {
		t.Fatalf("MarkChunkComplete(0) 失败: %v", err)
	}
	if err := source.MarkChunkComplete(4); err != nil {
		t.Fatalf("MarkChunkComplete(4) 失败: %v", err)
	}

	target := NewStateManager("other.bin", metadata)
	if err := target.SetBitsetFromBytes(source.GetBitsetBytes()); err != nil {
		t.Fatalf("SetBitsetFromBytes() 失败: %v", err)
	}

	if !target.IsChunkComplete(0) || !target.IsChunkComplete(4) {
		t.Fatalf("bitset 往返后应保留完成块信息")
	}
}

func TestStateManagerGetCompletedBytes(t *testing.T) {
	metadata := models.FileMetadata{
		FileName:    "partial.bin",
		FileSize:    10,
		ChunkSize:   4,
		TotalChunks: 3,
		Blake3Hash:  "hash",
	}

	sm := NewStateManager("partial.bin", metadata)
	// 完成第 0 块（4 字节）和第 2 块（最后 2 字节）
	sm.MarkChunkComplete(0)
	sm.MarkChunkComplete(2)

	got := sm.GetCompletedBytes(metadata.FileSize, metadata.ChunkSize)
	want := int64(6)
	if got != want {
		t.Fatalf("GetCompletedBytes() = %d, want %d", got, want)
	}
}

func TestStateManagerNoSaveAndFlush(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "nosave.bin")
	metadata := models.FileMetadata{
		FileName:    "nosave.bin",
		FileSize:    1024,
		ChunkSize:   256,
		TotalChunks: 4,
		Blake3Hash:  "abc",
	}

	sm := NewStateManager(filePath, metadata)
	if err := sm.SaveState(); err != nil {
		t.Fatalf("SaveState() 失败: %v", err)
	}

	sm.MarkChunkCompleteNoSave(1)

	// 未 Flush 前加载不应看到新变更
	loadedBefore, err := LoadState(filePath)
	if err != nil {
		t.Fatalf("LoadState() 失败: %v", err)
	}
	if loadedBefore.IsChunkComplete(1) {
		t.Fatalf("Flush 前应未保存第 1 块")
	}

	if err := sm.Flush(); err != nil {
		t.Fatalf("Flush() 失败: %v", err)
	}

	loadedAfter, err := LoadState(filePath)
	if err != nil {
		t.Fatalf("LoadState() 失败: %v", err)
	}
	if !loadedAfter.IsChunkComplete(1) {
		t.Fatalf("Flush 后应保存第 1 块")
	}
}
