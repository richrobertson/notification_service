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
	"github.com/richrobertson/notification-platform/internal/queue"
)

func main() {
	cfg := config.Load()
	logger := platform.NewLogger(cfg.LogLevel)
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

	logger.Info("starting dispatcher", slog.String("source_queue", queue.DispatchQueueName), slog.String("processing_queue", queue.ProcessingQueueName(queue.DispatchQueueName)), slog.String("redis_addr", cfg.RedisAddr))
	for {
		reserved, err := redisQueue.ReserveDispatch(ctx, 1)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				logger.Info("dispatcher shutdown complete")
				return
			}
			logger.Error("failed to reserve dispatch job", slog.Any("error", err), slog.String("source_queue", queue.DispatchQueueName))
			time.Sleep(time.Second)
			continue
		}
		job := reserved.Job

		targetQueue, err := queue.QueueNameForChannel(job.Channel)
		if err != nil {
			logger.Error("discarding dispatch job with unsupported channel", slog.Any("error", err), slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel))
			if ackErr := redisQueue.AckReserved(ctx, reserved); ackErr != nil {
				logger.Error("unsupported dispatch job could not be acked; job remains reserved in processing queue for manual recovery", slog.Any("error", ackErr), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("processing_queue", queue.ProcessingQueueName(queue.DispatchQueueName)))
			}
			continue
		}

		if err := redisQueue.EnqueueChannel(ctx, job); err != nil {
			if requeueErr := redisQueue.RequeueReserved(ctx, reserved); requeueErr != nil {
				logger.Error("dispatch routing failed and source job left reserved in processing queue for manual recovery", slog.Any("error", requeueErr), slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel), slog.String("source_queue", queue.DispatchQueueName), slog.String("processing_queue", queue.ProcessingQueueName(queue.DispatchQueueName)), slog.String("target_queue", targetQueue))
				time.Sleep(time.Second)
				continue
			}
			logger.Error("dispatch job requeued to source queue after routing failure", slog.Any("error", err), slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel), slog.String("source_queue", queue.DispatchQueueName), slog.String("target_queue", targetQueue))
			time.Sleep(time.Second)
			continue
		}

		if err := redisQueue.AckReserved(ctx, reserved); err != nil {
			logger.Error("dispatch job routed but source job remains reserved in processing queue for manual recovery", slog.Any("error", err), slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel), slog.String("source_queue", queue.DispatchQueueName), slog.String("processing_queue", queue.ProcessingQueueName(queue.DispatchQueueName)), slog.String("target_queue", targetQueue))
			continue
		}

		logger.Info("routed dispatch job and acked source reservation", slog.String("job_id", job.JobID), slog.String("notification_id", job.NotificationID), slog.String("attempt_id", job.AttemptID), slog.String("channel", job.Channel), slog.String("source_queue", queue.DispatchQueueName), slog.String("target_queue", targetQueue))
	}
}
