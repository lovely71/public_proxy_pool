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

	wd, _ := os.Getwd()
	defs, err := sources.LoadGitHubSourcesFromServicePy(wd)
	if err != nil {
		slog.Warn("load default github sources from service.py failed; falling back to built-in source set", "error", err)
		defs = sources.BuiltInGitHubSources()
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
