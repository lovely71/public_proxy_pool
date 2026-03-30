package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type NodeMavenProxy struct {
	IPAddress  string `json:"ip_address"`
	Port       int    `json:"port"`
	Country    string `json:"country"`
	Protocol   string `json:"protocol"`
	Type       string `json:"type"`
	LatencyRaw any    `json:"latency"`
}

type nodeMavenResp struct {
	Total   int              `json:"total"`
	Proxies []NodeMavenProxy `json:"proxies"`
}

func (p *NodeMavenProxy) UnmarshalJSON(data []byte) error {
	type rawNodeMavenProxy struct {
		IPAddress  string `json:"ip_address"`
		Port       any    `json:"port"`
		Country    string `json:"country"`
		Protocol   string `json:"protocol"`
		Type       string `json:"type"`
		LatencyRaw any    `json:"latency"`
	}

	var raw rawNodeMavenProxy
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return err
	}

	p.IPAddress = raw.IPAddress
	p.Port = SafeInt(raw.Port)
	p.Country = raw.Country
	p.Protocol = raw.Protocol
	p.Type = raw.Type
	p.LatencyRaw = raw.LatencyRaw
	return nil
}

type NodeMavenClient struct {
	BaseURL   string
	UserAgent string
	Timeout   time.Duration
}

func (c NodeMavenClient) FetchPage(ctx context.Context, page, perPage int) (total int, items []NodeMavenProxy, err error) {
	base := strings.TrimRight(c.BaseURL, "/")
	u, _ := url.Parse(base + "/wp-json/proxy-list/v1/proxies")
	q := u.Query()
	q.Set("page", strconv.Itoa(page))
	q.Set("per_page", strconv.Itoa(perPage))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, nil, err
	}
	ua := c.UserAgent
	if strings.TrimSpace(ua) == "" {
		ua = "Mozilla/5.0"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	client := &http.Client{Timeout: c.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return 0, nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var obj nodeMavenResp
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return 0, nil, err
	}
	return obj.Total, obj.Proxies, nil
}

type NodeMavenFetchConfig struct {
	BaseURL     string
	UserAgent   string
	PerPage     int
	MaxPages    int
	Concurrency int
	Timeout     time.Duration
}

func FetchAllNodeMaven(ctx context.Context, cfg NodeMavenFetchConfig) ([]NodeMavenProxy, error) {
	perPage := cfg.PerPage
	if perPage <= 0 {
		perPage = 100
	}
	maxPages := cfg.MaxPages
	if maxPages <= 0 {
		maxPages = 5
	}
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}

	client := NodeMavenClient{
		BaseURL:   cfg.BaseURL,
		UserAgent: cfg.UserAgent,
		Timeout:   cfg.Timeout,
	}

	total, first, err := client.FetchPage(ctx, 1, 1)
	if err != nil {
		return nil, err
	}
	_ = first
	if total <= 0 {
		return nil, nil
	}
	totalPages := (total + perPage - 1) / perPage
	if totalPages > maxPages {
		totalPages = maxPages
	}

	type result struct {
		page  int
		items []NodeMavenProxy
		err   error
	}
	sem := make(chan struct{}, concurrency)
	outCh := make(chan result, totalPages)
	var wg sync.WaitGroup
	for p := 1; p <= totalPages; p++ {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			_, items, err := client.FetchPage(ctx, p, perPage)
			outCh <- result{page: p, items: items, err: err}
		}()
	}
	go func() {
		wg.Wait()
		close(outCh)
	}()

	seen := make(map[string]struct{}, perPage*totalPages)
	var out []NodeMavenProxy
	for r := range outCh {
		if r.err != nil {
			slog.Warn("nodemaven fetch page failed", "page", r.page, "error", r.err)
			continue
		}
		for _, it := range r.items {
			proto := strings.ToUpper(strings.TrimSpace(it.Protocol))
			key := proto + "|" + strings.TrimSpace(it.IPAddress) + ":" + strconv.Itoa(it.Port)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, it)
		}
	}
	return out, nil
}

func SafeInt(v any) int {
	switch t := v.(type) {
	case nil:
		return 0
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0
		}
		if i, err := strconv.Atoi(s); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int(f)
		}
		return 0
	default:
		s := strings.TrimSpace(fmt.Sprintf("%v", v))
		if s == "" {
			return 0
		}
		i, _ := strconv.Atoi(s)
		return i
	}
}
