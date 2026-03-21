package delivery

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

func TestRetryDelayExpandsUnderPressure(t *testing.T) {
	to := "to@example.test"
	st := &stubStore{notification: store.Notification{ID: "notif-1", RecipientEmail: &to, Variables: map[string]any{"name": "Ada"}}, template: store.Template{Name: "welcome", Body: "hello {{.name}}"}, attempt: store.DeliveryAttempt{ID: "attempt-1", AttemptNumber: 1}}
	svc, err := NewService(st, stubWebhookSender{}, stubEmailSender{err: &RetryableError{Err: errors.New("smtp down")}}, RetryPolicy{MaxAttempts: 3, BaseDelay: 5 * time.Second, MaxDelay: time.Minute, ExponentialBackoff: true, Now: func() time.Time { return time.Unix(10, 0).UTC() }, IDGenerator: func() string { return "dead-1" }, RandSource: rand.New(rand.NewSource(1)), QueueSoftLimit: 10, PressureMultiplier: 2, PressureMinDelay: 20 * time.Second, QueueDepth: func(string) int { return 10 }})
	if err != nil {
		t.Fatal(err)
	}
	result, _ := svc.ProcessEmail(context.Background(), queue.DispatchJob{AttemptID: "attempt-1", NotificationID: "notif-1", Channel: "email"})
	if result.NextRetryAt == nil {
		t.Fatal("expected next retry")
	}
	if got := result.NextRetryAt.Sub(time.Unix(10, 0).UTC()); got < 20*time.Second {
		t.Fatalf("retry delay=%s", got)
	}
}
