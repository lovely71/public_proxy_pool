package subcheck

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/qiyiyun/public_proxy_pool/internal/config"
	"github.com/qiyiyun/public_proxy_pool/internal/sources"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
	"github.com/qiyiyun/public_proxy_pool/internal/validator"
)

const (
	FormatAuto  = "auto"
	FormatPlain = "plain"
	FormatV2Ray = "v2ray"
	FormatClash = "clash"
)

type Result struct {
	Index          int    `json:"index"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Protocol       string `json:"protocol"`
	Endpoint       string `json:"endpoint"`
	Alive          bool   `json:"alive"`
	AliveLatencyMS int    `json:"alive_latency_ms"`
	AliveError     string `json:"alive_error,omitempty"`
	GoogleOK       bool   `json:"google_ok"`
	GoogleTarget   string `json:"google_target_url"`
	GoogleFinalURL string `json:"google_final_url"`
	GoogleStatus   int    `json:"google_http_status"`
	GoogleLatency  int    `json:"google_latency_ms"`
	GoogleError    string `json:"google_error,omitempty"`
	HuggingFaceOK  bool   `json:"huggingface_ok"`
	HuggingFaceURL string `json:"huggingface_target_url"`
	HFFinalURL     string `json:"huggingface_final_url"`
	HFStatus       int    `json:"huggingface_http_status"`
	HFLatency      int    `json:"huggingface_latency_ms"`
	HFError        string `json:"huggingface_error,omitempty"`
	CheckIPOK      bool   `json:"checkip_ok"`
	TargetURL      string `json:"target_url"`
	FinalURL       string `json:"final_url"`
	HTTPStatus     int    `json:"http_status"`
	LatencyMS      int    `json:"latency_ms"`
	ExitIP         string `json:"exit_ip"`
	OK             bool   `json:"ok"`
	CheckIPError   string `json:"checkip_error,omitempty"`
	Error          string `json:"error,omitempty"`
}

type Counts struct {
	Total         int `json:"total"`
	Alive         int `json:"alive"`
	GoogleOK      int `json:"google_ok"`
	HuggingFaceOK int `json:"huggingface_ok"`
	CheckIPOK     int `json:"checkip_ok"`
	Fail          int `json:"fail"`
}

func FetchSubscription(ctx context.Context, subscriptionURL, apiKey string, timeout time.Duration) (body, contentType string, err error) {
	if strings.TrimSpace(subscriptionURL) == "" {
		return "", "", fmt.Errorf("subscription url is required")
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subscriptionURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "public_proxy_pool-subcheck/1.0")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("X-API-Key", strings.TrimSpace(apiKey))
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", resp.Header.Get("Content-Type"), fmt.Errorf("fetch subscription failed: status=%d", resp.StatusCode)
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.Header.Get("Content-Type"), err
	}
	return string(buf), resp.Header.Get("Content-Type"), nil
}

func DetectFormat(subscriptionURL, contentType, body, explicit string) string {
	format := strings.ToLower(strings.TrimSpace(explicit))
	switch format {
	case "", FormatAuto:
	case FormatPlain, FormatV2Ray:
		return FormatPlain
	case FormatClash:
		return FormatClash
	default:
		return FormatPlain
	}

	contentType = strings.ToLower(strings.TrimSpace(contentType))
	body = strings.TrimSpace(body)
	if strings.Contains(contentType, "yaml") || strings.Contains(contentType, "yml") {
		return FormatClash
	}
	if looksLikeClash(body) {
		return FormatClash
	}

	if u, err := url.Parse(strings.TrimSpace(subscriptionURL)); err == nil {
		if strings.Contains(strings.ToLower(u.Path), "/sub/clash") {
			return FormatClash
		}
	}
	return FormatPlain
}

func ParseSubscriptionNodes(body, subscriptionURL, contentType, explicitFormat string) ([]store.Node, string, error) {
	format := DetectFormat(subscriptionURL, contentType, body, explicitFormat)

	var (
		cands []sources.URICandidate
		err   error
	)
	switch format {
	case FormatClash:
		cands, err = sources.ParseClashYAML(body)
	default:
		cands, err = sources.ParseBase64Subscription(body)
	}
	if err != nil {
		return nil, format, err
	}

	nodes := CandidatesToNodes(cands)
	if len(nodes) == 0 {
		return nil, format, fmt.Errorf("subscription did not contain any supported nodes")
	}
	return nodes, format, nil
}

func CandidatesToNodes(cands []sources.URICandidate) []store.Node {
	nodes := make([]store.Node, 0, len(cands))
	for _, cand := range cands {
		p := cand.Parsed
		if p == nil {
			continue
		}
		nodes = append(nodes, store.Node{
			Kind:        p.Kind,
			Protocol:    p.Protocol,
			Fingerprint: p.Fingerprint,
			Host:        p.Host,
			Port:        p.Port,
			Username:    p.Username,
			Password:    p.Password,
			RawURI:      p.RawURI,
			Name:        p.Name,
		})
	}
	return nodes
}

func CheckNodes(ctx context.Context, cfg *config.Config, nodes []store.Node, huggingFaceURL, targetURL string, concurrency int) []Result {
	if concurrency <= 0 {
		concurrency = 10
	}

	results := make([]Result, len(nodes))
	jobs := make(chan int)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				node := nodes[idx]
				res := Result{
					Index:          idx + 1,
					Name:           displayName(node),
					Kind:           node.Kind,
					Protocol:       node.Protocol,
					Endpoint:       endpoint(node),
					GoogleTarget:   validator.GoogleGenerate204URL,
					HuggingFaceURL: strings.TrimSpace(huggingFaceURL),
				}
				if res.HuggingFaceURL == "" {
					res.HuggingFaceURL = validator.DefaultHuggingFaceURL
				}

				probeRes, err := validator.ProbeNodeAccessSuite(ctx, cfg, node, huggingFaceURL, targetURL)
				res.Alive = probeRes.Alive.OK
				res.AliveLatencyMS = probeRes.Alive.LatencyMS
				res.AliveError = probeRes.Alive.Error
				res.GoogleOK = probeRes.Google.OK
				res.GoogleTarget = probeRes.Google.TargetURL
				res.GoogleFinalURL = probeRes.Google.FinalURL
				res.GoogleStatus = probeRes.Google.HTTPStatus
				res.GoogleLatency = probeRes.Google.LatencyMS
				res.GoogleError = probeRes.Google.Error
				res.HuggingFaceOK = probeRes.HuggingFace.OK
				res.HuggingFaceURL = probeRes.HuggingFace.TargetURL
				res.HFFinalURL = probeRes.HuggingFace.FinalURL
				res.HFStatus = probeRes.HuggingFace.HTTPStatus
				res.HFLatency = probeRes.HuggingFace.LatencyMS
				res.HFError = probeRes.HuggingFace.Error
				res.CheckIPOK = probeRes.CheckIP.OK
				res.TargetURL = probeRes.CheckIP.TargetURL
				res.FinalURL = probeRes.CheckIP.FinalURL
				res.HTTPStatus = probeRes.CheckIP.HTTPStatus
				res.LatencyMS = probeRes.CheckIP.LatencyMS
				res.ExitIP = probeRes.CheckIP.ExitIP
				res.OK = res.CheckIPOK
				res.CheckIPError = probeRes.CheckIP.Error
				res.Error = primaryError(res)
				if err != nil && res.Error == "" {
					res.Error = err.Error()
				}
				results[idx] = res
			}
		}()
	}

sendLoop:
	for idx := range nodes {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobs <- idx:
		}
	}
	close(jobs)
	wg.Wait()

	return results
}

func Summary(results []Result) (okCount, failCount int) {
	for _, res := range results {
		if res.OK {
			okCount++
		} else {
			failCount++
		}
	}
	return okCount, failCount
}

func TierSummary(results []Result) Counts {
	counts := Counts{Total: len(results)}
	for _, res := range results {
		if res.Alive {
			counts.Alive++
		}
		if res.GoogleOK {
			counts.GoogleOK++
		}
		if res.HuggingFaceOK {
			counts.HuggingFaceOK++
		}
		if res.CheckIPOK {
			counts.CheckIPOK++
		}
	}
	counts.Fail = counts.Total - counts.CheckIPOK
	return counts
}

func looksLikeClash(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	return strings.Contains(body, "\nproxies:") ||
		strings.HasPrefix(body, "proxies:") ||
		(strings.Contains(body, "proxy-groups:") && strings.Contains(body, "proxies:"))
}

func displayName(node store.Node) string {
	if strings.TrimSpace(node.Name) != "" {
		return strings.TrimSpace(node.Name)
	}
	if ep := endpoint(node); ep != "" {
		return node.Protocol + "@" + ep
	}
	return strings.TrimSpace(node.RawURI)
}

func endpoint(node store.Node) string {
	if strings.TrimSpace(node.Host) == "" || node.Port <= 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", strings.TrimSpace(node.Host), node.Port)
}

func primaryError(res Result) string {
	switch {
	case strings.TrimSpace(res.CheckIPError) != "":
		return strings.TrimSpace(res.CheckIPError)
	case strings.TrimSpace(res.GoogleError) != "":
		return strings.TrimSpace(res.GoogleError)
	case strings.TrimSpace(res.HFError) != "":
		return strings.TrimSpace(res.HFError)
	case strings.TrimSpace(res.AliveError) != "":
		return strings.TrimSpace(res.AliveError)
	default:
		return ""
	}
}
