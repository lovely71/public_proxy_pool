package sub

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/qiyiyun/public_proxy_pool/internal/model"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
	"gopkg.in/yaml.v3"
)

func TestRenderClash_Basic(t *testing.T) {
	vmessObj := map[string]any{
		"v":    "2",
		"ps":   "vmess1",
		"add":  "example.com",
		"port": "443",
		"id":   "00000000-0000-0000-0000-000000000000",
		"aid":  "0",
		"net":  "tcp",
		"type": "none",
		"host": "",
		"path": "",
		"tls":  "tls",
	}
	b, _ := json.Marshal(vmessObj)
	vmessURI := "vmess://" + base64.RawStdEncoding.EncodeToString(b)

	nodes := []store.Node{
		{Kind: model.KindProxy, Protocol: "http", Host: "1.1.1.1", Port: 8080, RawURI: "http://1.1.1.1:8080", Country: "HK"},
		{Kind: model.KindProxy, Protocol: "socks5", Host: "2.2.2.2", Port: 1080, RawURI: "socks5://2.2.2.2:1080", Country: "JP"},
		{Kind: model.KindV2Ray, Protocol: "ss", RawURI: "ss://YWVzLTEyOC1nY206cGFzcw==@3.3.3.3:8388#ss1", Country: "US"},
		{Kind: model.KindV2Ray, Protocol: "vmess", RawURI: vmessURI, Country: "SG"},
	}

	yml, err := RenderClash(nodes, "https://www.cloudflare.com/cdn-cgi/trace")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	s := string(yml)
	if !strings.Contains(s, "proxy-groups:") {
		t.Fatalf("expected proxy-groups, got:\n%s", s)
	}
	if !strings.Contains(s, "proxies:") {
		t.Fatalf("expected proxies, got:\n%s", s)
	}
	if !strings.Contains(s, "mixed-port: 7890") {
		t.Fatalf("expected mixed-port, got:\n%s", s)
	}
	if !strings.Contains(s, "dns:") {
		t.Fatalf("expected dns section, got:\n%s", s)
	}
	if !strings.Contains(s, "节点选择") || !strings.Contains(s, "自动选择") || !strings.Contains(s, "故障转移") {
		t.Fatalf("expected chinese proxy groups, got:\n%s", s)
	}
	if !strings.Contains(s, "港澳节点") || !strings.Contains(s, "日本节点") || !strings.Contains(s, "美国节点") {
		t.Fatalf("expected region groups, got:\n%s", s)
	}
	if !strings.Contains(s, "DOMAIN-SUFFIX,cn,DIRECT") || !strings.Contains(s, "GEOIP,CN,DIRECT") {
		t.Fatalf("expected china routing rules, got:\n%s", s)
	}

	var cfg clashConfig
	if err := yaml.Unmarshal(yml, &cfg); err != nil {
		t.Fatalf("unmarshal clash yaml failed: %v", err)
	}
	if len(cfg.Proxies) < 4 {
		t.Fatalf("expected 4 proxies, got %d", len(cfg.Proxies))
	}

	var names []string
	for _, proxy := range cfg.Proxies {
		if name, _ := proxy["name"].(string); name != "" {
			names = append(names, name)
		}
	}
	joined := strings.Join(names, "\n")
	if !strings.Contains(joined, "🇭🇰 香港 | http-1.1.1.1:8080") {
		t.Fatalf("expected decorated hk node name, got:\n%s", joined)
	}
	if !strings.Contains(joined, "🇯🇵 日本 | socks5-2.2.2.2:1080") {
		t.Fatalf("expected decorated jp node name, got:\n%s", joined)
	}
	if !strings.Contains(joined, "🇺🇸 美国 | ss1") {
		t.Fatalf("expected decorated us node name, got:\n%s", joined)
	}
}
