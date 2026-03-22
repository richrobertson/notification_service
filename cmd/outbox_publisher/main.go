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
	"github.com/richrobertson/notification-platform/internal/outbox"
	"github.com/richrobertson/notification-platform/internal/platform"
	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", slog.Any("error", err))
		os.Exit(1)
	}
	cfg.AppName = "outbox-publisher"
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

	ticker := time.NewTicker(cfg.OutboxPollInterval)
	defer ticker.Stop()

	for {
		if err := outbox.RunOnce(ctx, logger, postgres, redisQueue, cfg.QueueSoftLimit, generateID); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("outbox publisher iteration failed", slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			logger.Info("outbox publisher shutdown complete")
			return
		case <-ticker.C:
		}
	}
}

func generateID(prefix string) string {
	return prefix + "-" + time.Now().UTC().Format("20060102150405.000000000")
}
