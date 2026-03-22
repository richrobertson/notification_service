package main

import (
	"context"
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
	"github.com/richrobertson/notification-platform/internal/worker"
)

func main() {
	cfg := config.Load()
	cfg.AppName = "email-worker"
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
	worker.RecoverProcessingQueues(startupCtx, logger, redisQueue)
	worker.StartRecoveryLoop(ctx, logger, redisQueue, cfg.RecoveryInterval)
	webhookSender := delivery.NewWebhookSender(cfg.WebhookTimeout)
	svc, err := delivery.NewService(postgres, webhookSender, webhookSender, delivery.NewSMTPSender(cfg), delivery.NewSecondarySMTPSender(cfg), delivery.RetryPolicy{MaxAttempts: cfg.RetryMaxAttempts, BaseDelay: cfg.RetryBaseDelay, MaxDelay: cfg.RetryMaxDelay, ExponentialBackoff: cfg.RetryExponentialBackoff, Jitter: cfg.RetryJitter, Now: func() time.Time { return time.Now().UTC() }, PressureMultiplier: cfg.RetryPressureMultiplier, PressureMinDelay: cfg.RetryPressureMinDelay, QueueSoftLimit: cfg.QueueSoftLimit, QueueDepth: func(channel string) int {
		name, err := queue.QueueNameForChannel(channel)
		if err != nil {
			return 0
		}
		depth, err := redisQueue.QueueDepth(context.Background(), name)
		if err != nil {
			return 0
		}
		return depth
	}})
	if err != nil {
		logger.Error("failed to initialize delivery service", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("starting worker", slog.String("queue", queue.DispatchEmailQueueName), slog.String("processing_queue", queue.ProcessingQueueName(queue.DispatchEmailQueueName)))
	worker.RunChannelWorker(ctx, logger, redisQueue, queue.DispatchEmailQueueName, cfg.QueueBlockTimeout, svc.ProcessEmail, worker.Options{Concurrency: cfg.EmailWorkerConcurrency, TenantBurst: cfg.PerTenantWorkerBurst, TenantMaxInFlight: cfg.PerTenantMaxInFlight})
}
