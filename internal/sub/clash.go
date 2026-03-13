package sub

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/qiyiyun/public_proxy_pool/internal/model"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
	"github.com/qiyiyun/public_proxy_pool/internal/v2ray"
	"gopkg.in/yaml.v3"
)

type clashConfig struct {
	MixedPort     int              `yaml:"mixed-port,omitempty"`
	AllowLan      bool             `yaml:"allow-lan"`
	Mode          string           `yaml:"mode"`
	LogLevel      string           `yaml:"log-level"`
	IPv6          bool             `yaml:"ipv6"`
	UnifiedDelay  bool             `yaml:"unified-delay,omitempty"`
	TCPConcurrent bool             `yaml:"tcp-concurrent,omitempty"`
	Profile       clashProfile     `yaml:"profile"`
	DNS           clashDNS         `yaml:"dns"`
	Proxies       []map[string]any `yaml:"proxies"`
	ProxyGroups   []clashGroup     `yaml:"proxy-groups"`
	Rules         []string         `yaml:"rules"`
}

type clashGroup struct {
	Name     string   `yaml:"name"`
	Type     string   `yaml:"type"`
	Proxies  []string `yaml:"proxies"`
	URL      string   `yaml:"url,omitempty"`
	Interval int      `yaml:"interval,omitempty"`
}

type clashProfile struct {
	StoreSelected bool `yaml:"store-selected"`
	StoreFakeIP   bool `yaml:"store-fake-ip"`
}

type clashDNS struct {
	Enable            bool                   `yaml:"enable"`
	IPv6              bool                   `yaml:"ipv6"`
	EnhancedMode      string                 `yaml:"enhanced-mode,omitempty"`
	FakeIPRange       string                 `yaml:"fake-ip-range,omitempty"`
	DefaultNameserver []string               `yaml:"default-nameserver,omitempty"`
	Nameserver        []string               `yaml:"nameserver,omitempty"`
	Fallback          []string               `yaml:"fallback,omitempty"`
	FakeIPFilter      []string               `yaml:"fake-ip-filter,omitempty"`
	FallbackFilter    clashDNSFallbackFilter `yaml:"fallback-filter,omitempty"`
}

type clashDNSFallbackFilter struct {
	GeoIP     bool     `yaml:"geoip"`
	GeoIPCode string   `yaml:"geoip-code,omitempty"`
	IPCIDR    []string `yaml:"ipcidr,omitempty"`
}

type renderedClashProxy struct {
	Name   string
	Entry  map[string]any
	Region string
}

func RenderClash(nodes []store.Node, testURL string) ([]byte, error) {
	nameUsed := map[string]int{}
	rendered := make([]renderedClashProxy, 0, len(nodes))

	for _, n := range nodes {
		entry, name := nodeToClashProxy(n)
		if entry == nil || name == "" {
			continue
		}
		name = decorateNodeName(n, name)
		name = ensureUniqueName(nameUsed, name)
		entry["name"] = name
		rendered = append(rendered, renderedClashProxy{
			Name:   name,
			Entry:  entry,
			Region: clashRegionName(n),
		})
	}

	sort.Slice(rendered, func(i, j int) bool {
		return rendered[i].Name < rendered[j].Name
	})

	proxies := make([]map[string]any, 0, len(rendered))
	names := make([]string, 0, len(rendered))
	regionNames := map[string][]string{}
	for _, item := range rendered {
		proxies = append(proxies, item.Entry)
		names = append(names, item.Name)
		regionNames[item.Region] = append(regionNames[item.Region], item.Name)
	}

	cfg := clashConfig{
		MixedPort:     7890,
		AllowLan:      false,
		Mode:          "rule",
		LogLevel:      "info",
		IPv6:          false,
		UnifiedDelay:  true,
		TCPConcurrent: true,
		Profile: clashProfile{
			StoreSelected: true,
			StoreFakeIP:   true,
		},
		DNS: clashDNS{
			Enable:            true,
			IPv6:              false,
			EnhancedMode:      "fake-ip",
			FakeIPRange:       "198.18.0.1/16",
			DefaultNameserver: []string{"223.5.5.5", "119.29.29.29"},
			Nameserver: []string{
				"https://dns.alidns.com/dns-query",
				"https://doh.pub/dns-query",
			},
			Fallback: []string{
				"https://1.1.1.1/dns-query",
				"https://dns.google/dns-query",
			},
			FakeIPFilter: []string{
				"*.lan",
				"localhost.ptlogin2.qq.com",
				"+.msftconnecttest.com",
				"+.msftncsi.com",
				"+.srv.nintendo.net",
				"+.stun.playstation.net",
				"+.xboxlive.com",
				"time.apple.com",
				"time.windows.com",
				"+.pool.ntp.org",
			},
			FallbackFilter: clashDNSFallbackFilter{
				GeoIP:     true,
				GeoIPCode: "CN",
				IPCIDR: []string{
					"240.0.0.0/4",
					"0.0.0.0/32",
				},
			},
		},
		Proxies:     proxies,
		ProxyGroups: buildClashGroups(names, regionNames, testURL),
		Rules:       buildClashRules(),
	}
	return yaml.Marshal(cfg)
}

func buildClashGroups(names []string, regionNames map[string][]string, testURL string) []clashGroup {
	groups := make([]clashGroup, 0, 10)
	selectProxies := []string{"DIRECT"}
	if len(names) > 0 {
		groups = append(groups,
			clashGroup{
				Name:     "自动选择",
				Type:     "url-test",
				Proxies:  append([]string(nil), names...),
				URL:      testURL,
				Interval: 300,
			},
			clashGroup{
				Name:     "故障转移",
				Type:     "fallback",
				Proxies:  append([]string(nil), names...),
				URL:      testURL,
				Interval: 300,
			},
		)
		selectProxies = append(selectProxies, "自动选择", "故障转移")
	}

	regionOrder := []string{
		"港澳节点",
		"台湾节点",
		"日本节点",
		"新加坡节点",
		"美国节点",
		"韩国节点",
		"欧洲节点",
		"其它地区",
	}
	for _, region := range regionOrder {
		items := append([]string(nil), regionNames[region]...)
		if len(items) == 0 {
			continue
		}
		sort.Strings(items)
		groups = append(groups, clashGroup{
			Name:    region,
			Type:    "select",
			Proxies: items,
		})
		selectProxies = append(selectProxies, region)
	}

	groups = append(groups, clashGroup{
		Name:    "节点选择",
		Type:    "select",
		Proxies: selectProxies,
	})
	return groups
}

func buildClashRules() []string {
	return []string{
		"DOMAIN-SUFFIX,local,DIRECT",
		"DOMAIN-SUFFIX,lan,DIRECT",
		"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,172.16.0.0/12,DIRECT,no-resolve",
		"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
		"IP-CIDR,224.0.0.0/4,DIRECT,no-resolve",
		"IP-CIDR,240.0.0.0/4,DIRECT,no-resolve",
		"IP-CIDR6,::1/128,DIRECT,no-resolve",
		"IP-CIDR6,fc00::/7,DIRECT,no-resolve",
		"IP-CIDR6,fe80::/10,DIRECT,no-resolve",
		"DOMAIN-SUFFIX,cn,DIRECT",
		"GEOIP,CN,DIRECT",
		"MATCH,节点选择",
	}
}

func clashRegionName(n store.Node) string {
	switch strings.ToUpper(strings.TrimSpace(n.Country)) {
	case "HK", "MO":
		return "港澳节点"
	case "TW":
		return "台湾节点"
	case "JP":
		return "日本节点"
	case "SG":
		return "新加坡节点"
	case "US", "CA":
		return "美国节点"
	case "KR":
		return "韩国节点"
	case "GB", "DE", "NL", "FR", "SE", "CH", "IT", "ES", "PL", "NO", "FI", "IE", "AT", "BE", "DK", "CZ", "PT", "HU", "RO", "LU":
		return "欧洲节点"
	default:
		return "其它地区"
	}
}

func nodeToClashProxy(n store.Node) (map[string]any, string) {
	switch n.Kind {
	case model.KindProxy:
		return proxyNodeToClash(n)
	case model.KindV2Ray:
		return v2rayNodeToClash(n)
	default:
		return nil, ""
	}
}

func proxyNodeToClash(n store.Node) (map[string]any, string) {
	proto := strings.ToLower(strings.TrimSpace(n.Protocol))
	name := n.Name
	if name == "" {
		name = fmt.Sprintf("%s-%s:%d", proto, n.Host, n.Port)
	}
	switch proto {
	case "http", "https":
		m := map[string]any{
			"type":   "http",
			"server": n.Host,
			"port":   n.Port,
		}
		if n.Username != "" {
			m["username"] = n.Username
			m["password"] = n.Password
		}
		if proto == "https" {
			m["tls"] = true
		}
		return m, name
	case "socks5", "socks5h":
		m := map[string]any{
			"type":   "socks5",
			"server": n.Host,
			"port":   n.Port,
		}
		if n.Username != "" {
			m["username"] = n.Username
			m["password"] = n.Password
		}
		return m, name
	default:
		// Clash typically doesn't support SOCKS4; omit.
		return nil, ""
	}
}

func v2rayNodeToClash(n store.Node) (map[string]any, string) {
	p, err := v2ray.ParseURI(n.RawURI)
	if err != nil || p == nil {
		return nil, ""
	}
	name := p.Name
	if name == "" {
		name = fmt.Sprintf("%s-%s:%d", p.Protocol, p.Host, p.Port)
	}

	switch p.Protocol {
	case "ss":
		if p.Method == "" || p.Password == "" {
			return nil, ""
		}
		return map[string]any{
			"type":     "ss",
			"server":   p.Host,
			"port":     p.Port,
			"cipher":   p.Method,
			"password": p.Password,
			"udp":      true,
		}, name
	case "vmess":
		m := map[string]any{
			"type":    "vmess",
			"server":  p.Host,
			"port":    p.Port,
			"uuid":    p.UUID,
			"alterId": p.AlterID,
			"cipher":  "auto",
			"udp":     true,
		}
		if strings.ToLower(p.Security) == "tls" {
			m["tls"] = true
			m["skip-cert-verify"] = true
		}
		if p.SNI != "" {
			m["servername"] = p.SNI
		}
		if strings.ToLower(p.Transport) == "ws" {
			m["network"] = "ws"
			wsOpts := map[string]any{}
			if p.Path != "" {
				wsOpts["path"] = p.Path
			}
			if p.HostHdr != "" {
				wsOpts["headers"] = map[string]any{"Host": p.HostHdr}
			}
			if len(wsOpts) > 0 {
				m["ws-opts"] = wsOpts
			}
		}
		return m, name
	case "vless":
		m := map[string]any{
			"type":   "vless",
			"server": p.Host,
			"port":   p.Port,
			"uuid":   p.UUID,
			"udp":    true,
		}
		if strings.ToLower(p.Security) == "tls" {
			m["tls"] = true
			m["skip-cert-verify"] = true
		}
		if p.SNI != "" {
			m["servername"] = p.SNI
		}
		if strings.ToLower(p.Transport) == "ws" {
			m["network"] = "ws"
			wsOpts := map[string]any{}
			if p.Path != "" {
				wsOpts["path"] = p.Path
			}
			if p.HostHdr != "" {
				wsOpts["headers"] = map[string]any{"Host": p.HostHdr}
			}
			if len(wsOpts) > 0 {
				m["ws-opts"] = wsOpts
			}
		}
		return m, name
	case "trojan":
		m := map[string]any{
			"type":     "trojan",
			"server":   p.Host,
			"port":     p.Port,
			"password": p.Password,
			"udp":      true,
		}
		m["tls"] = true
		m["skip-cert-verify"] = true
		if p.SNI != "" {
			m["sni"] = p.SNI
		}
		if strings.ToLower(p.Transport) == "ws" {
			m["network"] = "ws"
			wsOpts := map[string]any{}
			if p.Path != "" {
				wsOpts["path"] = p.Path
			}
			if p.HostHdr != "" {
				wsOpts["headers"] = map[string]any{"Host": p.HostHdr}
			}
			if len(wsOpts) > 0 {
				m["ws-opts"] = wsOpts
			}
		}
		return m, name
	default:
		return nil, ""
	}
}

func ensureUniqueName(used map[string]int, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "node"
	}
	if used[name] == 0 {
		used[name] = 1
		return name
	}
	used[name]++
	return name + "-" + strconv.Itoa(used[name])
}

func decorateNodeName(n store.Node, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	flag, countryName := countryFlagAndName(n.Country)
	if flag == "" && countryName == "" {
		return name
	}
	if flag != "" && strings.Contains(name, flag) {
		return name
	}
	if countryName != "" && strings.Contains(name, countryName) {
		return name
	}

	prefix := strings.TrimSpace(strings.TrimSpace(flag + " " + countryName))
	if prefix == "" {
		return name
	}
	return prefix + " | " + name
}

func countryFlagAndName(code string) (string, string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 2 {
		return "", ""
	}

	flag := countryCodeToFlag(code)
	nameMap := map[string]string{
		"HK": "香港",
		"MO": "澳门",
		"TW": "台湾",
		"JP": "日本",
		"SG": "新加坡",
		"US": "美国",
		"CA": "加拿大",
		"KR": "韩国",
		"GB": "英国",
		"DE": "德国",
		"NL": "荷兰",
		"FR": "法国",
		"CH": "瑞士",
		"IT": "意大利",
		"ES": "西班牙",
		"SE": "瑞典",
		"NO": "挪威",
		"FI": "芬兰",
		"DK": "丹麦",
		"IE": "爱尔兰",
		"AT": "奥地利",
		"BE": "比利时",
		"PL": "波兰",
		"CZ": "捷克",
		"PT": "葡萄牙",
		"RO": "罗马尼亚",
		"LU": "卢森堡",
		"AU": "澳大利亚",
		"NZ": "新西兰",
		"RU": "俄罗斯",
		"IN": "印度",
		"MY": "马来西亚",
		"TH": "泰国",
		"VN": "越南",
		"PH": "菲律宾",
		"ID": "印度尼西亚",
		"TR": "土耳其",
		"AE": "阿联酋",
		"BR": "巴西",
		"AR": "阿根廷",
		"CL": "智利",
		"MX": "墨西哥",
		"ZA": "南非",
	}
	if name, ok := nameMap[code]; ok {
		return flag, name
	}
	return flag, code
}

func countryCodeToFlag(code string) string {
	if len(code) != 2 {
		return ""
	}
	code = strings.ToUpper(code)
	runes := make([]rune, 0, 2)
	for _, ch := range code {
		if ch < 'A' || ch > 'Z' {
			return ""
		}
		runes = append(runes, rune(0x1F1E6+(ch-'A')))
	}
	return string(runes)
}
