package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

// Store captures the Postgres behavior the outbox publisher needs.
type Store interface {
	ClaimPendingDispatchIntents(ctx context.Context, limit int, staleAfter time.Duration) ([]store.PendingDispatchIntent, error)
	MarkDispatchIntentPublished(ctx context.Context, intentID string, claimedAt time.Time) error
	RecordDispatchIntentError(ctx context.Context, intentID string, claimedAt time.Time, lastError string) error
	RecordAuditEvent(ctx context.Context, id, tenantID, actor, action, resourceType, resourceID string, metadata map[string]any) error
}

// Queue captures the Redis operations the publisher needs.
type Queue interface {
	EnqueueDispatch(ctx context.Context, job queue.DispatchJob) error
	PressureSnapshot(ctx context.Context) (queue.PressureSnapshot, error)
}

// IDGenerator returns durable-ish IDs for emitted audit and queue records.
type IDGenerator func(prefix string) string

const (
	claimBatchSize = 10
	publishTimeout = 15 * time.Second
	// claimTimeout is the stale-reclaim window for a claimed batch.
	// Keep it comfortably above the worst-case time to publish the full batch.
	claimTimeout = (claimBatchSize + 1) * publishTimeout
)

// RunOnce claims a batch of pending dispatch intents, publishes them to Redis,
// and records the durable result back in Postgres.
//
// The function is designed for polling workers. Callers typically run it on a
// ticker and let pending intents remain in Postgres when Redis is unavailable.
func RunOnce(ctx context.Context, logger *slog.Logger, st Store, q Queue, softLimit int, generateID IDGenerator) error {
	if q == nil {
		return fmt.Errorf("outbox publisher queue is required")
	}

	for {
		snapshot, err := q.PressureSnapshot(ctx)
		if err == nil {
			if softLimit > 0 {
				snapshot.SoftLimit = softLimit
			}
			if snapshot.AnySoftLimited() {
				logger.Warn("outbox publisher delaying dispatch publication due to queue pressure", slog.Any("depths", snapshot.Depths))
				return nil
			}
		}

		pending, err := st.ClaimPendingDispatchIntents(ctx, claimBatchSize, claimTimeout)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return nil
		}

		hadPublishFailure := false
		for _, item := range pending {
			if item.Intent.ClaimedAt == nil {
				return fmt.Errorf("claimed dispatch intent missing claimed_at: %s", item.Intent.ID)
			}
			job := queue.DispatchJob{
				JobID:          generateID("job"),
				NotificationID: item.Intent.NotificationID,
				AttemptID:      item.Intent.AttemptID,
				TenantID:       item.Intent.TenantID,
				Channel:        item.Intent.Channel,
				CreatedAt:      time.Now().UTC(),
			}
			publishCtx, cancel := context.WithTimeout(ctx, publishTimeout)
			err := q.EnqueueDispatch(publishCtx, job)
			cancel()
			if err != nil {
				hadPublishFailure = true
				logger.Error("dispatch intent publish failed; intent remains pending", slog.Any("error", err), slog.String("intent_id", item.Intent.ID), slog.String("attempt_id", item.Intent.AttemptID), slog.String("source", item.Intent.Source))
				if recErr := st.RecordDispatchIntentError(ctx, item.Intent.ID, *item.Intent.ClaimedAt, err.Error()); recErr != nil {
					logger.Error("failed to record dispatch intent error", slog.Any("error", recErr), slog.String("intent_id", item.Intent.ID), slog.String("attempt_id", item.Intent.AttemptID), slog.String("source", item.Intent.Source))
				}
				continue
			}
			if err := st.MarkDispatchIntentPublished(ctx, item.Intent.ID, *item.Intent.ClaimedAt); err != nil {
				return err
			}
			_ = st.RecordAuditEvent(ctx, generateID("audit"), item.Intent.TenantID, "outbox-publisher", "dispatch_published", "dispatch_intent", item.Intent.ID, map[string]any{
				"notification_id": item.Intent.NotificationID,
				"attempt_id":      item.Intent.AttemptID,
				"channel":         item.Intent.Channel,
				"source":          item.Intent.Source,
			})
		}
		if hadPublishFailure {
			return nil
		}
	}
}

// ErrorString turns a nil-safe error into a stable string for logs and tests.
func ErrorString(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
