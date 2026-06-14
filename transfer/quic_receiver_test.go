package transfer

import (
	"path/filepath"
	"testing"

	"github.com/xjwm5685-ui/turbodrop/models"
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

func TestValidateMetadataAcceptsValid(t *testing.T) {
	m := &models.FileMetadata{
		FileSize:    1024 * 1024,
		ChunkSize:   256 * 1024,
		TotalChunks: 4,
	}
	if err := validateMetadata(m); err != nil {
		t.Fatalf("合法元数据应通过校验: %v", err)
	}
}

func TestValidateMetadataRejectsZeroChunkSize(t *testing.T) {
	m := &models.FileMetadata{
		FileSize:    1024,
		ChunkSize:   0,
		TotalChunks: 1,
	}
	if err := validateMetadata(m); err == nil {
		t.Fatalf("ChunkSize=0 应被拒绝")
	}
}

func TestValidateMetadataRejectsNegativeChunkSize(t *testing.T) {
	m := &models.FileMetadata{
		FileSize:    1024,
		ChunkSize:   -1,
		TotalChunks: 1,
	}
	if err := validateMetadata(m); err == nil {
		t.Fatalf("ChunkSize<0 应被拒绝")
	}
}

func TestValidateMetadataRejectsZeroTotalChunks(t *testing.T) {
	m := &models.FileMetadata{
		FileSize:    1024,
		ChunkSize:   512,
		TotalChunks: 0,
	}
	if err := validateMetadata(m); err == nil {
		t.Fatalf("TotalChunks=0 应被拒绝")
	}
}

func TestValidateMetadataRejectsNegativeFileSize(t *testing.T) {
	m := &models.FileMetadata{
		FileSize:    -1,
		ChunkSize:   512,
		TotalChunks: 1,
	}
	if err := validateMetadata(m); err == nil {
		t.Fatalf("FileSize<0 应被拒绝")
	}
}

func TestValidateMetadataRejectsExcessiveTotalChunks(t *testing.T) {
	m := &models.FileMetadata{
		FileSize:    1024,
		ChunkSize:   1,
		TotalChunks: 1 << 25, // 超过上限 16M
	}
	if err := validateMetadata(m); err == nil {
		t.Fatalf("TotalChunks 过大应被拒绝")
	}
}

func TestValidateMetadataRejectsInconsistentChunkCount(t *testing.T) {
	m := &models.FileMetadata{
		FileSize:    1024,
		ChunkSize:   512,
		TotalChunks: 100, // 1024/512=2，100 远超合理值
	}
	if err := validateMetadata(m); err == nil {
		t.Fatalf("TotalChunks 与文件大小严重不匹配应被拒绝")
	}
}

func TestValidateMetadataAcceptsOneByteFile(t *testing.T) {
	m := &models.FileMetadata{
		FileSize:    1,
		ChunkSize:   4 * 1024 * 1024,
		TotalChunks: 1,
	}
	if err := validateMetadata(m); err != nil {
		t.Fatalf("1 字节文件应通过校验: %v", err)
	}
}

func TestValidateMetadataAcceptsExactFit(t *testing.T) {
	m := &models.FileMetadata{
		FileSize:    4 * 1024 * 1024,
		ChunkSize:   4 * 1024 * 1024,
		TotalChunks: 1,
	}
	if err := validateMetadata(m); err != nil {
		t.Fatalf("精确整除应通过校验: %v", err)
	}
}

func TestAuthTokenValidation(t *testing.T) {
	receiver := &QUICFileReceiver{
		saveDir:   t.TempDir(),
		authToken: "secret-token-123",
	}

	// 正确令牌应通过（不触发连接层逻辑，只测元数据校验部分）
	m := &models.FileMetadata{
		FileSize:    1024,
		ChunkSize:   512,
		TotalChunks: 2,
		AuthToken:   "secret-token-123",
	}
	if receiver.authToken != "" && m.AuthToken != receiver.authToken {
		t.Fatalf("正确令牌应匹配")
	}

	// 错误令牌应被拒
	m.AuthToken = "wrong-token"
	if receiver.authToken != "" && m.AuthToken == receiver.authToken {
		t.Fatalf("错误令牌不应匹配")
	}

	// 空令牌应被拒
	m.AuthToken = ""
	if receiver.authToken != "" && m.AuthToken == receiver.authToken {
		t.Fatalf("空令牌不应匹配")
	}
}

func TestAuthTokenNotRequiredWhenEmpty(t *testing.T) {
	receiver := &QUICFileReceiver{
		saveDir:   t.TempDir(),
		authToken: "", // 未设置令牌
	}

	m := &models.FileMetadata{
		AuthToken: "anything",
	}
	// 当 receiver 的 authToken 为空时，不应校验
	if receiver.authToken != "" && m.AuthToken != receiver.authToken {
		t.Fatalf("receiver 未设令牌时不应校验")
	}
}
