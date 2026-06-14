package discovery

import (
	"context"
	"testing"
	"time"
)

func TestPINScannerDirectProbe(t *testing.T) {
	receiver, err := NewPINReceiver("loopback-device", 9001, "dummy-fingerprint")
	if err != nil {
		t.Fatalf("创建接收端失败: %v", err)
	}
	if err := receiver.Start(); err != nil {
		t.Fatalf("启动接收端失败: %v", err)
	}
	defer receiver.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	scanner, err := NewPINScannerWithContext(ctx)
	if err != nil {
		t.Fatalf("创建扫描端失败: %v", err)
	}
	defer scanner.Stop()

	// 直接对回环地址发起探测（绕过子网扫描）
	go scanner.probeIP("127.0.0.1", receiver.GetPIN())

	select {
	case device := <-scanner.foundChan:
		if device.Name != "loopback-device" {
			t.Fatalf("设备名称不匹配: got %q, want %q", device.Name, "loopback-device")
		}
		if device.QUICPort != 9001 {
			t.Fatalf("QUIC 端口不匹配: got %d, want %d", device.QUICPort, 9001)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("未在预期时间内发现设备")
	}
}

func TestPINScannerRejectsWrongPIN(t *testing.T) {
	receiver, err := NewPINReceiver("secure-device", 9002, "dummy-fingerprint")
	if err != nil {
		t.Fatalf("创建接收端失败: %v", err)
	}
	if err := receiver.Start(); err != nil {
		t.Fatalf("启动接收端失败: %v", err)
	}
	defer receiver.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	scanner, err := NewPINScannerWithContext(ctx)
	if err != nil {
		t.Fatalf("创建扫描端失败: %v", err)
	}
	defer scanner.Stop()

	go scanner.probeIP("127.0.0.1", "000000")

	select {
	case <-scanner.foundChan:
		t.Fatalf("错误 PIN 不应被发现")
	case <-time.After(1 * time.Second):
		// 正确：没有设备被发现
	}
}
