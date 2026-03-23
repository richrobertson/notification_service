# github.com/richrobertson/notification-platform/internal/outbox

_Generated from Go doc comments. Run `make docs-godoc` to refresh._

```text
package outbox // import "github.com/richrobertson/notification-platform/internal/outbox"

Package outbox publishes durable dispatch intents from Postgres into Redis.

The package is intentionally narrow: it is not a generalized event bus or a
change-data-capture layer. Its only responsibility is to take dispatch work
that has already been durably recorded in Postgres and enqueue it onto the Redis
execution transport.

RunOnce is the main entry point and is designed for simple polling workers.

FUNCTIONS

func ErrorString(err error) string
    ErrorString turns a nil-safe error into a stable string for logs and tests.

func RunOnce(ctx context.Context, logger *slog.Logger, st Store, q Queue, softLimit int, generateID IDGenerator) error
    RunOnce claims a batch of pending dispatch intents, publishes them to Redis,
    and records the durable result back in Postgres.

    The function is designed for polling workers. Callers typically run it on a
    ticker and let pending intents remain in Postgres when Redis is unavailable.


TYPES

type IDGenerator func(prefix string) string
    IDGenerator returns durable-ish IDs for emitted audit and queue records.

type Queue interface {
	EnqueueDispatch(ctx context.Context, job queue.DispatchJob) error
	PressureSnapshot(ctx context.Context) (queue.PressureSnapshot, error)
}
    Queue captures the Redis operations the publisher needs.

type Store interface {
	ClaimPendingDispatchIntents(ctx context.Context, limit int, staleAfter time.Duration) ([]store.PendingDispatchIntent, error)
	MarkDispatchIntentPublished(ctx context.Context, intentID string, claimedAt time.Time) error
	RecordDispatchIntentError(ctx context.Context, intentID string, claimedAt time.Time, lastError string) error
	RecordAuditEvent(ctx context.Context, id, tenantID, actor, action, resourceType, resourceID string, metadata map[string]any) error
}
    Store captures the Postgres behavior the outbox publisher needs.

```
