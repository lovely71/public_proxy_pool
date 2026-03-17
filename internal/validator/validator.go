package validator

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qiyiyun/public_proxy_pool/internal/config"
	"github.com/qiyiyun/public_proxy_pool/internal/geoip"
	"github.com/qiyiyun/public_proxy_pool/internal/model"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
	"github.com/qiyiyun/public_proxy_pool/internal/v2ray"
	"golang.org/x/net/proxy"
)

const (
	PriorityOnDemand     = 10
	PriorityPoolMaintain = 50
	PrioritySourceSample = 80
	GoogleGenerate204URL = "https://www.gstatic.com/generate_204"
)

type Task struct {
	Priority    int
	NodeID      int64
	Fingerprint string

	Kind     string
	Protocol string
	Host     string
	Port     int
	Username string
	Password string
	RawURI   string
	Name     string

	SourceID int64
}

type ValidationResult struct {
	NodeID      int64
	Fingerprint string
	OK          bool
	LatencyMS   int
	ExitIP      string
	Country     string
	ASN         string
	Anonymity   string
	PurityScore int
	Error       string
	Score       float64
	SourceID    int64
}

type URLTestResult struct {
	NodeID     int64
	TargetURL  string
	FinalURL   string
	HTTPStatus int
	LatencyMS  int
	OK         bool
	Error      string
}

type Snapshot struct {
	Started        bool
	StartedAt      time.Time
	WorkersStarted int
	QueueLen       int
	InFlight       int
	WaitingClients int
}

type Validator struct {
	st  *store.Store
	cfg *config.Config
	geo *geoip.DB

	mu             sync.Mutex
	started        bool
	startedAt      time.Time
	workersStarted int
	queue          *priorityQueue
	inFlight       map[int64]struct{}
	waiters        map[int64][]chan ValidationResult
}

func New(st *store.Store, cfg *config.Config, geo *geoip.DB) *Validator {
	return &Validator{
		st:       st,
		cfg:      cfg,
		geo:      geo,
		queue:    newPriorityQueue(),
		inFlight: map[int64]struct{}{},
		waiters:  map[int64][]chan ValidationResult{},
	}
}

func (v *Validator) Start(ctx context.Context) {
	v.mu.Lock()
	if v.started {
		v.mu.Unlock()
		return
	}
	v.started = true
	v.startedAt = time.Now()
	initialWorkers := v.initialValidateWorkers()
	v.mu.Unlock()

	v.startWorkers(ctx, initialWorkers)

	targetWorkers := v.cfg.ValidateWorkers
	if targetWorkers <= 0 {
		targetWorkers = 50
	}
	if initialWorkers < targetWorkers && v.cfg.StartupWarmup.Duration > 0 {
		go v.scaleWorkersAfterWarmup(ctx, targetWorkers-initialWorkers, v.cfg.StartupWarmup.Duration)
	}
}

func (v *Validator) Enqueue(t Task) {
	if t.NodeID == 0 && strings.TrimSpace(t.Fingerprint) == "" {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if t.NodeID != 0 {
		if _, ok := v.inFlight[t.NodeID]; ok {
			return
		}
		v.inFlight[t.NodeID] = struct{}{}
	}
	v.queue.Push(t)
}

func (v *Validator) RequestCheck(ctx context.Context, nodeID int64) (ValidationResult, error) {
	ch := make(chan ValidationResult, 1)
	v.mu.Lock()
	v.waiters[nodeID] = append(v.waiters[nodeID], ch)
	if _, ok := v.inFlight[nodeID]; !ok {
		v.inFlight[nodeID] = struct{}{}
		v.queue.Push(Task{Priority: PriorityOnDemand, NodeID: nodeID})
	}
	v.mu.Unlock()

	select {
	case <-ctx.Done():
		// Remove waiter to avoid leaks on timeout/cancel.
		v.mu.Lock()
		ws := v.waiters[nodeID]
		for i := 0; i < len(ws); i++ {
			if ws[i] == ch {
				ws = append(ws[:i], ws[i+1:]...)
				break
			}
		}
		if len(ws) == 0 {
			delete(v.waiters, nodeID)
		} else {
			v.waiters[nodeID] = ws
		}
		v.mu.Unlock()
		return ValidationResult{}, ctx.Err()
	case res := <-ch:
		return res, nil
	}
}

func (v *Validator) TestNodeGoogle(ctx context.Context, nodeID int64) (URLTestResult, error) {
	return v.TestNodeURL(ctx, nodeID, GoogleGenerate204URL)
}

func (v *Validator) TestNodeURL(ctx context.Context, nodeID int64, targetURL string) (URLTestResult, error) {
	res := URLTestResult{
		NodeID:    nodeID,
		TargetURL: strings.TrimSpace(targetURL),
	}
	if res.TargetURL == "" {
		res.TargetURL = GoogleGenerate204URL
	}
	if nodeID <= 0 {
		return res, fmt.Errorf("invalid node id")
	}

	node, err := v.st.GetNodeByID(ctx, nodeID)
	if err != nil {
		return res, err
	}

	ok, httpStatus, latencyMS, finalURL, testErr := v.testURLThroughNode(ctx, *node, res.TargetURL)
	res.OK = ok
	res.HTTPStatus = httpStatus
	res.LatencyMS = latencyMS
	res.FinalURL = finalURL
	if testErr != nil {
		res.Error = testErr.Error()
	}
	return res, nil
}

func (v *Validator) worker(ctx context.Context, idx int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		task, ok := v.pop()
		if !ok {
			time.Sleep(120 * time.Millisecond)
			continue
		}
		res := v.runOne(ctx, task)
		v.finish(task, res)
	}
}

func (v *Validator) initialValidateWorkers() int {
	workers := v.cfg.ValidateWorkers
	if workers <= 0 {
		workers = 50
	}
	if v.cfg.StartupWarmup.Duration > 0 && v.cfg.StartupWarmup.ValidateWorkers > 0 {
		return min(v.cfg.StartupWarmup.ValidateWorkers, workers)
	}
	return workers
}

func (v *Validator) startWorkers(ctx context.Context, count int) {
	if count <= 0 {
		return
	}
	v.mu.Lock()
	v.workersStarted += count
	v.mu.Unlock()
	for i := 0; i < count; i++ {
		go v.worker(ctx, i)
	}
}

func (v *Validator) Snapshot() Snapshot {
	v.mu.Lock()
	defer v.mu.Unlock()

	waitingClients := 0
	for _, ws := range v.waiters {
		waitingClients += len(ws)
	}

	return Snapshot{
		Started:        v.started,
		StartedAt:      v.startedAt,
		WorkersStarted: v.workersStarted,
		QueueLen:       v.queue.Len(),
		InFlight:       len(v.inFlight),
		WaitingClients: waitingClients,
	}
}

func (v *Validator) scaleWorkersAfterWarmup(ctx context.Context, extra int, wait time.Duration) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		v.startWorkers(ctx, extra)
	}
}

func (v *Validator) pop() (Task, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.queue.Pop()
}

func (v *Validator) finish(task Task, res ValidationResult) {
	v.mu.Lock()
	if task.NodeID != 0 {
		delete(v.inFlight, task.NodeID)
		if ws := v.waiters[task.NodeID]; len(ws) > 0 {
			delete(v.waiters, task.NodeID)
			for _, ch := range ws {
				select {
				case ch <- res:
				default:
				}
				close(ch)
			}
		}
	}
	v.mu.Unlock()
}

func (v *Validator) runOne(ctx context.Context, task Task) ValidationResult {
	now := time.Now()
	checkedAt := now.Unix()

	var node *store.Node
	var err error
	if task.NodeID != 0 {
		node, err = v.st.GetNodeByID(ctx, task.NodeID)
		if err != nil {
			return ValidationResult{NodeID: task.NodeID, OK: false, Error: err.Error(), SourceID: task.SourceID}
		}
	} else {
		// Upsert then load by fingerprint.
		_, _ = v.st.UpsertNodes(ctx, now, []store.NodeUpsert{{
			Kind:        task.Kind,
			Protocol:    task.Protocol,
			Fingerprint: task.Fingerprint,
			Host:        task.Host,
			Port:        task.Port,
			Username:    task.Username,
			Password:    task.Password,
			RawURI:      task.RawURI,
			Name:        task.Name,
			LastSource:  task.SourceID,
		}})
		node, err = v.st.GetNodeByFingerprint(ctx, task.Fingerprint)
		if err != nil {
			return ValidationResult{Fingerprint: task.Fingerprint, OK: false, Error: err.Error(), SourceID: task.SourceID}
		}
	}

	if node.BanUntil > now.Unix() {
		return ValidationResult{NodeID: node.ID, Fingerprint: node.Fingerprint, OK: false, Error: "banned", SourceID: task.SourceID}
	}

	var ok bool
	var latency int
	var exitIP, country, asn, anonymity string
	var purity int
	var checkErr error

	switch node.Kind {
	case model.KindProxy:
		ok, latency, exitIP, country, asn, anonymity, purity, checkErr = v.checkProxy(ctx, *node)
	case model.KindV2Ray:
		ok, latency, exitIP, country, asn, anonymity, purity, checkErr = v.checkV2Ray(ctx, *node)
	default:
		ok = false
		checkErr = fmt.Errorf("unknown kind: %s", node.Kind)
	}

	errMsg := ""
	if checkErr != nil {
		errMsg = checkErr.Error()
	}

	score := computeScore(*node, ok, latency, purity, checkedAt)
	up := store.NodeCheckUpdate{
		CheckedAt:   checkedAt,
		OK:          ok,
		LatencyMS:   latency,
		ExitIP:      exitIP,
		Country:     country,
		ASN:         asn,
		Anonymity:   anonymity,
		PurityScore: purity,
		Error:       errMsg,
		Score:       score,
	}
	_ = v.st.ApplyNodeCheck(ctx, node.ID, up)

	// Automatic node backoff on repeated failures to save resources (1c1g friendly).
	if !ok {
		failStreak := node.FailStreak + 1
		if failStreak >= 3 {
			until := time.Now().Add(5 * time.Minute)
			if failStreak == 4 {
				until = time.Now().Add(15 * time.Minute)
			}
			if failStreak >= 5 {
				until = time.Now().Add(1 * time.Hour)
			}
			_ = v.st.BanNode(ctx, node.ID, until, "auto backoff")
		}
	}

	// Update source EMA (best-effort).
	if task.SourceID != 0 {
		v.updateSourceEMA(ctx, task.SourceID, ok, score)
	}

	return ValidationResult{
		NodeID:      node.ID,
		Fingerprint: node.Fingerprint,
		OK:          ok,
		LatencyMS:   latency,
		ExitIP:      exitIP,
		Country:     country,
		ASN:         asn,
		Anonymity:   anonymity,
		PurityScore: purity,
		Error:       errMsg,
		Score:       score,
		SourceID:    task.SourceID,
	}
}

func (v *Validator) updateSourceEMA(ctx context.Context, sourceID int64, ok bool, score float64) {
	src, err := v.st.GetSourceByID(ctx, sourceID)
	if err != nil {
		return
	}
	alpha := 0.2
	yield := 0.0
	if ok {
		yield = 1.0
	}
	scoreNorm := clamp(score/1000.0, 0, 1)
	newYield := src.EMAYield*(1-alpha) + alpha*yield
	newScore := src.EMAAvgScore*(1-alpha) + alpha*scoreNorm
	_ = v.st.UpdateSourceEMA(ctx, sourceID, newYield, newScore)
}

func (v *Validator) checkProxy(ctx context.Context, node store.Node) (ok bool, latencyMS int, exitIP, country, asn, anonymity string, purity int, err error) {
	client, err := v.httpClientForProxy(node)
	if err != nil {
		return false, 0, "", "", "", "", 0, err
	}

	ok, latencyMS, exitIP, country, err = v.validateViaURL(ctx, client, v.cfg.ValidateURL, v.cfg.ValidateKeyword, parseCloudflareTrace)
	if !ok && v.cfg.ValidateHTTPURL != "" && (node.Protocol == "http" || node.Protocol == "https") {
		// Fallback for HTTP proxies that cannot reach HTTPS targets reliably.
		ok2, latency2, exitIP2, country2, err2 := v.validateViaURL(ctx, client, v.cfg.ValidateHTTPURL, "", parseIPAPI)
		if ok2 {
			ok, latencyMS, exitIP, country, err = ok2, latency2, exitIP2, country2, nil
		} else if err2 != nil {
			// Keep primary error; fallback is best-effort.
		}
	}
	if !ok {
		return false, latencyMS, "", "", "", "", 0, err
	}

	// Optional: probe echo for anonymity.
	if v.cfg.ProbeEchoURL != "" {
		a, err := v.detectAnonymityViaProbe(ctx, client)
		if err == nil {
			anonymity = a
		}
	}

	purity = basePurityFromAnonymity(anonymity)
	if exitIP != "" && v.cfg.PurityLookup.Enabled {
		if facts, _ := v.getIPFacts(ctx, exitIP); facts != nil {
			if country == "" && facts.Country != "" {
				country = facts.Country
			}
			if facts.Hosting || facts.Proxy {
				purity -= 30
			}
			if facts.Mobile {
				purity += 10
			}
		}
	}

	if exitIP != "" && v.geo != nil {
		if country == "" {
			country = v.geo.CountryISO(exitIP)
		}
		asn = v.geo.ASN(exitIP)
	}
	purity = clampInt(purity, 0, 100)
	return true, latencyMS, exitIP, strings.ToUpper(country), strings.TrimSpace(asn), anonymity, purity, nil
}

func (v *Validator) validateViaURL(
	ctx context.Context,
	client *http.Client,
	targetURL string,
	keyword string,
	parse func(string) (ip string, country string),
) (ok bool, latencyMS int, exitIP, country string, err error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return false, 0, "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	latencyMS = int(time.Since(start).Milliseconds())
	if err != nil {
		return false, latencyMS, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return false, latencyMS, "", "", fmt.Errorf("status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	txt := string(body)
	if kw := strings.TrimSpace(keyword); kw != "" && !strings.Contains(txt, kw) {
		return false, latencyMS, "", "", fmt.Errorf("keyword not found")
	}
	exitIP, country = parse(txt)
	if strings.TrimSpace(exitIP) == "" {
		return false, latencyMS, "", "", fmt.Errorf("no exit ip")
	}
	return true, latencyMS, strings.TrimSpace(exitIP), strings.TrimSpace(country), nil
}

func (v *Validator) checkV2Ray(ctx context.Context, node store.Node) (ok bool, latencyMS int, exitIP, country, asn, anonymity string, purity int, err error) {
	start := time.Now()
	switch strings.ToLower(strings.TrimSpace(v.cfg.V2RayValidateMode)) {
	case "sing-box":
		ok, exitIP, country, asn, anonymity, purity, err = v.checkV2RayViaSingBox(ctx, node)
	default:
		ok, err = tcpDialCheck(ctx, node.Host, node.Port, v.cfg.TCPCheckTimeout, v.cfg.TCPCheckRetries)
		purity = 50
	}
	latencyMS = int(time.Since(start).Milliseconds())
	if err != nil {
		return false, latencyMS, "", "", "", "", 0, err
	}
	return ok, latencyMS, exitIP, strings.ToUpper(country), strings.TrimSpace(asn), anonymity, clampInt(purity, 0, 100), nil
}

func (v *Validator) testURLThroughNode(ctx context.Context, node store.Node, targetURL string) (ok bool, httpStatus, latencyMS int, finalURL string, err error) {
	switch node.Kind {
	case model.KindProxy:
		client, err := v.httpClientForProxy(node)
		if err != nil {
			return false, 0, 0, "", err
		}
		return performGenerate204Request(ctx, client, targetURL)
	case model.KindV2Ray:
		if !strings.EqualFold(strings.TrimSpace(v.cfg.V2RayValidateMode), "sing-box") {
			return false, 0, 0, "", fmt.Errorf("v2ray google test requires V2RAY_VALIDATE_MODE=sing-box")
		}
		ctxRun, client, cleanup, stderr, err := v.startSingBoxHTTPClient(ctx, node)
		if err != nil {
			return false, 0, 0, "", err
		}
		defer cleanup()

		ok, httpStatus, latencyMS, finalURL, err = performGenerate204Request(ctxRun, client, targetURL)
		if err != nil {
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				return false, httpStatus, latencyMS, finalURL, fmt.Errorf("%w (%s)", err, msg)
			}
			return false, httpStatus, latencyMS, finalURL, err
		}
		return ok, httpStatus, latencyMS, finalURL, nil
	default:
		return false, 0, 0, "", fmt.Errorf("unsupported node kind: %s", node.Kind)
	}
}

func (v *Validator) httpClientForProxy(node store.Node) (*http.Client, error) {
	timeout := v.cfg.ValidateTimeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	switch strings.ToLower(node.Protocol) {
	case "http", "https":
		proxyURL, err := url.Parse(node.RawURI)
		if err != nil {
			return nil, err
		}
		// Many public lists label "https" proxies but they are plain HTTP proxies that support CONNECT.
		// Go's standard library has best support for HTTP proxies; treat https:// as http:// here.
		if strings.EqualFold(proxyURL.Scheme, "https") {
			proxyURL.Scheme = "http"
		}
		tr := &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
			DisableKeepAlives:     true,
			ForceAttemptHTTP2:     false,
			IdleConnTimeout:       5 * time.Second,
			ResponseHeaderTimeout: timeout,
		}
		client := &http.Client{Timeout: timeout, Transport: tr}

		// Fallback: some lists use "https" to mean "http proxy supporting CONNECT".
		if strings.ToLower(proxyURL.Scheme) == "https" {
			// Try a tiny request in background? Too expensive. We'll fallback on specific error in request path.
		}
		return client, nil
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if node.Username != "" {
			auth = &proxy.Auth{User: node.Username, Password: node.Password}
		}
		d, err := proxy.SOCKS5("tcp", net.JoinHostPort(node.Host, strconv.Itoa(node.Port)), auth, &net.Dialer{Timeout: timeout})
		if err != nil {
			return nil, err
		}
		tr := &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return d.Dial(network, addr)
			},
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			DisableKeepAlives:     true,
			ForceAttemptHTTP2:     false,
			IdleConnTimeout:       5 * time.Second,
			ResponseHeaderTimeout: timeout,
		}
		return &http.Client{Timeout: timeout, Transport: tr}, nil
	case "socks4":
		tr := &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialSOCKS4(ctx, node, network, addr, timeout)
			},
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			DisableKeepAlives:     true,
			ForceAttemptHTTP2:     false,
			IdleConnTimeout:       5 * time.Second,
			ResponseHeaderTimeout: timeout,
		}
		return &http.Client{Timeout: timeout, Transport: tr}, nil
	default:
		return nil, fmt.Errorf("unsupported proxy protocol: %s", node.Protocol)
	}
}

func parseCloudflareTrace(body string) (ip string, country string) {
	// Example lines:
	// ip=1.2.3.4
	// loc=US
	lines := strings.Split(body, "\n")
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "ip=") {
			ip = strings.TrimSpace(strings.TrimPrefix(ln, "ip="))
		}
		if strings.HasPrefix(ln, "loc=") {
			country = strings.TrimSpace(strings.TrimPrefix(ln, "loc="))
		}
	}
	return ip, country
}

func parseIPAPI(body string) (ip string, country string) {
	type resp struct {
		Status      string `json:"status"`
		Query       string `json:"query"`
		CountryCode string `json:"countryCode"`
	}
	var r resp
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		return "", ""
	}
	if strings.ToLower(strings.TrimSpace(r.Status)) != "success" {
		return "", ""
	}
	return strings.TrimSpace(r.Query), strings.TrimSpace(r.CountryCode)
}

func basePurityFromAnonymity(anonymity string) int {
	switch strings.ToLower(anonymity) {
	case "elite":
		return 80
	case "anonymous":
		return 60
	case "transparent":
		return 40
	default:
		return 50
	}
}

type probeResp struct {
	RemoteIP string              `json:"remote_ip"`
	Headers  map[string][]string `json:"headers"`
}

func (v *Validator) detectAnonymityViaProbe(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.ProbeEchoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("probe status=%d", resp.StatusCode)
	}
	var pr probeResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&pr); err != nil {
		return "", err
	}
	h := make(map[string]string, len(pr.Headers))
	for k, vs := range pr.Headers {
		if len(vs) == 0 {
			continue
		}
		h[strings.ToLower(k)] = vs[0]
	}
	if v, ok := h["x-forwarded-for"]; ok && strings.TrimSpace(v) != "" {
		return "transparent", nil
	}
	if v, ok := h["forwarded"]; ok && strings.TrimSpace(v) != "" {
		return "anonymous", nil
	}
	if v, ok := h["via"]; ok && strings.TrimSpace(v) != "" {
		return "anonymous", nil
	}
	return "elite", nil
}

type ipAPIBatchReq struct {
	Query string `json:"query"`
}

type ipAPIBatchResp struct {
	Status      string `json:"status"`
	Message     string `json:"message"`
	Query       string `json:"query"`
	CountryCode string `json:"countryCode"`
	Proxy       bool   `json:"proxy"`
	Hosting     bool   `json:"hosting"`
	Mobile      bool   `json:"mobile"`
}

func (v *Validator) getIPFacts(ctx context.Context, ip string) (*store.IPFacts, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil, store.ErrNotFound
	}
	if !v.cfg.PurityLookup.Enabled {
		return nil, store.ErrNotFound
	}

	if cached, err := v.st.GetIPFacts(ctx, ip); err == nil && cached != nil {
		if v.cfg.PurityLookup.CacheTTL <= 0 || time.Since(time.Unix(cached.UpdatedAt, 0)) <= v.cfg.PurityLookup.CacheTTL {
			return cached, nil
		}
	}

	// POST batch with one item.
	body, _ := json.Marshal([]ipAPIBatchReq{{Query: ip}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.cfg.PurityLookup.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: v.cfg.PurityLookup.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("purity lookup status=%d", resp.StatusCode)
	}
	var arr []ipAPIBatchResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&arr); err != nil {
		return nil, err
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("purity lookup empty")
	}
	r := arr[0]
	if strings.ToLower(r.Status) != "success" {
		return nil, fmt.Errorf("purity lookup failed: %s", r.Message)
	}
	facts := store.IPFacts{
		IP:        ip,
		UpdatedAt: time.Now().Unix(),
		Country:   strings.ToUpper(strings.TrimSpace(r.CountryCode)),
		Proxy:     r.Proxy,
		Hosting:   r.Hosting,
		Mobile:    r.Mobile,
	}
	_ = v.st.UpsertIPFacts(ctx, facts)
	return &facts, nil
}

func computeScore(prev store.Node, ok bool, latencyMS int, purity int, checkedAt int64) float64 {
	okCount := prev.OKCount
	failCount := prev.FailCount
	failStreak := prev.FailStreak
	lastOKAt := prev.LastOKAt
	if ok {
		okCount++
		failStreak = 0
		lastOKAt = checkedAt
	} else {
		failCount++
		failStreak++
	}
	total := okCount + failCount
	successRate := 0.5
	if total > 0 {
		successRate = float64(okCount) / float64(total)
	}

	recency := 0.0
	if lastOKAt > 0 {
		age := float64(checkedAt - lastOKAt)
		tau := float64((6 * time.Hour).Seconds())
		if tau > 0 {
			recency = math.Exp(-age / tau)
		}
	}

	latFactor := 0.0
	if latencyMS > 0 {
		latFactor = 1.0 / (1.0 + float64(latencyMS)/300.0)
	}
	purityFactor := clamp(float64(purity)/100.0, 0, 1)

	score := 400*successRate + 200*recency + 200*latFactor + 200*purityFactor - 50*float64(failStreak)
	if score < 0 {
		score = 0
	}
	return score
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func tcpDialCheck(ctx context.Context, host string, port int, timeout time.Duration, retries int) (bool, error) {
	if retries <= 0 {
		retries = 1
	}
	addr := net.JoinHostPort(strings.TrimSpace(host), strconv.Itoa(port))
	var lastErr error
	for i := 0; i < retries; i++ {
		d := net.Dialer{Timeout: timeout}
		c, err := d.DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = c.Close()
			return true, nil
		}
		lastErr = err
	}
	return false, lastErr
}

func dialSOCKS4(ctx context.Context, node store.Node, network, addr string, timeout time.Duration) (net.Conn, error) {
	// SOCKS4 only supports TCP connect with IPv4 in our implementation.
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("unsupported network: %s", network)
	}
	targetHost, targetPortStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	targetPort, _ := strconv.Atoi(targetPortStr)
	targetIP := net.ParseIP(targetHost)
	if targetIP == nil || targetIP.To4() == nil {
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", targetHost)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("socks4 resolve failed")
		}
		targetIP = ips[0].To4()
	}

	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(node.Host, strconv.Itoa(node.Port)))
	if err != nil {
		return nil, err
	}

	user := node.Username
	if user == "" {
		user = "go"
	}
	// Request:
	// VN=4, CD=1, DSTPORT(2), DSTIP(4), USERID, 0
	req := make([]byte, 0, 9+len(user))
	req = append(req, 0x04, 0x01)
	req = append(req, byte(targetPort>>8), byte(targetPort&0xff))
	req = append(req, targetIP.To4()...)
	req = append(req, []byte(user)...)
	req = append(req, 0x00)
	_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(req); err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	resp := make([]byte, 8)
	if _, err := io.ReadFull(conn, resp); err != nil {
		_ = conn.Close()
		return nil, err
	}
	// resp[1] == 0x5A request granted
	if len(resp) < 2 || resp[1] != 0x5A {
		_ = conn.Close()
		return nil, fmt.Errorf("socks4 rejected: 0x%02x", resp[1])
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

func (v *Validator) checkV2RayViaSingBox(ctx context.Context, node store.Node) (ok bool, exitIP, country, asn, anonymity string, purity int, err error) {
	ctxRun, client, cleanup, stderr, err := v.startSingBoxHTTPClient(ctx, node)
	if err != nil {
		return false, "", "", "", "", 0, err
	}
	defer cleanup()

	req, err := http.NewRequestWithContext(ctxRun, http.MethodGet, v.cfg.ValidateURL, nil)
	if err != nil {
		return false, "", "", "", "", 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return false, "", "", "", "", 0, fmt.Errorf("validate via sing-box failed: %w (%s)", err, msg)
		}
		return false, "", "", "", "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return false, "", "", "", "", 0, fmt.Errorf("status=%d (%s)", resp.StatusCode, msg)
		}
		return false, "", "", "", "", 0, fmt.Errorf("status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	txt := string(body)
	if kw := strings.TrimSpace(v.cfg.ValidateKeyword); kw != "" && !strings.Contains(txt, kw) {
		return false, "", "", "", "", 0, fmt.Errorf("keyword not found")
	}
	exitIP, country = parseCloudflareTrace(txt)

	if v.cfg.ProbeEchoURL != "" {
		if a, err := v.detectAnonymityViaProbe(ctxRun, client); err == nil {
			anonymity = a
		}
	}
	purity = basePurityFromAnonymity(anonymity)
	if exitIP != "" && v.cfg.PurityLookup.Enabled {
		if facts, _ := v.getIPFacts(ctxRun, exitIP); facts != nil {
			if country == "" && facts.Country != "" {
				country = facts.Country
			}
			if facts.Hosting || facts.Proxy {
				purity -= 30
			}
			if facts.Mobile {
				purity += 10
			}
		}
	}
	if exitIP != "" && v.geo != nil {
		if country == "" {
			country = v.geo.CountryISO(exitIP)
		}
		asn = v.geo.ASN(exitIP)
	}
	return true, exitIP, country, strings.TrimSpace(asn), anonymity, purity, nil
}

func (v *Validator) startSingBoxHTTPClient(parent context.Context, node store.Node) (context.Context, *http.Client, func(), *bytes.Buffer, error) {
	if strings.TrimSpace(v.cfg.SingBoxPath) == "" {
		return nil, nil, nil, nil, fmt.Errorf("sing-box path not set")
	}
	if _, err := exec.LookPath(v.cfg.SingBoxPath); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("sing-box not found: %w", err)
	}

	parsed, err := v2ray.ParseURI(node.RawURI)
	if err != nil || parsed == nil {
		return nil, nil, nil, nil, fmt.Errorf("parse v2ray uri: %w", err)
	}
	outbound, err := singBoxOutbound(parsed)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	port, err := pickFreePort()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	tmpDir, err := os.MkdirTemp("", "proxypool-singbox-*")
	if err != nil {
		return nil, nil, nil, nil, err
	}

	cfg := map[string]any{
		"log": map[string]any{
			"level": "error",
		},
		"inbounds": []any{
			map[string]any{
				"type":        "socks",
				"tag":         "in",
				"listen":      "127.0.0.1",
				"listen_port": port,
			},
		},
		"outbounds": []any{
			outbound,
			map[string]any{"type": "direct", "tag": "direct"},
		},
		"route": map[string]any{
			"final": "proxy",
		},
	}
	b, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(cfgPath, b, 0o600); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, nil, nil, nil, err
	}

	ctxRun, cancel := context.WithTimeout(parent, v.cfg.ValidateTimeout+15*time.Second)

	cmd := exec.CommandContext(ctxRun, v.cfg.SingBoxPath, "run", "-c", cfgPath)
	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		cancel()
		_ = os.RemoveAll(tmpDir)
		return nil, nil, nil, nil, err
	}

	cleanup := func() {
		cancel()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		_ = os.RemoveAll(tmpDir)
	}

	// Wait for socks to be ready.
	if err := waitTCP(ctxRun, "127.0.0.1", port, 2*time.Second); err != nil {
		cleanup()
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, nil, nil, nil, fmt.Errorf("sing-box not ready: %w (%s)", err, msg)
		}
		return nil, nil, nil, nil, fmt.Errorf("sing-box not ready: %w", err)
	}

	client, err := httpClientViaSOCKS("127.0.0.1", port, v.cfg.ValidateTimeout)
	if err != nil {
		cleanup()
		return nil, nil, nil, nil, err
	}
	return ctxRun, client, cleanup, &stderr, nil
}

func performGenerate204Request(ctx context.Context, client *http.Client, targetURL string) (ok bool, httpStatus, latencyMS int, finalURL string, err error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return false, 0, 0, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	latencyMS = int(time.Since(start).Milliseconds())
	if err != nil {
		return false, 0, latencyMS, "", err
	}
	defer resp.Body.Close()
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	httpStatus = resp.StatusCode
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusNoContent {
		return false, httpStatus, latencyMS, finalURL, fmt.Errorf("unexpected status=%d", resp.StatusCode)
	}
	return true, httpStatus, latencyMS, finalURL, nil
}

func pickFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("bad addr")
	}
	return addr.Port, nil
}

func waitTCP(ctx context.Context, host string, port int, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	for {
		d := net.Dialer{Timeout: 200 * time.Millisecond}
		c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err == nil {
			_ = c.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(120 * time.Millisecond):
		}
	}
}

func httpClientViaSOCKS(host string, port int, timeout time.Duration) (*http.Client, error) {
	d, err := proxy.SOCKS5("tcp", net.JoinHostPort(host, strconv.Itoa(port)), nil, &net.Dialer{Timeout: timeout})
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return d.Dial(network, addr)
		},
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		DisableKeepAlives:     true,
		ForceAttemptHTTP2:     false,
		IdleConnTimeout:       5 * time.Second,
		ResponseHeaderTimeout: timeout,
	}
	return &http.Client{Timeout: timeout, Transport: tr}, nil
}

func singBoxOutbound(p *v2ray.Parsed) (map[string]any, error) {
	tag := "proxy"
	switch p.Protocol {
	case "ss":
		if p.Method == "" || p.Password == "" {
			return nil, fmt.Errorf("ss missing method/password")
		}
		return map[string]any{
			"type":        "shadowsocks",
			"tag":         tag,
			"server":      p.Host,
			"server_port": p.Port,
			"method":      p.Method,
			"password":    p.Password,
		}, nil
	case "vmess":
		if p.UUID == "" {
			return nil, fmt.Errorf("vmess missing uuid")
		}
		out := map[string]any{
			"type":        "vmess",
			"tag":         tag,
			"server":      p.Host,
			"server_port": p.Port,
			"uuid":        p.UUID,
			"alter_id":    p.AlterID,
			"security":    "auto",
		}
		applySingBoxTLS(out, p)
		applySingBoxTransport(out, p)
		return out, nil
	case "vless":
		if p.UUID == "" {
			return nil, fmt.Errorf("vless missing uuid")
		}
		out := map[string]any{
			"type":        "vless",
			"tag":         tag,
			"server":      p.Host,
			"server_port": p.Port,
			"uuid":        p.UUID,
		}
		applySingBoxTLS(out, p)
		applySingBoxTransport(out, p)
		return out, nil
	case "trojan":
		if p.Password == "" {
			return nil, fmt.Errorf("trojan missing password")
		}
		out := map[string]any{
			"type":        "trojan",
			"tag":         tag,
			"server":      p.Host,
			"server_port": p.Port,
			"password":    p.Password,
		}
		// Trojan is TLS-based.
		if strings.TrimSpace(p.Security) == "" {
			p.Security = "tls"
		}
		applySingBoxTLS(out, p)
		applySingBoxTransport(out, p)
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported protocol for sing-box: %s", p.Protocol)
	}
}

func applySingBoxTLS(out map[string]any, p *v2ray.Parsed) {
	sec := strings.ToLower(strings.TrimSpace(p.Security))
	if p.Protocol == "trojan" {
		sec = "tls"
	}
	if sec != "tls" && sec != "" {
		return
	}
	if sec == "tls" {
		serverName := p.SNI
		if serverName == "" {
			serverName = p.Host
		}
		out["tls"] = map[string]any{
			"enabled":     true,
			"server_name": serverName,
			"insecure":    true,
		}
	}
}

func applySingBoxTransport(out map[string]any, p *v2ray.Parsed) {
	switch strings.ToLower(strings.TrimSpace(p.Transport)) {
	case "ws":
		tr := map[string]any{
			"type": "ws",
		}
		if p.Path != "" {
			tr["path"] = p.Path
		}
		if p.HostHdr != "" {
			tr["headers"] = map[string]any{"Host": p.HostHdr}
		}
		out["transport"] = tr
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
