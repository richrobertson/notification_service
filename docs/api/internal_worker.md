# github.com/richrobertson/notification-platform/internal/worker

_Generated from Go doc comments. Run `make docs-godoc` to refresh._

```text
package worker // import "github.com/richrobertson/notification-platform/internal/worker"

Package worker contains the queue-consuming runtime loops used by the dispatcher
and channel workers.

The package focuses on practical execution concerns:

  - fair per-tenant scheduling
  - bounded worker concurrency
  - reserve/process/ack behavior
  - recovery of stranded processing queues
  - graceful shutdown that drains in-flight work

It does not own delivery semantics itself; instead it coordinates queue
mechanics around a caller-supplied Processor function.

FUNCTIONS

func RecoverProcessingQueues(ctx context.Context, logger *slog.Logger, redisQueue *queue.RedisQueue)
    RecoverProcessingQueues drains known processing queues back into their
    source queues.

    This is a best-effort recovery pass used at startup and during periodic
    recovery loops.

func RunChannelWorker(ctx context.Context, logger *slog.Logger, redisQueue *queue.RedisQueue, queueName string, blockTimeout time.Duration, processor Processor, opts Options)
    RunChannelWorker consumes jobs from a channel queue using bounded
    concurrency and lightweight tenant fairness.

    On shutdown, the worker stops reserving new jobs and waits for in-flight
    processors to complete before returning.

func StartRecoveryLoop(ctx context.Context, logger *slog.Logger, redisQueue *queue.RedisQueue, interval time.Duration)
    StartRecoveryLoop launches the periodic stranded-job recovery loop.


TYPES

type Options struct {
	Concurrency       int
	TenantBurst       int
	TenantMaxInFlight int
	Monitor           *pressure.Monitor
}
    Options controls concurrency and fairness behavior for RunChannelWorker.

type Processor func(context.Context, queue.DispatchJob) (delivery.Result, error)
    Processor performs the business work for one reserved job.

    Workers supply queue and shutdown mechanics; the Processor owns the
    channel-specific behavior.

```
