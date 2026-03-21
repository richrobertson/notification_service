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
		w.Header().Set("X-Request-Id", "abc-123")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	sender := NewWebhookSender(2 * time.Second)
	providerID, err := sender.Send(context.Background(), WebhookRequest{URL: server.URL, Body: `{"ok":true}`})
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
