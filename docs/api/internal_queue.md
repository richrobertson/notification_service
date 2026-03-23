# github.com/richrobertson/notification-platform/internal/queue

_Generated from Go doc comments. Run `make docs-godoc` to refresh._

```text
package queue // import "github.com/richrobertson/notification-platform/internal/queue"

Package queue provides the Redis-backed execution transport used by the
dispatcher, channel workers, and Stage 7 pressure controls.

The package exposes a small set of concepts:

  - DispatchJob is the durable execution payload placed on Redis lists
  - RedisQueue handles enqueue, reserve, ack, and recovery operations
  - PressureSnapshot summarizes queue depth for backpressure decisions
  - TenantRateLimiter uses Redis counters for fixed-window API throttling

The transport is intentionally at-least-once. Reserve/ack semantics and
processing-queue recovery exist to make failures inspectable and recoverable,
not to promise exactly-once routing.

CONSTANTS

const (
	// DispatchQueueName is the shared entry queue used by the Stage 8 outbox
	// publisher before channel-specific routing happens.
	DispatchQueueName = "notify:dispatch"
	// DispatchWebhookQueueName is the queue consumed by webhook workers.
	DispatchWebhookQueueName = "notify:dispatch:webhook"
	// DispatchEmailQueueName is the queue consumed by email workers.
	DispatchEmailQueueName = "notify:dispatch:email"
)

FUNCTIONS

func ProcessingQueueName(queueName string) string
    ProcessingQueueName returns the name of the processing queue paired with a
    source queue.

func QueueNameForChannel(channel string) (string, error)
    QueueNameForChannel returns the worker queue that should handle a channel.


TYPES

type DispatchJob struct {
	JobID          string    `json:"job_id"`
	NotificationID string    `json:"notification_id"`
	AttemptID      string    `json:"attempt_id"`
	TenantID       string    `json:"tenant_id"`
	Channel        string    `json:"channel"`
	CreatedAt      time.Time `json:"created_at"`
}
    DispatchJob is the transport payload carried through Redis.

type PressureSnapshot struct {
	Depths     map[string]int `json:"depths"`
	SoftLimit  int            `json:"soft_limit"`
	HardLimit  int            `json:"hard_limit"`
	RetryAfter time.Duration  `json:"retry_after"`
}
    PressureSnapshot summarizes current queue depths and the thresholds the API
    should use for soft warnings and hard write rejection.

func (s PressureSnapshot) AcceptingWrites() bool
    AcceptingWrites reports whether the current snapshot still permits new work
    to be accepted safely.

func (s PressureSnapshot) AnyHardLimited() bool
    AnyHardLimited reports whether any tracked queue has reached the hard limit.

func (s PressureSnapshot) AnySoftLimited() bool
    AnySoftLimited reports whether any tracked queue has reached the soft limit.

type RedisQueue struct {
	// Has unexported fields.
}
    RedisQueue is the Redis-backed queue client used by the runtime.

func NewRedisQueue(addr, password string, db int) *RedisQueue
    NewRedisQueue creates a queue client for the given Redis connection
    settings.

func (q *RedisQueue) AckReserved(ctx context.Context, reserved ReservedJob) error
    AckReserved removes a previously reserved job from its processing queue.

func (q *RedisQueue) AllowTenant(ctx context.Context, tenantID string, limit int, window time.Duration) (bool, time.Duration, error)
    AllowTenant applies the Redis-backed fixed-window rate limiter for a tenant.

func (q *RedisQueue) Close() error
    Close releases the current Redis connection.

func (q *RedisQueue) ConsumeChannel(ctx context.Context, queueName string, timeoutSeconds int) (DispatchJob, error)
    ConsumeChannel reserves and immediately acknowledges a job from the named
    channel queue.

func (q *RedisQueue) ConsumeDispatch(ctx context.Context) (DispatchJob, error)
    ConsumeDispatch reserves and immediately acknowledges a job from the shared
    dispatch queue.

func (q *RedisQueue) EnqueueChannel(ctx context.Context, job DispatchJob) error
    EnqueueChannel routes a job directly to its channel-specific queue.

func (q *RedisQueue) EnqueueDispatch(ctx context.Context, job DispatchJob) error
    EnqueueDispatch writes a job to the shared dispatch queue.

func (q *RedisQueue) Ping(ctx context.Context) error
    Ping verifies that Redis is reachable and responding to basic commands.

func (q *RedisQueue) PressureSnapshot(ctx context.Context) (PressureSnapshot, error)
    PressureSnapshot returns the current depth of the shared and
    channel-specific dispatch queues.

func (q *RedisQueue) QueueDepth(ctx context.Context, queueName string) (int, error)
    QueueDepth returns the current Redis list length for a queue.

func (q *RedisQueue) RecoverKnownProcessingQueues(ctx context.Context) (map[string]int, error)
    RecoverKnownProcessingQueues drains every processing queue known to the
    service runtime and reports how many jobs were recovered per queue.

func (q *RedisQueue) RecoverProcessingQueue(ctx context.Context, queueName string) (int, error)
    RecoverProcessingQueue drains one processing queue back into its source
    queue.

func (q *RedisQueue) RequeueReserved(ctx context.Context, reserved ReservedJob) error
    RequeueReserved moves a reserved job back to its source queue.

    This is used when routing failed after the job was already reserved, so the
    dispatcher can preserve work without waiting for a later recovery sweep.

func (q *RedisQueue) ReserveChannel(ctx context.Context, queueName string, timeoutSeconds int) (ReservedJob, error)
    ReserveChannel moves one job from the named queue to its processing queue
    and returns the reserved payload.

func (q *RedisQueue) ReserveDispatch(ctx context.Context, timeoutSeconds int) (ReservedJob, error)
    ReserveDispatch reserves work from the shared dispatch queue.

type ReservedJob struct {
	Job DispatchJob

	// Has unexported fields.
}
    ReservedJob represents a job that has been moved into a processing queue but
    not yet acknowledged.

type TenantRateLimiter struct {
	// Has unexported fields.
}

func NewTenantRateLimiter(redis *RedisQueue, limit int, window time.Duration) *TenantRateLimiter
    NewTenantRateLimiter builds the Stage 7 fixed-window rate limiter backed by
    Redis counters.

func (l *TenantRateLimiter) Allow(ctx context.Context, tenantID string) (bool, time.Duration, error)
    Allow reports whether the tenant is still within the configured request
    budget and, when rejected, how long the caller should wait before retrying.

```
