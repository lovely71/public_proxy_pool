package core

import (
	"testing"

	"github.com/qiyiyun/public_proxy_pool/internal/config"
	"github.com/qiyiyun/public_proxy_pool/internal/sources"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
)

func TestSeedDefaultSourcesUpsertsMissingDefaultsWithoutOverwritingLocalSettings(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	topchina := store.Source{
		Name:          "topchina-readme",
		Type:          "github_raw_text",
		URL:           "https://example.com/old.txt",
		Parser:        "generic",
		DefaultScheme: "http",
		RepoURL:       "https://example.com/repo",
		UpdateHint:    "old",
		Enabled:       false,
		IntervalSec:   123,
		NextFetchAt:   1,
	}
	if _, err := st.UpsertSource(t.Context(), topchina); err != nil {
		t.Fatalf("seed old source: %v", err)
	}

	cfg := &config.Config{}
	if err := SeedDefaultSources(st, cfg); err != nil {
		t.Fatalf("seed defaults: %v", err)
	}

	gotCount, err := st.SourceCount(t.Context())
	if err != nil {
		t.Fatalf("source count: %v", err)
	}
	wantCount := len(sources.BuiltInGitHubSources())
	if gotCount != wantCount {
		t.Fatalf("expected %d sources after sync, got %d", wantCount, gotCount)
	}

	items, err := st.ListSources(t.Context())
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	var found store.Source
	var ok bool
	for _, item := range items {
		if item.Name == "topchina-readme" {
			found = item
			ok = true
			break
		}
	}
	if !ok {
		t.Fatalf("expected topchina-readme to exist")
	}
	if found.Enabled {
		t.Fatalf("expected existing enabled flag to be preserved")
	}
	if found.IntervalSec != 123 {
		t.Fatalf("expected existing interval to be preserved, got %d", found.IntervalSec)
	}
	if found.URL != "https://raw.githubusercontent.com/TopChina/proxy-list/main/README.md" {
		t.Fatalf("expected metadata to refresh from built-in default, got %q", found.URL)
	}
}
