package core

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/qiyiyun/public_proxy_pool/internal/config"
	"github.com/qiyiyun/public_proxy_pool/internal/sources"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
)

func SeedDefaultSources(st *store.Store, cfg *config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	count, err := st.SourceCount(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	wd, _ := os.Getwd()
	defs, err := sources.LoadGitHubSourcesFromServicePy(wd)
	if err != nil {
		slog.Warn("load default github sources from service.py failed; falling back to minimal set", "error", err)
		defs = minimalGitHubSources()
	}

	for _, d := range defs {
		_, err := st.UpsertSource(ctx, store.Source{
			Name:          d.Name,
			Type:          d.Type,
			URL:           d.URL,
			Parser:        d.Parser,
			DefaultScheme: d.DefaultScheme,
			RepoURL:       d.RepoURL,
			UpdateHint:    d.UpdateHint,
			Enabled:       d.Enabled,
			IntervalSec:   d.IntervalSec,
			NextFetchAt:   time.Now().Unix(),
		})
		if err != nil {
			return err
		}
	}

	if cfg.NodeMaven.Enabled {
		_, err := st.UpsertSource(ctx, store.Source{
			Name:        "nodemaven",
			Type:        "nodemaven_api",
			URL:         cfg.NodeMaven.BaseURL,
			Parser:      "nodemaven",
			RepoURL:     cfg.NodeMaven.BaseURL,
			UpdateHint:  "NodeMaven public proxy list API",
			Enabled:     true,
			IntervalSec: 1800,
			NextFetchAt: time.Now().Unix(),
		})
		if err != nil {
			return err
		}
	}

	slog.Info("seeded sources", "count", len(defs))
	return nil
}

func minimalGitHubSources() []sources.SourceDef {
	return []sources.SourceDef{
		{
			Name:          "topchina-readme",
			Type:          "github_raw_text",
			URL:           "https://raw.githubusercontent.com/TopChina/proxy-list/main/README.md",
			Parser:        "topchina",
			DefaultScheme: "http",
			RepoURL:       "https://github.com/TopChina/proxy-list",
			UpdateHint:    "README 表格维护，持续更新",
			Enabled:       true,
			IntervalSec:   3600,
		},
		{
			Name:          "proxyscraper-http",
			Type:          "github_raw_text",
			URL:           "https://raw.githubusercontent.com/ProxyScraper/ProxyScraper/main/http.txt",
			Parser:        "generic",
			DefaultScheme: "http",
			RepoURL:       "https://github.com/ProxyScraper/ProxyScraper",
			UpdateHint:    "公开说明约每 30 分钟更新",
			Enabled:       true,
			IntervalSec:   3600,
		},
	}
}

