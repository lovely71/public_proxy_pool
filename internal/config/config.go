package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr      string
	PublicBaseURL string

	SQLitePath         string
	SQLiteMaxOpenConns int
	SQLiteBusyTimeout  time.Duration

	GeoIPCountryMMDB string
	GeoIPASNMMDB     string

	APIKeys           []string
	RateLimitRPS      float64
	RateLimitBurst    int
	StatsQueryTimeout time.Duration

	AutoFetchEnabled    bool
	AutoValidateEnabled bool

	FetchTickInterval  time.Duration
	FetchMaxPerTick    int
	IngestMaxPerSource int
	SourceTimeout      time.Duration
	SourceWorkers      int

	ValidateWorkers      int
	ValidateTimeout      time.Duration
	ValidateURL          string
	ValidateKeyword      string
	ValidateHTTPURL      string
	ProbeEchoURL         string
	FreshWithinDefault   time.Duration
	SyncVerifyTimeout    time.Duration
	SourceSampleValidate int
	MinFreshPoolSize     int

	TCPCheckTimeout   time.Duration
	TCPCheckRetries   int
	TCPCheckCacheTTL  time.Duration
	TCPCheckWorkers   int
	PurityLookup      PurityLookupConfig
	NodeMaven         NodeMavenConfig
	StartupWarmup     StartupWarmupConfig
	V2RayValidateMode string // "tcp" | "sing-box" (best-effort)
	SingBoxPath       string

	ChecksRetention time.Duration
	CleanupInterval time.Duration
}

type NodeMavenConfig struct {
	Enabled     bool
	BaseURL     string
	UserAgent   string
	PerPage     int
	MaxPages    int
	Concurrency int
}

type PurityLookupConfig struct {
	Enabled  bool
	URL      string
	Timeout  time.Duration
	CacheTTL time.Duration
}

type StartupWarmupConfig struct {
	Duration             time.Duration
	FetchTickInterval    time.Duration
	FetchMaxPerTick      int
	SourceWorkers        int
	ValidateWorkers      int
	SourceSampleValidate int
	MinFreshPoolSize     int
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:           envString("HTTP_ADDR", ":8080"),
		PublicBaseURL:      strings.TrimRight(envString("PUBLIC_BASE_URL", ""), "/"),
		SQLitePath:         envString("SQLITE_PATH", "./data/proxypool.db"),
		SQLiteMaxOpenConns: envInt("SQLITE_MAX_OPEN_CONNS", 4),
		SQLiteBusyTimeout:  envDuration("SQLITE_BUSY_TIMEOUT", 15*time.Second),
		GeoIPCountryMMDB:   envString("GEOIP_COUNTRY_MMDB", ""),
		GeoIPASNMMDB:       envString("GEOIP_ASN_MMDB", ""),
		APIKeys:            splitCSV(envString("API_KEYS", "")),
		RateLimitRPS:       envFloat("RATE_LIMIT_RPS", 0),
		RateLimitBurst:     envInt("RATE_LIMIT_BURST", 0),
		StatsQueryTimeout:  envDuration("STATS_QUERY_TIMEOUT", 3*time.Second),

		AutoFetchEnabled:    envBool("AUTO_FETCH_ENABLED", true),
		AutoValidateEnabled: envBool("AUTO_VALIDATE_ENABLED", true),

		FetchTickInterval:  envDuration("FETCH_TICK_INTERVAL", 30*time.Second),
		FetchMaxPerTick:    envInt("FETCH_MAX_PER_TICK", 10),
		IngestMaxPerSource: envInt("INGEST_MAX_PER_SOURCE", 5000),
		SourceTimeout:      envDuration("SOURCE_TIMEOUT", 18*time.Second),
		SourceWorkers:      envInt("SOURCE_WORKERS", 12),

		ValidateWorkers:      envInt("VALIDATE_WORKERS", 50),
		ValidateTimeout:      envDuration("VALIDATE_TIMEOUT", 8*time.Second),
		ValidateURL:          envString("VALIDATE_URL", "https://www.cloudflare.com/cdn-cgi/trace"),
		ValidateKeyword:      envString("VALIDATE_KEYWORD", "loc="),
		ValidateHTTPURL:      envString("VALIDATE_HTTP_URL", "http://ip-api.com/json/?fields=status,query,countryCode"),
		FreshWithinDefault:   envDuration("FRESH_WITHIN_DEFAULT", 5*time.Minute),
		SyncVerifyTimeout:    envDuration("SYNC_VERIFY_TIMEOUT", 2*time.Second),
		SourceSampleValidate: envInt("SOURCE_SAMPLE_VALIDATE", 30),
		MinFreshPoolSize:     envInt("MIN_FRESH_POOL_SIZE", 200),

		TCPCheckTimeout:  envDuration("TCP_CHECK_TIMEOUT", 1500*time.Millisecond),
		TCPCheckRetries:  envInt("TCP_CHECK_RETRIES", 3),
		TCPCheckCacheTTL: envDuration("TCP_CHECK_CACHE_TTL", 10*time.Minute),
		TCPCheckWorkers:  envInt("TCP_CHECK_WORKERS", 100),

		V2RayValidateMode: envString("V2RAY_VALIDATE_MODE", "tcp"),
		SingBoxPath:       envString("SING_BOX_PATH", "sing-box"),

		ChecksRetention: envDuration("CHECKS_RETENTION", 30*24*time.Hour),
		CleanupInterval: envDuration("CLEANUP_INTERVAL", 6*time.Hour),
	}

	if cfg.PublicBaseURL != "" {
		cfg.ProbeEchoURL = cfg.PublicBaseURL + "/probe/echo"
	}

	cfg.PurityLookup = PurityLookupConfig{
		Enabled:  envBool("PURITY_LOOKUP_ENABLED", true),
		URL:      envString("PURITY_LOOKUP_URL", "http://ip-api.com/batch?fields=status,message,query,countryCode,proxy,hosting,mobile"),
		Timeout:  envDuration("PURITY_LOOKUP_TIMEOUT", 5*time.Second),
		CacheTTL: envDuration("PURITY_LOOKUP_CACHE_TTL", 24*time.Hour),
	}

	cfg.NodeMaven = NodeMavenConfig{
		Enabled:     envBool("NODEMAVEN_ENABLED", true),
		BaseURL:     envString("NODEMAVEN_BASE", "https://nodemaven.com"),
		UserAgent:   envString("NODEMAVEN_UA", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"),
		PerPage:     envInt("NODEMAVEN_PER_PAGE", 100),
		MaxPages:    envInt("NODEMAVEN_MAX_PAGES", 5),
		Concurrency: envInt("NODEMAVEN_CONCURRENCY", 5),
	}

	cfg.StartupWarmup = StartupWarmupConfig{
		Duration:             envDuration("STARTUP_WARMUP_DURATION", 0),
		FetchTickInterval:    envDuration("STARTUP_WARMUP_FETCH_TICK_INTERVAL", 0),
		FetchMaxPerTick:      envInt("STARTUP_WARMUP_FETCH_MAX_PER_TICK", 0),
		SourceWorkers:        envInt("STARTUP_WARMUP_SOURCE_WORKERS", 0),
		ValidateWorkers:      envInt("STARTUP_WARMUP_VALIDATE_WORKERS", 0),
		SourceSampleValidate: envInt("STARTUP_WARMUP_SOURCE_SAMPLE_VALIDATE", 0),
		MinFreshPoolSize:     envInt("STARTUP_WARMUP_MIN_FRESH_POOL_SIZE", 0),
	}

	return cfg, nil
}

func envString(name, def string) string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	return raw
}

func envInt(name string, def int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

func envBool(name string, def bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
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

func envDuration(name string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return def
	}
	return d
}

func envFloat(name string, def float64) float64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return v
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
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
