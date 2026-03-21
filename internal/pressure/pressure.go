package pressure

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/richrobertson/notification-platform/internal/queue"
)

type QueueSnapshotter interface {
	PressureSnapshot(ctx context.Context) (queue.PressureSnapshot, error)
}

type Monitor struct {
	queues           QueueSnapshotter
	softLimit        int
	hardLimit        int
	retryAfter       time.Duration
	rateLimitedTotal atomic.Int64
	rejectedTotal    atomic.Int64
	workerSaturated  atomic.Int64
	throttledTenants atomic.Int64
}

type MetricsSnapshot struct {
	Queue            queue.PressureSnapshot `json:"queue"`
	RateLimitedTotal int64                  `json:"rate_limited_total"`
	RejectedTotal    int64                  `json:"rejected_total"`
	WorkerSaturated  int64                  `json:"worker_saturated_total"`
	TenantThrottled  int64                  `json:"tenant_throttled_total"`
	CollectedAt      time.Time              `json:"collected_at"`
}

func NewMonitor(queues QueueSnapshotter, softLimit, hardLimit int, retryAfter time.Duration) *Monitor {
	return &Monitor{queues: queues, softLimit: softLimit, hardLimit: hardLimit, retryAfter: retryAfter}
}
func (m *Monitor) Snapshot(ctx context.Context) (queue.PressureSnapshot, error) {
	if m == nil {
		return queue.PressureSnapshot{}, nil
	}
	if m.queues == nil {
		return queue.PressureSnapshot{SoftLimit: m.softLimit, HardLimit: m.hardLimit, RetryAfter: m.retryAfter}, nil
	}
	snapshot, err := m.queues.PressureSnapshot(ctx)
	if err != nil {
		return queue.PressureSnapshot{}, err
	}
	snapshot.SoftLimit = m.softLimit
	snapshot.HardLimit = m.hardLimit
	if snapshot.RetryAfter <= 0 {
		snapshot.RetryAfter = m.retryAfter
	}
	return snapshot, nil
}
func (m *Monitor) IncRateLimited(string) {
	if m != nil {
		m.rateLimitedTotal.Add(1)
		m.throttledTenants.Add(1)
	}
}
func (m *Monitor) IncRejected(string, string) {
	if m != nil {
		m.rejectedTotal.Add(1)
	}
}
func (m *Monitor) IncWorkerSaturated() {
	if m != nil {
		m.workerSaturated.Add(1)
	}
}
func (m *Monitor) Metrics(ctx context.Context) (MetricsSnapshot, error) {
	q, err := m.Snapshot(ctx)
	if err != nil {
		return MetricsSnapshot{}, err
	}
	return MetricsSnapshot{Queue: q, RateLimitedTotal: m.rateLimitedTotal.Load(), RejectedTotal: m.rejectedTotal.Load(), WorkerSaturated: m.workerSaturated.Load(), TenantThrottled: m.throttledTenants.Load(), CollectedAt: time.Now().UTC()}, nil
}
