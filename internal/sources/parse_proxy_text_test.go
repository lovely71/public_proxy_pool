package sources

import "testing"

func TestParseProxyText_Generic(t *testing.T) {
	content := `
http://user:pass@1.2.3.4:8080
1.1.1.1:80
socks5://5.6.7.8:1080
invalid:123
`
	out := ParseProxyText(content, "generic", "http")
	if len(out) < 3 {
		t.Fatalf("expected >=3, got %d", len(out))
	}
	found := map[string]bool{}
	for _, c := range out {
		found[c.ProxyURL] = true
	}
	if !found["http://user:pass@1.2.3.4:8080"] {
		t.Fatalf("missing normalized auth proxy")
	}
	if !found["http://1.1.1.1:80"] {
		t.Fatalf("missing ip:port with default scheme")
	}
	if !found["socks5://5.6.7.8:1080"] {
		t.Fatalf("missing socks5 proxy")
	}
}

func TestParseProxyText_TopChina(t *testing.T) {
	content := `
| IP:PORT | Country | User | H |
| 8.8.8.8:3128 | x | abc | y |
| 9.9.9.9:8080 | x | -- | y |
`
	out := ParseProxyText(content, "topchina", "http")
	if len(out) != 1 {
		t.Fatalf("expected 1, got %d", len(out))
	}
	if out[0].ProxyURL != "http://abc:1@8.8.8.8:3128" {
		t.Fatalf("unexpected proxy url: %q", out[0].ProxyURL)
	}
}

