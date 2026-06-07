package transfer

import (
	"path/filepath"
	"testing"
)

func TestResolveTargetFilePath(t *testing.T) {
	tempDir := t.TempDir()
	receiver := &QUICFileReceiver{saveDir: tempDir}

	got, err := receiver.resolveTargetFilePath("../../evil.txt")
	if err != nil {
		t.Fatalf("resolveTargetFilePath() 返回错误: %v", err)
	}

	want := filepath.Join(tempDir, "evil.txt")
	if got != want {
		t.Fatalf("resolveTargetFilePath() = %q, want %q", got, want)
	}
}

func TestResolveTargetFilePathRejectsEmptyName(t *testing.T) {
	receiver := &QUICFileReceiver{saveDir: t.TempDir()}
	if _, err := receiver.resolveTargetFilePath(""); err == nil {
		t.Fatalf("空文件名应返回错误")
	}
}
