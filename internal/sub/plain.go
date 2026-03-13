package sub

import (
	"fmt"
	"strings"

	"github.com/qiyiyun/public_proxy_pool/internal/model"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
)

func RenderPlain(nodes []store.Node, format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	var sb strings.Builder
	for _, n := range nodes {
		line := ""
		switch format {
		case "hostport":
			if n.Kind == model.KindProxy {
				line = fmt.Sprintf("%s:%d", n.Host, n.Port)
			} else {
				line = n.RawURI
			}
		default:
			line = n.RawURI
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String()
}

