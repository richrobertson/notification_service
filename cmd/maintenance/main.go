package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/richrobertson/notification-platform/internal/config"
	"github.com/richrobertson/notification-platform/internal/platform"
	"github.com/richrobertson/notification-platform/internal/store"
)

type maintenanceStore interface {
	RunMaintenance(ctx context.Context, params store.CleanupParams) (store.CleanupResult, error)
}

func main() {
	cfg := config.Load()
	cfg.AppName = "maintenance"
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", slog.Any("error", err))
		os.Exit(1)
	}

	logger := platform.NewLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	postgres, err := store.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to postgres", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if err := postgres.Close(); err != nil {
			logger.Error("failed to close postgres", slog.Any("error", err))
		}
	}()

	if err := run(ctx, logger, postgres, cfg, time.Now().UTC(), os.Stdout); err != nil {
		logger.Error("maintenance run failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger, st maintenanceStore, cfg config.Config, now time.Time, out io.Writer) error {
	result, err := st.RunMaintenance(ctx, store.CleanupParams{
		Now:                 now,
		AuditRetention:      cfg.MaintenanceAuditRetention,
		OutboxRetention:     cfg.MaintenanceOutboxRetention,
		DeadLetterRetention: cfg.MaintenanceDeadLetterRetention,
		DryRun:              cfg.MaintenanceDryRun,
	})
	if err != nil {
		return err
	}
	logger.Info("maintenance run complete",
		slog.Bool("dry_run", result.DryRun),
		slog.Int64("audit_events_deleted", result.AuditEventsDeleted),
		slog.Int64("published_outbox_deleted", result.PublishedOutboxDeleted),
		slog.Int64("dead_letters_deleted", result.DeadLettersDeleted),
	)
	return json.NewEncoder(out).Encode(result)
}
