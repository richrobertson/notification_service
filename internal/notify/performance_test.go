package notify

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

func BenchmarkNotificationSubmissionLatency(b *testing.B) {
	server := newTestServer()
	handler := server.Handler()

	doJSONRequest(b, handler, http.MethodPost, "/v1/tenants", map[string]any{
		"id":          "acme",
		"name":        "Acme",
		"daily_quota": b.N + 100,
	})
	doJSONRequest(b, handler, http.MethodPost, "/v1/templates", map[string]any{
		"id":        "benchmark-template",
		"tenant_id": "acme",
		"name":      "Benchmark template",
		"channel":   "email",
		"body":      "Hello {{ name }}",
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		recorder := doJSONRequest(b, handler, http.MethodPost, "/v1/notifications", map[string]any{
			"tenant_id":       "acme",
			"template_id":     "benchmark-template",
			"channels":        []string{"email"},
			"recipient":       map[string]any{"email": "user@example.com"},
			"variables":       map[string]any{"name": "Sam"},
			"idempotency_key": fmt.Sprintf("bench-%d", i),
		})
		if recorder.Code != http.StatusAccepted {
			b.Fatalf("The benchmarked submission path should stay functional while measuring latency: expected HTTP 202 but received HTTP %d", recorder.Code)
		}
	}
}

func TestSubmissionPathStaysLowLatencyForTheMVP(t *testing.T) {
	server := newTestServer()
	handler := server.Handler()

	doJSONRequest(t, handler, http.MethodPost, "/v1/tenants", map[string]any{
		"id":          "acme",
		"name":        "Acme",
		"daily_quota": 2000,
	})
	doJSONRequest(t, handler, http.MethodPost, "/v1/templates", map[string]any{
		"id":        "performance-template",
		"tenant_id": "acme",
		"name":      "Performance template",
		"channel":   "email",
		"body":      "Hello {{ name }}",
	})

	const requestCount = 250
	var total time.Duration

	for i := 0; i < requestCount; i++ {
		start := time.Now()
		recorder := doJSONRequest(t, handler, http.MethodPost, "/v1/notifications", map[string]any{
			"tenant_id":       "acme",
			"template_id":     "performance-template",
			"channels":        []string{"email"},
			"recipient":       map[string]any{"email": "user@example.com"},
			"variables":       map[string]any{"name": "Sam"},
			"idempotency_key": fmt.Sprintf("perf-%d", i),
		})
		total += time.Since(start)
		expectStatus(t, recorder, http.StatusAccepted, "The performance test should still submit valid notifications successfully")
	}

	average := total / requestCount
	expectTrue(t, average < 5*time.Millisecond, fmt.Sprintf("The low-latency submission path should keep average in-process request time below 5ms for the MVP, but the measured average was %s", average))
}

func TestConcurrentIdempotentSubmissionDoesNotCreateDuplicates(t *testing.T) {
	server := newTestServer()
	handler := server.Handler()

	doJSONRequest(t, handler, http.MethodPost, "/v1/tenants", map[string]any{
		"id":          "acme",
		"name":        "Acme",
		"daily_quota": 100,
	})
	doJSONRequest(t, handler, http.MethodPost, "/v1/templates", map[string]any{
		"id":        "concurrency-template",
		"tenant_id": "acme",
		"name":      "Concurrency template",
		"channel":   "email",
		"body":      "Hello {{ name }}",
	})

	const workers = 16
	results := make(chan Notification, workers)
	statuses := make(chan int, workers)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			recorder := doJSONRequest(t, handler, http.MethodPost, "/v1/notifications", map[string]any{
				"tenant_id":       "acme",
				"template_id":     "concurrency-template",
				"channels":        []string{"email"},
				"recipient":       map[string]any{"email": "user@example.com"},
				"variables":       map[string]any{"name": "Sam"},
				"idempotency_key": "shared-key",
			})
			statuses <- recorder.Code
			results <- decodeBody[Notification](t, recorder)
		}()
	}
	wg.Wait()
	close(results)
	close(statuses)

	var firstID string
	for status := range statuses {
		expectTrue(t, status == http.StatusAccepted || status == http.StatusOK, fmt.Sprintf("Concurrent idempotent submissions should return either HTTP 202 for the winner or HTTP 200 for deduplicated followers, but one request returned HTTP %d", status))
	}
	for notification := range results {
		if firstID == "" {
			firstID = notification.ID
		}
		expectEqual(t, notification.ID, firstID, "Concurrent requests that share an idempotency key should all resolve to the same notification identifier")
	}

	usage := doJSONRequest(t, handler, http.MethodGet, "/v1/tenants/acme/usage", nil)
	expectStatus(t, usage, http.StatusOK, "The usage endpoint should remain available after concurrent submissions")
	usageBody := decodeBody[Usage](t, usage)
	expectEqual(t, usageBody.AcceptedNotifications, 1, "Concurrent idempotent submissions should count as one accepted notification")
}
