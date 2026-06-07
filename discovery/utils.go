package discovery

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"

	"github.com/zeebo/blake3"
)

// GeneratePIN 生成一个 6 位数字 PIN 码
func GeneratePIN() string {
	b := make([]byte, 3) // 3 字节可以生成足够的随机性
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("生成 PIN 随机数失败: %v", err))
	}
	// 确保生成 100000-999999 之间的数字
	pin := (int(b[0])<<16 | int(b[1])<<8 | int(b[2])) % 900000
	return fmt.Sprintf("%06d", pin+100000)
}

// HashPIN 使用 BLAKE3 对 PIN 码加盐哈希
func HashPIN(pin string, salt string) string {
	h := blake3.New()
	h.Write([]byte(pin + salt))
	return hex.EncodeToString(h.Sum(nil))
}

// GetLocalIP 获取本机局域网 IP 地址
func GetLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// GetSubnetIPs 根据本机 IP 和子网掩码生成子网内所有 IP 地址
func GetSubnetIPs(localIP string) ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var ips []string
	targetIP := net.ParseIP(localIP)

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}

			// 找到匹配的网卡
			if ipNet.IP.Equal(targetIP) {
				// 计算网络地址
				network := ipNet.IP.Mask(ipNet.Mask)
				
				// 生成该子网内所有可能的 IP
				for i := 1; i < 255; i++ {
					ip := make(net.IP, len(network))
					copy(ip, network)
					ip[3] = byte(i)
					ips = append(ips, ip.String())
				}
				return ips, nil
			}
		}
	}

	return nil, fmt.Errorf("无法找到匹配的网络接口")
}

// GenerateDeviceID 生成唯一的设备标识
func GenerateDeviceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("生成设备 ID 随机数失败: %v", err))
	}
	return hex.EncodeToString(b)
}
