package outbox

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

type fakeStore struct {
	pending        []store.PendingDispatchIntent
	published      []string
	recordedErrors []string
	recordedAudits []string
	publishedByID  map[string]bool
	claimed        []string
}

func (f *fakeStore) ClaimPendingDispatchIntents(_ context.Context, _ int, _ time.Time) ([]store.PendingDispatchIntent, error) {
	var out []store.PendingDispatchIntent
	for _, item := range f.pending {
		if !f.publishedByID[item.Intent.ID] && item.Intent.Status == "pending" {
			item.Intent.Status = "publishing"
			out = append(out, item)
			f.claimed = append(f.claimed, item.Intent.ID)
		}
	}
	return out, nil
}

func (f *fakeStore) MarkDispatchIntentPublished(_ context.Context, intentID string) error {
	f.published = append(f.published, intentID)
	if f.publishedByID == nil {
		f.publishedByID = map[string]bool{}
	}
	f.publishedByID[intentID] = true
	return nil
}

func (f *fakeStore) RecordDispatchIntentError(_ context.Context, intentID, lastError string) error {
	f.recordedErrors = append(f.recordedErrors, intentID+":"+lastError)
	return nil
}

func (f *fakeStore) RecordAuditEvent(_ context.Context, _, tenantID, _, action, _, resourceID string, _ map[string]any) error {
	f.recordedAudits = append(f.recordedAudits, tenantID+":"+action+":"+resourceID)
	return nil
}

type fakeQueue struct {
	jobs     []queue.DispatchJob
	err      error
	snapshot queue.PressureSnapshot
}

func (f *fakeQueue) EnqueueDispatch(_ context.Context, job queue.DispatchJob) error {
	if f.err != nil {
		return f.err
	}
	f.jobs = append(f.jobs, job)
	return nil
}

func (f *fakeQueue) PressureSnapshot(context.Context) (queue.PressureSnapshot, error) {
	return f.snapshot, nil
}

func logger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func gen(prefix string) string { return prefix + "-1" }

func TestRunOncePublishesPendingIntent(t *testing.T) {
	st := &fakeStore{pending: []store.PendingDispatchIntent{{
		Intent: store.DispatchIntent{ID: "intent-1", NotificationID: "notif-1", AttemptID: "attempt-1", TenantID: "tenant-1", Channel: "email", Source: "initial", Status: "pending"},
	}}}
	q := &fakeQueue{}

	if err := RunOnce(context.Background(), logger(), st, q, 0, gen); err != nil {
		t.Fatal(err)
	}
	if len(q.jobs) != 1 || q.jobs[0].AttemptID != "attempt-1" {
		t.Fatalf("jobs=%+v", q.jobs)
	}
	if len(st.published) != 1 || st.published[0] != "intent-1" {
		t.Fatalf("published=%v", st.published)
	}
	if len(st.claimed) != 1 || st.claimed[0] != "intent-1" {
		t.Fatalf("claimed=%v", st.claimed)
	}
}

func TestRunOnceLeavesIntentPendingOnRedisFailure(t *testing.T) {
	st := &fakeStore{pending: []store.PendingDispatchIntent{{
		Intent: store.DispatchIntent{ID: "intent-1", NotificationID: "notif-1", AttemptID: "attempt-1", TenantID: "tenant-1", Channel: "email", Source: "initial", Status: "pending"},
	}}}
	q := &fakeQueue{err: errors.New("redis down")}

	if err := RunOnce(context.Background(), logger(), st, q, 0, gen); err != nil {
		t.Fatal(err)
	}
	if len(st.published) != 0 {
		t.Fatalf("published=%v", st.published)
	}
	if len(st.recordedErrors) != 1 {
		t.Fatalf("recordedErrors=%v", st.recordedErrors)
	}
}

func TestRunOnceIsSafeAcrossDuplicatePasses(t *testing.T) {
	st := &fakeStore{pending: []store.PendingDispatchIntent{{
		Intent: store.DispatchIntent{ID: "intent-1", NotificationID: "notif-1", AttemptID: "attempt-1", TenantID: "tenant-1", Channel: "email", Source: "initial", Status: "pending"},
	}}}
	q := &fakeQueue{}

	if err := RunOnce(context.Background(), logger(), st, q, 0, gen); err != nil {
		t.Fatal(err)
	}
	if err := RunOnce(context.Background(), logger(), st, q, 0, gen); err != nil {
		t.Fatal(err)
	}
	if len(q.jobs) != 1 {
		t.Fatalf("jobs=%+v", q.jobs)
	}
	if len(st.published) != 1 {
		t.Fatalf("published=%v", st.published)
	}
}
