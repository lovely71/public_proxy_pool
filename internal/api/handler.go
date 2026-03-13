package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qiyiyun/public_proxy_pool/internal/config"
	"github.com/qiyiyun/public_proxy_pool/internal/ratelimit"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
	"github.com/qiyiyun/public_proxy_pool/internal/sub"
	"github.com/qiyiyun/public_proxy_pool/internal/validator"
)

type Handler struct {
	st  *store.Store
	val *validator.Validator
	cfg *config.Config

	limiter *ratelimit.Limiter
}

type respNode struct {
	ID          int64   `json:"id"`
	Kind        string  `json:"kind"`
	Protocol    string  `json:"protocol"`
	Host        string  `json:"host"`
	Port        int     `json:"port"`
	RawURI      string  `json:"raw_uri"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	LatencyMS   int     `json:"latency_ms"`
	Country     string  `json:"country"`
	ExitIP      string  `json:"exit_ip"`
	ASN         string  `json:"asn"`
	Anonymity   string  `json:"anonymity"`
	PurityScore int     `json:"purity_score"`
	Score       float64 `json:"score"`
	LastChecked int64   `json:"last_checked_at"`
	LastOK      int64   `json:"last_ok_at"`
	LastError   string  `json:"last_error"`
	SourceID    int64   `json:"last_source_id"`
}

func toRespNode(n store.Node) respNode {
	return respNode{
		ID:          n.ID,
		Kind:        n.Kind,
		Protocol:    n.Protocol,
		Host:        n.Host,
		Port:        n.Port,
		RawURI:      n.RawURI,
		Name:        n.Name,
		Status:      n.Status,
		LatencyMS:   n.LatencyMS,
		Country:     n.Country,
		ExitIP:      n.ExitIP,
		ASN:         n.ASN,
		Anonymity:   n.Anonymity,
		PurityScore: n.PurityScore,
		Score:       n.Score,
		LastChecked: n.LastCheckedAt,
		LastOK:      n.LastOKAt,
		LastError:   n.LastError,
		SourceID:    n.LastSource,
	}
}

func NewHandler(st *store.Store, val *validator.Validator, cfg *config.Config) *Handler {
	limiter := ratelimit.New(cfg.RateLimitRPS, cfg.RateLimitBurst)
	return &Handler{st: st, val: val, cfg: cfg, limiter: limiter}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	r.Get("/probe/echo", h.probeEcho)

	r.Group(func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.rateLimitMiddleware)
		r.Get("/metrics", h.metrics)

		r.Get("/api/v1/nodes", h.listNodes)
		r.Get("/api/v1/nodes/random", h.randomNode)
		r.Get("/api/v1/stats", h.stats)
		r.Get("/api/v1/sources", h.sources)
		r.Get("/sub/plain", h.subPlain)
		r.Get("/sub/v2ray", h.subV2Ray)
		r.Get("/sub/clash", h.subClash)
	})

	return r
}

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !checkAPIKey(r, h.cfg) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.limiter == nil || !h.limiter.Enabled() {
			next.ServeHTTP(w, r)
			return
		}
		key := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if key == "" {
			key = strings.TrimSpace(r.URL.Query().Get("token"))
		}
		ip := clientIP(r)
		if !h.limiter.Allow(key+"|"+ip, time.Now()) {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	remote := r.RemoteAddr
	host, _, err := net.SplitHostPort(remote)
	if err == nil && host != "" {
		return host
	}
	return remote
}

func (h *Handler) listNodes(w http.ResponseWriter, r *http.Request) {
	filter, limit, verify, freshWithin, err := parseFilters(r, h.cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	now := time.Now()
	nodes, err := h.st.QueryFreshValidNodes(r.Context(), now, store.NodeFilter{
		Protocols:   filter.Protocols,
		Countries:   filter.Countries,
		MinPurity:   filter.MinPurity,
		MaxLatency:  filter.MaxLatencyMS,
		Kind:        filter.Kind,
		FreshWithin: freshWithin,
		Verify:      verify,
	}, limit)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	if verify && len(nodes) < limit {
		// Sync verify to fill.
		ctx, cancel := context.WithTimeout(r.Context(), h.cfg.SyncVerifyTimeout)
		defer cancel()
		h.syncVerify(ctx, filter, freshWithin, limit)
		nodes, _ = h.st.QueryFreshValidNodes(r.Context(), now, store.NodeFilter{
			Protocols:   filter.Protocols,
			Countries:   filter.Countries,
			MinPurity:   filter.MinPurity,
			MaxLatency:  filter.MaxLatencyMS,
			Kind:        filter.Kind,
			FreshWithin: freshWithin,
			Verify:      true,
		}, limit)
		if len(nodes) < limit {
			w.Header().Set("X-Pool-Partial", "1")
		}
	}

	out := make([]respNode, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, toRespNode(n))
	}

	writeJSON(w, out)
}

func (h *Handler) randomNode(w http.ResponseWriter, r *http.Request) {
	filter, _, verify, freshWithin, err := parseFilters(r, h.cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	now := time.Now()
	nodes, err := h.st.QueryFreshValidNodes(r.Context(), now, store.NodeFilter{
		Protocols:   filter.Protocols,
		Countries:   filter.Countries,
		MinPurity:   filter.MinPurity,
		MaxLatency:  filter.MaxLatencyMS,
		Kind:        filter.Kind,
		FreshWithin: freshWithin,
		Verify:      verify,
	}, 200)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	if len(nodes) == 0 && verify {
		ctx, cancel := context.WithTimeout(r.Context(), h.cfg.SyncVerifyTimeout)
		defer cancel()
		h.syncVerify(ctx, filter, freshWithin, 1)
		nodes, _ = h.st.QueryFreshValidNodes(r.Context(), now, store.NodeFilter{
			Protocols:   filter.Protocols,
			Countries:   filter.Countries,
			MinPurity:   filter.MinPurity,
			MaxLatency:  filter.MaxLatencyMS,
			Kind:        filter.Kind,
			FreshWithin: freshWithin,
			Verify:      true,
		}, 200)
	}
	if len(nodes) == 0 {
		http.Error(w, "no node", http.StatusNotFound)
		return
	}
	n := nodes[rand.Intn(len(nodes))]
	writeJSON(w, toRespNode(n))
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	stats, err := h.st.GetStats(r.Context(), now, h.cfg.FreshWithinDefault)
	if err != nil {
		http.Error(w, "stats failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"time":  now.Unix(),
		"stats": stats,
	})
}

func (h *Handler) sources(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListSources(r.Context())
	if err != nil {
		http.Error(w, "list sources failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}

func (h *Handler) subPlain(w http.ResponseWriter, r *http.Request) {
	filter, limit, verify, freshWithin, err := parseFilters(r, h.cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	now := time.Now()
	nodes, err := h.st.QueryFreshValidNodes(r.Context(), now, store.NodeFilter{
		Protocols:   filter.Protocols,
		Countries:   filter.Countries,
		MinPurity:   filter.MinPurity,
		MaxLatency:  filter.MaxLatencyMS,
		Kind:        filter.Kind,
		FreshWithin: freshWithin,
		Verify:      verify,
	}, limit)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	if verify && len(nodes) < limit {
		ctx, cancel := context.WithTimeout(r.Context(), h.cfg.SyncVerifyTimeout)
		defer cancel()
		h.syncVerify(ctx, filter, freshWithin, limit)
		nodes, _ = h.st.QueryFreshValidNodes(r.Context(), now, store.NodeFilter{
			Protocols:   filter.Protocols,
			Countries:   filter.Countries,
			MinPurity:   filter.MinPurity,
			MaxLatency:  filter.MaxLatencyMS,
			Kind:        filter.Kind,
			FreshWithin: freshWithin,
			Verify:      true,
		}, limit)
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	body := sub.RenderPlain(nodes, format)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func (h *Handler) subV2Ray(w http.ResponseWriter, r *http.Request) {
	filter, limit, verify, freshWithin, err := parseFilters(r, h.cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	now := time.Now()
	nodes, err := h.st.QueryFreshValidNodes(r.Context(), now, store.NodeFilter{
		Protocols:   filter.Protocols,
		Countries:   filter.Countries,
		MinPurity:   filter.MinPurity,
		MaxLatency:  filter.MaxLatencyMS,
		Kind:        filter.Kind,
		FreshWithin: freshWithin,
		Verify:      verify,
	}, limit)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	if verify && len(nodes) < limit {
		ctx, cancel := context.WithTimeout(r.Context(), h.cfg.SyncVerifyTimeout)
		defer cancel()
		h.syncVerify(ctx, filter, freshWithin, limit)
		nodes, _ = h.st.QueryFreshValidNodes(r.Context(), now, store.NodeFilter{
			Protocols:   filter.Protocols,
			Countries:   filter.Countries,
			MinPurity:   filter.MinPurity,
			MaxLatency:  filter.MaxLatencyMS,
			Kind:        filter.Kind,
			FreshWithin: freshWithin,
			Verify:      true,
		}, limit)
	}
	body := sub.RenderV2Ray(nodes)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func (h *Handler) subClash(w http.ResponseWriter, r *http.Request) {
	filter, limit, verify, freshWithin, err := parseFilters(r, h.cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	now := time.Now()
	nodes, err := h.st.QueryFreshValidNodes(r.Context(), now, store.NodeFilter{
		Protocols:   filter.Protocols,
		Countries:   filter.Countries,
		MinPurity:   filter.MinPurity,
		MaxLatency:  filter.MaxLatencyMS,
		Kind:        filter.Kind,
		FreshWithin: freshWithin,
		Verify:      verify,
	}, limit)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	if verify && len(nodes) < limit {
		ctx, cancel := context.WithTimeout(r.Context(), h.cfg.SyncVerifyTimeout)
		defer cancel()
		h.syncVerify(ctx, filter, freshWithin, limit)
		nodes, _ = h.st.QueryFreshValidNodes(r.Context(), now, store.NodeFilter{
			Protocols:   filter.Protocols,
			Countries:   filter.Countries,
			MinPurity:   filter.MinPurity,
			MaxLatency:  filter.MaxLatencyMS,
			Kind:        filter.Kind,
			FreshWithin: freshWithin,
			Verify:      true,
		}, limit)
	}
	testURL := strings.TrimSpace(r.URL.Query().Get("test_url"))
	if testURL == "" {
		testURL = h.cfg.ValidateURL
	}
	yml, err := sub.RenderClash(nodes, testURL)
	if err != nil {
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	_, _ = w.Write(yml)
}

func (h *Handler) syncVerify(ctx context.Context, filter parsedFilter, freshWithin time.Duration, limit int) {
	now := time.Now()
	cands, err := h.st.QueryValidationCandidates(ctx, now, store.NodeFilter{
		Protocols:   filter.Protocols,
		Countries:   filter.Countries,
		MinPurity:   filter.MinPurity,
		MaxLatency:  filter.MaxLatencyMS,
		Kind:        filter.Kind,
		FreshWithin: freshWithin,
	}, max(limit*3, 30))
	if err != nil {
		return
	}

	need := limit
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var mu sync.Mutex
	okCount := 0
	for _, n := range cands {
		if okCount >= need {
			break
		}
		n := n
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := h.val.RequestCheck(ctx, n.ID)
			if err == nil && res.OK {
				mu.Lock()
				okCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
}

func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	stats, err := h.st.GetStats(r.Context(), now, h.cfg.FreshWithinDefault)
	if err != nil {
		http.Error(w, "stats failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "proxypool_nodes_total %d\n", stats.NodesTotal)
	fmt.Fprintf(w, "proxypool_nodes_valid %d\n", stats.NodesValid)
	fmt.Fprintf(w, "proxypool_nodes_invalid %d\n", stats.NodesInvalid)
	fmt.Fprintf(w, "proxypool_nodes_unknown %d\n", stats.NodesUnknown)
	fmt.Fprintf(w, "proxypool_nodes_fresh_valid %d\n", stats.NodesFreshOK)
	fmt.Fprintf(w, "proxypool_sources_total %d\n", stats.SourcesTotal)
	fmt.Fprintf(w, "proxypool_sources_enabled %d\n", stats.SourcesEnabled)
}

func (h *Handler) probeEcho(w http.ResponseWriter, r *http.Request) {
	remote := r.RemoteAddr
	host, _, _ := net.SplitHostPort(remote)
	if host == "" {
		host = remote
	}
	resp := map[string]any{
		"remote_ip": host,
		"headers":   r.Header,
		"time":      time.Now().Unix(),
	}
	writeJSON(w, resp)
}

type parsedFilter struct {
	Kind         string
	Protocols    []string
	Countries    []string
	MinPurity    int
	MaxLatencyMS int
}

func parseFilters(r *http.Request, cfg *config.Config) (parsedFilter, int, bool, time.Duration, error) {
	q := r.URL.Query()
	limit := parseInt(q.Get("limit"), 50)
	if limit <= 0 {
		limit = 50
	}
	if limit > 2000 {
		limit = 2000
	}

	verify := parseBool(q.Get("verify"), true)

	freshWithin := cfg.FreshWithinDefault
	if raw := strings.TrimSpace(q.Get("fresh_within")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			freshWithin = d
		}
	}

	filter := parsedFilter{
		Kind:         strings.TrimSpace(q.Get("kind")),
		Protocols:    splitCSV(q.Get("protocols")),
		Countries:    splitCSVUpper(q.Get("countries")),
		MinPurity:    parseInt(q.Get("min_purity"), 0),
		MaxLatencyMS: parseInt(q.Get("max_latency_ms"), 0),
	}
	return filter, limit, verify, freshWithin, nil
}

func parseBool(raw string, def bool) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return def
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func parseInt(raw string, def int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(strings.ToLower(p))
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func splitCSVUpper(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(strings.ToUpper(p))
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
