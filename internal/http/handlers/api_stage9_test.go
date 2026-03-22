package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/richrobertson/notification-platform/internal/store"
)

func TestCreateScheduledNotificationDoesNotCreateImmediateIntent(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	when := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	body := []byte(`{"id":"notif-scheduled","tenant_id":"tenant-1","template_id":"tpl-1","recipient_email":"ada@example.test","scheduled_for":"` + when + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(body))
	res := httptest.NewRecorder()

	api.CreateNotification().ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if st.createdIntent != nil {
		t.Fatalf("expected no immediate dispatch intent, got %+v", st.createdIntent)
	}
	if st.notification.Status != "scheduled" {
		t.Fatalf("notification status=%q", st.notification.Status)
	}
	if len(st.auditActions) < 2 || st.auditActions[1] != "notification_scheduled" {
		t.Fatalf("auditActions=%v", st.auditActions)
	}
}

func TestCancelScheduledNotification(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	now := time.Now().UTC().Add(time.Hour)
	st.notification = store.Notification{ID: "notif-1", TenantID: "tenant-1", Status: "scheduled", ScheduledFor: &now}

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications/notif-1/cancel", nil)
	req.SetPathValue("id", "notif-1")
	res := httptest.NewRecorder()

	api.CancelNotification().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if st.notification.CancelledAt == nil {
		t.Fatal("expected cancelled_at to be set")
	}
	if len(st.auditActions) == 0 || st.auditActions[len(st.auditActions)-1] != "scheduled_notification_cancelled" {
		t.Fatalf("auditActions=%v", st.auditActions)
	}
}

func TestCancelScheduledNotificationNotFound(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	st.cancelErr = store.ErrNotFound

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications/missing/cancel", nil)
	req.SetPathValue("id", "missing")
	res := httptest.NewRecorder()

	api.CancelNotification().ServeHTTP(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestReplayBlockedWhenPolicyDisablesReplay(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	st.resolvedPolicy = store.ResolvedDeliveryPolicy{TenantID: "tenant-1", Channel: "email", SchedulingEnabled: true, ReplayAllowed: false}

	req := httptest.NewRequest(http.MethodPost, "/v1/dead-letters/dead-1/replay", nil)
	req.SetPathValue("id", "dead-1")
	res := httptest.NewRecorder()

	api.ReplayDeadLetter().ServeHTTP(res, req)

	if res.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestPolicyEndpointsPauseAndResume(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	body := []byte(`{"id":"policy-1","tenant_id":"tenant-1","channel":"email","failover_enabled":true,"scheduling_enabled":true,"replay_allowed":true}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/policies", bytes.NewReader(body))
	res := httptest.NewRecorder()
	api.UpsertPolicy().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("upsert status=%d body=%s", res.Code, res.Body.String())
	}

	pauseReq := httptest.NewRequest(http.MethodPost, "/v1/policies/policy-1/pause", nil)
	pauseReq.SetPathValue("id", "policy-1")
	pauseRes := httptest.NewRecorder()
	api.PausePolicy().ServeHTTP(pauseRes, pauseReq)
	if pauseRes.Code != http.StatusOK {
		t.Fatalf("pause status=%d body=%s", pauseRes.Code, pauseRes.Body.String())
	}
	if policy := st.policies["policy-1"]; policy.Paused == nil || !*policy.Paused {
		t.Fatalf("paused policy=%+v", policy)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/v1/policies/policy-1/resume", nil)
	resumeReq.SetPathValue("id", "policy-1")
	resumeRes := httptest.NewRecorder()
	api.ResumePolicy().ServeHTTP(resumeRes, resumeReq)
	if resumeRes.Code != http.StatusOK {
		t.Fatalf("resume status=%d body=%s", resumeRes.Code, resumeRes.Body.String())
	}
	if policy := st.policies["policy-1"]; policy.Paused == nil || *policy.Paused {
		t.Fatalf("resumed policy=%+v", policy)
	}
}

func TestUpsertPolicyTreatsBlankTenantIDAsGlobal(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	body := []byte(`{"id":"policy-global","tenant_id":"   ","channel":"email","failover_enabled":true}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/policies", bytes.NewReader(body))
	res := httptest.NewRecorder()
	api.UpsertPolicy().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if policy := st.policies["policy-global"]; policy.TenantID != nil {
		t.Fatalf("expected global policy tenant_id to be nil, got %+v", policy.TenantID)
	}
}

func TestListPolicies(t *testing.T) {
	st, _, api := newDeadLetterTestAPI()
	paused := true
	st.policies = map[string]store.DeliveryPolicy{
		"policy-1": {ID: "policy-1", Paused: &paused},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/policies", nil)
	res := httptest.NewRecorder()
	api.ListPolicies().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var policies []store.DeliveryPolicy
	if err := json.NewDecoder(res.Body).Decode(&policies); err != nil {
		t.Fatal(err)
	}
	if len(policies) != 1 || policies[0].ID != "policy-1" {
		t.Fatalf("policies=%+v", policies)
	}
}
