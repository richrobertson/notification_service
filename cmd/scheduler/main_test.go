package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/richrobertson/notification-platform/internal/store"
)

type fakeSchedulerStore struct {
	promoted []store.PromotedScheduledNotification
	audits   []string
}

func (f *fakeSchedulerStore) PromoteDueScheduledNotifications(context.Context, int, time.Time) ([]store.PromotedScheduledNotification, error) {
	return f.promoted, nil
}

func (f *fakeSchedulerStore) RecordAuditEvent(context.Context, string, string, string, string, string, string, map[string]any) error {
	f.audits = append(f.audits, "notification_promoted")
	return nil
}

func schedulerLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRunOncePromotesScheduledNotifications(t *testing.T) {
	st := &fakeSchedulerStore{
		promoted: []store.PromotedScheduledNotification{{
			Notification: store.Notification{ID: "notif-1", TenantID: "tenant-1"},
			Attempt:      store.DeliveryAttempt{ID: "attempt-1", Channel: "email"},
			Intent:       store.DispatchIntent{ID: "intent-1"},
		}},
	}

	if err := runOnce(context.Background(), schedulerLogger(), st, time.Unix(100, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if len(st.audits) != 1 {
		t.Fatalf("audits=%v", st.audits)
	}
}
