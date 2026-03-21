package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

type fakeRetryStore struct {
	items         []store.RetryDueAttempt
	finalized     []string
	finalizeCalls int
}

func (f *fakeRetryStore) ListDueRetryAttempts(context.Context, int) ([]store.RetryDueAttempt, error) {
	return f.items, nil
}
func (f *fakeRetryStore) FinalizeRetryDispatch(_ context.Context, scheduledAttemptID, newAttemptID string) (store.RetryDueAttempt, error) {
	f.finalizeCalls++
	f.finalized = append(f.finalized, scheduledAttemptID+"->"+newAttemptID)
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

type fakeRetryQueue struct {
	jobs []queue.DispatchJob
	err  error
}

func (f *fakeRetryQueue) EnqueueDispatch(_ context.Context, job queue.DispatchJob) error {
	if f.err != nil {
		return f.err
	}
	f.jobs = append(f.jobs, job)
	return nil
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRunOnceRetryEnqueueFailureLeavesAttemptScheduled(t *testing.T) {
	st := &fakeRetryStore{items: []store.RetryDueAttempt{{Attempt: store.DeliveryAttempt{ID: "attempt-1", NotificationID: "notif-1", Channel: "email", AttemptNumber: 1, Status: "retry_scheduled", NextRetryAt: ptr(time.Now())}, TenantID: "tenant-1"}}}
	q := &fakeRetryQueue{err: errors.New("redis down")}
	if err := runOnce(context.Background(), testLogger(), st, q); err != nil {
		t.Fatal(err)
	}
	if len(q.jobs) != 0 {
		t.Fatalf("jobs=%d, want 0", len(q.jobs))
	}
	if st.finalizeCalls != 0 {
		t.Fatalf("finalizeCalls=%d, want 0", st.finalizeCalls)
	}
}

func TestRunOnceEnqueuesThenFinalizesRetry(t *testing.T) {
	st := &fakeRetryStore{items: []store.RetryDueAttempt{{Attempt: store.DeliveryAttempt{ID: "attempt-1", NotificationID: "notif-1", Channel: "email", AttemptNumber: 1, Status: "retry_scheduled", NextRetryAt: ptr(time.Now())}, TenantID: "tenant-1"}}}
	q := &fakeRetryQueue{}
	if err := runOnce(context.Background(), testLogger(), st, q); err != nil {
		t.Fatal(err)
	}
	if len(q.jobs) != 1 {
		t.Fatalf("jobs=%d", len(q.jobs))
	}
	if got, want := q.jobs[0].AttemptID, retryAttemptID("attempt-1"); got != want {
		t.Fatalf("AttemptID=%q want %q", got, want)
	}
	if st.finalizeCalls != 1 {
		t.Fatalf("finalizeCalls=%d", st.finalizeCalls)
	}
}

func TestRetryAttemptIDIsDeterministic(t *testing.T) {
	if got, want := retryAttemptID("attempt-1"), retryAttemptID("attempt-1"); got != want {
		t.Fatalf("retryAttemptID not deterministic: %q != %q", got, want)
	}
}

func ptr[T any](v T) *T { return &v }
