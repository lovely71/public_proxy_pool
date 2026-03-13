package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const (
	KindProxy = "proxy"
	KindV2Ray = "v2ray"
)

func IsValidIPv4(ip string) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	return parsed != nil && parsed.To4() != nil
}

func Fingerprint(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			_, _ = h.Write([]byte{0})
		}
		_, _ = h.Write([]byte(strings.TrimSpace(p)))
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16]) // short but stable
}

func NormalizeProxyURL(raw string) (*url.URL, string, error) {
	item := strings.TrimSpace(raw)
	item = strings.TrimRight(item, ").,;]")
	if item == "" {
		return nil, "", fmt.Errorf("empty")
	}
	parsed, err := url.Parse(item)
	if err != nil {
		return nil, "", err
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "http", "https", "socks4", "socks5", "socks5h":
	default:
		return nil, "", fmt.Errorf("unsupported scheme: %s", scheme)
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return nil, "", fmt.Errorf("missing host")
	}
	port := parsed.Port()
	if port == "" {
		return nil, "", fmt.Errorf("missing port")
	}
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return nil, "", fmt.Errorf("invalid port")
	}
	if IsValidIPv4(host) == true && host != parsed.Hostname() {
		// Defensive; should not happen.
		host = parsed.Hostname()
	}
	// Normalize: scheme://[user[:pass]@]host:port
	userInfo := ""
	if parsed.User != nil {
		username := parsed.User.Username()
		password, hasPassword := parsed.User.Password()
		if username != "" {
			userInfo = username
			if hasPassword {
				userInfo += ":" + password
			}
			userInfo += "@"
		}
	}
	normalized := fmt.Sprintf("%s://%s%s:%d", scheme, userInfo, host, portNum)
	return parsed, normalized, nil
}

