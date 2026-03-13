package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/qiyiyun/public_proxy_pool/internal/v2ray"
	"gopkg.in/yaml.v3"
)

type URICandidate struct {
	Parsed *v2ray.Parsed
}

func ParseBase64Subscription(content string) ([]URICandidate, error) {
	raw := strings.TrimSpace(content)
	if raw == "" {
		return nil, fmt.Errorf("empty subscription")
	}
	decoded, err := decodeBase64Maybe(raw)
	if err != nil {
		return nil, err
	}
	lines := splitLines(string(decoded))
	out := make([]URICandidate, 0, len(lines))
	for _, line := range lines {
		p, err := v2ray.ParseURI(line)
		if err != nil || p == nil {
			continue
		}
		out = append(out, URICandidate{Parsed: p})
	}
	return out, nil
}

func ParseClashYAML(content string) ([]URICandidate, error) {
	type doc struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	var d doc
	if err := yaml.Unmarshal([]byte(content), &d); err != nil {
		return nil, err
	}
	out := make([]URICandidate, 0, len(d.Proxies))
	for _, p := range d.Proxies {
		uri := clashProxyToURI(p)
		if uri == "" {
			continue
		}
		parsed, err := v2ray.ParseURI(uri)
		if err != nil || parsed == nil {
			continue
		}
		out = append(out, URICandidate{Parsed: parsed})
	}
	return out, nil
}

func FetchAndParseSubscriptionBase64(ctx context.Context, url string, timeout time.Duration, etag, lastModified string) (FetchResult, []URICandidate) {
	res := FetchText(ctx, url, timeout, etag, lastModified)
	if !res.OK || res.NotModified {
		return res, nil
	}
	nodes, err := ParseBase64Subscription(res.Content)
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		return res, nil
	}
	return res, nodes
}

func FetchAndParseClashYAML(ctx context.Context, url string, timeout time.Duration, etag, lastModified string) (FetchResult, []URICandidate) {
	res := FetchText(ctx, url, timeout, etag, lastModified)
	if !res.OK || res.NotModified {
		return res, nil
	}
	nodes, err := ParseClashYAML(res.Content)
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		return res, nil
	}
	return res, nodes
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func decodeBase64Maybe(raw string) ([]byte, error) {
	// If it already looks like uri lines, return as-is.
	if strings.Contains(raw, "://") {
		return []byte(raw), nil
	}
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimRight(trimmed, "=")
	if trimmed == "" {
		return nil, fmt.Errorf("empty")
	}
	padded := trimmed
	for len(padded)%4 != 0 {
		padded += "="
	}
	if b, err := base64.StdEncoding.DecodeString(padded); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(trimmed); err == nil {
		return b, nil
	}
	if b, err := base64.RawURLEncoding.DecodeString(trimmed); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(padded); err == nil {
		return b, nil
	}
	return nil, fmt.Errorf("invalid base64")
}

func clashProxyToURI(p map[string]any) string {
	getS := func(path string) string {
		v := getPath(p, path)
		if v == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	getI := func(path string) int {
		v := getPath(p, path)
		switch t := v.(type) {
		case int:
			return t
		case int64:
			return int(t)
		case float64:
			return int(t)
		case string:
			i, _ := strconv.Atoi(strings.TrimSpace(t))
			return i
		default:
			i, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprintf("%v", v)))
			return i
		}
	}
	getB := func(path string) bool {
		v := getPath(p, path)
		switch t := v.(type) {
		case bool:
			return t
		case string:
			switch strings.ToLower(strings.TrimSpace(t)) {
			case "1", "true", "yes", "on":
				return true
			default:
				return false
			}
		default:
			return false
		}
	}

	typ := strings.ToLower(getS("type"))
	name := getS("name")
	server := getS("server")
	port := getI("port")
	if server == "" || port <= 0 {
		return ""
	}
	switch typ {
	case "http":
		username := getS("username")
		password := getS("password")
		if username != "" {
			return fmt.Sprintf("http://%s:%s@%s:%d#%s", url.QueryEscape(username), url.QueryEscape(password), server, port, url.QueryEscape(name))
		}
		return fmt.Sprintf("http://%s:%d#%s", server, port, url.QueryEscape(name))
	case "socks5":
		username := getS("username")
		password := getS("password")
		if username != "" {
			return fmt.Sprintf("socks5://%s:%s@%s:%d#%s", url.QueryEscape(username), url.QueryEscape(password), server, port, url.QueryEscape(name))
		}
		return fmt.Sprintf("socks5://%s:%d#%s", server, port, url.QueryEscape(name))
	case "ss":
		cipher := getS("cipher")
		password := getS("password")
		creds := base64.RawStdEncoding.EncodeToString([]byte(cipher + ":" + password))
		return fmt.Sprintf("ss://%s@%s:%d#%s", creds, server, port, url.QueryEscape(name))
	case "vmess":
		uuid := getS("uuid")
		tls := ""
		if getB("tls") {
			tls = "tls"
		}
		network := getS("network")
		if network == "" {
			network = getS("network") // legacy
		}
		path := getS("ws-opts.path")
		hostHdr := getS("ws-opts.headers.Host")
		obj := map[string]any{
			"v":   "2",
			"ps":  name,
			"add": server,
			"port": fmt.Sprintf("%d", port),
			"id":  uuid,
			"aid": "0",
			"net": network,
			"type": "none",
			"host": hostHdr,
			"path": path,
			"tls":  tls,
		}
		b, _ := json.Marshal(obj)
		return "vmess://" + base64.RawStdEncoding.EncodeToString(b)
	case "vless":
		uuid := getS("uuid")
		transport := getS("network")
		path := getS("ws-opts.path")
		hostHdr := getS("ws-opts.headers.Host")
		q := url.Values{}
		if transport != "" {
			q.Set("type", transport)
		}
		if getB("tls") {
			q.Set("security", "tls")
		}
		if path != "" {
			q.Set("path", path)
		}
		if hostHdr != "" {
			q.Set("host", hostHdr)
		}
		return fmt.Sprintf("vless://%s@%s:%d?%s#%s", uuid, server, port, q.Encode(), url.QueryEscape(name))
	case "trojan":
		password := getS("password")
		sni := getS("sni")
		q := url.Values{}
		q.Set("security", "tls")
		if sni != "" {
			q.Set("sni", sni)
		}
		return fmt.Sprintf("trojan://%s@%s:%d?%s#%s", url.QueryEscape(password), server, port, q.Encode(), url.QueryEscape(name))
	default:
		return ""
	}
}

func getPath(m map[string]any, path string) any {
	if m == nil {
		return nil
	}
	parts := strings.Split(path, ".")
	var cur any = m
	for _, p := range parts {
		switch mm := cur.(type) {
		case map[string]any:
			cur = mm[p]
		case map[any]any:
			cur = mm[p]
		default:
			return nil
		}
	}
	return cur
}
