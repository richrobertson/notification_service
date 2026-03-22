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
	deadLetters            map[string]store.DeadLetter
	attempt                store.DeliveryAttempt
	notification           store.Notification
	ensureCalls            int
	ensureInitialCalls     int
	createAttemptCalls     int
	createNotifyCalls      int
	createdAttempt         *store.CreateDeliveryAttemptParams
	createdIntent          *store.CreateDispatchIntentParams
	createdNotification    *store.CreateNotificationParams
	ensureErr              error
	existingByKey          map[string]store.Notification
	createNotificationErr  error
	fallbackExisting       *store.Notification
	lookupCalls            int
	fallbackAfterLookup    int
	initialAttemptMissing  bool
	ensureInitialErr       error
	templates              map[string]store.Template
	attemptsByNotification map[string][]store.DeliveryAttempt
	recalculateCalls       int
	recalculateErr         error
	recalculatedIDs        []string
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
	if f.templates != nil {
		if tpl, ok := f.templates[id]; ok {
			return tpl, nil
		}
	}
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

func (f *fakeAPIStore) CreateNotificationWithInitialDispatch(_ context.Context, params store.CreateNotificationDispatchParams) (store.Notification, store.DeliveryAttempt, store.DispatchIntent, error) {
	notification, err := f.CreateNotification(context.Background(), params.Notification)
	if err != nil {
		return store.Notification{}, store.DeliveryAttempt{}, store.DispatchIntent{}, err
	}
	attemptParams := store.CreateDeliveryAttemptParams{
		ID:             params.AttemptID,
		NotificationID: notification.ID,
		Channel:        params.Channel,
		AttemptNumber:  1,
		Status:         "pending",
		EnqueueKind:    "initial",
	}
	f.createdAttempt = &attemptParams
	f.createAttemptCalls++
	intentParams := store.CreateDispatchIntentParams{
		ID:             params.IntentID,
		NotificationID: notification.ID,
		AttemptID:      params.AttemptID,
		TenantID:       notification.TenantID,
		Channel:        params.Channel,
		Source:         "initial",
	}
	f.createdIntent = &intentParams
	f.notification = notification
	f.notification.Status = "processing"
	f.recalculateCalls++
	f.recalculatedIDs = append(f.recalculatedIDs, notification.ID)
	if f.recalculateErr != nil {
		return store.Notification{}, store.DeliveryAttempt{}, store.DispatchIntent{}, f.recalculateErr
	}
	return notification, store.DeliveryAttempt{ID: params.AttemptID, NotificationID: notification.ID, Channel: params.Channel, AttemptNumber: 1, Status: "pending", EnqueueKind: "initial"}, store.DispatchIntent{ID: params.IntentID, NotificationID: notification.ID, AttemptID: params.AttemptID, TenantID: notification.TenantID, Channel: params.Channel, Source: "initial", Status: "pending"}, nil
}

func (f *fakeAPIStore) GetInitialAttemptByNotificationID(_ context.Context, notificationID string) (store.DeliveryAttempt, error) {
	if f.initialAttemptMissing {
		return store.DeliveryAttempt{}, store.ErrNotFound
	}
	if f.createdAttempt != nil && f.createdAttempt.NotificationID == notificationID {
		attempt := store.DeliveryAttempt{ID: f.createdAttempt.ID, NotificationID: notificationID, Channel: f.createdAttempt.Channel, AttemptNumber: f.createdAttempt.AttemptNumber, Status: f.createdAttempt.Status, EnqueueKind: f.createdAttempt.EnqueueKind}
		if f.createdAttempt.DispatchEnqueuedAt != nil {
			attempt.DispatchEnqueuedAt = f.createdAttempt.DispatchEnqueuedAt
		}
		return attempt, nil
	}
	return store.DeliveryAttempt{}, store.ErrNotFound
}

func (f *fakeAPIStore) EnsureInitialAttempt(_ context.Context, notificationID, channel, attemptID, intentID string) (store.DeliveryAttempt, store.DispatchIntent, error) {
	f.ensureInitialCalls++
	if f.ensureInitialErr != nil {
		return store.DeliveryAttempt{}, store.DispatchIntent{}, f.ensureInitialErr
	}
	created := false
	if f.createdAttempt == nil || f.createdAttempt.NotificationID != notificationID {
		params := store.CreateDeliveryAttemptParams{ID: attemptID, NotificationID: notificationID, Channel: channel, AttemptNumber: 1, Status: "pending", EnqueueKind: "initial"}
		f.createAttemptCalls++
		f.createdAttempt = &params
		f.createdIntent = &store.CreateDispatchIntentParams{ID: intentID, NotificationID: notificationID, AttemptID: attemptID, TenantID: f.notification.TenantID, Channel: channel, Source: "initial"}
		f.notification.ID = notificationID
		f.notification.Status = "processing"
		created = true
	} else if f.createdIntent == nil {
		f.createdIntent = &store.CreateDispatchIntentParams{ID: intentID, NotificationID: notificationID, AttemptID: f.createdAttempt.ID, TenantID: f.notification.TenantID, Channel: channel, Source: "initial"}
	}
	if created {
		f.recalculateCalls++
		f.recalculatedIDs = append(f.recalculatedIDs, notificationID)
		if f.recalculateErr != nil {
			return store.DeliveryAttempt{}, store.DispatchIntent{}, f.recalculateErr
		}
	}
	f.initialAttemptMissing = false
	attempt, err := f.GetInitialAttemptByNotificationID(context.Background(), notificationID)
	if err != nil {
		return store.DeliveryAttempt{}, store.DispatchIntent{}, err
	}
	return attempt, store.DispatchIntent{ID: f.createdIntent.ID, NotificationID: notificationID, AttemptID: attempt.ID, TenantID: f.notification.TenantID, Channel: channel, Source: "initial", Status: "pending"}, nil
}
func (f *fakeAPIStore) CreateDeliveryAttempt(_ context.Context, params store.CreateDeliveryAttemptParams) (store.DeliveryAttempt, error) {
	f.createAttemptCalls++
	f.createdAttempt = &params
	return store.DeliveryAttempt{ID: params.ID, NotificationID: params.NotificationID, Channel: params.Channel, AttemptNumber: params.AttemptNumber, Status: params.Status, EnqueueKind: params.EnqueueKind}, nil
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
	created := dl.ReplayAttemptID == nil
	dl.ReplayAttemptID = &attempt.ID
	f.deadLetters[id] = dl
	f.createdIntent = &store.CreateDispatchIntentParams{ID: "intent-" + attempt.ID, NotificationID: attempt.NotificationID, AttemptID: attempt.ID, TenantID: f.notification.TenantID, Channel: attempt.Channel, Source: "replay"}
	f.notification.ID = dl.NotificationID
	f.notification.Status = "processing"
	if created {
		f.recalculateCalls++
		f.recalculatedIDs = append(f.recalculatedIDs, dl.NotificationID)
		if f.recalculateErr != nil {
			return store.ReplayDeadLetterResult{}, f.recalculateErr
		}
	}
	return store.ReplayDeadLetterResult{DeadLetter: dl, Attempt: attempt}, nil
}
func (f *fakeAPIStore) RecalculateNotificationStatus(_ context.Context, notificationID string) error {
	f.recalculateCalls++
	f.recalculatedIDs = append(f.recalculatedIDs, notificationID)
	return f.recalculateErr
}
func (f *fakeAPIStore) GetNotificationByID(context.Context, string) (store.Notification, error) {
	return f.notification, nil
}
func (f *fakeAPIStore) GetDeliveryAttemptByID(_ context.Context, id string) (store.DeliveryAttempt, error) {
	for _, attempts := range f.attemptsByNotification {
		for _, attempt := range attempts {
			if attempt.ID == id {
				return attempt, nil
			}
		}
	}
	if f.createdAttempt != nil && f.createdAttempt.ID == id {
		return store.DeliveryAttempt{ID: id, NotificationID: f.createdAttempt.NotificationID, Channel: f.createdAttempt.Channel, AttemptNumber: f.createdAttempt.AttemptNumber, Status: f.createdAttempt.Status, EnqueueKind: f.createdAttempt.EnqueueKind}, nil
	}
	return store.DeliveryAttempt{}, store.ErrNotFound
}
func (f *fakeAPIStore) ListDeliveryAttemptsByNotificationID(_ context.Context, notificationID string) ([]store.DeliveryAttempt, error) {
	if attempts, ok := f.attemptsByNotification[notificationID]; ok {
		return attempts, nil
	}
	if f.createdAttempt != nil && f.createdAttempt.NotificationID == notificationID {
		return []store.DeliveryAttempt{{ID: f.createdAttempt.ID, NotificationID: notificationID, Channel: f.createdAttempt.Channel, AttemptNumber: f.createdAttempt.AttemptNumber, Status: f.createdAttempt.Status, EnqueueKind: f.createdAttempt.EnqueueKind}}, nil
	}
	return []store.DeliveryAttempt{}, nil
}
func (f *fakeAPIStore) RecordAuditEvent(context.Context, string, string, string, string, string, string, map[string]any) error {
	return nil
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

func assertNotificationInspectionStatus(t *testing.T, api *API, notificationID, wantStatus string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications/"+notificationID, nil)
	req.SetPathValue("id", notificationID)
	res := httptest.NewRecorder()
	api.GetNotification().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Notification store.Notification `json:"notification"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Notification.Status != wantStatus {
		t.Fatalf("notification status=%q", payload.Notification.Status)
	}
}

func assertStatusRefresh(t *testing.T, st *fakeAPIStore, wantID string) {
	t.Helper()

	if st.recalculateCalls != 1 || len(st.recalculatedIDs) != 1 || st.recalculatedIDs[0] != wantID {
		t.Fatalf("recalculateCalls=%d recalculatedIDs=%v", st.recalculateCalls, st.recalculatedIDs)
	}
}

func newDeadLetterTestAPI() (*fakeAPIStore, *fakeDispatchQueue, *API) {
	st := &fakeAPIStore{
		deadLetters:  map[string]store.DeadLetter{"dead-1": {ID: "dead-1", NotificationID: "notif-1", Channel: "email", FinalError: "smtp down", DeadLetteredAt: time.Unix(100, 0).UTC()}},
		attempt:      store.DeliveryAttempt{ID: "attempt-old", NotificationID: "notif-1", Channel: "email", AttemptNumber: 4, Status: "pending"},
		notification: store.Notification{ID: "notif-1", TenantID: "tenant-1"},
		templates: map[string]store.Template{
			"tpl-1": {ID: "tpl-1", TenantID: "tenant-1", Channel: "email", Name: "welcome"},
			"tpl-2": {ID: "tpl-2", TenantID: "tenant-1", Channel: "webhook", Name: "webhook"},
		},
		attemptsByNotification: map[string][]store.DeliveryAttempt{
			"notif-1": {{ID: "attempt-old", NotificationID: "notif-1", Channel: "email", AttemptNumber: 4, Status: "pending", EnqueueKind: "initial"}},
		},
	}
	q := &fakeDispatchQueue{}
	return st, q, NewAPI(st, q, nil, nil)
}

func TestDeadLetterHandlersListGetReplay(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
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
		if st.ensureCalls != 1 {
			t.Fatalf("ensure=%d", st.ensureCalls)
		}
		if st.createdIntent == nil || st.createdIntent.Source != "replay" {
			t.Fatalf("createdIntent=%+v", st.createdIntent)
		}
	})
}

func TestReplayDeadLetterCreatesRecoverableDispatchIntent(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	req := httptest.NewRequest(http.MethodPost, "/v1/dead-letters/dead-1/replay", bytes.NewReader(nil))
	req.SetPathValue("id", "dead-1")
	res := httptest.NewRecorder()
	api.ReplayDeadLetter().ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if st.ensureCalls != 1 {
		t.Fatalf("ensure=%d", st.ensureCalls)
	}
	if st.deadLetters["dead-1"].ReplayedAt != nil {
		t.Fatal("replayed_at should remain nil")
	}
	if st.deadLetters["dead-1"].ReplayAttemptID == nil {
		t.Fatal("replay attempt should remain durable and recoverable")
	}
}

func TestCreateNotificationUpdatesInspectionStatusWhenAttemptIsPending(t *testing.T) {
	_, _, api := newDeadLetterTestAPI()

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-1","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"user@example.test","variables":{}}`)))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/notifications/notif-1", nil)
	getReq.SetPathValue("id", "notif-1")
	getRes := httptest.NewRecorder()
	api.GetNotification().ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRes.Code, getRes.Body.String())
	}
	var payload struct {
		Notification store.Notification `json:"notification"`
	}
	if err := json.NewDecoder(getRes.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Notification.Status != "processing" {
		t.Fatalf("notification status=%q", payload.Notification.Status)
	}
	assertNotificationInspectionStatus(t, api, "notif-1", "processing")
}

func TestReplayDeadLetterUpdatesInspectionStatusWhenReplayAttemptIsPending(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	st.notification.Status = "dead_lettered"

	req := httptest.NewRequest(http.MethodPost, "/v1/dead-letters/dead-1/replay", bytes.NewReader(nil))
	req.SetPathValue("id", "dead-1")
	res := httptest.NewRecorder()
	api.ReplayDeadLetter().ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/notifications/notif-1", nil)
	getReq.SetPathValue("id", "notif-1")
	getRes := httptest.NewRecorder()
	api.GetNotification().ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRes.Code, getRes.Body.String())
	}
	var payload struct {
		Notification store.Notification `json:"notification"`
	}
	if err := json.NewDecoder(getRes.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Notification.Status != "processing" {
		t.Fatalf("notification status=%q", payload.Notification.Status)
	}
	assertNotificationInspectionStatus(t, api, "notif-1", "processing")
}

func TestReplayDeadLetterReturns500WhenStatusRefreshFails(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	st.recalculateErr = errors.New("refresh failed")
	req := httptest.NewRequest(http.MethodPost, "/v1/dead-letters/dead-1/replay", bytes.NewReader(nil))
	req.SetPathValue("id", "dead-1")
	res := httptest.NewRecorder()
	api.ReplayDeadLetter().ServeHTTP(res, req)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestCreateNotificationCreatesInitialDispatchIntent(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
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
	if st.ensureInitialCalls != 0 {
		t.Fatalf("ensureInitialCalls=%d", st.ensureInitialCalls)
	}
	if st.createdIntent == nil || st.createdIntent.Source != "initial" {
		t.Fatalf("createdIntent=%+v", st.createdIntent)
	}
	assertStatusRefresh(t, st, "notif-1")
}

func TestCreateNotificationReturns500WhenStatusRefreshFails(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	st.recalculateErr = errors.New("refresh failed")
	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-1","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"user@example.test","variables":{}}`)))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(res, req)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
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
	assertStatusRefresh(t, st, "notif-1")
	if st.createdIntent == nil || st.createdIntent.Source != "initial" {
		t.Fatalf("createdIntent=%+v", st.createdIntent)
	}
}

func TestInspectionEndpoints(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	st.notification = store.Notification{ID: "notif-1", TenantID: "tenant-1", TemplateID: "tpl-1", Status: "processing"}
	st.attemptsByNotification["notif-1"] = []store.DeliveryAttempt{
		{ID: "attempt-1", NotificationID: "notif-1", Channel: "email", AttemptNumber: 1, Status: "retry_scheduled", EnqueueKind: "initial"},
		{ID: "attempt-2", NotificationID: "notif-1", Channel: "email", AttemptNumber: 2, Status: "pending", EnqueueKind: "retry"},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications/notif-1", nil)
	req.SetPathValue("id", "notif-1")
	res := httptest.NewRecorder()
	api.GetNotification().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/notifications/notif-1/attempts", nil)
	req.SetPathValue("id", "notif-1")
	res = httptest.NewRecorder()
	api.ListNotificationAttempts().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/attempts/attempt-2", nil)
	req.SetPathValue("id", "attempt-2")
	res = httptest.NewRecorder()
	api.GetAttempt().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
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
	assertStatusRefresh(t, st, "notif-1")
}

func TestIdempotentRetryRepairsMissingInitialAttempt(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
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
	if st.createdAttempt.Channel != "email" {
		t.Fatalf("createdAttempt.Channel=%s", st.createdAttempt.Channel)
	}
	if st.createdIntent == nil || st.createdIntent.AttemptID != st.createdAttempt.ID {
		t.Fatalf("createdIntent=%+v createdAttempt=%+v", st.createdIntent, st.createdAttempt)
	}
}

func TestIdempotentRetryWithChangedRequestTemplateRepairsUsingStoredTemplateChannel(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	key := "stable-key"
	existing := store.Notification{ID: "notif-1", TenantID: "tenant-1", TemplateID: "tpl-1"}
	st.existingByKey = map[string]store.Notification{"tenant-1/" + key: existing}
	st.initialAttemptMissing = true

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-2","tenant_id":"tenant-1","template_id":"tpl-2","recipient_webhook_url":"https://example.test/hook","variables":{},"idempotency_key":"`+key+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if st.ensureInitialCalls != 1 {
		t.Fatalf("ensureInitialCalls=%d", st.ensureInitialCalls)
	}
	if st.createdAttempt == nil {
		t.Fatal("createdAttempt=nil")
	}
	if st.createdAttempt.Channel != "email" {
		t.Fatalf("createdAttempt.Channel=%s", st.createdAttempt.Channel)
	}
	if st.createdIntent == nil || st.createdIntent.Channel != "email" {
		t.Fatalf("createdIntent=%+v", st.createdIntent)
	}
}

func TestIdempotentRetryReturnsExistingNotificationWhenInitialAlreadyEnqueued(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	key := "stable-key"
	existing := store.Notification{ID: "notif-1", TenantID: "tenant-1", TemplateID: "tpl-1"}
	st.existingByKey = map[string]store.Notification{"tenant-1/" + key: existing}
	enqueuedAt := time.Unix(300, 0).UTC()
	st.createdAttempt = &store.CreateDeliveryAttemptParams{ID: "attempt-1", NotificationID: "notif-1", Channel: "email", AttemptNumber: 1, Status: "pending", DispatchEnqueuedAt: &enqueuedAt, EnqueueKind: "initial"}
	st.createdIntent = nil

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-2","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"user@example.test","variables":{},"idempotency_key":"`+key+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if st.ensureInitialCalls != 1 {
		t.Fatalf("ensureInitialCalls=%d", st.ensureInitialCalls)
	}
	if st.createNotifyCalls != 0 {
		t.Fatalf("createNotifyCalls=%d", st.createNotifyCalls)
	}
}

func TestIdempotentRetryWithChangedRequestTemplateReusesPendingStoredChannelAttempt(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	key := "stable-key"
	existing := store.Notification{ID: "notif-1", TenantID: "tenant-1", TemplateID: "tpl-1"}
	st.existingByKey = map[string]store.Notification{"tenant-1/" + key: existing}
	st.createdAttempt = &store.CreateDeliveryAttemptParams{ID: "attempt-1", NotificationID: "notif-1", Channel: "email", AttemptNumber: 1, Status: "pending", EnqueueKind: "initial"}
	st.createdIntent = &store.CreateDispatchIntentParams{ID: "intent-attempt-1", NotificationID: "notif-1", AttemptID: "attempt-1", TenantID: "tenant-1", Channel: "email", Source: "initial"}

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-2","tenant_id":"tenant-1","template_id":"tpl-2","recipient_webhook_url":"https://example.test/hook","variables":{},"idempotency_key":"`+key+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if st.ensureInitialCalls != 1 {
		t.Fatalf("ensureInitialCalls=%d", st.ensureInitialCalls)
	}
	if st.createdIntent == nil || st.createdIntent.Channel != "email" {
		t.Fatalf("createdIntent=%+v", st.createdIntent)
	}
}

func TestSecondIdempotentRequestRepairsMissingInitialAttempt(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	key := "stable-key"
	first := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte(`{"id":"notif-1","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"user@example.test","variables":{},"idempotency_key":"`+key+`"}`)))
	first.Header.Set("Content-Type", "application/json")
	firstRes := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(firstRes, first)
	if firstRes.Code != http.StatusAccepted {
		t.Fatalf("first status=%d body=%s", firstRes.Code, firstRes.Body.String())
	}
	if st.createNotifyCalls != 1 {
		t.Fatalf("createNotifyCalls=%d", st.createNotifyCalls)
	}
	if st.createAttemptCalls != 1 {
		t.Fatalf("createAttemptCalls=%d", st.createAttemptCalls)
	}

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
	if st.ensureInitialCalls != 1 {
		t.Fatalf("ensureInitialCalls=%d", st.ensureInitialCalls)
	}
	if st.createAttemptCalls != 1 {
		t.Fatalf("createAttemptCalls=%d", st.createAttemptCalls)
	}
}
