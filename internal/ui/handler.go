package ui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime"
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

const overviewEventsInterval = 2 * time.Second

type Handler struct {
	st  *store.Store
	val *validator.Validator
	cfg *config.Config

	tpl *template.Template

	limiter   *ratelimit.Limiter
	startedAt time.Time
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
	return &Handler{st: st, val: val, cfg: cfg, tpl: tpl, limiter: limiter, startedAt: time.Now()}
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
	Now          time.Time
	Token        string
	Title        string
	Stats        *store.Stats
	Sources      []store.Source
	Nodes        []store.Node
	Query        map[string]string
	BaseURL      string
	APIKeysOn    bool
	ServerItems  []dashboardItem
	RefreshItems []dashboardItem
}

type dashboardItem struct {
	Label string
	Value string
	Hint  string
	Tone  string
	Badge bool
	Mono  bool
}

func (h *Handler) statsContext(parent context.Context) (context.Context, context.CancelFunc) {
	if h.cfg == nil || h.cfg.StatsQueryTimeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, h.cfg.StatsQueryTimeout)
}

func (h *Handler) loadStats(parent context.Context, now time.Time) (*store.Stats, error) {
	ctx, cancel := h.statsContext(parent)
	defer cancel()
	return h.st.GetStats(ctx, now, h.cfg.FreshWithinDefault)
}

func (h *Handler) buildOverviewServerItems(now time.Time) []dashboardItem {
	validatorSnap := h.val.Snapshot()

	authValue, authTone := statusText(len(h.cfg.APIKeys) > 0, "已启用", "未启用")
	rateLimitEnabled := h.cfg.RateLimitRPS > 0
	rateValue, rateTone := statusText(rateLimitEnabled, "已启用", "未启用")
	validatorValue, validatorTone := statusText(validatorSnap.Started, "运行中", "未启动")
	fetchValue, fetchTone := statusText(h.cfg.AutoFetchEnabled, "运行中", "已关闭")
	nodeMavenValue, _ := statusText(h.cfg.NodeMaven.Enabled, "已启用", "已关闭")
	purityValue, _ := statusText(h.cfg.PurityLookup.Enabled, "已启用", "已关闭")

	return []dashboardItem{
		{
			Label: "服务监听",
			Value: h.cfg.HTTPAddr,
			Hint:  "容器内服务当前绑定地址。",
			Mono:  true,
		},
		{
			Label: "对外地址",
			Value: valueOrDefault(h.cfg.PublicBaseURL, "未配置"),
			Hint:  "用于浏览器访问 UI 和 /probe/echo 匿名度探测。",
			Mono:  true,
		},
		{
			Label: "服务运行时长",
			Value: formatUptime(now.Sub(h.startedAt)),
			Hint:  "按 UI handler 启动时间估算，页面刷新时会更新。",
		},
		{
			Label: "CPU / 调度",
			Value: fmt.Sprintf("%d CPU / GOMAXPROCS %d", runtime.NumCPU(), runtime.GOMAXPROCS(0)),
			Hint:  "用于确认这台机器当前的 Go 调度并发。",
		},
		{
			Label: "Go 运行时",
			Value: fmt.Sprintf("%s / goroutine %d / pid %d", runtime.Version(), runtime.NumGoroutine(), os.Getpid()),
			Hint:  "用于观察进程数量级和排查异常膨胀。",
		},
		{
			Label: "API 鉴权",
			Value: authValue,
			Hint:  "浏览器访问 UI 用 ?token=...，脚本更推荐 X-API-Key。",
			Tone:  authTone,
			Badge: true,
		},
		{
			Label: "限流状态",
			Value: rateValue,
			Hint:  disabledOrValue(rateLimitEnabled, fmt.Sprintf("RPS %.1f / Burst %d", h.cfg.RateLimitRPS, h.cfg.RateLimitBurst), "当前未限制请求速率。"),
			Tone:  rateTone,
			Badge: true,
		},
		{
			Label: "SQLite 路径",
			Value: h.cfg.SQLitePath,
			Hint:  "当前节点库与统计数据所在文件。",
			Mono:  true,
		},
		{
			Label: "SQLite 并发",
			Value: fmt.Sprintf("%d 连接 / %s", h.cfg.SQLiteMaxOpenConns, formatDurationValue(h.cfg.SQLiteBusyTimeout, "默认")),
			Hint:  "读并发与锁等待时间，4c4g 机器建议适当放宽。",
		},
		{
			Label: "校验器状态",
			Value: validatorValue,
			Hint:  fmt.Sprintf("worker %d / 在飞 %d / 排队 %d / 等待 %d", validatorSnap.WorkersStarted, validatorSnap.InFlight, validatorSnap.QueueLen, validatorSnap.WaitingClients),
			Tone:  validatorTone,
			Badge: true,
		},
		{
			Label: "抓取主循环",
			Value: fetchValue,
			Hint:  "决定后台是否继续从各抓取源拉取新节点。",
			Tone:  fetchTone,
			Badge: true,
		},
		{
			Label: "附加能力",
			Value: fmt.Sprintf("NodeMaven %s / Purity %s", nodeMavenValue, purityValue),
			Hint:  fmt.Sprintf("NodeMaven=%s，纯净度查询=%s。", nodeMavenValue, purityValue),
			Tone:  toneForBool(h.cfg.NodeMaven.Enabled || h.cfg.PurityLookup.Enabled),
		},
	}
}

func (h *Handler) buildOverviewRefreshItems() []dashboardItem {
	return []dashboardItem{
		{
			Label: "抓取调度",
			Value: boolLabel(h.cfg.AutoFetchEnabled),
			Hint:  fmt.Sprintf("tick %s / 每轮最多 %d 个源。", formatDurationValue(h.cfg.FetchTickInterval, "默认"), h.cfg.FetchMaxPerTick),
			Tone:  toneForBool(h.cfg.AutoFetchEnabled),
			Badge: true,
		},
		{
			Label: "抓取并发",
			Value: fmt.Sprintf("%d worker", h.cfg.SourceWorkers),
			Hint:  fmt.Sprintf("单源超时 %s。", formatDurationValue(h.cfg.SourceTimeout, "默认")),
		},
		{
			Label: "校验调度",
			Value: boolLabel(h.cfg.AutoValidateEnabled),
			Hint:  fmt.Sprintf("worker %d / 校验超时 %s。", h.cfg.ValidateWorkers, formatDurationValue(h.cfg.ValidateTimeout, "默认")),
			Tone:  toneForBool(h.cfg.AutoValidateEnabled),
			Badge: true,
		},
		{
			Label: "抽样校验",
			Value: fmt.Sprintf("%d / source", h.cfg.SourceSampleValidate),
			Hint:  "每个抓取源入库后会抽样进入校验队列。",
		},
		{
			Label: "fresh 窗口",
			Value: formatDurationValue(h.cfg.FreshWithinDefault, "5m"),
			Hint:  fmt.Sprintf("目标 fresh 池大小 %d。", h.cfg.MinFreshPoolSize),
		},
		{
			Label: "同步校验超时",
			Value: formatDurationValue(h.cfg.SyncVerifyTimeout, "默认"),
			Hint:  "用于即时 verify 接口，避免单次请求等待过久。",
		},
		{
			Label: "统计查询超时",
			Value: formatDurationValue(h.cfg.StatsQueryTimeout, "默认"),
			Hint:  "保护概览页与 /api/v1/stats，避免数据库繁忙时长期 pending。",
		},
		{
			Label: "页面实时刷新",
			Value: formatDurationValue(overviewEventsInterval, "默认"),
			Hint:  "概览页通过 SSE 每隔固定周期推送核心统计。",
		},
		{
			Label: "NodeMaven 并发",
			Value: fmt.Sprintf("%d", h.cfg.NodeMaven.Concurrency),
			Hint:  disabledOrValue(h.cfg.NodeMaven.Enabled, fmt.Sprintf("当前 NodeMaven 已启用，最多并发 %d 请求。", h.cfg.NodeMaven.Concurrency), "当前 NodeMaven 已关闭。"),
		},
		{
			Label: "启动预热",
			Value: formatDurationValue(h.cfg.StartupWarmup.Duration, "未启用"),
			Hint:  fmt.Sprintf("预热抓取 tick %s / validate %d / source %d / 池目标 %d。", formatDurationValue(h.cfg.StartupWarmup.FetchTickInterval, "默认"), h.cfg.StartupWarmup.ValidateWorkers, h.cfg.StartupWarmup.SourceWorkers, h.cfg.StartupWarmup.MinFreshPoolSize),
		},
	}
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func statusText(ok bool, yes, no string) (string, string) {
	if ok {
		return yes, "ok"
	}
	return no, "warn"
}

func toneForBool(v bool) string {
	if v {
		return "ok"
	}
	return "warn"
}

func boolLabel(v bool) string {
	if v {
		return "已启用"
	}
	return "已关闭"
}

func disabledOrValue(enabled bool, enabledValue, disabledValue string) string {
	if enabled {
		return enabledValue
	}
	return disabledValue
}

func formatDurationValue(d time.Duration, zero string) string {
	if d <= 0 {
		return zero
	}
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return d.String()
}

func formatUptime(d time.Duration) string {
	if d <= 0 {
		return "刚启动"
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return d.String()
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	stats, err := h.loadStats(r.Context(), now)
	if err != nil {
		http.Error(w, "读取统计数据失败", http.StatusInternalServerError)
		return
	}
	h.render(w, r, "overview.html", viewData{
		Now:          now,
		Title:        "概览",
		Stats:        stats,
		Token:        r.URL.Query().Get("token"),
		BaseURL:      h.cfg.PublicBaseURL,
		APIKeysOn:    len(h.cfg.APIKeys) > 0,
		ServerItems:  h.buildOverviewServerItems(now),
		RefreshItems: h.buildOverviewRefreshItems(),
	})
}

func (h *Handler) sources(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	stats, _ := h.loadStats(r.Context(), now)
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
	stats, _ := h.loadStats(r.Context(), now)

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
	stats, _ := h.loadStats(r.Context(), now)
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
	stats, _ := h.loadStats(r.Context(), now)
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

	ticker := time.NewTicker(overviewEventsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			now := time.Now()
			stats, err := h.loadStats(r.Context(), now)
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
