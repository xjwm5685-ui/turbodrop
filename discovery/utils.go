package discovery

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
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

// HashPIN 使用 HMAC-SHA256 对 PIN 码做加盐（挑战）哈希
// challenge 是接收端生成的每次会话不同的挑战值/nonce，防止离线预计算整个 PIN 表
func HashPIN(pin string, challenge string) string {
	return HashPINWithChallenge(pin, challenge)
}

// HashPINWithChallenge 是 HashPIN 的显式别名，语义上强调 challenge/nonce
func HashPINWithChallenge(pin string, challenge string) string {
	h := hmac.New(sha256.New, []byte(challenge))
	h.Write([]byte(pin))
	return hex.EncodeToString(h.Sum(nil))
}

// GetLocalIP 获取本机局域网 IP 地址
func GetLocalIP() (string, error) {
	if ip, ok := getPreferredLocalIPv4(); ok {
		return ip.String(), nil
	}

	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

func getPreferredLocalIPv4() (net.IP, bool) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, false
	}

	var bestIP net.IP
	bestScore := -1
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			score := scoreLocalIPv4(ip, iface)
			if score > bestScore {
				bestScore = score
				bestIP = append(net.IP(nil), ip...)
			}
		}
	}

	return bestIP, bestScore >= 0
}

func scoreLocalIPv4(ip net.IP, iface net.Interface) int {
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() {
		return -1
	}

	score := 0
	if ip.IsPrivate() {
		score += 1000
	}
	if !isBenchmarkIPv4(ip) {
		score += 100
	}
	if iface.Flags&net.FlagBroadcast != 0 {
		score += 30
	}
	if iface.Flags&net.FlagPointToPoint == 0 {
		score += 10
	}
	if len(iface.HardwareAddr) > 0 {
		score += 20
	}
	return score
}

func isBenchmarkIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	return ip4 != nil && ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19)
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
