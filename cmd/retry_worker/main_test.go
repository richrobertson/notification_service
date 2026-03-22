package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/richrobertson/notification-platform/internal/store"
)

type fakeRetryStore struct {
	due     []store.RetryDueAttempt
	ensured []string
}

func (f *fakeRetryStore) ListDueRetryAttempts(context.Context, int) ([]store.RetryDueAttempt, error) {
	return f.due, nil
}

func (f *fakeRetryStore) EnsureRetryAttempt(_ context.Context, scheduledAttemptID, newAttemptID string) (store.RetryDueAttempt, error) {
	f.ensured = append(f.ensured, scheduledAttemptID+"->"+newAttemptID)
	return store.RetryDueAttempt{
		Attempt:  store.DeliveryAttempt{ID: newAttemptID, NotificationID: "notif-1", Channel: "email", AttemptNumber: 2, Status: "pending", EnqueueKind: "retry"},
		TenantID: "tenant-1",
	}, nil
}

func (f *fakeRetryStore) RecordAuditEvent(context.Context, string, string, string, string, string, string, map[string]any) error {
	return nil
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRunOnceCreatesDurableRetryAttempts(t *testing.T) {
	st := &fakeRetryStore{
		due: []store.RetryDueAttempt{{
			Attempt:  store.DeliveryAttempt{ID: "attempt-1", NotificationID: "notif-1", Channel: "email", AttemptNumber: 1, Status: "retry_scheduled"},
			TenantID: "tenant-1",
		}},
	}

	if err := runOnce(context.Background(), testLogger(), st); err != nil {
		t.Fatal(err)
	}
	if len(st.ensured) != 1 || st.ensured[0] != "attempt-1->retry-attempt-1" {
		t.Fatalf("ensured=%v", st.ensured)
	}
}
