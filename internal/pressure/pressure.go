package pressure

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/richrobertson/notification-platform/internal/queue"
)

// QueueSnapshotter provides queue-depth visibility to the pressure monitor.
type QueueSnapshotter interface {
	PressureSnapshot(ctx context.Context) (queue.PressureSnapshot, error)
}

// Monitor tracks queue pressure and related in-process counters that are useful
// for overload handling and operator diagnostics.
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

// MetricsSnapshot is the combined point-in-time view returned by Monitor.
type MetricsSnapshot struct {
	Queue            queue.PressureSnapshot `json:"queue"`
	RateLimitedTotal int64                  `json:"rate_limited_total"`
	RejectedTotal    int64                  `json:"rejected_total"`
	WorkerSaturated  int64                  `json:"worker_saturated_total"`
	TenantThrottled  int64                  `json:"tenant_throttled_total"`
	CollectedAt      time.Time              `json:"collected_at"`
}

// NewMonitor constructs a Monitor around a queue snapshot source and the local
// thresholds the service should treat as soft and hard pressure boundaries.
func NewMonitor(queues QueueSnapshotter, softLimit, hardLimit int, retryAfter time.Duration) *Monitor {
	return &Monitor{queues: queues, softLimit: softLimit, hardLimit: hardLimit, retryAfter: retryAfter}
}

// Snapshot returns the current queue-depth view with the configured soft and
// hard limits applied.
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

// IncRateLimited records that a request was rejected by the tenant rate limiter.
func (m *Monitor) IncRateLimited(string) {
	if m != nil {
		m.rateLimitedTotal.Add(1)
		m.throttledTenants.Add(1)
	}
}

// IncRejected records that new work was rejected because queue pressure was too
// high to accept it safely.
func (m *Monitor) IncRejected(string, string) {
	if m != nil {
		m.rejectedTotal.Add(1)
	}
}

// IncWorkerSaturated records that a worker loop hit its concurrency ceiling.
func (m *Monitor) IncWorkerSaturated() {
	if m != nil {
		m.workerSaturated.Add(1)
	}
}

// Metrics returns the combined queue snapshot and local counters that back the
// metrics endpoints.
func (m *Monitor) Metrics(ctx context.Context) (MetricsSnapshot, error) {
	q, err := m.Snapshot(ctx)
	if err != nil {
		return MetricsSnapshot{}, err
	}
	return MetricsSnapshot{Queue: q, RateLimitedTotal: m.rateLimitedTotal.Load(), RejectedTotal: m.rejectedTotal.Load(), WorkerSaturated: m.workerSaturated.Load(), TenantThrottled: m.throttledTenants.Load(), CollectedAt: time.Now().UTC()}, nil
}
