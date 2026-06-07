package discovery

import (
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
