package transfer

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zeebo/blake3"
)

// fileHash 计算文件 BLAKE3 哈希（十六进制）
func fileHash(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("打开文件失败: %v", err)
	}
	defer f.Close()

	h := blake3.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("计算哈希失败: %v", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// waitForListener 等待接收端实际监听地址就绪（支持端口 0）
func waitForListener(t *testing.T, receiver *QUICFileReceiver) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		addr := receiver.GetListenAddr()
		if _, port, err := net.SplitHostPort(addr); err == nil && port != "0" && port != "" {
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("接收端监听地址未就绪")
	return ""
}

func TestQUICSenderReceiverIntegration(t *testing.T) {
	tests := []struct {
		name string
		size int64
	}{
		{name: "small file", size: 12345},
		{name: "zero-byte file", size: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			saveDir := t.TempDir()
			srcDir := t.TempDir()
			srcPath := filepath.Join(srcDir, "payload.bin")

			data := make([]byte, tc.size)
			for i := range data {
				data[i] = byte(i % 256)
			}
			if err := os.WriteFile(srcPath, data, 0644); err != nil {
				t.Fatalf("创建源文件失败: %v", err)
			}
			expectedHash := fileHash(t, srcPath)

			receiver := NewQUICFileReceiver(0, saveDir, nil, "")
			recvCtx, recvCancel := context.WithCancel(context.Background())
			defer recvCancel()

			recvDone := make(chan error, 1)
			go func() {
				recvDone <- receiver.Start(recvCtx)
			}()

			addr := waitForListener(t, receiver)
			_, portStr, err := net.SplitHostPort(addr)
			if err != nil {
				t.Fatalf("解析监听地址失败: %v", err)
			}
			port, err := net.LookupPort("udp", portStr)
			if err != nil {
				port, _ = net.LookupPort("tcp", portStr)
			}

			sender := NewQUICFileSender(srcPath, "127.0.0.1", port)
			sender.SetExpectedCertFingerprint(receiver.GetCertFingerprint())

			sendCtx, sendCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer sendCancel()

			if err := sender.Send(sendCtx); err != nil {
				t.Fatalf("发送失败: %v", err)
			}

			recvCancel()
			select {
			case <-recvDone:
			case <-time.After(5 * time.Second):
				t.Fatalf("接收端未在预期时间内退出")
			}

			dstPath := filepath.Join(saveDir, "payload.bin")
			info, err := os.Stat(dstPath)
			if err != nil {
				t.Fatalf("接收文件不存在: %v", err)
			}
			if info.Size() != tc.size {
				t.Fatalf("接收文件大小不匹配: got %d, want %d", info.Size(), tc.size)
			}

			gotHash := fileHash(t, dstPath)
			if gotHash != expectedHash {
				t.Fatalf("接收文件哈希不匹配: got %s, want %s", gotHash, expectedHash)
			}
		})
	}
}

func TestQUICReceiverRandomPort(t *testing.T) {
	receiver := NewQUICFileReceiver(0, t.TempDir(), nil, "")
	if !strings.HasSuffix(receiver.listenAddr, ":0") {
		t.Fatalf("创建时应使用端口 0, got %q", receiver.listenAddr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- receiver.Start(ctx)
	}()

	addr := waitForListener(t, receiver)
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("解析地址失败: %v", err)
	}
	if host == "" || port == "" || port == "0" {
		t.Fatalf("随机端口未正确分配: %s", addr)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("接收端未退出")
	}
}
