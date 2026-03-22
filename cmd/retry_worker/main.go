package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/richrobertson/notification-platform/internal/config"
	"github.com/richrobertson/notification-platform/internal/platform"
	"github.com/richrobertson/notification-platform/internal/store"
)

func main() {
	cfg := config.Load()
	cfg.AppName = "retry-worker"
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
	ticker := time.NewTicker(cfg.RetryWorkerPollInterval)
	defer ticker.Stop()
	for {
		if err := runOnce(ctx, logger, postgres); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("retry worker iteration failed", slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			logger.Info("retry worker shutdown complete")
			return
		case <-ticker.C:
		}
	}
}

type retryStore interface {
	ListDueRetryAttempts(ctx context.Context, limit int) ([]store.RetryDueAttempt, error)
	EnsureRetryAttempt(ctx context.Context, scheduledAttemptID, newAttemptID string) (store.RetryDueAttempt, error)
	RecordAuditEvent(ctx context.Context, id, tenantID, actor, action, resourceType, resourceID string, metadata map[string]any) error
}

func runOnce(ctx context.Context, logger *slog.Logger, postgres retryStore) error {
	due, err := postgres.ListDueRetryAttempts(ctx, 50)
	if err != nil {
		return err
	}
	for _, item := range due {
		created, err := postgres.EnsureRetryAttempt(ctx, item.Attempt.ID, retryAttemptID(item.Attempt.ID))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return err
		}
		logger.Info("retry attempt dispatch intent created for outbox publisher", slog.String("scheduled_attempt_id", item.Attempt.ID), slog.String("retry_attempt_id", created.Attempt.ID))
		_ = postgres.RecordAuditEvent(ctx, generateID("audit"), created.TenantID, "retry-worker", "dispatch_intent_created", "dispatch_intent", "intent-"+created.Attempt.ID, map[string]any{"scheduled_attempt_id": item.Attempt.ID, "notification_id": created.Attempt.NotificationID, "channel": created.Attempt.Channel, "source": "retry", "attempt_id": created.Attempt.ID})
	}
	return nil
}

func generateID(prefix string) string {
	return prefix + "-" + time.Now().UTC().Format("20060102150405.000000000")
}
func retryAttemptID(scheduledAttemptID string) string {
	return fmt.Sprintf("retry-%s", scheduledAttemptID)
}
