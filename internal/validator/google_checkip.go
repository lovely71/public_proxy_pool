package validator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/qiyiyun/public_proxy_pool/internal/config"
	"github.com/qiyiyun/public_proxy_pool/internal/model"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
)

const DefaultCheckIPURL = "https://www.cloudflare.com/cdn-cgi/trace"
const DefaultHuggingFaceURL = "https://huggingface.co/robots.txt"

// GoogleCheckIPURL is kept as a compatibility alias for existing callers.
const GoogleCheckIPURL = DefaultCheckIPURL

type AliveResult struct {
	LatencyMS int
	OK        bool
	Error     string
}

type URLProbeResult struct {
	TargetURL  string
	FinalURL   string
	HTTPStatus int
	LatencyMS  int
	OK         bool
	Error      string
}

type GoogleCheckIPResult struct {
	TargetURL  string
	FinalURL   string
	HTTPStatus int
	LatencyMS  int
	ExitIP     string
	OK         bool
	Error      string
}

type GoogleProbeResult struct {
	Alive       AliveResult
	Google      URLProbeResult
	HuggingFace URLProbeResult
	CheckIP     GoogleCheckIPResult
}

func ProbeNodeAlive(ctx context.Context, cfg *config.Config, node store.Node) (AliveResult, error) {
	res := AliveResult{}
	v := &Validator{cfg: defaultGoogleCheckIPConfig(cfg)}

	if strings.TrimSpace(node.Host) == "" || node.Port <= 0 {
		err := fmt.Errorf("missing host or port")
		res.Error = err.Error()
		return res, err
	}

	timeout := v.cfg.TCPCheckTimeout
	if timeout <= 0 {
		timeout = minDuration(v.cfg.ValidateTimeout, 1500*time.Millisecond)
	}
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}
	retries := v.cfg.TCPCheckRetries
	if retries <= 0 {
		retries = 1
	}

	start := time.Now()
	ok, err := tcpDialCheck(ctx, node.Host, node.Port, timeout, retries)
	res.LatencyMS = int(time.Since(start).Milliseconds())
	res.OK = ok
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	return res, nil
}

func ProbeNodeGoogle204(ctx context.Context, cfg *config.Config, node store.Node) (URLProbeResult, error) {
	res := URLProbeResult{TargetURL: GoogleGenerate204URL}
	v := &Validator{cfg: defaultGoogleCheckIPConfig(cfg)}

	ctxRun, client, cleanup, stderr, err := v.startNodeHTTPClient(ctx, node)
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	defer cleanup()

	res.OK, res.HTTPStatus, res.LatencyMS, res.FinalURL, err = performGenerate204Request(ctxRun, client, res.TargetURL)
	if err != nil {
		err = appendProbeStderr(err, stderr)
		res.Error = err.Error()
		return res, err
	}
	return res, nil
}

func ProbeNodeHuggingFace(ctx context.Context, cfg *config.Config, node store.Node, targetURL string) (URLProbeResult, error) {
	res := URLProbeResult{TargetURL: strings.TrimSpace(targetURL)}
	if res.TargetURL == "" {
		res.TargetURL = DefaultHuggingFaceURL
	}
	v := &Validator{cfg: defaultGoogleCheckIPConfig(cfg)}

	ctxRun, client, cleanup, stderr, err := v.startNodeHTTPClient(ctx, node)
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	defer cleanup()

	res.OK, res.HTTPStatus, res.LatencyMS, res.FinalURL, err = performOKRequest(ctxRun, client, res.TargetURL)
	if err != nil {
		err = appendProbeStderr(err, stderr)
		res.Error = err.Error()
		return res, err
	}
	return res, nil
}

func ProbeNodeAccessSuite(ctx context.Context, cfg *config.Config, node store.Node, huggingFaceURL, checkIPURL string) (GoogleProbeResult, error) {
	res := GoogleProbeResult{
		Google:      URLProbeResult{TargetURL: GoogleGenerate204URL},
		HuggingFace: URLProbeResult{TargetURL: strings.TrimSpace(huggingFaceURL)},
		CheckIP: GoogleCheckIPResult{
			TargetURL: strings.TrimSpace(checkIPURL),
		},
	}
	if res.HuggingFace.TargetURL == "" {
		res.HuggingFace.TargetURL = DefaultHuggingFaceURL
	}
	if res.CheckIP.TargetURL == "" {
		res.CheckIP.TargetURL = DefaultCheckIPURL
	}

	aliveRes, aliveErr := ProbeNodeAlive(ctx, cfg, node)
	res.Alive = aliveRes
	if aliveErr != nil {
		return res, aliveErr
	}

	v := &Validator{cfg: defaultGoogleCheckIPConfig(cfg)}
	ctxRun, client, cleanup, stderr, err := v.startNodeHTTPClient(ctx, node)
	if err != nil {
		res.Google.Error = err.Error()
		res.CheckIP.Error = err.Error()
		return res, err
	}
	defer cleanup()

	res.Google.OK, res.Google.HTTPStatus, res.Google.LatencyMS, res.Google.FinalURL, err = performGenerate204Request(ctxRun, client, res.Google.TargetURL)
	if err != nil {
		err = appendProbeStderr(err, stderr)
		res.Google.Error = err.Error()
	} else {
		res.Google.Error = ""
	}

	res.HuggingFace.OK, res.HuggingFace.HTTPStatus, res.HuggingFace.LatencyMS, res.HuggingFace.FinalURL, err = performOKRequest(ctxRun, client, res.HuggingFace.TargetURL)
	if err != nil {
		err = appendProbeStderr(err, stderr)
		res.HuggingFace.Error = err.Error()
	} else {
		res.HuggingFace.Error = ""
	}

	res.CheckIP.OK, res.CheckIP.HTTPStatus, res.CheckIP.LatencyMS, res.CheckIP.FinalURL, res.CheckIP.ExitIP, err = performCheckIPRequest(ctxRun, client, res.CheckIP.TargetURL)
	if err != nil {
		err = appendProbeStderr(err, stderr)
		res.CheckIP.Error = err.Error()
		return res, err
	}
	return res, nil
}

func ProbeNodeGoogleSuite(ctx context.Context, cfg *config.Config, node store.Node, checkIPURL string) (GoogleProbeResult, error) {
	return ProbeNodeAccessSuite(ctx, cfg, node, DefaultHuggingFaceURL, checkIPURL)
}

func ProbeNodeGoogleCheckIP(ctx context.Context, cfg *config.Config, node store.Node, targetURL string) (GoogleCheckIPResult, error) {
	res := GoogleCheckIPResult{
		TargetURL: strings.TrimSpace(targetURL),
	}
	if res.TargetURL == "" {
		res.TargetURL = DefaultCheckIPURL
	}

	v := &Validator{cfg: defaultGoogleCheckIPConfig(cfg)}
	ctxRun, client, cleanup, stderr, err := v.startNodeHTTPClient(ctx, node)
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	defer cleanup()

	res.OK, res.HTTPStatus, res.LatencyMS, res.FinalURL, res.ExitIP, err = performCheckIPRequest(ctxRun, client, res.TargetURL)
	if err != nil {
		err = appendProbeStderr(err, stderr)
		res.Error = err.Error()
		return res, err
	}
	return res, nil
}

func defaultGoogleCheckIPConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		cfg = &config.Config{}
	}
	clone := *cfg
	if clone.ValidateTimeout <= 0 {
		clone.ValidateTimeout = 8 * time.Second
	}
	if strings.TrimSpace(clone.SingBoxPath) == "" {
		clone.SingBoxPath = "sing-box"
	}
	if strings.TrimSpace(clone.V2RayValidateMode) == "" {
		clone.V2RayValidateMode = "sing-box"
	}
	return &clone
}

func (v *Validator) startNodeHTTPClient(parent context.Context, node store.Node) (context.Context, *http.Client, func(), *bytes.Buffer, error) {
	switch node.Kind {
	case model.KindProxy:
		client, err := v.httpClientForProxy(node)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		return parent, client, func() {}, nil, nil
	case model.KindV2Ray:
		if !strings.EqualFold(strings.TrimSpace(v.cfg.V2RayValidateMode), "sing-box") {
			return nil, nil, nil, nil, fmt.Errorf("v2ray probe requires V2RAY_VALIDATE_MODE=sing-box")
		}
		return v.startSingBoxHTTPClient(parent, node)
	default:
		return nil, nil, nil, nil, fmt.Errorf("unsupported node kind: %s", node.Kind)
	}
}

func performCheckIPRequest(ctx context.Context, client *http.Client, targetURL string) (ok bool, httpStatus, latencyMS int, finalURL, exitIP string, err error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return false, 0, 0, "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	latencyMS = int(time.Since(start).Milliseconds())
	if err != nil {
		return false, 0, latencyMS, "", "", err
	}
	defer resp.Body.Close()

	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	httpStatus = resp.StatusCode

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, httpStatus, latencyMS, finalURL, "", fmt.Errorf("unexpected status=%d", resp.StatusCode)
	}

	bodyText := string(body)
	exitIP, _ = parseCloudflareTrace(bodyText)
	if exitIP == "" {
		exitIP = parseIPFromText(bodyText)
	}
	if exitIP == "" {
		return false, httpStatus, latencyMS, finalURL, "", fmt.Errorf("response did not contain an ip")
	}
	return true, httpStatus, latencyMS, finalURL, exitIP, nil
}

func performOKRequest(ctx context.Context, client *http.Client, targetURL string) (ok bool, httpStatus, latencyMS int, finalURL string, err error) {
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, httpStatus, latencyMS, finalURL, fmt.Errorf("unexpected status=%d", resp.StatusCode)
	}
	return true, httpStatus, latencyMS, finalURL, nil
}

func parseIPFromText(body string) string {
	for _, field := range strings.Fields(body) {
		candidate := strings.TrimSpace(field)
		candidate = strings.Trim(candidate, "[](),;\"'")
		if candidate == "" {
			continue
		}
		ip := net.ParseIP(candidate)
		if ip == nil {
			continue
		}
		return ip.String()
	}
	return ""
}

func appendProbeStderr(err error, stderr *bytes.Buffer) error {
	if err == nil || stderr == nil {
		return err
	}
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		return err
	}
	return fmt.Errorf("%w (%s)", err, msg)
}

func minDuration(a, b time.Duration) time.Duration {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}
