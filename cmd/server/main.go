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

	"launchdarkly/internal/api"
	"launchdarkly/internal/config"
	"launchdarkly/internal/db"
	flagstore "launchdarkly/internal/store"
	"launchdarkly/internal/sync"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var repo api.FlagRepository
	if cfg.DatabaseURL != "" {
		database, err := db.OpenPostgres(ctx, cfg.DatabaseURL)
		if err != nil {
			slog.Error("failed to connect to database", "error", err)
			os.Exit(1)
		}
		defer database.Close()

		if err := db.RunMigrations(ctx, database); err != nil {
			slog.Error("failed to run database migrations", "error", err)
			os.Exit(1)
		}

		repo = db.NewRepository(database)
	}

	flagStore := flagstore.NewHolder(flagstore.Empty())
	apiServer := api.NewServer(cfg, flagStore, repo)

	// Set up syncer if repository is available
	var pollingDone <-chan struct{}
	var realtimeDone <-chan struct{}
	if repo != nil {
		syncer := sync.NewSyncer(repo, flagStore)

		// Set the refresh function on the API server for manual refreshes after writes
		apiServer.SetRefreshFunc(func(ctx context.Context) error {
			return syncer.Sync(ctx)
		})

		// Start polling sync with interval from config
		syncInterval := cfg.SyncInterval
		if syncInterval == 0 {
			syncInterval = 5 * time.Second
		}
		pollingDone = sync.StartPolling(ctx, syncer, syncInterval)
		slog.Info("sync polling started", "interval", syncInterval)

		realtimeDone = sync.StartRealtimeListener(ctx, cfg.DatabaseURL, syncer)
		slog.Info("realtime sync listener started", "channel", "flags_updated")
	}

	handler := apiServer.Routes()
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("server started", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}

	// Wait for polling to stop
	if pollingDone != nil {
		<-pollingDone
	}
	if realtimeDone != nil {
		<-realtimeDone
	}

	slog.Info("server stopped")
}
