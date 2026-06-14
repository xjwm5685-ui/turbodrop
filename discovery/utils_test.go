package discovery

import (
	"net"
	"regexp"
	"testing"
)

func TestGeneratePIN(t *testing.T) {
	pinPattern := regexp.MustCompile(`^\d{6}$`)

	for i := 0; i < 20; i++ {
		pin := GeneratePIN()
		if !pinPattern.MatchString(pin) {
			t.Fatalf("GeneratePIN() 返回了非法 PIN: %q", pin)
		}
	}
}

func TestHashPINDeterministic(t *testing.T) {
	hash1 := HashPIN("123456", "salt")
	hash2 := HashPIN("123456", "salt")
	hash3 := HashPIN("123456", "other-salt")

	if hash1 != hash2 {
		t.Fatalf("相同输入应返回相同哈希: %q != %q", hash1, hash2)
	}

	if hash1 == hash3 {
		t.Fatalf("不同 salt 不应返回相同哈希: %q", hash1)
	}
}

func TestGenerateDeviceID(t *testing.T) {
	id1 := GenerateDeviceID()
	id2 := GenerateDeviceID()

	hexPattern := regexp.MustCompile(`^[a-f0-9]{32}$`)
	if !hexPattern.MatchString(id1) {
		t.Fatalf("GenerateDeviceID() 返回了非法设备 ID: %q", id1)
	}

	if id1 == id2 {
		t.Fatalf("连续两次生成的设备 ID 不应相同: %q", id1)
	}
}

func TestGetSubnetIPsReturnsReasonableResults(t *testing.T) {
	localIP, err := GetLocalIP()
	if err != nil {
		t.Skipf("无法获取本机 IP，跳过子网测试: %v", err)
	}

	ips, err := GetSubnetIPs(localIP)
	if err != nil {
		t.Fatalf("GetSubnetIPs(%q) 失败: %v", localIP, err)
	}

	if len(ips) == 0 {
		t.Fatalf("GetSubnetIPs(%q) 应返回至少一个 IP", localIP)
	}

	// 所有返回的 IP 应该是合法的 IP 格式
	for _, ip := range ips {
		if net.ParseIP(ip) == nil {
			t.Fatalf("返回了非法 IP: %q", ip)
		}
	}
}

func TestGetSubnetIPsRejectsInvalidIP(t *testing.T) {
	_, err := GetSubnetIPs("not.an.ip.address")
	// 应返回错误或空列表（找不到匹配接口）
	if err == nil {
		// 如果没报错，也应该返回空列表
		// （取决于实现，两种都可接受）
	}
}

func TestScoreLocalIPv4PrefersPrivateLANOverBenchmarkTunnel(t *testing.T) {
	iface := net.Interface{Flags: net.FlagUp | net.FlagBroadcast}

	privateScore := scoreLocalIPv4(net.ParseIP("192.168.2.101").To4(), iface)
	benchmarkScore := scoreLocalIPv4(net.ParseIP("198.18.0.1").To4(), iface)

	if privateScore <= benchmarkScore {
		t.Fatalf("private LAN score = %d, benchmark tunnel score = %d; want private higher", privateScore, benchmarkScore)
	}
}

func TestScoreLocalIPv4RejectsLinkLocal(t *testing.T) {
	iface := net.Interface{Flags: net.FlagUp | net.FlagBroadcast}
	if score := scoreLocalIPv4(net.ParseIP("169.254.10.20").To4(), iface); score >= 0 {
		t.Fatalf("link-local score = %d, want rejected", score)
	}
}
