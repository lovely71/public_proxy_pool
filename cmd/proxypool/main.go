package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qiyiyun/public_proxy_pool/internal/api"
	"github.com/qiyiyun/public_proxy_pool/internal/config"
	"github.com/qiyiyun/public_proxy_pool/internal/core"
	"github.com/qiyiyun/public_proxy_pool/internal/geoip"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
	"github.com/qiyiyun/public_proxy_pool/internal/ui"
	"github.com/qiyiyun/public_proxy_pool/internal/validator"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}

	st, err := store.Open(cfg.SQLitePath)
	if err != nil {
		logger.Error("open sqlite failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := st.Migrate(); err != nil {
		logger.Error("migrate failed", "error", err)
		os.Exit(1)
	}

	if err := core.SeedDefaultSources(st, cfg); err != nil {
		logger.Error("seed default sources failed", "error", err)
		os.Exit(1)
	}

	geo, err := geoip.Open(cfg.GeoIPCountryMMDB, cfg.GeoIPASNMMDB)
	if err != nil {
		logger.Error("open geoip mmdb failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = geo.Close() }()

	v := validator.New(st, cfg, geo)
	s := core.NewSupervisor(st, v, cfg)

	uiHandler := ui.NewHandler(st, v, cfg)
	apiHandler := api.NewHandler(st, v, cfg)

	root := chi.NewRouter()
	root.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/overview", http.StatusFound)
	})
	root.Mount("/ui", uiHandler.Routes())
	root.Mount("/", apiHandler.Routes())

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           root,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	if cfg.AutoFetchEnabled || cfg.AutoValidateEnabled {
		go func() {
			if err := s.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("supervisor stopped", "error", err)
			}
		}()
	}

	logger.Info("server listening", "addr", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("http server failed", "error", err)
		os.Exit(1)
	}
}
