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

// GetSubnetIPs 根据本机 IP 和实际子网掩码生成子网内所有可用主机 IP 地址
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
				mask := ipNet.Mask
				network := ipNet.IP.Mask(mask)

				// 计算子网大小
				maskOnes, maskBits := mask.Size()
				if maskBits != 32 {
					continue
				}
				hostBits := 32 - maskOnes
				if hostBits <= 0 || hostBits > 24 {
					// /32 无主机，/0 或过大的子网不扫描
					return nil, fmt.Errorf("子网掩码不适用扫描: /%d", maskOnes)
				}

				totalHosts := 1 << hostBits
				// 限制最大扫描范围，防止过大子网
				if totalHosts > 65536 {
					totalHosts = 65536
				}

				for i := 1; i < totalHosts-1; i++ { // 跳过网络地址和广播地址
					ip := make(net.IP, 4)
					copy(ip, network.To4())
					// 从低位开始加，支持任意子网大小
					for b := 3; b >= 0; b-- {
						carry := i >> ((3 - b) * 8)
						ip[b] = network.To4()[b] + byte(carry&0xff)
					}
					// 跳过网络地址本身
					if ip.Equal(network) {
						continue
					}
					// 跳过广播地址
					broadcast := make(net.IP, 4)
					for b := 0; b < 4; b++ {
						broadcast[b] = network.To4()[b] | ^mask[b]
					}
					if ip.Equal(broadcast) {
						continue
					}
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
