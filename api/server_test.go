package api

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeUploadFileName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "normal", in: "report.txt", want: "report.txt"},
		{name: "empty", in: "   ", want: "uploaded-file"},
		{name: "nested path", in: "folder/bad:name?.txt", want: "bad_name_.txt"},
		{name: "unix path", in: "dir/sub/file.txt", want: "file.txt"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeUploadFileName(tc.in)
			if got != tc.want {
				t.Fatalf("sanitizeUploadFileName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsManagedUploadPath(t *testing.T) {
	absUploadDir, err := filepath.Abs(uploadDir)
	if err != nil {
		t.Fatalf("filepath.Abs(uploadDir) 失败: %v", err)
	}

	inside := filepath.Join(absUploadDir, "upload-123.txt")
	outside := filepath.Join(filepath.Dir(absUploadDir), "other", "upload-123.txt")

	if !isManagedUploadPath(inside) {
		t.Fatalf("isManagedUploadPath(%q) = false, want true", inside)
	}

	if isManagedUploadPath(outside) {
		t.Fatalf("isManagedUploadPath(%q) = true, want false", outside)
	}
}

func TestNormalizeSendItems(t *testing.T) {
	server := NewServer("127.0.0.1:0", "test-device", 9001)
	tempDir := t.TempDir()
	fileA := filepath.Join(tempDir, "a.txt")
	fileB := filepath.Join(tempDir, "b.txt")

	if err := os.WriteFile(fileA, []byte("hello"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("world"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	items, err := server.normalizeSendItems(SendRequest{
		PIN: "123456",
		Files: []SendFileItem{
			{FilePath: fileA},
			{FilePath: fileB, FileName: "custom-b.txt"},
		},
	})
	if err != nil {
		t.Fatalf("normalizeSendItems() 失败: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("normalizeSendItems() 返回 %d 项, want 2", len(items))
	}
	if items[0].FileName != "a.txt" {
		t.Fatalf("第一个文件名应自动推导, got %q", items[0].FileName)
	}
	if items[1].FileName != "custom-b.txt" {
		t.Fatalf("第二个文件名应保留自定义名称, got %q", items[1].FileName)
	}
	if items[0].Size == 0 || items[1].Size == 0 {
		t.Fatalf("应自动补齐文件大小")
	}
}

func TestNormalizeSendItemsRejectsMissingFile(t *testing.T) {
	server := NewServer("127.0.0.1:0", "test-device", 9001)

	_, err := server.normalizeSendItems(SendRequest{
		PIN:      "123456",
		FilePath: filepath.Join(t.TempDir(), "missing.txt"),
	})
	if err == nil {
		t.Fatalf("缺失文件应返回错误")
	}
	if !strings.Contains(err.Error(), "文件不存在或不可读取") {
		t.Fatalf("错误消息不符合预期: %v", err)
	}
}

func TestIsAllowedOrigin(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		// 默认（空 host）：只允许 loopback
		{name: "empty origin", host: "", origin: "", want: true},
		{name: "localhost", host: "", origin: "http://localhost:48080", want: true},
		{name: "loopback", host: "", origin: "http://127.0.0.1:48080", want: true},
		{name: "ipv6 loopback", host: "", origin: "http://[::1]:48080", want: true},
		{name: "remote rejected", host: "", origin: "http://evil.example.com", want: false},
		{name: "invalid origin", host: "", origin: "://bad", want: false},

		// 特定 LAN IP：匹配放行，不匹配拒绝
		{name: "lan ip match", host: "192.168.1.100", origin: "http://192.168.1.100:48080", want: true},
		{name: "lan ip mismatch", host: "192.168.1.100", origin: "http://192.168.1.200:48080", want: false},
		{name: "lan ip localhost still ok", host: "192.168.1.100", origin: "http://localhost:48080", want: true},

		// 0.0.0.0：接受局域网来源，拒绝公网来源
		{name: "wildcard lan ip", host: "0.0.0.0", origin: "http://192.168.1.50:48080", want: true},
		{name: "wildcard private class a", host: "0.0.0.0", origin: "http://10.1.2.3:48080", want: true},
		{name: "wildcard link local", host: "0.0.0.0", origin: "http://169.254.10.20:48080", want: true},
		{name: "wildcard local hostname", host: "0.0.0.0", origin: "http://desktop-pc:48080", want: true},
		{name: "wildcard mdns hostname", host: "0.0.0.0", origin: "http://turbodrop.local:48080", want: true},
		{name: "wildcard public ip rejected", host: "0.0.0.0", origin: "http://8.8.8.8:48080", want: false},
		{name: "wildcard remote rejected", host: "0.0.0.0", origin: "http://external.host.com", want: false},
		{name: "wildcard localhost", host: "0.0.0.0", origin: "http://localhost:48080", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAllowedOrigin(tc.origin, tc.host); got != tc.want {
				t.Fatalf("isAllowedOrigin(%q, %q) = %v, want %v", tc.origin, tc.host, got, tc.want)
			}
		})
	}
}

func TestDecodeJSONBodyRejectsOversizedBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/config", strings.NewReader(`{"device_name":"`+strings.Repeat("a", 128)+`"}`))
	rec := httptest.NewRecorder()
	var payload map[string]string

	err := decodeJSONBody(rec, req, &payload, 32)
	if err == nil {
		t.Fatalf("超大 JSON 请求应返回错误")
	}
}
