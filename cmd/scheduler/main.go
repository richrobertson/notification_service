package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/richrobertson/notification-platform/internal/config"
	"github.com/richrobertson/notification-platform/internal/platform"
	"github.com/richrobertson/notification-platform/internal/store"
)

type schedulerStore interface {
	PromoteDueScheduledNotifications(ctx context.Context, limit int, now time.Time) ([]store.PromotedScheduledNotification, error)
	RecordAuditEvent(ctx context.Context, id, tenantID, actor, action, resourceType, resourceID string, metadata map[string]any) error
}

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", slog.Any("error", err))
		os.Exit(1)
	}
	cfg.AppName = "scheduler"
	logger := platform.NewLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	postgres, err := store.NewPostgres(startupCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to postgres", slog.Any("error", err))
		os.Exit(1)
	}
	defer postgres.Close()

	ticker := time.NewTicker(cfg.SchedulerPollInterval)
	defer ticker.Stop()

	for {
		if err := runOnce(ctx, logger, postgres, time.Now().UTC()); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("scheduler iteration failed", slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			logger.Info("scheduler shutdown complete")
			return
		case <-ticker.C:
		}
	}
}

func runOnce(ctx context.Context, logger *slog.Logger, st schedulerStore, now time.Time) error {
	promoted, err := st.PromoteDueScheduledNotifications(ctx, 50, now)
	if err != nil {
		return err
	}
	for _, item := range promoted {
		logger.Info("scheduled notification promoted", slog.String("notification_id", item.Notification.ID), slog.String("attempt_id", item.Attempt.ID), slog.String("intent_id", item.Intent.ID))
		_ = st.RecordAuditEvent(ctx, generateID("audit"), item.Notification.TenantID, "scheduler", "notification_promoted", "notification", item.Notification.ID, map[string]any{
			"attempt_id": item.Attempt.ID,
			"intent_id":  item.Intent.ID,
			"channel":    item.Attempt.Channel,
		})
	}
	return nil
}

func generateID(prefix string) string {
	return prefix + "-" + time.Now().UTC().Format("20060102150405.000000000")
}
