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
	"github.com/richrobertson/notification-platform/internal/delivery"
	"github.com/richrobertson/notification-platform/internal/platform"
	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

func main() {
	run("webhook-worker", queue.DispatchWebhookQueueName, func(cfg config.Config, postgres *store.Postgres) (*delivery.Service, error) {
		return delivery.NewService(postgres, delivery.NewWebhookSender(cfg.WebhookTimeout), delivery.NewSMTPSender(cfg))
	})
}

func run(appName, queueName string, svcFactory func(config.Config, *store.Postgres) (*delivery.Service, error)) {
	cfg := config.Load()
	cfg.AppName = appName
	logger := platform.NewLogger(cfg.LogLevel)
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	telemetryShutdown, err := platform.SetupTelemetry(startupCtx, cfg)
	if err != nil {
		logger.Error("failed to initialize telemetry", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = telemetryShutdown(c)
	}()
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
	svc, err := svcFactory(cfg, postgres)
	if err != nil {
		logger.Error("failed to initialize delivery service", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("starting worker", slog.String("queue", queueName))
	for {
		job, err := redisQueue.ConsumeChannel(ctx, queueName, int(cfg.QueueBlockTimeout/time.Second))
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				logger.Info("worker shutdown complete", slog.String("queue", queueName))
				return
			}
			logger.Error("failed to consume worker job", slog.Any("error", err), slog.String("queue", queueName))
			time.Sleep(time.Second)
			continue
		}
		logger.Info("received worker job", slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel), slog.String("queue", queueName))
		var processErr error
		switch queueName {
		case queue.DispatchWebhookQueueName:
			processErr = svc.ProcessWebhook(ctx, job)
		default:
			processErr = errors.New("unsupported worker queue")
		}
		if processErr != nil {
			logger.Error("worker job failed", slog.Any("error", processErr), slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel), slog.String("queue", queueName))
			continue
		}
		logger.Info("worker job completed", slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel), slog.String("queue", queueName))
	}
}
