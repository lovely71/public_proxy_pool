package subcheck

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/qiyiyun/public_proxy_pool/internal/model"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
	"github.com/qiyiyun/public_proxy_pool/internal/sub"
)

func TestDetectFormat(t *testing.T) {
	if got := DetectFormat("http://127.0.0.1:8080/sub/clash?token=x", "text/plain", "proxies:\n  - name: test", FormatAuto); got != FormatClash {
		t.Fatalf("expected clash, got %s", got)
	}
	if got := DetectFormat("http://127.0.0.1:8080/sub/v2ray?token=x", "text/plain", "dm1lc3M6Ly8=", FormatAuto); got != FormatPlain {
		t.Fatalf("expected plain fallback, got %s", got)
	}
}

func TestParseSubscriptionNodes_Base64AndPlain(t *testing.T) {
	obj := map[string]any{
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
	b, _ := json.Marshal(obj)
	raw := "vmess://" + base64.RawStdEncoding.EncodeToString(b)
	body := base64.StdEncoding.EncodeToString([]byte(raw + "\n"))

	nodes, format, err := ParseSubscriptionNodes(body, "http://127.0.0.1:8080/sub/v2ray", "text/plain", FormatAuto)
	if err != nil {
		t.Fatalf("ParseSubscriptionNodes failed: %v", err)
	}
	if format != FormatPlain {
		t.Fatalf("unexpected format: %s", format)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Protocol != "vmess" {
		t.Fatalf("unexpected protocol: %s", nodes[0].Protocol)
	}

	nodes, format, err = ParseSubscriptionNodes("http://1.1.1.1:8080\n", "http://127.0.0.1:8080/sub/plain", "text/plain", FormatAuto)
	if err != nil {
		t.Fatalf("ParseSubscriptionNodes plain failed: %v", err)
	}
	if format != FormatPlain {
		t.Fatalf("unexpected format for plain: %s", format)
	}
	if len(nodes) != 1 || nodes[0].Protocol != "http" {
		t.Fatalf("unexpected plain parse result: %#v", nodes)
	}
}

func TestParseSubscriptionNodes_Clash(t *testing.T) {
	nodes := []store.Node{
		{Kind: model.KindProxy, Protocol: "http", Host: "1.1.1.1", Port: 8080, RawURI: "http://1.1.1.1:8080"},
	}
	yml, err := sub.RenderClash(nodes, "https://www.cloudflare.com/cdn-cgi/trace")
	if err != nil {
		t.Fatalf("RenderClash failed: %v", err)
	}

	parsedNodes, format, err := ParseSubscriptionNodes(string(yml), "http://127.0.0.1:8080/sub/clash", "text/yaml", FormatAuto)
	if err != nil {
		t.Fatalf("ParseSubscriptionNodes clash failed: %v", err)
	}
	if format != FormatClash {
		t.Fatalf("unexpected clash format: %s", format)
	}
	if len(parsedNodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(parsedNodes))
	}
	if parsedNodes[0].Protocol != "http" {
		t.Fatalf("unexpected clash protocol: %s", parsedNodes[0].Protocol)
	}
}

func TestSummary(t *testing.T) {
	okCount, failCount := Summary([]Result{
		{OK: true},
		{OK: false},
		{OK: true},
	})
	if okCount != 2 || failCount != 1 {
		t.Fatalf("unexpected summary: ok=%d fail=%d", okCount, failCount)
	}
}

func TestTierSummary(t *testing.T) {
	counts := TierSummary([]Result{
		{Alive: true, GoogleOK: true, HuggingFaceOK: true, CheckIPOK: true},
		{Alive: true, GoogleOK: true, HuggingFaceOK: false, CheckIPOK: false},
		{Alive: true, GoogleOK: false, HuggingFaceOK: true, CheckIPOK: false},
		{Alive: false, GoogleOK: false, HuggingFaceOK: false, CheckIPOK: false},
	})
	if counts.Total != 4 || counts.Alive != 3 || counts.GoogleOK != 2 || counts.HuggingFaceOK != 2 || counts.CheckIPOK != 1 || counts.Fail != 3 {
		t.Fatalf("unexpected counts: %#v", counts)
	}
}

func TestDisplayNameFallback(t *testing.T) {
	got := displayName(store.Node{Protocol: "socks5", Host: "1.1.1.1", Port: 1080})
	if !strings.Contains(got, "socks5@1.1.1.1:1080") {
		t.Fatalf("unexpected display name: %s", got)
	}
}

func TestPrimaryErrorPriority(t *testing.T) {
	res := Result{
		AliveError:   "alive failed",
		GoogleError:  "google failed",
		HFError:      "hf failed",
		CheckIPError: "checkip failed",
	}
	if got := primaryError(res); got != "checkip failed" {
		t.Fatalf("unexpected primary error: %s", got)
	}
}

func TestPrimaryErrorPriority_HuggingFace(t *testing.T) {
	res := Result{
		AliveError:  "alive failed",
		HFError:     "hf failed",
		GoogleError: "",
	}
	if got := primaryError(res); got != "hf failed" {
		t.Fatalf("unexpected primary error: %s", got)
	}
}
