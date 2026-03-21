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

type stubStore struct {
	notification     store.Notification
	template         store.Template
	attempt          store.DeliveryAttempt
	loadErr          error
	inProgressErr    error
	sentErr          error
	failedErr        error
	scheduleRetryErr error
	deadLetterErr    error
	insertDeadErr    error
	inProgressCall   int
	failedMsg        string
	scheduledMsg     string
	scheduledAt      *time.Time
	deadLetterMsg    string
	insertedDead     *store.DeadLetter
}

func (s *stubStore) LoadDeliveryJob(context.Context, string, string) (store.Notification, store.Template, store.DeliveryAttempt, error) {
	if s.loadErr != nil {
		return store.Notification{}, store.Template{}, store.DeliveryAttempt{}, s.loadErr
	}
	return s.notification, s.template, s.attempt, nil
}
func (s *stubStore) MarkAttemptInProgress(context.Context, string) error {
	s.inProgressCall++
	return s.inProgressErr
}
func (s *stubStore) MarkAttemptSent(context.Context, string, *string) error { return s.sentErr }
func (s *stubStore) MarkAttemptFailed(context.Context, string, string) error {
	if s.failedErr != nil {
		return s.failedErr
	}
	return nil
}
func (s *stubStore) ScheduleRetry(_ context.Context, _ string, lastError string, nextRetryAt time.Time) error {
	s.scheduledMsg = lastError
	s.scheduledAt = &nextRetryAt
	return s.scheduleRetryErr
}
func (s *stubStore) MarkAttemptDeadLettered(_ context.Context, _ string, lastError string) error {
	s.deadLetterMsg = lastError
	return s.deadLetterErr
}
func (s *stubStore) InsertDeadLetter(_ context.Context, id, notificationID, channel, finalError string) (store.DeadLetter, error) {
	if s.insertDeadErr != nil {
		return store.DeadLetter{}, s.insertDeadErr
	}
	dl := store.DeadLetter{ID: id, NotificationID: notificationID, Channel: channel, FinalError: finalError, DeadLetteredAt: time.Unix(100, 0).UTC()}
	s.insertedDead = &dl
	return dl, nil
}

type stubWebhookSender struct {
	providerID string
	err        error
}

func (s stubWebhookSender) Send(context.Context, WebhookRequest) (string, error) {
	return s.providerID, s.err
}

type stubEmailSender struct{ err error }

func (s stubEmailSender) Send(context.Context, EmailRequest) error { return s.err }

func newTestService(t *testing.T, st *stubStore) *Service {
	t.Helper()
	svc, err := NewService(st, stubWebhookSender{}, stubEmailSender{}, RetryPolicy{MaxAttempts: 3, BaseDelay: 5 * time.Second, MaxDelay: time.Minute, ExponentialBackoff: true, Jitter: 0, Now: func() time.Time { return time.Unix(10, 0).UTC() }, IDGenerator: func() string { return "dead-1" }, RandSource: rand.New(rand.NewSource(1))})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestServiceSchedulesRetryOnTransientEmailFailure(t *testing.T) {
	to := "to@example.test"
	st := &stubStore{notification: store.Notification{ID: "notif-1", RecipientEmail: &to, Variables: map[string]any{"name": "Ada"}}, template: store.Template{Name: "welcome", Body: "hello {{.name}}"}, attempt: store.DeliveryAttempt{ID: "attempt-1", AttemptNumber: 1}}
	svc, err := NewService(st, stubWebhookSender{}, stubEmailSender{err: &RetryableError{Err: errors.New("smtp down")}}, RetryPolicy{MaxAttempts: 3, BaseDelay: 5 * time.Second, MaxDelay: time.Minute, ExponentialBackoff: true, Now: func() time.Time { return time.Unix(10, 0).UTC() }, IDGenerator: func() string { return "dead-1" }, RandSource: rand.New(rand.NewSource(1))})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ProcessEmail(context.Background(), queue.DispatchJob{AttemptID: "attempt-1", NotificationID: "notif-1", Channel: "email"})
	if !IsRetryable(err) {
		t.Fatalf("err retryable=false: %v", err)
	}
	if result.Outcome != OutcomeRetryScheduled {
		t.Fatalf("Outcome=%v", result.Outcome)
	}
	if st.scheduledMsg != "smtp down" {
		t.Fatalf("scheduledMsg=%q", st.scheduledMsg)
	}
	if st.scheduledAt == nil || !st.scheduledAt.Equal(time.Unix(15, 0).UTC()) {
		t.Fatalf("nextRetryAt=%v", st.scheduledAt)
	}
}

func TestServiceDoesNotScheduleRetryOnTerminalFailure(t *testing.T) {
	st := &stubStore{notification: store.Notification{Variables: map[string]any{"name": "Ada"}}, template: store.Template{Body: "hello {{.name}}"}, attempt: store.DeliveryAttempt{ID: "attempt-1", AttemptNumber: 1}}
	svc := newTestService(t, st)
	result, err := svc.ProcessWebhook(context.Background(), queue.DispatchJob{AttemptID: "attempt-1", NotificationID: "notif-1", Channel: "webhook"})
	if !IsTerminal(err) {
		t.Fatalf("expected terminal error, got %v", err)
	}
	if result.Outcome != OutcomeFailedTerminal {
		t.Fatalf("Outcome=%v", result.Outcome)
	}
	if st.scheduledAt != nil {
		t.Fatalf("unexpected retry scheduled: %v", st.scheduledAt)
	}
}

func TestServiceDeadLettersWhenRetryBudgetExhausted(t *testing.T) {
	to := "to@example.test"
	st := &stubStore{notification: store.Notification{ID: "notif-1", RecipientEmail: &to, Variables: map[string]any{"name": "Ada"}}, template: store.Template{Name: "welcome", Body: "hello {{.name}}"}, attempt: store.DeliveryAttempt{ID: "attempt-3", AttemptNumber: 3}}
	svc, err := NewService(st, stubWebhookSender{}, stubEmailSender{err: &RetryableError{Err: errors.New("smtp down")}}, RetryPolicy{MaxAttempts: 3, BaseDelay: 5 * time.Second, MaxDelay: time.Minute, ExponentialBackoff: true, Now: time.Now, IDGenerator: func() string { return "dead-1" }, RandSource: rand.New(rand.NewSource(1))})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ProcessEmail(context.Background(), queue.DispatchJob{AttemptID: "attempt-3", NotificationID: "notif-1", Channel: "email"})
	if !IsRetryable(err) {
		t.Fatalf("expected retryable error, got %v", err)
	}
	if result.Outcome != OutcomeDeadLettered {
		t.Fatalf("Outcome=%v", result.Outcome)
	}
	if st.deadLetterMsg != "smtp down" {
		t.Fatalf("deadLetterMsg=%q", st.deadLetterMsg)
	}
	if st.insertedDead == nil || st.insertedDead.ID != "dead-1" {
		t.Fatalf("inserted dead letter=%+v", st.insertedDead)
	}
}
