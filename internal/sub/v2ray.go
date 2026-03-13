package sub

import (
	"encoding/base64"
	"strings"

	"github.com/qiyiyun/public_proxy_pool/internal/store"
)

func RenderV2Ray(nodes []store.Node) string {
	plain := RenderPlain(nodes, "uri")
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(plain))
}

