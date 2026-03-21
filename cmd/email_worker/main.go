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
	svc, err := delivery.NewService(postgres, delivery.NewWebhookSender(cfg.WebhookTimeout), delivery.NewSMTPSender(cfg))
	if err != nil {
		logger.Error("failed to initialize delivery service", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("starting worker", slog.String("queue", queue.DispatchEmailQueueName), slog.String("processing_queue", queue.ProcessingQueueName(queue.DispatchEmailQueueName)))
	for {
		reserved, err := redisQueue.ReserveChannel(ctx, queue.DispatchEmailQueueName, int(cfg.QueueBlockTimeout/time.Second))
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				logger.Info("worker shutdown complete", slog.String("queue", queue.DispatchEmailQueueName))
				return
			}
			logger.Error("failed to reserve worker job", slog.Any("error", err), slog.String("queue", queue.DispatchEmailQueueName))
			time.Sleep(time.Second)
			continue
		}
		job := reserved.Job
		logger.Info("received worker job", slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel), slog.String("queue", queue.DispatchEmailQueueName))
		_, processErr := svc.ProcessEmail(ctx, job)
		if processErr != nil {
			if delivery.IsTerminal(processErr) {
				if ackErr := redisQueue.AckReserved(ctx, reserved); ackErr != nil {
					logger.Error("worker terminal failure but ack failed; job remains reserved in processing queue for manual recovery", slog.Any("error", ackErr), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("queue", queue.DispatchEmailQueueName))
					continue
				}
				logger.Warn("worker job reached terminal failed state", slog.Any("error", processErr), slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel), slog.String("queue", queue.DispatchEmailQueueName))
				continue
			}
			if requeueErr := redisQueue.RequeueReserved(ctx, reserved); requeueErr != nil {
				logger.Error("worker transient failure and requeue failed; job left in processing queue", slog.Any("error", requeueErr), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("queue", queue.DispatchEmailQueueName))
				continue
			}
			logger.Error("worker job failed transiently and was requeued to the main queue", slog.Any("error", processErr), slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel), slog.String("queue", queue.DispatchEmailQueueName))
			continue
		}
		if err := redisQueue.AckReserved(ctx, reserved); err != nil {
			logger.Error("worker job completed but ack failed; job remains reserved in processing queue for manual recovery", slog.Any("error", err), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("queue", queue.DispatchEmailQueueName))
			continue
		}
		logger.Info("worker job completed", slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel), slog.String("queue", queue.DispatchEmailQueueName))
	}
}
