package auth

import (
	"net/http"
	"strings"

	"github.com/qiyiyun/public_proxy_pool/internal/config"
)

func Check(r *http.Request, cfg *config.Config) bool {
	if cfg == nil || len(cfg.APIKeys) == 0 {
		return true
	}
	token := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if token == "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			token = strings.TrimSpace(auth[7:])
		}
	}
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if token == "" {
		return false
	}
	for _, k := range cfg.APIKeys {
		if token == k {
			return true
		}
	}
	return false
}

