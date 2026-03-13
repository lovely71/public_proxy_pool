package v2ray

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestParseURI_SS(t *testing.T) {
	p, err := ParseURI("ss://YWVzLTEyOC1nY206cGFzcw==@1.2.3.4:8388#ss1")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if p.Protocol != "ss" || p.Host != "1.2.3.4" || p.Port != 8388 {
		t.Fatalf("unexpected parsed: %#v", p)
	}
	if p.Method == "" || p.Password == "" {
		t.Fatalf("missing ss creds: %#v", p)
	}
}

func TestParseURI_VMess(t *testing.T) {
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
	p, err := ParseURI(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if p.Protocol != "vmess" || p.Host != "example.com" || p.Port != 443 {
		t.Fatalf("unexpected parsed: %#v", p)
	}
	if p.UUID == "" {
		t.Fatalf("missing uuid: %#v", p)
	}
}

