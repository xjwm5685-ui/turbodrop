package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst interface{}, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

// isAllowedOrigin 校验请求 Origin 是否被允许
// allowedHost 为当前 Server 绑定的 Web 主机地址
func isAllowedOrigin(origin, allowedHost string) bool {
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		if allowedHost == "" {
			return false
		}
		wh := strings.ToLower(allowedHost)
		// 0.0.0.0 / [::] 表示绑定所有网卡，接受局域网来源
		if isWildcardWebHost(wh) {
			return isLocalNetworkOriginHost(host)
		}
		return host == wh
	}
}

func isWildcardWebHost(host string) bool {
	wh := strings.ToLower(strings.TrimSpace(host))
	return wh == "0.0.0.0" || wh == "[::]" || wh == "::"
}

func isLocalNetworkOriginHost(host string) bool {
	if host == "" {
		return false
	}

	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || isLocalInterfaceIP(ip)
	}

	return !strings.Contains(host, ".") || strings.HasSuffix(host, ".local")
}

func isLocalInterfaceIP(ip net.IP) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}

	for _, addr := range addrs {
		var localIP net.IP
		switch typedAddr := addr.(type) {
		case *net.IPNet:
			localIP = typedAddr.IP
		case *net.IPAddr:
			localIP = typedAddr.IP
		}

		if localIP != nil && localIP.Equal(ip) {
			return true
		}
	}
	return false
}

func (s *Server) isOriginAllowed(origin string) bool {
	return isAllowedOrigin(origin, s.allowedWebHost)
}

func (s *Server) localOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRemote(r.RemoteAddr) {
			respondError(w, http.StatusForbidden, "此操作只能在电脑本机打开 localhost 后使用")
			return
		}
		next(w, r)
	}
}

func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}

	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func sanitizeUploadFileName(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "." || base == "" {
		return "uploaded-file"
	}

	replacer := strings.NewReplacer(
		"\\", "_",
		"/", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(base)
}

func isManagedUploadPath(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	absUploadDir, err := filepath.Abs(uploadDir)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(absUploadDir, absPath)
	if err != nil {
		return false
	}

	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func splitHostPortOrDefault(addr string, defaultHost string, defaultPort int) (string, int) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return defaultHost, defaultPort
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return host, defaultPort
	}
	return host, portNum
}
