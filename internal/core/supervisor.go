package core

import (
	"context"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/qiyiyun/public_proxy_pool/internal/config"
	"github.com/qiyiyun/public_proxy_pool/internal/model"
	"github.com/qiyiyun/public_proxy_pool/internal/sources"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
	"github.com/qiyiyun/public_proxy_pool/internal/validator"
)

type Supervisor struct {
	st        *store.Store
	val       *validator.Validator
	cfg       *config.Config
	startedAt time.Time
}

func NewSupervisor(st *store.Store, val *validator.Validator, cfg *config.Config) *Supervisor {
	return &Supervisor{st: st, val: val, cfg: cfg, startedAt: time.Now()}
}

func (s *Supervisor) Run(ctx context.Context) error {
	slog.Info("supervisor starting", "auto_fetch", s.cfg.AutoFetchEnabled, "auto_validate", s.cfg.AutoValidateEnabled)
	s.val.Start(ctx)

	errCh := make(chan error, 3)
	if s.cfg.AutoFetchEnabled {
		go func() { errCh <- s.fetchLoop(ctx) }()
	}
	if s.cfg.AutoValidateEnabled {
		go func() { errCh <- s.validateLoop(ctx) }()
	}
	if s.cfg.CleanupInterval > 0 && s.cfg.ChecksRetention > 0 {
		go func() { errCh <- s.cleanupLoop(ctx) }()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (s *Supervisor) fetchLoop(ctx context.Context) error {
	for {
		now := time.Now()
		if err := s.fetchTick(ctx, now); err != nil {
			slog.Warn("fetch tick failed", "error", err)
		}
		wait := s.fetchTickInterval(now)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *Supervisor) fetchTick(ctx context.Context, now time.Time) error {
	due, err := s.st.GetSourcesDue(ctx, now, s.fetchMaxPerTick(now))
	if err != nil {
		return err
	}
	if len(due) == 0 {
		return nil
	}

	sem := make(chan struct{}, max(1, s.sourceWorkers(now)))
	type fetchOut struct {
		src store.Source
		err error
	}
	outCh := make(chan fetchOut, len(due))

	for _, src := range due {
		src := src
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			outCh <- fetchOut{src: src, err: s.fetchOne(ctx, now, src)}
		}()
	}
	for i := 0; i < len(due); i++ {
		r := <-outCh
		if r.err != nil {
			slog.Warn("source fetch failed", "source", r.src.Name, "error", r.err)
		}
	}
	return nil
}

func (s *Supervisor) fetchOne(ctx context.Context, now time.Time, src store.Source) error {
	nextFetch := now.Add(time.Duration(src.IntervalSec) * time.Second).Unix()
	meta := store.FetchMetaUpdate{
		LastFetchAt:  now.Unix(),
		ETag:         src.ETag,
		LastModified: src.LastModified,
		LastError:    "",
		NextFetchAt:  nextFetch,
		BackoffUntil: src.BackoffUntil,
	}

	var upserts []store.NodeUpsert
	fetched := 0
	notModified := false
	fetchOK := false
	switch src.Type {
	case "github_raw_text":
		res := sources.FetchText(ctx, src.URL, s.cfg.SourceTimeout, src.ETag, src.LastModified)
		if res.ETag != "" {
			meta.ETag = res.ETag
		}
		if res.LastModified != "" {
			meta.LastModified = res.LastModified
		}
		notModified = res.NotModified
		fetchOK = res.OK
		if !res.OK {
			meta.LastError = res.Error
			meta.FetchFailInc = 1
			meta.BackoffUntil = backoffUntil(now, src.BackoffUntil)
			meta.NextFetchAt = meta.BackoffUntil
			_ = s.st.UpdateSourceFetchMeta(ctx, src.ID, meta)
			return nil
		}
		meta.FetchOKInc = 1
		meta.BackoffUntil = 0
		if res.NotModified {
			meta.NotModifiedInc = 1
			_ = s.st.UpdateSourceFetchMeta(ctx, src.ID, meta)
			return nil
		}
		cands := sources.ParseProxyText(res.Content, src.Parser, src.DefaultScheme)
		if s.cfg.IngestMaxPerSource > 0 && len(cands) > s.cfg.IngestMaxPerSource {
			rand.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })
			cands = cands[:s.cfg.IngestMaxPerSource]
		}
		fetched = len(cands)
		meta.FetchedInc = int64(fetched)
		upserts = make([]store.NodeUpsert, 0, len(cands))
		for _, c := range cands {
			fp := model.Fingerprint(model.KindProxy, c.Scheme, c.Host, itoa(c.Port), c.Username, c.Password)
			upserts = append(upserts, store.NodeUpsert{
				Kind:        model.KindProxy,
				Protocol:    c.Scheme,
				Fingerprint: fp,
				Host:        c.Host,
				Port:        c.Port,
				Username:    c.Username,
				Password:    c.Password,
				RawURI:      c.ProxyURL,
				Name:        "",
				LastSource:  src.ID,
				Country:     "",
				LatencyMS:   0,
			})
		}
	case "nodemaven_api":
		if !s.cfg.NodeMaven.Enabled {
			return nil
		}
		items, err := sources.FetchAllNodeMaven(ctx, sources.NodeMavenFetchConfig{
			BaseURL:     src.URL,
			UserAgent:   s.cfg.NodeMaven.UserAgent,
			PerPage:     s.cfg.NodeMaven.PerPage,
			MaxPages:    s.cfg.NodeMaven.MaxPages,
			Concurrency: s.cfg.NodeMaven.Concurrency,
			Timeout:     20 * time.Second,
		})
		if err != nil {
			meta.LastError = err.Error()
			meta.FetchFailInc = 1
			meta.BackoffUntil = backoffUntil(now, src.BackoffUntil)
			meta.NextFetchAt = meta.BackoffUntil
			_ = s.st.UpdateSourceFetchMeta(ctx, src.ID, meta)
			return nil
		}
		fetchOK = true
		meta.FetchOKInc = 1
		meta.BackoffUntil = 0
		fetched = len(items)
		if s.cfg.IngestMaxPerSource > 0 && fetched > s.cfg.IngestMaxPerSource {
			rand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
			items = items[:s.cfg.IngestMaxPerSource]
			fetched = len(items)
		}
		meta.FetchedInc = int64(fetched)
		upserts = make([]store.NodeUpsert, 0, len(items))
		for _, it := range items {
			proto := strings.ToLower(strings.TrimSpace(it.Protocol))
			switch proto {
			case "http", "https", "socks4", "socks5":
			default:
				proto = "http"
			}
			raw := proto + "://" + strings.TrimSpace(it.IPAddress) + ":" + itoa(it.Port)
			_, normalized, err := model.NormalizeProxyURL(raw)
			if err != nil {
				continue
			}
			fp := model.Fingerprint(model.KindProxy, proto, strings.TrimSpace(it.IPAddress), itoa(it.Port))
			upserts = append(upserts, store.NodeUpsert{
				Kind:        model.KindProxy,
				Protocol:    proto,
				Fingerprint: fp,
				Host:        strings.TrimSpace(it.IPAddress),
				Port:        it.Port,
				RawURI:      normalized,
				LastSource:  src.ID,
				Country:     it.Country,
				LatencyMS:   sources.SafeInt(it.LatencyRaw),
			})
		}
	case "sub_base64":
		res, nodes := sources.FetchAndParseSubscriptionBase64(ctx, src.URL, s.cfg.SourceTimeout, src.ETag, src.LastModified)
		if res.ETag != "" {
			meta.ETag = res.ETag
		}
		if res.LastModified != "" {
			meta.LastModified = res.LastModified
		}
		notModified = res.NotModified
		fetchOK = res.OK
		if !res.OK {
			meta.LastError = res.Error
			meta.FetchFailInc = 1
			meta.BackoffUntil = backoffUntil(now, src.BackoffUntil)
			meta.NextFetchAt = meta.BackoffUntil
			_ = s.st.UpdateSourceFetchMeta(ctx, src.ID, meta)
			return nil
		}
		meta.FetchOKInc = 1
		meta.BackoffUntil = 0
		if res.NotModified {
			meta.NotModifiedInc = 1
			_ = s.st.UpdateSourceFetchMeta(ctx, src.ID, meta)
			return nil
		}
		fetched = len(nodes)
		if s.cfg.IngestMaxPerSource > 0 && fetched > s.cfg.IngestMaxPerSource {
			rand.Shuffle(len(nodes), func(i, j int) { nodes[i], nodes[j] = nodes[j], nodes[i] })
			nodes = nodes[:s.cfg.IngestMaxPerSource]
			fetched = len(nodes)
		}
		meta.FetchedInc = int64(fetched)
		upserts = make([]store.NodeUpsert, 0, len(nodes))
		for _, n := range nodes {
			p := n.Parsed
			if p == nil {
				continue
			}
			upserts = append(upserts, store.NodeUpsert{
				Kind:        p.Kind,
				Protocol:    p.Protocol,
				Fingerprint: p.Fingerprint,
				Host:        p.Host,
				Port:        p.Port,
				Username:    p.Username,
				Password:    p.Password,
				RawURI:      p.RawURI,
				Name:        p.Name,
				LastSource:  src.ID,
				Country:     "",
				LatencyMS:   0,
			})
		}
	case "clash_yaml":
		res, nodes := sources.FetchAndParseClashYAML(ctx, src.URL, s.cfg.SourceTimeout, src.ETag, src.LastModified)
		if res.ETag != "" {
			meta.ETag = res.ETag
		}
		if res.LastModified != "" {
			meta.LastModified = res.LastModified
		}
		notModified = res.NotModified
		fetchOK = res.OK
		if !res.OK {
			meta.LastError = res.Error
			meta.FetchFailInc = 1
			meta.BackoffUntil = backoffUntil(now, src.BackoffUntil)
			meta.NextFetchAt = meta.BackoffUntil
			_ = s.st.UpdateSourceFetchMeta(ctx, src.ID, meta)
			return nil
		}
		meta.FetchOKInc = 1
		meta.BackoffUntil = 0
		if res.NotModified {
			meta.NotModifiedInc = 1
			_ = s.st.UpdateSourceFetchMeta(ctx, src.ID, meta)
			return nil
		}
		fetched = len(nodes)
		if s.cfg.IngestMaxPerSource > 0 && fetched > s.cfg.IngestMaxPerSource {
			rand.Shuffle(len(nodes), func(i, j int) { nodes[i], nodes[j] = nodes[j], nodes[i] })
			nodes = nodes[:s.cfg.IngestMaxPerSource]
			fetched = len(nodes)
		}
		meta.FetchedInc = int64(fetched)
		upserts = make([]store.NodeUpsert, 0, len(nodes))
		for _, n := range nodes {
			p := n.Parsed
			if p == nil {
				continue
			}
			upserts = append(upserts, store.NodeUpsert{
				Kind:        p.Kind,
				Protocol:    p.Protocol,
				Fingerprint: p.Fingerprint,
				Host:        p.Host,
				Port:        p.Port,
				Username:    p.Username,
				Password:    p.Password,
				RawURI:      p.RawURI,
				Name:        p.Name,
				LastSource:  src.ID,
				Country:     "",
				LatencyMS:   0,
			})
		}
	default:
		meta.LastError = "unknown source type"
		_ = s.st.UpdateSourceFetchMeta(ctx, src.ID, meta)
		return nil
	}

	if len(upserts) > 0 {
		_, _ = s.st.UpsertNodes(ctx, now, upserts)
	}

	// Update meta.
	if notModified {
		meta.NotModifiedInc = 1
	}
	if fetchOK {
		meta.FetchOKInc = 1
	} else {
		meta.FetchFailInc = 1
	}
	_ = s.st.UpdateSourceFetchMeta(ctx, src.ID, meta)

	// Sample validate (best-effort).
	sampleValidate := s.sourceSampleValidate(now)
	if len(upserts) > 0 && sampleValidate > 0 {
		sampleN := min(sampleValidate, len(upserts))
		rand.Shuffle(len(upserts), func(i, j int) { upserts[i], upserts[j] = upserts[j], upserts[i] })
		for i := 0; i < sampleN; i++ {
			u := upserts[i]
			s.val.Enqueue(validator.Task{
				Priority:    validator.PrioritySourceSample,
				NodeID:      0,
				Fingerprint: u.Fingerprint,
				Kind:        u.Kind,
				Protocol:    u.Protocol,
				Host:        u.Host,
				Port:        u.Port,
				Username:    u.Username,
				Password:    u.Password,
				RawURI:      u.RawURI,
				Name:        u.Name,
				SourceID:    src.ID,
			})
		}
	}

	return nil
}

func (s *Supervisor) cleanupLoop(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		if err := s.cleanupOnce(ctx); err != nil {
			slog.Warn("cleanup failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Supervisor) cleanupOnce(ctx context.Context) error {
	now := time.Now()
	cut := now.Add(-s.cfg.ChecksRetention).Unix()
	_, _ = s.st.PruneChecksBefore(ctx, cut)
	_, _ = s.st.PruneIPFactsBefore(ctx, now.Add(-30*24*time.Hour).Unix())
	return nil
}

func (s *Supervisor) validateLoop(ctx context.Context) error {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.ensureFreshPool(ctx); err != nil {
			slog.Warn("ensure fresh pool failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Supervisor) ensureFreshPool(ctx context.Context) error {
	now := time.Now()
	stats, err := s.st.GetStats(ctx, now, s.cfg.FreshWithinDefault)
	if err != nil {
		return err
	}
	minFreshPoolSize := s.minFreshPoolSize(now)
	if int(stats.NodesFreshOK) >= minFreshPoolSize {
		return nil
	}

	need := minFreshPoolSize - int(stats.NodesFreshOK)
	if need <= 0 {
		return nil
	}
	limit := min(max(need*3, 50), 500)
	cands, err := s.st.QueryValidationCandidates(ctx, now, store.NodeFilter{
		FreshWithin: s.cfg.FreshWithinDefault,
		Verify:      true,
	}, limit)
	if err != nil {
		return err
	}
	for _, n := range cands {
		s.val.Enqueue(validator.Task{
			Priority: validator.PriorityPoolMaintain,
			NodeID:   n.ID,
		})
	}
	return nil
}

func backoffUntil(now time.Time, prev int64) int64 {
	// 5m -> 15m -> 1h (cap)
	if prev <= now.Unix() {
		return now.Add(5 * time.Minute).Unix()
	}
	remain := time.Unix(prev, 0).Sub(now)
	switch {
	case remain < 10*time.Minute:
		return now.Add(15 * time.Minute).Unix()
	case remain < 45*time.Minute:
		return now.Add(1 * time.Hour).Unix()
	default:
		return now.Add(1 * time.Hour).Unix()
	}
}

func itoa(v int) string { return strconv.Itoa(v) }

func (s *Supervisor) inWarmup(now time.Time) bool {
	w := s.cfg.StartupWarmup.Duration
	return w > 0 && now.Sub(s.startedAt) < w
}

func (s *Supervisor) fetchTickInterval(now time.Time) time.Duration {
	if s.inWarmup(now) && s.cfg.StartupWarmup.FetchTickInterval > 0 {
		return s.cfg.StartupWarmup.FetchTickInterval
	}
	return s.cfg.FetchTickInterval
}

func (s *Supervisor) fetchMaxPerTick(now time.Time) int {
	if s.inWarmup(now) && s.cfg.StartupWarmup.FetchMaxPerTick > 0 {
		return s.cfg.StartupWarmup.FetchMaxPerTick
	}
	return s.cfg.FetchMaxPerTick
}

func (s *Supervisor) sourceWorkers(now time.Time) int {
	if s.inWarmup(now) && s.cfg.StartupWarmup.SourceWorkers > 0 {
		return s.cfg.StartupWarmup.SourceWorkers
	}
	return s.cfg.SourceWorkers
}

func (s *Supervisor) sourceSampleValidate(now time.Time) int {
	if s.inWarmup(now) && s.cfg.StartupWarmup.SourceSampleValidate > 0 {
		return s.cfg.StartupWarmup.SourceSampleValidate
	}
	return s.cfg.SourceSampleValidate
}

func (s *Supervisor) minFreshPoolSize(now time.Time) int {
	if s.inWarmup(now) && s.cfg.StartupWarmup.MinFreshPoolSize > 0 {
		return s.cfg.StartupWarmup.MinFreshPoolSize
	}
	return s.cfg.MinFreshPoolSize
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
