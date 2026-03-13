package v2ray

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/qiyiyun/public_proxy_pool/internal/model"
)

type Parsed struct {
	Kind        string
	Protocol    string
	Fingerprint string
	Host        string
	Port        int
	Username    string
	Password    string
	Name        string
	RawURI      string

	// V2Ray specific (best-effort, for rendering/validation plugins).
	Transport string // tcp/ws/grpc/quic (etc)
	Security  string // tls/reality/none
	SNI       string
	HostHdr   string
	Path      string
	UUID      string
	Method    string
	AlterID   int
}

func ParseURI(raw string) (*Parsed, error) {
	item := strings.TrimSpace(raw)
	item = strings.Trim(item, "\uFEFF")
	if item == "" {
		return nil, fmt.Errorf("empty")
	}

	if strings.HasPrefix(item, "http://") || strings.HasPrefix(item, "https://") ||
		strings.HasPrefix(item, "socks4://") || strings.HasPrefix(item, "socks5://") || strings.HasPrefix(item, "socks5h://") {
		_, normalized, err := model.NormalizeProxyURL(item)
		if err != nil {
			return nil, err
		}
		u, _, _ := model.NormalizeProxyURL(normalized)
		p := &Parsed{
			Kind:     model.KindProxy,
			Protocol: strings.ToLower(u.Scheme),
			Host:     u.Hostname(),
			RawURI:   normalized,
		}
		if portStr := u.Port(); portStr != "" {
			p.Port, _ = strconv.Atoi(portStr)
		}
		if u.User != nil {
			p.Username = u.User.Username()
			if pw, ok := u.User.Password(); ok {
				p.Password = pw
			}
		}
		p.Fingerprint = model.Fingerprint(p.Kind, p.Protocol, p.Host, strconv.Itoa(p.Port), p.Username, p.Password)
		return p, nil
	}

	switch {
	case strings.HasPrefix(item, "ss://"):
		return parseSS(item)
	case strings.HasPrefix(item, "vmess://"):
		return parseVMess(item)
	case strings.HasPrefix(item, "vless://"):
		return parseVLess(item)
	case strings.HasPrefix(item, "trojan://"):
		return parseTrojan(item)
	default:
		return nil, fmt.Errorf("unsupported uri")
	}
}

func parseSS(raw string) (*Parsed, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	out := &Parsed{
		Kind:     model.KindV2Ray,
		Protocol: "ss",
		RawURI:   strings.TrimSpace(raw),
		Name:     strings.TrimSpace(u.Fragment),
	}

	// Variant A: ss://BASE64@host:port
	if u.Host != "" && u.User != nil {
		out.Host = u.Hostname()
		if ps := u.Port(); ps != "" {
			out.Port, _ = strconv.Atoi(ps)
		}
		decoded, err := decodeBase64Auto(u.User.Username())
		if err != nil {
			return nil, fmt.Errorf("ss userinfo base64: %w", err)
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("ss creds invalid")
		}
		out.Method = parts[0]
		out.Password = parts[1]
		out.Fingerprint = model.Fingerprint(out.Kind, out.Protocol, out.Host, strconv.Itoa(out.Port), out.Method, out.Password)
		return out, nil
	}

	// Variant B: ss://BASE64(method:pass@host:port)#name
	opaque := u.Opaque
	if opaque == "" {
		opaque = strings.TrimPrefix(raw, "ss://")
		if i := strings.IndexByte(opaque, '#'); i >= 0 {
			opaque = opaque[:i]
		}
		if i := strings.IndexByte(opaque, '?'); i >= 0 {
			opaque = opaque[:i]
		}
	}
	decoded, err := decodeBase64Auto(opaque)
	if err != nil {
		return nil, fmt.Errorf("ss opaque base64: %w", err)
	}
	// decoded: method:pass@host:port
	at := bytes.LastIndexByte(decoded, '@')
	if at < 0 {
		return nil, fmt.Errorf("ss missing @")
	}
	creds := string(decoded[:at])
	hostport := string(decoded[at+1:])
	hpParts := strings.Split(hostport, ":")
	if len(hpParts) != 2 {
		return nil, fmt.Errorf("ss hostport invalid")
	}
	out.Host = hpParts[0]
	out.Port, _ = strconv.Atoi(hpParts[1])
	credParts := strings.SplitN(creds, ":", 2)
	if len(credParts) != 2 {
		return nil, fmt.Errorf("ss creds invalid")
	}
	out.Method = credParts[0]
	out.Password = credParts[1]
	out.Fingerprint = model.Fingerprint(out.Kind, out.Protocol, out.Host, strconv.Itoa(out.Port), out.Method, out.Password)
	return out, nil
}

type vmessJSON struct {
	V   string `json:"v"`
	PS  string `json:"ps"`
	Add string `json:"add"`
	Port string `json:"port"`
	ID  string `json:"id"`
	AID string `json:"aid"`
	Net string `json:"net"`
	Type string `json:"type"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
}

func parseVMess(raw string) (*Parsed, error) {
	payload := strings.TrimPrefix(strings.TrimSpace(raw), "vmess://")
	decoded, err := decodeBase64Auto(payload)
	if err != nil {
		return nil, err
	}
	var obj vmessJSON
	if err := json.Unmarshal(decoded, &obj); err != nil {
		return nil, err
	}
	host := strings.TrimSpace(obj.Add)
	port, _ := strconv.Atoi(strings.TrimSpace(obj.Port))
	aid, _ := strconv.Atoi(strings.TrimSpace(obj.AID))
	name := strings.TrimSpace(obj.PS)

	out := &Parsed{
		Kind:      model.KindV2Ray,
		Protocol:  "vmess",
		RawURI:    strings.TrimSpace(raw),
		Host:      host,
		Port:      port,
		UUID:      strings.TrimSpace(obj.ID),
		AlterID:   aid,
		Transport: strings.TrimSpace(obj.Net),
		Security:  strings.TrimSpace(obj.TLS),
		HostHdr:   strings.TrimSpace(obj.Host),
		Path:      strings.TrimSpace(obj.Path),
		SNI:       strings.TrimSpace(obj.SNI),
		Name:      name,
	}
	out.Fingerprint = model.Fingerprint(out.Kind, out.Protocol, out.Host, strconv.Itoa(out.Port), out.UUID, out.Transport, out.Security, out.Path, out.HostHdr, out.SNI, strconv.Itoa(out.AlterID))
	return out, nil
}

func parseVLess(raw string) (*Parsed, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	host := u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	uuid := ""
	if u.User != nil {
		uuid = u.User.Username()
	}
	q := u.Query()
	transport := strings.TrimSpace(q.Get("type"))
	security := strings.TrimSpace(q.Get("security"))
	path := strings.TrimSpace(q.Get("path"))
	hostHdr := strings.TrimSpace(q.Get("host"))
	sni := strings.TrimSpace(q.Get("sni"))
	if sni == "" {
		sni = strings.TrimSpace(q.Get("peer"))
	}
	name := strings.TrimSpace(u.Fragment)

	out := &Parsed{
		Kind:      model.KindV2Ray,
		Protocol:  "vless",
		RawURI:    strings.TrimSpace(raw),
		Host:      host,
		Port:      port,
		UUID:      uuid,
		Transport: transport,
		Security:  security,
		Path:      path,
		HostHdr:   hostHdr,
		SNI:       sni,
		Name:      name,
	}
	out.Fingerprint = model.Fingerprint(out.Kind, out.Protocol, out.Host, strconv.Itoa(out.Port), out.UUID, out.Transport, out.Security, out.Path, out.HostHdr, out.SNI)
	return out, nil
}

func parseTrojan(raw string) (*Parsed, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	host := u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	password := ""
	if u.User != nil {
		password = u.User.Username()
	}
	q := u.Query()
	transport := strings.TrimSpace(q.Get("type"))
	security := strings.TrimSpace(q.Get("security"))
	path := strings.TrimSpace(q.Get("path"))
	hostHdr := strings.TrimSpace(q.Get("host"))
	sni := strings.TrimSpace(q.Get("sni"))
	name := strings.TrimSpace(u.Fragment)
	out := &Parsed{
		Kind:      model.KindV2Ray,
		Protocol:  "trojan",
		RawURI:    strings.TrimSpace(raw),
		Host:      host,
		Port:      port,
		Password:  password,
		Transport: transport,
		Security:  security,
		Path:      path,
		HostHdr:   hostHdr,
		SNI:       sni,
		Name:      name,
	}
	out.Fingerprint = model.Fingerprint(out.Kind, out.Protocol, out.Host, strconv.Itoa(out.Port), out.Password, out.Transport, out.Security, out.Path, out.HostHdr, out.SNI)
	return out, nil
}

func decodeBase64Auto(raw string) ([]byte, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimRight(s, "=")
	if s == "" {
		return nil, fmt.Errorf("empty base64")
	}

	// Try standard (with padding added back).
	padded := s
	for len(padded)%4 != 0 {
		padded += "="
	}
	if b, err := base64.StdEncoding.DecodeString(padded); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	// URL-safe variants.
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(padded); err == nil {
		return b, nil
	}
	return nil, fmt.Errorf("invalid base64")
}

