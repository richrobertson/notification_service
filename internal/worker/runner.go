package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/richrobertson/notification-platform/internal/delivery"
	"github.com/richrobertson/notification-platform/internal/pressure"
	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

type Processor func(context.Context, queue.DispatchJob) (delivery.Result, error)

type Options struct {
	Concurrency       int
	TenantBurst       int
	TenantMaxInFlight int
	Monitor           *pressure.Monitor
}

type fairScheduler struct {
	burst     int
	inflight  map[string]int
	pending   map[string][]queue.ReservedJob
	order     []string
	next      int
	maxFlight int
}

func newFairScheduler(burst, maxFlight int) *fairScheduler {
	if burst <= 0 {
		burst = 1
	}
	if maxFlight <= 0 {
		maxFlight = 1
	}
	return &fairScheduler{burst: burst, maxFlight: maxFlight, inflight: map[string]int{}, pending: map[string][]queue.ReservedJob{}}
}
func (s *fairScheduler) add(job queue.ReservedJob) {
	t := job.Job.TenantID
	if len(s.pending[t]) == 0 {
		s.order = append(s.order, t)
	}
	s.pending[t] = append(s.pending[t], job)
}
func (s *fairScheduler) complete(tenant string) {
	if s.inflight[tenant] > 0 {
		s.inflight[tenant]--
	}
}
func (s *fairScheduler) nextJob() (queue.ReservedJob, bool) {
	if len(s.order) == 0 {
		return queue.ReservedJob{}, false
	}
	for range s.order {
		t := s.order[s.next%len(s.order)]
		s.next = (s.next + 1) % len(s.order)
		jobs := s.pending[t]
		if len(jobs) == 0 || s.inflight[t] >= s.maxFlight {
			continue
		}
		job := jobs[0]
		limit := s.burst
		if limit > len(jobs) {
			limit = len(jobs)
		}
		s.pending[t] = jobs[1:]
		s.inflight[t]++
		if len(s.pending[t]) == 0 {
			s.order = removeTenant(s.order, t)
			if len(s.order) > 0 {
				s.next %= len(s.order)
			} else {
				s.next = 0
			}
		}
		_ = limit
		return job, true
	}
	return queue.ReservedJob{}, false
}
func removeTenant(order []string, tenant string) []string {
	out := order[:0]
	for _, item := range order {
		if item != tenant {
			out = append(out, item)
		}
	}
	return out
}

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

func RunChannelWorker(ctx context.Context, logger *slog.Logger, redisQueue *queue.RedisQueue, queueName string, blockTimeout time.Duration, processor Processor, opts Options) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 1
	}
	scheduler := newFairScheduler(opts.TenantBurst, opts.TenantMaxInFlight)
	sem := make(chan struct{}, opts.Concurrency)
	var mu sync.Mutex
	for {
		for len(sem) < cap(sem) {
			job, ok := scheduler.nextJob()
			if !ok {
				break
			}
			sem <- struct{}{}
			go func(reserved queue.ReservedJob) {
				defer func() { <-sem; mu.Lock(); scheduler.complete(reserved.Job.TenantID); mu.Unlock() }()
				runReserved(ctx, logger, redisQueue, queueName, reserved, processor)
			}(job)
		}
		if ctx.Err() != nil {
			logger.Info("worker shutdown complete", slog.String("queue", queueName))
			return
		}
		if len(sem) == cap(sem) {
			if opts.Monitor != nil {
				opts.Monitor.IncWorkerSaturated()
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
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
		mu.Lock()
		scheduler.add(reserved)
		mu.Unlock()
	}
}

func runReserved(ctx context.Context, logger *slog.Logger, redisQueue *queue.RedisQueue, queueName string, reserved queue.ReservedJob, processor Processor) {
	job := reserved.Job
	result, processErr := processor(ctx, job)
	if processErr != nil {
		if errors.Is(processErr, store.ErrNotFound) {
			if ackErr := redisQueue.AckReserved(ctx, reserved); ackErr != nil {
				logger.Error("orphan worker job could not be acked; job remains in processing queue until recovery", slog.Any("error", ackErr), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("queue", queueName))
				return
			}
			logger.Error("worker received orphan job for nonexistent attempt; acking and dropping", slog.Any("error", processErr), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("queue", queueName))
			return
		}
		if ackErr := redisQueue.AckReserved(ctx, reserved); ackErr != nil {
			logger.Error("worker completion state recorded but ack failed; job remains in processing queue until recovery", slog.Any("error", ackErr), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("queue", queueName))
			return
		}
		logger.Warn("worker job finished with non-success outcome", slog.Any("error", processErr), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.Any("outcome", result.Outcome), slog.String("queue", queueName))
		return
	}
	if err := redisQueue.AckReserved(ctx, reserved); err != nil {
		logger.Error("worker job completed but ack failed; job remains reserved in processing queue until recovery", slog.Any("error", err), slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("queue", queueName))
		return
	}
	logger.Info("worker job completed", slog.String("job_id", job.JobID), slog.String("attempt_id", job.AttemptID), slog.String("queue", queueName))
}
