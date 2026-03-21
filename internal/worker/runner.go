package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/richrobertson/notification-platform/internal/delivery"
	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

type Processor func(context.Context, queue.DispatchJob) (delivery.Result, error)

func RecoverProcessingQueues(ctx context.Context, logger *slog.Logger, redisQueue *queue.RedisQueue) {
	results, err := redisQueue.RecoverKnownProcessingQueues(ctx)
	if err != nil {
		logger.Error("failed to recover processing queues", slog.Any("error", err))
		return
	}
	for queueName, recovered := range results {
		if recovered > 0 {
			logger.Warn("recovered stranded reserved jobs", slog.String("queue", queueName), slog.Int("recovered", recovered))
		}
	}
}

func StartRecoveryLoop(ctx context.Context, logger *slog.Logger, redisQueue *queue.RedisQueue, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				RecoverProcessingQueues(ctx, logger, redisQueue)
			}
		}
	}()
}

func RunChannelWorker(ctx context.Context, logger *slog.Logger, redisQueue *queue.RedisQueue, queueName string, blockTimeout time.Duration, processor Processor) {
	for {
		reserved, err := redisQueue.ReserveChannel(ctx, queueName, int(blockTimeout/time.Second))
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				logger.Info("worker shutdown complete", slog.String("queue", queueName))
				return
			}
			logger.Error("failed to reserve worker job", slog.Any("error", err), slog.String("queue", queueName))
			time.Sleep(time.Second)
			continue
		}
		job := reserved.Job
		result, processErr := processor(ctx, job)
		if processErr != nil {
			if errors.Is(processErr, store.ErrNotFound) {
				if ackErr := redisQueue.AckReserved(ctx, reserved); ackErr != nil {
					logger.Error("orphan worker job could not be acked; job remains in processing queue until recovery", slog.Any("error", ackErr), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("queue", queueName))
					continue
				}
				logger.Error("worker received orphan job for nonexistent attempt; acking and dropping", slog.Any("error", processErr), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("queue", queueName))
				continue
			}
			if ackErr := redisQueue.AckReserved(ctx, reserved); ackErr != nil {
				logger.Error("worker completion state recorded but ack failed; job remains in processing queue until recovery", slog.Any("error", ackErr), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("queue", queueName))
				continue
			}
			logger.Warn("worker job finished with non-success outcome", slog.Any("error", processErr), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.Any("outcome", result.Outcome), slog.String("queue", queueName))
			continue
		}
		if err := redisQueue.AckReserved(ctx, reserved); err != nil {
			logger.Error("worker job completed but ack failed; job remains reserved in processing queue until recovery", slog.Any("error", err), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("queue", queueName))
			continue
		}
		logger.Info("worker job completed", slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("queue", queueName))
	}
}
