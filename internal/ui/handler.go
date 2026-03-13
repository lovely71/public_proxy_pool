package ui

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qiyiyun/public_proxy_pool/internal/auth"
	"github.com/qiyiyun/public_proxy_pool/internal/config"
	"github.com/qiyiyun/public_proxy_pool/internal/ratelimit"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
	"github.com/qiyiyun/public_proxy_pool/internal/validator"
)

//go:embed assets/templates/*.html assets/static/*
var embeddedFS embed.FS

type Handler struct {
	st  *store.Store
	val *validator.Validator
	cfg *config.Config

	tpl *template.Template

	limiter *ratelimit.Limiter
}

func NewHandler(st *store.Store, val *validator.Validator, cfg *config.Config) *Handler {
	funcMap := template.FuncMap{
		"fmtUnix": func(ts int64) string {
			if ts <= 0 {
				return "未记录"
			}
			return time.Unix(ts, 0).Local().Format("2006-01-02 15:04:05")
		},
	}
	tpl := template.Must(template.New("ui").Funcs(funcMap).ParseFS(embeddedFS, "assets/templates/*.html"))
	limiter := ratelimit.New(cfg.RateLimitRPS, cfg.RateLimitBurst)
	return &Handler{st: st, val: val, cfg: cfg, tpl: tpl, limiter: limiter}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/static/*", h.static)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		if tok := strings.TrimSpace(r.URL.Query().Get("token")); tok != "" {
			http.Redirect(w, r, "/ui/overview?token="+template.URLQueryEscaper(tok), http.StatusFound)
			return
		}
		http.Redirect(w, r, "/ui/overview", http.StatusFound)
	})

	r.Group(func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.rateLimitMiddleware)
		r.Get("/overview", h.overview)
		r.Get("/sources", h.sources)
		r.Post("/sources/{id}/toggle", h.toggleSource)
		r.Post("/sources/{id}/fetch", h.fetchSourceNow)
		r.Post("/sources/add", h.addSource)
		r.Get("/nodes", h.nodes)
		r.Post("/nodes/{id}/ban", h.banNode)
		r.Post("/nodes/{id}/unban", h.unbanNode)
		r.Get("/api", h.apiDocs)
		r.Get("/sub", h.subBuilder)
		r.Get("/events", h.events)
	})

	return r
}

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.Check(r, h.cfg) {
			http.Error(w, "未授权，请在地址后追加 ?token=... 或通过 X-API-Key 访问", http.StatusUnauthorized)
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
		key := strings.TrimSpace(r.URL.Query().Get("token"))
		ip := clientIP(r)
		if !h.limiter.Allow(key+"|"+ip, time.Now()) {
			http.Error(w, "请求过于频繁，请稍后再试", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func (h *Handler) static(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/ui/static/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	data, err := embeddedFS.ReadFile("assets/static/" + path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(path, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
	_, _ = w.Write(data)
}

type viewData struct {
	Now       time.Time
	Token     string
	Title     string
	Stats     *store.Stats
	Sources   []store.Source
	Nodes     []store.Node
	Query     map[string]string
	BaseURL   string
	APIKeysOn bool
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	stats, err := h.st.GetStats(r.Context(), now, h.cfg.FreshWithinDefault)
	if err != nil {
		http.Error(w, "读取统计数据失败", http.StatusInternalServerError)
		return
	}
	h.render(w, r, "overview.html", viewData{
		Now:       now,
		Title:     "概览",
		Stats:     stats,
		Token:     r.URL.Query().Get("token"),
		BaseURL:   h.cfg.PublicBaseURL,
		APIKeysOn: len(h.cfg.APIKeys) > 0,
	})
}

func (h *Handler) sources(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	stats, _ := h.st.GetStats(r.Context(), now, h.cfg.FreshWithinDefault)
	items, err := h.st.ListSources(r.Context())
	if err != nil {
		http.Error(w, "读取抓取源列表失败", http.StatusInternalServerError)
		return
	}
	h.render(w, r, "sources.html", viewData{
		Now:       now,
		Title:     "抓取源",
		Stats:     stats,
		Sources:   items,
		Token:     r.URL.Query().Get("token"),
		BaseURL:   h.cfg.PublicBaseURL,
		APIKeysOn: len(h.cfg.APIKeys) > 0,
	})
}

func (h *Handler) toggleSource(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	if id <= 0 {
		http.Error(w, "无效的抓取源 ID", http.StatusBadRequest)
		return
	}
	src, err := h.st.GetSourceByID(r.Context(), id)
	if err != nil {
		http.Error(w, "抓取源不存在", http.StatusNotFound)
		return
	}
	if err := h.st.SetSourceEnabled(r.Context(), id, !src.Enabled); err != nil {
		http.Error(w, "更新抓取源状态失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/sources?token="+template.URLQueryEscaper(r.URL.Query().Get("token")), http.StatusFound)
}

func (h *Handler) fetchSourceNow(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	if id <= 0 {
		http.Error(w, "无效的抓取源 ID", http.StatusBadRequest)
		return
	}
	src, err := h.st.GetSourceByID(r.Context(), id)
	if err != nil {
		http.Error(w, "抓取源不存在", http.StatusNotFound)
		return
	}
	now := time.Now()
	meta := store.FetchMetaUpdate{
		LastFetchAt:    src.LastFetchAt,
		ETag:           src.ETag,
		LastModified:   src.LastModified,
		LastError:      src.LastError,
		NextFetchAt:    now.Unix(),
		BackoffUntil:   0,
		FetchOKInc:     0,
		FetchFailInc:   0,
		FetchedInc:     0,
		NotModifiedInc: 0,
	}
	if err := h.st.UpdateSourceFetchMeta(r.Context(), id, meta); err != nil {
		http.Error(w, "更新抓取计划失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/sources?token="+template.URLQueryEscaper(r.URL.Query().Get("token")), http.StatusFound)
}

func (h *Handler) addSource(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	typ := strings.TrimSpace(r.FormValue("type"))
	urlStr := strings.TrimSpace(r.FormValue("url"))
	parser := strings.TrimSpace(r.FormValue("parser"))
	scheme := strings.TrimSpace(r.FormValue("default_scheme"))
	if name == "" || typ == "" || urlStr == "" {
		http.Error(w, "名称、类型和地址不能为空", http.StatusBadRequest)
		return
	}
	if parser == "" {
		parser = "generic"
	}
	if scheme == "" {
		scheme = "http"
	}
	_, err := h.st.UpsertSource(r.Context(), store.Source{
		Name:          name,
		Type:          typ,
		URL:           urlStr,
		Parser:        parser,
		DefaultScheme: scheme,
		Enabled:       true,
		IntervalSec:   3600,
		NextFetchAt:   time.Now().Unix(),
	})
	if err != nil {
		http.Error(w, "新增抓取源失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/sources?token="+template.URLQueryEscaper(r.URL.Query().Get("token")), http.StatusFound)
}

func (h *Handler) nodes(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	stats, _ := h.st.GetStats(r.Context(), now, h.cfg.FreshWithinDefault)

	limit := int(parseInt64(r.URL.Query().Get("limit")))
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}
	verify := strings.TrimSpace(r.URL.Query().Get("verify")) == "1"
	freshWithin := h.cfg.FreshWithinDefault
	if raw := strings.TrimSpace(r.URL.Query().Get("fresh_within")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			freshWithin = d
		}
	}

	filter := store.NodeFilter{
		Protocols:   splitCSV(strings.ToLower(r.URL.Query().Get("protocols"))),
		Countries:   splitCSV(strings.ToUpper(r.URL.Query().Get("countries"))),
		MinPurity:   int(parseInt64(r.URL.Query().Get("min_purity"))),
		MaxLatency:  int(parseInt64(r.URL.Query().Get("max_latency_ms"))),
		Kind:        strings.TrimSpace(r.URL.Query().Get("kind")),
		FreshWithin: freshWithin,
		Verify:      verify,
	}

	nodes, err := h.st.QueryFreshValidNodes(r.Context(), now, filter, limit)
	if err != nil {
		http.Error(w, "查询节点失败", http.StatusInternalServerError)
		return
	}
	q := map[string]string{
		"kind":           r.URL.Query().Get("kind"),
		"protocols":      r.URL.Query().Get("protocols"),
		"countries":      r.URL.Query().Get("countries"),
		"min_purity":     r.URL.Query().Get("min_purity"),
		"max_latency_ms": r.URL.Query().Get("max_latency_ms"),
		"fresh_within":   r.URL.Query().Get("fresh_within"),
		"verify":         r.URL.Query().Get("verify"),
		"limit":          r.URL.Query().Get("limit"),
	}
	h.render(w, r, "nodes.html", viewData{
		Now:       now,
		Title:     "节点池",
		Stats:     stats,
		Nodes:     nodes,
		Query:     q,
		Token:     r.URL.Query().Get("token"),
		BaseURL:   h.cfg.PublicBaseURL,
		APIKeysOn: len(h.cfg.APIKeys) > 0,
	})
}

func (h *Handler) banNode(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	if id <= 0 {
		http.Error(w, "无效的节点 ID", http.StatusBadRequest)
		return
	}
	until := time.Now().Add(24 * time.Hour)
	if err := h.st.BanNode(r.Context(), id, until, "manual ban"); err != nil {
		http.Error(w, "封禁节点失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/nodes?token="+template.URLQueryEscaper(r.URL.Query().Get("token")), http.StatusFound)
}

func (h *Handler) unbanNode(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	if id <= 0 {
		http.Error(w, "无效的节点 ID", http.StatusBadRequest)
		return
	}
	if err := h.st.UnbanNode(r.Context(), id); err != nil {
		http.Error(w, "解除封禁失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/nodes?token="+template.URLQueryEscaper(r.URL.Query().Get("token")), http.StatusFound)
}

func (h *Handler) apiDocs(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	stats, _ := h.st.GetStats(r.Context(), now, h.cfg.FreshWithinDefault)
	h.render(w, r, "api.html", viewData{
		Now:       now,
		Title:     "接口帮助",
		Stats:     stats,
		Token:     r.URL.Query().Get("token"),
		BaseURL:   h.cfg.PublicBaseURL,
		APIKeysOn: len(h.cfg.APIKeys) > 0,
	})
}

func (h *Handler) subBuilder(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	stats, _ := h.st.GetStats(r.Context(), now, h.cfg.FreshWithinDefault)
	h.render(w, r, "sub.html", viewData{
		Now:       now,
		Title:     "订阅生成",
		Stats:     stats,
		Token:     r.URL.Query().Get("token"),
		BaseURL:   h.cfg.PublicBaseURL,
		APIKeysOn: len(h.cfg.APIKeys) > 0,
	})
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "当前环境不支持实时事件流", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			now := time.Now()
			stats, err := h.st.GetStats(r.Context(), now, h.cfg.FreshWithinDefault)
			if err != nil {
				continue
			}
			b, _ := json.Marshal(stats)
			fmt.Fprintf(w, "event: stats\ndata: %s\n\n", string(b))
			flusher.Flush()
		}
	}
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, name string, data viewData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tpl.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("template render failed", "name", name, "error", err)
		http.Error(w, "页面渲染失败", http.StatusInternalServerError)
		return
	}
}

func parseInt64(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	v, _ := strconv.ParseInt(raw, 10, 64)
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
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}
