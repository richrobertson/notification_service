package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/richrobertson/notification-platform/internal/queue"
)

type fakeLimiter struct {
	allowed bool
	retry   time.Duration
}

func (f fakeLimiter) Allow(context.Context, string) (bool, time.Duration, error) {
	return f.allowed, f.retry, nil
}

type fakeMonitor struct{ snapshot queue.PressureSnapshot }

func (f *fakeMonitor) Snapshot(context.Context) (queue.PressureSnapshot, error) {
	return f.snapshot, nil
}
func (f *fakeMonitor) IncRateLimited(string)      {}
func (f *fakeMonitor) IncRejected(string, string) {}

func TestCreateNotificationRateLimited(t *testing.T) {
	st := &fakeAPIStore{}
	api := NewAPI(st, &fakeDispatchQueue{}, fakeLimiter{allowed: false, retry: 3 * time.Second}, &fakeMonitor{})
	body := []byte(`{"id":"n1","tenant_id":"tenant-1","template_id":"tmpl-1","recipient_email":"a@example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(body))
	res := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Retry-After"); got != "3" {
		t.Fatalf("retry-after=%q", got)
	}
}

func TestCreateNotificationBackpressureRejects(t *testing.T) {
	st := &fakeAPIStore{}
	api := NewAPI(st, &fakeDispatchQueue{}, fakeLimiter{allowed: true}, &fakeMonitor{snapshot: queue.PressureSnapshot{Depths: map[string]int{queue.DispatchQueueName: 10}, SoftLimit: 5, HardLimit: 10, RetryAfter: 2 * time.Second}})
	body := []byte(`{"id":"n1","tenant_id":"tenant-1","template_id":"tmpl-1","recipient_email":"a@example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(body))
	res := httptest.NewRecorder()
	api.CreateNotification().ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}
