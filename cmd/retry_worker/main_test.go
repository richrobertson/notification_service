package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

type fakeRetryStore struct {
	due             []store.RetryDueAttempt
	pending         []store.PendingEnqueueAttempt
	ensured         []string
	enqueued        []string
	finalizedReplay []string
}

func (f *fakeRetryStore) ListDueRetryAttempts(context.Context, int) ([]store.RetryDueAttempt, error) {
	return f.due, nil
}
func (f *fakeRetryStore) EnsureRetryAttempt(_ context.Context, scheduledAttemptID, newAttemptID string) (store.RetryDueAttempt, error) {
	f.ensured = append(f.ensured, scheduledAttemptID+"->"+newAttemptID)
	attempt := store.DeliveryAttempt{ID: newAttemptID, NotificationID: "notif-1", Channel: "email", AttemptNumber: 2, Status: "pending"}
	f.pending = append(f.pending, store.PendingEnqueueAttempt{Attempt: attempt, TenantID: "tenant-1"})
	return store.RetryDueAttempt{Attempt: attempt, TenantID: "tenant-1"}, nil
}
func (f *fakeRetryStore) ListAttemptsPendingEnqueue(context.Context, int) ([]store.PendingEnqueueAttempt, error) {
	return f.pending, nil
}
func (f *fakeRetryStore) MarkAttemptEnqueued(_ context.Context, attemptID string) error {
	f.enqueued = append(f.enqueued, attemptID)
	return nil
}
func (f *fakeRetryStore) FinalizeReplayEnqueue(_ context.Context, deadLetterID, attemptID string) error {
	f.finalizedReplay = append(f.finalizedReplay, deadLetterID+"->"+attemptID)
	return nil
}
func (f *fakeRetryStore) RecordAuditEvent(context.Context, string, string, string, string, string, string, map[string]any) error {
	return nil
}

type fakeRetryQueue struct {
	jobs     []queue.DispatchJob
	err      error
	snapshot queue.PressureSnapshot
}

func (f *fakeRetryQueue) EnqueueDispatch(_ context.Context, job queue.DispatchJob) error {
	if f.err != nil {
		return f.err
	}
	f.jobs = append(f.jobs, job)
	return nil
}
func (f *fakeRetryQueue) PressureSnapshot(context.Context) (queue.PressureSnapshot, error) {
	return f.snapshot, nil
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRunOnceRetryEnqueueFailureLeavesDurableAttemptPending(t *testing.T) {
	st := &fakeRetryStore{due: []store.RetryDueAttempt{{Attempt: store.DeliveryAttempt{ID: "attempt-1", NotificationID: "notif-1", Channel: "email", AttemptNumber: 1, Status: "retry_scheduled"}, TenantID: "tenant-1"}}}
	q := &fakeRetryQueue{err: errors.New("redis down")}
	if err := runOnce(context.Background(), testLogger(), st, q, 0); err != nil {
		t.Fatal(err)
	}
	if len(st.ensured) != 1 {
		t.Fatalf("ensured=%v", st.ensured)
	}
	if len(st.pending) != 1 {
		t.Fatalf("pending=%d", len(st.pending))
	}
	if len(st.enqueued) != 0 {
		t.Fatalf("enqueued=%v", st.enqueued)
	}
	if len(q.jobs) != 0 {
		t.Fatalf("jobs=%d", len(q.jobs))
	}
}

func TestRunOnceOnlyEnqueuesExistingAttempts(t *testing.T) {
	st := &fakeRetryStore{pending: []store.PendingEnqueueAttempt{{Attempt: store.DeliveryAttempt{ID: "attempt-2", NotificationID: "notif-1", Channel: "email", AttemptNumber: 2, Status: "pending"}, TenantID: "tenant-1"}}}
	q := &fakeRetryQueue{}
	if err := runOnce(context.Background(), testLogger(), st, q, 10); err != nil {
		t.Fatal(err)
	}
	if len(q.jobs) != 1 || q.jobs[0].AttemptID != "attempt-2" {
		t.Fatalf("jobs=%+v", q.jobs)
	}
	if len(st.enqueued) != 1 || st.enqueued[0] != "attempt-2" {
		t.Fatalf("enqueued=%v", st.enqueued)
	}
}

func TestRunOnceFinalizesReplayAfterSuccessfulEnqueue(t *testing.T) {
	dl := "dead-1"
	st := &fakeRetryStore{pending: []store.PendingEnqueueAttempt{{Attempt: store.DeliveryAttempt{ID: "replay_" + dl, NotificationID: "notif-1", Channel: "email", AttemptNumber: 4, Status: "pending"}, TenantID: "tenant-1", DeadLetterID: &dl}}}
	q := &fakeRetryQueue{}
	if err := runOnce(context.Background(), testLogger(), st, q, 10); err != nil {
		t.Fatal(err)
	}
	if len(st.finalizedReplay) != 1 {
		t.Fatalf("finalizedReplay=%v", st.finalizedReplay)
	}
}

func TestRunOnceRecoversPendingInitialAttempt(t *testing.T) {
	st := &fakeRetryStore{pending: []store.PendingEnqueueAttempt{{Attempt: store.DeliveryAttempt{ID: "attempt-initial", NotificationID: "notif-1", Channel: "email", AttemptNumber: 1, Status: "pending", EnqueueKind: "initial"}, TenantID: "tenant-1"}}}
	q := &fakeRetryQueue{}
	if err := runOnce(context.Background(), testLogger(), st, q, 10); err != nil {
		t.Fatal(err)
	}
	if len(q.jobs) != 1 || q.jobs[0].AttemptID != "attempt-initial" {
		t.Fatalf("jobs=%+v", q.jobs)
	}
}

func TestRunOnceSkipsRetryStormWhenQueuePressured(t *testing.T) {
	st := &fakeRetryStore{pending: []store.PendingEnqueueAttempt{{Attempt: store.DeliveryAttempt{ID: "attempt-2", NotificationID: "notif-1", Channel: "email", AttemptNumber: 2, Status: "pending"}, TenantID: "tenant-1"}}}
	q := &fakeRetryQueue{snapshot: queue.PressureSnapshot{Depths: map[string]int{queue.DispatchQueueName: 100}, SoftLimit: 10, HardLimit: 100}}
	if err := runOnce(context.Background(), testLogger(), st, q, 10); err != nil {
		t.Fatal(err)
	}
	if len(q.jobs) != 0 {
		t.Fatalf("jobs=%+v", q.jobs)
	}
}
