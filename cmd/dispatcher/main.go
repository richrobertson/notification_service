package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/richrobertson/notification-platform/internal/config"
	"github.com/richrobertson/notification-platform/internal/queue"
)

func main() {
	cfg := config.Load()
	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err := redisQueue.Ping(startupCtx); err != nil {
		logger.Error("failed to connect to redis", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if err := redisQueue.Close(); err != nil {
			logger.Error("failed to close redis", slog.Any("error", err))
		}
	}()

	logger.Info("starting dispatcher", slog.String("source_queue", queue.DispatchQueueName), slog.String("redis_addr", cfg.RedisAddr))
	for {
		job, err := redisQueue.ConsumeDispatch(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				logger.Info("dispatcher shutdown complete")
				return
			}
			logger.Error("failed to consume dispatch job", slog.Any("error", err))
			time.Sleep(time.Second)
			continue
		}

		targetQueue, err := queue.QueueNameForChannel(job.Channel)
		if err != nil {
			logger.Error("discarding dispatch job with unsupported channel", slog.Any("error", err), slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel))
			continue
		}

		if err := redisQueue.EnqueueChannel(ctx, job); err != nil {
			logger.Error("failed to route dispatch job", slog.Any("error", err), slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel), slog.String("target_queue", targetQueue))
			time.Sleep(time.Second)
			continue
		}

		logger.Info("routed dispatch job", slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel), slog.String("source_queue", queue.DispatchQueueName), slog.String("target_queue", targetQueue))
	}
}

func newLogger(level string) *slog.Logger {
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel}))
}
