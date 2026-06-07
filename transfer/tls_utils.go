package transfer

import (
	"crypto/sha256"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// GenerateEphemeralTLSConfig 生成临时的自签名 TLS 证书
// 用于 QUIC 连接，因为 QUIC 强制要求 TLS 1.3
func GenerateEphemeralTLSConfig() (*tls.Config, string, error) {
	// 生成 RSA 私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, "", fmt.Errorf("生成私钥失败: %w", err)
	}

	// 创建证书模板
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"TurboDrop P2P"},
			CommonName:   "turbodrop.local",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour), // 24小时有效期
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// 自签名证书
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, "", fmt.Errorf("创建证书失败: %w", err)
	}

	// 转换为 PEM 格式
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// 加载为 TLS 证书
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, "", fmt.Errorf("加载证书失败: %w", err)
	}

	// 创建 TLS 配置
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"turbodrop-v1"}, // ALPN 协议标识
		MinVersion:   tls.VersionTLS13,          // QUIC 要求 TLS 1.3
	}

	return tlsConfig, CertificateFingerprint(certDER), nil
}

// GenerateClientTLSConfig 生成客户端 TLS 配置
// 跳过证书验证，因为是局域网 P2P 自签名证书
func GenerateClientTLSConfig(expectedFingerprint string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"turbodrop-v1"},
		MinVersion:         tls.VersionTLS13,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if expectedFingerprint == "" {
				return fmt.Errorf("缺少目标证书指纹")
			}
			if len(rawCerts) == 0 {
				return fmt.Errorf("未收到对端证书")
			}
			actual := CertificateFingerprint(rawCerts[0])
			if actual != expectedFingerprint {
				return fmt.Errorf("证书指纹不匹配")
			}
			return nil
		},
	}
}

func CertificateFingerprint(certDER []byte) string {
	sum := sha256.Sum256(certDER)
	return hex.EncodeToString(sum[:])
}
