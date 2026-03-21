package main

import (
	"context"
	"testing"
	"time"

	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

type fakeRetryStore struct {
	items   []store.RetryDueAttempt
	created []string
}

func (f *fakeRetryStore) ListDueRetryAttempts(context.Context, int) ([]store.RetryDueAttempt, error) {
	return f.items, nil
}
func (f *fakeRetryStore) CreateRetryAttempt(_ context.Context, scheduledAttemptID, newAttemptID string) (store.RetryDueAttempt, error) {
	f.created = append(f.created, scheduledAttemptID)
	for _, item := range f.items {
		if item.Attempt.ID == scheduledAttemptID {
			item.Attempt.ID = newAttemptID
			item.Attempt.AttemptNumber++
			item.Attempt.Status = "pending"
			return item, nil
		}
	}
	return store.RetryDueAttempt{}, store.ErrNotFound
}

type fakeRetryQueue struct{ jobs []queue.DispatchJob }

func (f *fakeRetryQueue) EnqueueDispatch(_ context.Context, job queue.DispatchJob) error {
	f.jobs = append(f.jobs, job)
	return nil
}

func TestRunOnceEnqueuesDueRetries(t *testing.T) {
	st := &fakeRetryStore{items: []store.RetryDueAttempt{{Attempt: store.DeliveryAttempt{ID: "attempt-1", NotificationID: "notif-1", Channel: "email", AttemptNumber: 1, Status: "retry_scheduled", NextRetryAt: ptr(time.Now())}, TenantID: "tenant-1"}}}
	q := &fakeRetryQueue{}
	if err := runOnce(context.Background(), st, q); err != nil {
		t.Fatal(err)
	}
	if len(q.jobs) != 1 {
		t.Fatalf("jobs=%d", len(q.jobs))
	}
	if q.jobs[0].AttemptID == "attempt-1" {
		t.Fatalf("attempt id was not refreshed: %+v", q.jobs[0])
	}
}
func ptr[T any](v T) *T { return &v }
