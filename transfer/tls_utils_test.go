package transfer

import "testing"

func TestCertificateFingerprintDeterministic(t *testing.T) {
	cert := []byte("sample-cert")
	fp1 := CertificateFingerprint(cert)
	fp2 := CertificateFingerprint(cert)
	if fp1 == "" {
		t.Fatalf("CertificateFingerprint() 不应返回空字符串")
	}
	if fp1 != fp2 {
		t.Fatalf("相同证书应得到相同指纹: %q != %q", fp1, fp2)
	}
}

func TestGenerateClientTLSConfigRequiresExpectedFingerprint(t *testing.T) {
	cfg := GenerateClientTLSConfig("")
	if cfg.VerifyPeerCertificate == nil {
		t.Fatalf("VerifyPeerCertificate 不应为空")
	}
	if err := cfg.VerifyPeerCertificate([][]byte{{1, 2, 3}}, nil); err == nil {
		t.Fatalf("缺少预期指纹时应返回错误")
	}
}
