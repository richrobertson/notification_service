package delivery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebhookSenderSendSuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}
		if got := r.Header.Get("Idempotency-Key"); got != "attempt-1" {
			t.Fatalf("Idempotency-Key = %q", got)
		}
		if got := r.Header.Get("X-Notification-Attempt-ID"); got != "attempt-1" {
			t.Fatalf("X-Notification-Attempt-ID = %q", got)
		}
		if got := r.Header.Get("X-Notification-ID"); got != "notif-1" {
			t.Fatalf("X-Notification-ID = %q", got)
		}
		w.Header().Set("X-Request-Id", "abc-123")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	sender := NewWebhookSender(2 * time.Second)
	providerID, err := sender.Send(context.Background(), WebhookRequest{URL: server.URL, Body: `{"ok":true}`, AttemptID: "attempt-1", NotificationID: "notif-1"})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if providerID != "abc-123" {
		t.Fatalf("providerID = %q, want abc-123", providerID)
	}
}

func TestWebhookSenderSendFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad downstream", http.StatusBadGateway)
	}))
	defer server.Close()

	sender := NewWebhookSender(2 * time.Second)
	_, err := sender.Send(context.Background(), WebhookRequest{URL: server.URL, Body: `hello`})
	if err == nil {
		t.Fatal("Send() error = nil, want failure")
	}
	want := "webhook returned 502"
	if got := err.Error(); len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("Send() error = %q, want prefix %q", got, want)
	}
}

func TestContentTypeForBody(t *testing.T) {
	t.Parallel()
	cases := map[string]string{`{"x":1}`: "application/json", " hi ": "text/plain; charset=utf-8"}
	for body, want := range cases {
		if got := contentTypeForBody(body); got != want {
			t.Fatalf("contentTypeForBody(%q) = %q, want %q", body, got, want)
		}
	}
}

func TestWebhookSenderSanitizesIdentifierHeaders(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Idempotency-Key"); got != "attemptBccbad" {
			t.Fatalf("Idempotency-Key = %q", got)
		}
		if got := r.Header.Get("X-Notification-Attempt-ID"); got != "attemptBccbad" {
			t.Fatalf("X-Notification-Attempt-ID = %q", got)
		}
		if got := r.Header.Get("X-Notification-ID"); got != "notifInjectedbad" {
			t.Fatalf("X-Notification-ID = %q", got)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	sender := NewWebhookSender(2 * time.Second)
	if _, err := sender.Send(context.Background(), WebhookRequest{
		URL:            server.URL,
		Body:           `{"ok":true}`,
		AttemptID:      "attempt\r\nBcc:bad",
		NotificationID: "notif\r\nInjected:bad",
	}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}
