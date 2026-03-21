package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

type fakeAPIStore struct {
	deadLetters           map[string]store.DeadLetter
	attempt               store.DeliveryAttempt
	notification          store.Notification
	ensureCalls           int
	ensureInitialCalls    int
	markEnqueuedCalls     int
	createAttemptCalls    int
	createNotifyCalls     int
	createdAttempt        *store.CreateDeliveryAttemptParams
	createdNotification   *store.CreateNotificationParams
	finalizeCalls         int
	finalizedAttemptID    string
	ensureErr             error
	finalizeErr           error
	existingByKey         map[string]store.Notification
	createNotificationErr error
	fallbackExisting      *store.Notification
	lookupCalls           int
	fallbackAfterLookup   int
	initialAttemptMissing bool
	ensureInitialErr      error
}

func (f *fakeAPIStore) CreateTenant(context.Context, store.CreateTenantParams) (store.Tenant, error) {
	panic("unused")
}
func (f *fakeAPIStore) GetTenantByID(_ context.Context, id string) (store.Tenant, error) {
	return store.Tenant{ID: id}, nil
}
func (f *fakeAPIStore) CreateTemplate(context.Context, store.CreateTemplateParams) (store.Template, error) {
	panic("unused")
}
func (f *fakeAPIStore) GetTemplateByID(_ context.Context, id string) (store.Template, error) {
	return store.Template{ID: id, TenantID: "tenant-1", Channel: "email", Name: "welcome"}, nil
}
func (f *fakeAPIStore) GetNotificationByTenantAndIdempotencyKey(_ context.Context, tenantID, key string) (store.Notification, error) {
	f.lookupCalls++
	if f.existingByKey != nil {
		if n, ok := f.existingByKey[tenantID+"/"+key]; ok {
			return n, nil
		}
	}
	if f.fallbackExisting != nil && f.lookupCalls >= f.fallbackAfterLookup {
		return *f.fallbackExisting, nil
	}
	return store.Notification{}, store.ErrNotFound
}
func (f *fakeAPIStore) CreateNotification(_ context.Context, params store.CreateNotificationParams) (store.Notification, error) {
	f.createNotifyCalls++
	if f.createNotificationErr != nil {
		return store.Notification{}, f.createNotificationErr
	}
	f.createdNotification = &params
	n := store.Notification{ID: params.ID, TenantID: params.TenantID, TemplateID: params.TemplateID, IdempotencyKey: params.IdempotencyKey}
	if params.IdempotencyKey != nil {
		if f.existingByKey == nil {
			f.existingByKey = map[string]store.Notification{}
		}
		f.existingByKey[params.TenantID+"/"+*params.IdempotencyKey] = n
	}
	return n, nil
}

func (f *fakeAPIStore) GetInitialAttemptByNotificationID(_ context.Context, notificationID string) (store.DeliveryAttempt, error) {
	if f.initialAttemptMissing {
		return store.DeliveryAttempt{}, store.ErrNotFound
	}
	if f.createdAttempt != nil && f.createdAttempt.NotificationID == notificationID {
		attempt := store.DeliveryAttempt{ID: f.createdAttempt.ID, NotificationID: notificationID, Channel: f.createdAttempt.Channel, AttemptNumber: f.createdAttempt.AttemptNumber, Status: f.createdAttempt.Status, EnqueueKind: f.createdAttempt.EnqueueKind}
		if f.markEnqueuedCalls > 0 {
			now := time.Unix(300, 0).UTC()
			attempt.DispatchEnqueuedAt = &now
		}
		return attempt, nil
	}
	return store.DeliveryAttempt{}, store.ErrNotFound
}

func (f *fakeAPIStore) EnsureInitialAttempt(_ context.Context, notificationID, channel, attemptID string) (store.DeliveryAttempt, error) {
	f.ensureInitialCalls++
	if f.ensureInitialErr != nil {
		return store.DeliveryAttempt{}, f.ensureInitialErr
	}
	if f.createdAttempt == nil || f.createdAttempt.NotificationID != notificationID {
		params := store.CreateDeliveryAttemptParams{ID: attemptID, NotificationID: notificationID, Channel: channel, AttemptNumber: 1, Status: "pending", EnqueueKind: "initial"}
		f.createAttemptCalls++
		f.createdAttempt = &params
	}
	f.initialAttemptMissing = false
	return f.GetInitialAttemptByNotificationID(context.Background(), notificationID)
}
func (f *fakeAPIStore) CreateDeliveryAttempt(_ context.Context, params store.CreateDeliveryAttemptParams) (store.DeliveryAttempt, error) {
	f.createAttemptCalls++
	f.createdAttempt = &params
	return store.DeliveryAttempt{ID: params.ID, NotificationID: params.NotificationID, Channel: params.Channel, AttemptNumber: params.AttemptNumber, Status: params.Status, EnqueueKind: params.EnqueueKind}, nil
}
func (f *fakeAPIStore) MarkAttemptEnqueued(_ context.Context, attemptID string) error {
	f.markEnqueuedCalls++
	return nil
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
func (f *fakeAPIStore) EnsureReplayAttempt(_ context.Context, id, newAttemptID string) (store.ReplayDeadLetterResult, error) {
	f.ensureCalls++
	if f.ensureErr != nil {
		return store.ReplayDeadLetterResult{}, f.ensureErr
	}
	dl := f.deadLetters[id]
	attempt := f.attempt
	attempt.ID = newAttemptID
	dl.ReplayAttemptID = &attempt.ID
	f.deadLetters[id] = dl
	return store.ReplayDeadLetterResult{DeadLetter: dl, Attempt: attempt}, nil
}
func (f *fakeAPIStore) FinalizeReplayEnqueue(_ context.Context, id, attemptID string) error {
	f.finalizeCalls++
	f.finalizedAttemptID = attemptID
	if f.finalizeErr != nil {
		return f.finalizeErr
	}
	dl := f.deadLetters[id]
	now := time.Unix(200, 0).UTC()
	dl.ReplayedAt = &now
	f.deadLetters[id] = dl
	return nil
}
func (f *fakeAPIStore) GetNotificationByID(context.Context, string) (store.Notification, error) {
	return f.notification, nil
}

type fakeDispatchQueue struct {
	jobs []queue.DispatchJob
	err  error
}

func (f *fakeDispatchQueue) EnqueueDispatch(_ context.Context, job queue.DispatchJob) error {
	if f.err != nil {
		return f.err
	}
	f.jobs = append(f.jobs, job)
	return nil
}

func newDeadLetterTestAPI() (*fakeAPIStore, *fakeDispatchQueue, *API) {
	st := &fakeAPIStore{deadLetters: map[string]store.DeadLetter{"dead-1": {ID: "dead-1", NotificationID: "notif-1", Channel: "email", FinalError: "smtp down", DeadLetteredAt: time.Unix(100, 0).UTC()}}, attempt: store.DeliveryAttempt{ID: "attempt-old", NotificationID: "notif-1", Channel: "email", AttemptNumber: 4, Status: "pending"}, notification: store.Notification{ID: "notif-1", TenantID: "tenant-1"}}
	q := &fakeDispatchQueue{}
	return st, q, NewAPI(st, q)
}

func TestDeadLetterHandlersListGetReplay(t *testing.T) {
	st, q, api := newDeadLetterTestAPI()
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
	t.Run("replay_success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/dead-letters/dead-1/replay", bytes.NewReader(nil))
		req.SetPathValue("id", "dead-1")
		res := httptest.NewRecorder()
		api.ReplayDeadLetter().ServeHTTP(res, req)
		if res.Code != http.StatusAccepted {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
		if len(q.jobs) != 1 || q.jobs[0].AttemptID != replayAttemptID("dead-1") {
			t.Fatalf("jobs=%+v", q.jobs)
		}
		if st.ensureCalls != 1 || st.finalizeCalls != 1 {
			t.Fatalf("ensure=%d finalize=%d", st.ensureCalls, st.finalizeCalls)
		}
	})
}

func TestReplayDeadLetterEnqueueFailureLeavesRecoverableAttempt(t *testing.T) {
	st, q, api := newDeadLetterTestAPI()
	q.err = errors.New("redis down")
	req := httptest.NewRequest(http.MethodPost, "/v1/dead-letters/dead-1/replay", bytes.NewReader(nil))
	req.SetPathValue("id", "dead-1")
	res := httptest.NewRecorder()
	api.ReplayDeadLetter().ServeHTTP(res, req)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if st.ensureCalls != 1 || st.finalizeCalls != 0 {
		t.Fatalf("ensure=%d finalize=%d", st.ensureCalls, st.finalizeCalls)
	}
	if st.deadLetters["dead-1"].ReplayedAt != nil {
		t.Fatal("replayed_at should remain nil")
	}
	if st.deadLetters["dead-1"].ReplayAttemptID == nil {
		t.Fatal("replay attempt should remain durable and recoverable")
	}
}

func TestCreateNotificationMarksInitialAttemptEnqueued(t *testing.T) {
	st, q, api := newDeadLetterTestAPI()
	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-1","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"user@example.test","variables":{}}`)))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if st.createdAttempt == nil || st.createdAttempt.EnqueueKind != "initial" {
		t.Fatalf("createdAttempt=%+v", st.createdAttempt)
	}
	if st.ensureInitialCalls != 1 {
		t.Fatalf("ensureInitialCalls=%d", st.ensureInitialCalls)
	}
	if st.markEnqueuedCalls != 1 {
		t.Fatalf("markEnqueuedCalls=%d", st.markEnqueuedCalls)
	}
	if len(q.jobs) != 1 {
		t.Fatalf("jobs=%d", len(q.jobs))
	}
}

func TestIdempotentRetryRecoversPendingInitialAttempt(t *testing.T) {
	st, q, api := newDeadLetterTestAPI()
	key := "stable-key"
	first := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-1","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"user@example.test","variables":{},"idempotency_key":"`+key+`"}`)))
	first.Header.Set("Content-Type", "application/json")
	q.err = errors.New("redis down")
	firstRes := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(firstRes, first)
	if firstRes.Code != http.StatusAccepted {
		t.Fatalf("first status=%d body=%s", firstRes.Code, firstRes.Body.String())
	}
	if st.markEnqueuedCalls != 0 {
		t.Fatalf("markEnqueuedCalls=%d", st.markEnqueuedCalls)
	}
	q.err = nil
	second := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-2","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"user@example.test","variables":{},"idempotency_key":"`+key+`"}`)))
	second.Header.Set("Content-Type", "application/json")
	secondRes := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(secondRes, second)
	if secondRes.Code != http.StatusAccepted {
		t.Fatalf("second status=%d body=%s", secondRes.Code, secondRes.Body.String())
	}
	if st.createdNotification == nil || st.createdNotification.ID != "notif-1" {
		t.Fatalf("createdNotification=%+v", st.createdNotification)
	}
	if st.createNotifyCalls != 1 {
		t.Fatalf("createNotifyCalls=%d", st.createNotifyCalls)
	}
	if st.createAttemptCalls != 1 {
		t.Fatalf("createAttemptCalls=%d", st.createAttemptCalls)
	}
	if st.ensureInitialCalls != 1 {
		t.Fatalf("ensureInitialCalls=%d", st.ensureInitialCalls)
	}
	if st.markEnqueuedCalls != 1 {
		t.Fatalf("markEnqueuedCalls=%d", st.markEnqueuedCalls)
	}
	if len(q.jobs) != 1 {
		t.Fatalf("jobs=%d", len(q.jobs))
	}
}

func TestIdempotentConflictRecoversPendingInitialAttempt(t *testing.T) {
	st, q, api := newDeadLetterTestAPI()
	key := "stable-key"
	first := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-1","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"user@example.test","variables":{},"idempotency_key":"`+key+`"}`)))
	first.Header.Set("Content-Type", "application/json")
	q.err = errors.New("redis down")
	firstRes := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(firstRes, first)
	if firstRes.Code != http.StatusAccepted {
		t.Fatalf("first status=%d body=%s", firstRes.Code, firstRes.Body.String())
	}

	st.fallbackExisting = &store.Notification{ID: "notif-1", TenantID: "tenant-1"}
	st.fallbackAfterLookup = 3
	st.existingByKey = nil
	st.createNotificationErr = store.ErrConflict
	q.err = nil
	second := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-2","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"user@example.test","variables":{},"idempotency_key":"`+key+`"}`)))
	second.Header.Set("Content-Type", "application/json")
	secondRes := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(secondRes, second)
	if secondRes.Code != http.StatusAccepted {
		t.Fatalf("second status=%d body=%s", secondRes.Code, secondRes.Body.String())
	}
	if st.createNotifyCalls != 2 {
		t.Fatalf("createNotifyCalls=%d", st.createNotifyCalls)
	}
	if st.createAttemptCalls != 1 {
		t.Fatalf("createAttemptCalls=%d", st.createAttemptCalls)
	}
	if st.ensureInitialCalls != 1 {
		t.Fatalf("ensureInitialCalls=%d", st.ensureInitialCalls)
	}
	if st.markEnqueuedCalls != 1 {
		t.Fatalf("markEnqueuedCalls=%d", st.markEnqueuedCalls)
	}
	if len(q.jobs) != 1 {
		t.Fatalf("jobs=%d", len(q.jobs))
	}
}

func TestIdempotentRetryRepairsMissingInitialAttempt(t *testing.T) {
	st, q, api := newDeadLetterTestAPI()
	key := "stable-key"
	existing := store.Notification{ID: "notif-1", TenantID: "tenant-1", TemplateID: "tpl-1"}
	st.existingByKey = map[string]store.Notification{"tenant-1/" + key: existing}
	st.initialAttemptMissing = true

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-2","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"user@example.test","variables":{},"idempotency_key":"`+key+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if st.createNotifyCalls != 0 {
		t.Fatalf("createNotifyCalls=%d", st.createNotifyCalls)
	}
	if st.ensureInitialCalls != 1 {
		t.Fatalf("ensureInitialCalls=%d", st.ensureInitialCalls)
	}
	if st.createAttemptCalls != 1 {
		t.Fatalf("createAttemptCalls=%d", st.createAttemptCalls)
	}
	if st.createdAttempt == nil || st.createdAttempt.NotificationID != "notif-1" {
		t.Fatalf("createdAttempt=%+v", st.createdAttempt)
	}
	if len(q.jobs) != 1 || q.jobs[0].AttemptID != st.createdAttempt.ID {
		t.Fatalf("jobs=%+v createdAttempt=%+v", q.jobs, st.createdAttempt)
	}
}

func TestIdempotentRetryReturnsExistingNotificationWhenInitialAlreadyEnqueued(t *testing.T) {
	st, q, api := newDeadLetterTestAPI()
	key := "stable-key"
	existing := store.Notification{ID: "notif-1", TenantID: "tenant-1", TemplateID: "tpl-1"}
	st.existingByKey = map[string]store.Notification{"tenant-1/" + key: existing}
	st.createdAttempt = &store.CreateDeliveryAttemptParams{ID: "attempt-1", NotificationID: "notif-1", Channel: "email", AttemptNumber: 1, Status: "pending", EnqueueKind: "initial"}
	st.markEnqueuedCalls = 1

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-2","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"user@example.test","variables":{},"idempotency_key":"`+key+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if st.ensureInitialCalls != 0 {
		t.Fatalf("ensureInitialCalls=%d", st.ensureInitialCalls)
	}
	if st.createNotifyCalls != 0 {
		t.Fatalf("createNotifyCalls=%d", st.createNotifyCalls)
	}
	if len(q.jobs) != 0 {
		t.Fatalf("jobs=%d", len(q.jobs))
	}
}

func TestSecondIdempotentRequestRepairsMissingInitialAttemptAfterEnsureFailure(t *testing.T) {
	st, q, api := newDeadLetterTestAPI()
	key := "stable-key"
	st.ensureInitialErr = errors.New("db blip")
	first := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-1","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"user@example.test","variables":{},"idempotency_key":"`+key+`"}`)))
	first.Header.Set("Content-Type", "application/json")
	firstRes := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(firstRes, first)
	if firstRes.Code != http.StatusInternalServerError {
		t.Fatalf("first status=%d body=%s", firstRes.Code, firstRes.Body.String())
	}
	if st.createNotifyCalls != 1 {
		t.Fatalf("createNotifyCalls=%d", st.createNotifyCalls)
	}
	if st.createAttemptCalls != 0 {
		t.Fatalf("createAttemptCalls=%d", st.createAttemptCalls)
	}

	st.ensureInitialErr = nil
	st.initialAttemptMissing = true
	second := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-2","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"user@example.test","variables":{},"idempotency_key":"`+key+`"}`)))
	second.Header.Set("Content-Type", "application/json")
	secondRes := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(secondRes, second)
	if secondRes.Code != http.StatusAccepted {
		t.Fatalf("second status=%d body=%s", secondRes.Code, secondRes.Body.String())
	}
	if st.createNotifyCalls != 1 {
		t.Fatalf("createNotifyCalls=%d", st.createNotifyCalls)
	}
	if st.ensureInitialCalls != 2 {
		t.Fatalf("ensureInitialCalls=%d", st.ensureInitialCalls)
	}
	if st.createAttemptCalls != 1 {
		t.Fatalf("createAttemptCalls=%d", st.createAttemptCalls)
	}
	if len(q.jobs) != 1 {
		t.Fatalf("jobs=%d", len(q.jobs))
	}
}
