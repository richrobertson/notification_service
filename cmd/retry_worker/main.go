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
	"github.com/richrobertson/notification-platform/internal/queue"
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
	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err := redisQueue.Ping(startupCtx); err != nil {
		logger.Error("failed to connect to redis", slog.Any("error", err))
		os.Exit(1)
	}
	defer redisQueue.Close()
	ticker := time.NewTicker(cfg.RetryWorkerPollInterval)
	defer ticker.Stop()
	for {
		if err := runOnce(ctx, logger, postgres, redisQueue); err != nil && !errors.Is(err, context.Canceled) {
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
	ListAttemptsPendingEnqueue(ctx context.Context, limit int) ([]store.PendingEnqueueAttempt, error)
	MarkAttemptEnqueued(ctx context.Context, attemptID string) error
	FinalizeReplayEnqueue(ctx context.Context, deadLetterID, attemptID string) error
}

type retryQueue interface {
	EnqueueDispatch(ctx context.Context, job queue.DispatchJob) error
}

func runOnce(ctx context.Context, logger *slog.Logger, postgres retryStore, redisQueue retryQueue) error {
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
		logger.Info("retry attempt created and left pending enqueue", slog.String("scheduled_attempt_id", item.Attempt.ID), slog.String("retry_attempt_id", created.Attempt.ID))
	}

	pending, err := postgres.ListAttemptsPendingEnqueue(ctx, 100)
	if err != nil {
		return err
	}
	for _, item := range pending {
		job := queue.DispatchJob{JobID: generateID("job"), NotificationID: item.Attempt.NotificationID, AttemptID: item.Attempt.ID, TenantID: item.TenantID, Channel: item.Attempt.Channel, CreatedAt: time.Now().UTC()}
		if err := redisQueue.EnqueueDispatch(ctx, job); err != nil {
			logger.Error("attempt enqueue failed; attempt remains recoverable in postgres", slog.Any("error", err), slog.String("attempt_id", item.Attempt.ID), slog.String("channel", item.Attempt.Channel))
			continue
		}
		if err := postgres.MarkAttemptEnqueued(ctx, item.Attempt.ID); err != nil {
			return err
		}
		if item.Attempt.EnqueueKind == "initial" {
			logger.Info("recovered pending initial attempt and enqueued dispatch job", slog.String("attempt_id", item.Attempt.ID), slog.String("notification_id", item.Attempt.NotificationID), slog.String("channel", item.Attempt.Channel))
		}
		if item.DeadLetterID != nil {
			if err := postgres.FinalizeReplayEnqueue(ctx, *item.DeadLetterID, item.Attempt.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func generateID(prefix string) string {
	return prefix + "-" + time.Now().UTC().Format("20060102150405.000000000")
}
func retryAttemptID(scheduledAttemptID string) string {
	return fmt.Sprintf("retry-%s", scheduledAttemptID)
}
