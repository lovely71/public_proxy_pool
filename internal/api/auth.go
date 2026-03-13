package api

import (
	"net/http"

	"github.com/qiyiyun/public_proxy_pool/internal/auth"
	"github.com/qiyiyun/public_proxy_pool/internal/config"
)

func checkAPIKey(r *http.Request, cfg *config.Config) bool {
	return auth.Check(r, cfg)
}
