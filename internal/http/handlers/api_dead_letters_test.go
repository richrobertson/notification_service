package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

type fakeAPIStore struct {
	deadLetters  map[string]store.DeadLetter
	attempt      store.DeliveryAttempt
	notification store.Notification
}

func (f *fakeAPIStore) CreateTenant(context.Context, store.CreateTenantParams) (store.Tenant, error) {
	panic("unused")
}
func (f *fakeAPIStore) GetTenantByID(context.Context, string) (store.Tenant, error) { panic("unused") }
func (f *fakeAPIStore) CreateTemplate(context.Context, store.CreateTemplateParams) (store.Template, error) {
	panic("unused")
}
func (f *fakeAPIStore) GetTemplateByID(context.Context, string) (store.Template, error) {
	panic("unused")
}
func (f *fakeAPIStore) GetNotificationByTenantAndIdempotencyKey(context.Context, string, string) (store.Notification, error) {
	panic("unused")
}
func (f *fakeAPIStore) CreateNotification(context.Context, store.CreateNotificationParams) (store.Notification, error) {
	panic("unused")
}
func (f *fakeAPIStore) CreateDeliveryAttempt(context.Context, store.CreateDeliveryAttemptParams) (store.DeliveryAttempt, error) {
	panic("unused")
}
func (f *fakeAPIStore) ListDeadLetters(context.Context, int) ([]store.DeadLetter, error) {
	out := make([]store.DeadLetter, 0, len(f.deadLetters))
	for _, dl := range f.deadLetters {
		out = append(out, dl)
	}
	return out, nil
}
func (f *fakeAPIStore) GetDeadLetterByID(_ context.Context, id string) (store.DeadLetter, error) {
	dl, ok := f.deadLetters[id]
	if !ok {
		return store.DeadLetter{}, store.ErrNotFound
	}
	return dl, nil
}
func (f *fakeAPIStore) ReplayDeadLetter(_ context.Context, id, newAttemptID string) (store.ReplayDeadLetterResult, error) {
	dl, ok := f.deadLetters[id]
	if !ok {
		return store.ReplayDeadLetterResult{}, store.ErrNotFound
	}
	now := time.Unix(200, 0).UTC()
	dl.ReplayedAt = &now
	f.deadLetters[id] = dl
	attempt := f.attempt
	attempt.ID = newAttemptID
	return store.ReplayDeadLetterResult{DeadLetter: dl, Attempt: attempt}, nil
}
func (f *fakeAPIStore) GetNotificationByID(context.Context, string) (store.Notification, error) {
	return f.notification, nil
}

type fakeDispatchQueue struct{ jobs []queue.DispatchJob }

func (f *fakeDispatchQueue) EnqueueDispatch(_ context.Context, job queue.DispatchJob) error {
	f.jobs = append(f.jobs, job)
	return nil
}

func TestDeadLetterHandlersListGetReplay(t *testing.T) {
	st := &fakeAPIStore{deadLetters: map[string]store.DeadLetter{"dead-1": {ID: "dead-1", NotificationID: "notif-1", Channel: "email", FinalError: "smtp down", DeadLetteredAt: time.Unix(100, 0).UTC()}}, attempt: store.DeliveryAttempt{ID: "attempt-old", NotificationID: "notif-1", Channel: "email", AttemptNumber: 4, Status: "pending"}, notification: store.Notification{ID: "notif-1", TenantID: "tenant-1"}}
	q := &fakeDispatchQueue{}
	api := NewAPI(st, q)

	t.Run("list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dead-letters", nil)
		res := httptest.NewRecorder()
		api.ListDeadLetters().ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
		var items []store.DeadLetter
		if err := json.NewDecoder(res.Body).Decode(&items); err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].ID != "dead-1" {
			t.Fatalf("items=%+v", items)
		}
	})

	t.Run("get", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dead-letters/dead-1", nil)
		req.SetPathValue("id", "dead-1")
		res := httptest.NewRecorder()
		api.GetDeadLetter().ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
	})

	t.Run("replay", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/dead-letters/dead-1/replay", bytes.NewReader(nil))
		req.SetPathValue("id", "dead-1")
		res := httptest.NewRecorder()
		api.ReplayDeadLetter().ServeHTTP(res, req)
		if res.Code != http.StatusAccepted {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
		if len(q.jobs) != 1 || q.jobs[0].NotificationID != "notif-1" {
			t.Fatalf("jobs=%+v", q.jobs)
		}
	})
}
