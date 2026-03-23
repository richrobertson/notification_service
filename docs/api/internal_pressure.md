# github.com/richrobertson/notification-platform/internal/pressure

_Generated from Go doc comments. Run `make docs-godoc` to refresh._

```text
package pressure // import "github.com/richrobertson/notification-platform/internal/pressure"

Package pressure tracks overload signals used by the API, retry logic,
and workers.

It combines queue-depth snapshots with in-process counters so callers can answer
two different questions:

  - What is Redis reporting right now?
  - How often has the service recently rejected, throttled, or saturated?

The resulting metrics are intentionally lightweight and operationally useful,
not a full-blown policy engine.

TYPES

type MetricsSnapshot struct {
	Queue            queue.PressureSnapshot `json:"queue"`
	RateLimitedTotal int64                  `json:"rate_limited_total"`
	RejectedTotal    int64                  `json:"rejected_total"`
	WorkerSaturated  int64                  `json:"worker_saturated_total"`
	TenantThrottled  int64                  `json:"tenant_throttled_total"`
	CollectedAt      time.Time              `json:"collected_at"`
}
    MetricsSnapshot is the combined point-in-time view returned by Monitor.

type Monitor struct {
	// Has unexported fields.
}
    Monitor tracks queue pressure and related in-process counters that are
    useful for overload handling and operator diagnostics.

func NewMonitor(queues QueueSnapshotter, softLimit, hardLimit int, retryAfter time.Duration) *Monitor
    NewMonitor constructs a Monitor around a queue snapshot source and the local
    thresholds the service should treat as soft and hard pressure boundaries.

func (m *Monitor) IncRateLimited(string)
    IncRateLimited records that a request was rejected by the tenant rate
    limiter.

func (m *Monitor) IncRejected(string, string)
    IncRejected records that new work was rejected because queue pressure was
    too high to accept it safely.

func (m *Monitor) IncWorkerSaturated()
    IncWorkerSaturated records that a worker loop hit its concurrency ceiling.

func (m *Monitor) Metrics(ctx context.Context) (MetricsSnapshot, error)
    Metrics returns the combined queue snapshot and local counters that back the
    metrics endpoints.

func (m *Monitor) Snapshot(ctx context.Context) (queue.PressureSnapshot, error)
    Snapshot returns the current queue-depth view with the configured soft and
    hard limits applied.

type QueueSnapshotter interface {
	PressureSnapshot(ctx context.Context) (queue.PressureSnapshot, error)
}
    QueueSnapshotter provides queue-depth visibility to the pressure monitor.

```
