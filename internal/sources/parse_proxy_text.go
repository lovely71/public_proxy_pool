package sources

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/qiyiyun/public_proxy_pool/internal/model"
)

var (
	proxyURLRe = regexp.MustCompile(`(?i)(?:(?:https?|socks4|socks5|socks5h)://[^\s'\"<>]+)`)
	ipPortRe   = regexp.MustCompile(`\b((?:\d{1,3}\.){3}\d{1,3}):(\d{2,5})\b`)
	topChinaRe = regexp.MustCompile(`(?m)^\|\s*([\d.]+:\d+)\s*\|\s*[^|]+\|\s*([^|\s]+)\s*\|`)
)

func ParseProxyText(content string, parser string, defaultScheme string) []ProxyCandidate {
	switch strings.ToLower(strings.TrimSpace(parser)) {
	case "topchina":
		return parseTopChina(content)
	default:
		return parseGeneric(content, defaultScheme)
	}
}

type ProxyCandidate struct {
	ProxyURL string // normalized
	Scheme   string
	Host     string
	Port     int
	Username string
	Password string
}

func parseTopChina(content string) []ProxyCandidate {
	out := make([]ProxyCandidate, 0, 1024)
	seen := make(map[string]struct{}, 1024)
	matches := topChinaRe.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		ipPort := strings.TrimSpace(m[1])
		username := strings.TrimSpace(m[2])
		if ipPort == "" || username == "" || strings.HasPrefix(username, "--") {
			continue
		}
		parts := strings.Split(ipPort, ":")
		if len(parts) != 2 || !model.IsValidIPv4(parts[0]) {
			continue
		}
		port, err := strconv.Atoi(parts[1])
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		raw := "http://" + username + ":1@" + parts[0] + ":" + strconv.Itoa(port)
		_, normalized, err := model.NormalizeProxyURL(raw)
		if err != nil {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, candidateFromNormalized(normalized))
	}
	return out
}

func parseGeneric(content string, defaultScheme string) []ProxyCandidate {
	out := make([]ProxyCandidate, 0, 4096)
	seen := make(map[string]struct{}, 4096)

	for _, m := range proxyURLRe.FindAllString(content, -1) {
		_, normalized, err := model.NormalizeProxyURL(m)
		if err != nil {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, candidateFromNormalized(normalized))
	}

	scheme := strings.ToLower(strings.TrimSpace(defaultScheme))
	switch scheme {
	case "http", "https", "socks4", "socks5", "socks5h":
	default:
		scheme = "http"
	}

	for _, m := range ipPortRe.FindAllStringSubmatch(content, -1) {
		ip := strings.TrimSpace(m[1])
		portStr := strings.TrimSpace(m[2])
		if !model.IsValidIPv4(ip) {
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		raw := scheme + "://" + ip + ":" + strconv.Itoa(port)
		_, normalized, err := model.NormalizeProxyURL(raw)
		if err != nil {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, candidateFromNormalized(normalized))
	}

	return out
}

func candidateFromNormalized(normalized string) ProxyCandidate {
	parsed, _, err := model.NormalizeProxyURL(normalized)
	if err != nil || parsed == nil {
		return ProxyCandidate{ProxyURL: normalized}
	}
	host := parsed.Hostname()
	port := 0
	if p := parsed.Port(); p != "" {
		port, _ = strconv.Atoi(p)
	}
	username := ""
	password := ""
	if parsed.User != nil {
		username = parsed.User.Username()
		if pw, ok := parsed.User.Password(); ok {
			password = pw
		}
	}
	return ProxyCandidate{
		ProxyURL: normalized,
		Scheme:   strings.ToLower(parsed.Scheme),
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
	}
}
