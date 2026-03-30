package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFetchAllNodeMaven_DedupAcrossPages(t *testing.T) {
	type rawProxy struct {
		IPAddress string `json:"ip_address"`
		Port      any    `json:"port"`
		Protocol  string `json:"protocol"`
	}
	type resp struct {
		Total   int        `json:"total"`
		Proxies []rawProxy `json:"proxies"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		perPage, _ := strconv.Atoi(q.Get("per_page"))

		out := resp{Total: 5}
		if perPage == 1 {
			out.Proxies = []rawProxy{{IPAddress: "0.0.0.0", Port: "0", Protocol: "http"}}
		} else {
			switch page {
			case 1:
				out.Proxies = []rawProxy{
					{IPAddress: "1.1.1.1", Port: "80", Protocol: "http"},
					{IPAddress: "2.2.2.2", Port: 1080, Protocol: "socks5"},
				}
			case 2:
				out.Proxies = []rawProxy{
					{IPAddress: "2.2.2.2", Port: "1080", Protocol: "SOCKS5"}, // duplicate
					{IPAddress: "3.3.3.3", Port: "8080", Protocol: "http"},
				}
			case 3:
				out.Proxies = []rawProxy{
					{IPAddress: "4.4.4.4", Port: 3128, Protocol: "https"},
				}
			default:
				out.Proxies = nil
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()

	ctx := context.Background()
	items, err := FetchAllNodeMaven(ctx, NodeMavenFetchConfig{
		BaseURL:     srv.URL,
		UserAgent:   "test",
		PerPage:     2,
		MaxPages:    10,
		Concurrency: 3,
		Timeout:     2 * time.Second,
	})
	if err != nil {
		t.Fatalf("FetchAllNodeMaven: %v", err)
	}

	seen := map[string]bool{}
	for _, it := range items {
		key := strings.ToUpper(strings.TrimSpace(it.Protocol)) + "|" + it.IPAddress + ":" + strconv.Itoa(it.Port)
		if seen[key] {
			t.Fatalf("duplicate key: %s", key)
		}
		seen[key] = true
	}
	if len(items) != 4 {
		t.Fatalf("want 4 unique items, got %d", len(items))
	}
}

func TestNodeMavenProxy_UnmarshalStringPort(t *testing.T) {
	var got NodeMavenProxy
	if err := json.Unmarshal([]byte(`{
		"ip_address":"96.126.113.216",
		"port":"59166",
		"country":"United States",
		"protocol":"HTTPS",
		"type":"Unknown",
		"latency":"4032"
	}`), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Port != 59166 {
		t.Fatalf("expected port 59166, got %d", got.Port)
	}
	if SafeInt(got.LatencyRaw) != 4032 {
		t.Fatalf("expected latency 4032, got %d", SafeInt(got.LatencyRaw))
	}
}
