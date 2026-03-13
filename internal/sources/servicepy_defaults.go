package sources

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadGitHubSourcesFromServicePy parses the reference Python file:
// 参考代理池抓取/proxy_pool/service.py
// and extracts DEFAULT_GITHUB_PROXY_SOURCES.
//
// It is intentionally "best-effort": if parsing fails, caller can fall back.
func LoadGitHubSourcesFromServicePy(repoRoot string) ([]SourceDef, error) {
	path := filepath.Join(repoRoot, "参考代理池抓取", "proxy_pool", "service.py")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(b)
	idx := strings.Index(text, "DEFAULT_GITHUB_PROXY_SOURCES")
	if idx < 0 {
		return nil, fmt.Errorf("DEFAULT_GITHUB_PROXY_SOURCES not found")
	}
	text = text[idx:]

	var out []SourceDef
	for {
		i := strings.Index(text, "ProxySource(")
		if i < 0 {
			break
		}
		text = text[i+len("ProxySource("):]
		block, rest, ok := readBalancedParenBlock(text)
		if !ok {
			break
		}
		text = rest

		src := SourceDef{
			Type:          "github_raw_text",
			Parser:        findPyString(block, "parser"),
			DefaultScheme: findPyString(block, "default_scheme"),
			RepoURL:       findPyString(block, "repo_url"),
			UpdateHint:    findPyString(block, "update_hint"),
			Enabled:       true,
			IntervalSec:   3600,
		}
		src.Name = findPyString(block, "name")
		src.URL = findPyString(block, "url")
		if src.Name == "" || src.URL == "" {
			continue
		}
		if src.Parser == "" {
			src.Parser = "generic"
		}
		if src.DefaultScheme == "" {
			src.DefaultScheme = "http"
		}
		enabledRaw := findPyBool(block, "enabled")
		if enabledRaw != nil {
			src.Enabled = *enabledRaw
		}
		out = append(out, src)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no ProxySource parsed from service.py")
	}
	return out, nil
}

// readBalancedParenBlock reads until the matching ')' for the initial '(' that
// was removed. It is quote-aware for Python double quotes.
func readBalancedParenBlock(s string) (block string, rest string, ok bool) {
	depth := 1
	inString := false
	escape := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[:i], s[i+1:], true
			}
		}
	}
	return "", s, false
}

func findPyString(block string, key string) string {
	needle := key + "=\""
	i := strings.Index(block, needle)
	if i < 0 {
		return ""
	}
	rest := block[i+len(needle):]
	var sb strings.Builder
	escape := false
	for j := 0; j < len(rest); j++ {
		ch := rest[j]
		if escape {
			sb.WriteByte(ch)
			escape = false
			continue
		}
		if ch == '\\' {
			escape = true
			continue
		}
		if ch == '"' {
			return sb.String()
		}
		sb.WriteByte(ch)
	}
	return ""
}

func findPyBool(block string, key string) *bool {
	needle := key + "="
	i := strings.Index(block, needle)
	if i < 0 {
		return nil
	}
	rest := strings.TrimSpace(block[i+len(needle):])
	if strings.HasPrefix(rest, "True") {
		v := true
		return &v
	}
	if strings.HasPrefix(rest, "False") {
		v := false
		return &v
	}
	return nil
}

