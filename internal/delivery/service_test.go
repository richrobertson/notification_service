package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

type stubStore struct {
	notification   store.Notification
	template       store.Template
	attempt        store.DeliveryAttempt
	loadErr        error
	inProgressErr  error
	sentErr        error
	failedErr      error
	inProgressID   string
	sentID         *string
	failedMsg      string
	inProgressCall int
}

func (s *stubStore) LoadDeliveryJob(ctx context.Context, notificationID, attemptID string) (store.Notification, store.Template, store.DeliveryAttempt, error) {
	if s.loadErr != nil {
		return store.Notification{}, store.Template{}, store.DeliveryAttempt{}, s.loadErr
	}
	return s.notification, s.template, s.attempt, nil
}
func (s *stubStore) MarkAttemptInProgress(ctx context.Context, attemptID string) error {
	s.inProgressID = attemptID
	s.inProgressCall++
	return s.inProgressErr
}
func (s *stubStore) MarkAttemptSent(ctx context.Context, attemptID string, providerMessageID *string) error {
	s.sentID = providerMessageID
	return s.sentErr
}
func (s *stubStore) MarkAttemptFailed(ctx context.Context, attemptID string, lastError string) error {
	s.failedMsg = lastError
	return s.failedErr
}

type stubWebhookSender struct {
	providerID string
	err        error
}

func (s stubWebhookSender) Send(ctx context.Context, req WebhookRequest) (string, error) {
	return s.providerID, s.err
}

type stubEmailSender struct{ err error }

func (s stubEmailSender) Send(ctx context.Context, req EmailRequest) error { return s.err }

func TestServiceProcessWebhookMarksFailureOnMissingRecipient(t *testing.T) {
	t.Parallel()
	st := &stubStore{notification: store.Notification{Variables: map[string]any{"name": "Ada"}}, template: store.Template{Body: "hello {{.name}}"}}
	svc, err := NewService(st, stubWebhookSender{}, stubEmailSender{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ProcessWebhook(context.Background(), queue.DispatchJob{AttemptID: "attempt-1", Channel: "webhook"})
	if !IsTerminal(err) {
		t.Fatalf("ProcessWebhook() error terminal = false, err=%v", err)
	}
	if result.Outcome != OutcomeFailedTerminal {
		t.Fatalf("result.Outcome = %v", result.Outcome)
	}
	if st.failedMsg != "recipient_webhook_url is required" {
		t.Fatalf("failedMsg = %q", st.failedMsg)
	}
	if st.inProgressCall != 1 {
		t.Fatalf("inProgressCall = %d, want 1", st.inProgressCall)
	}
}

func TestServiceProcessWebhookMarksSent(t *testing.T) {
	t.Parallel()
	url := "http://example.test"
	st := &stubStore{notification: store.Notification{RecipientWebhookURL: &url, Variables: map[string]any{"name": "Ada"}}, template: store.Template{Body: `{"name":"{{.name}}"}`}}
	svc, err := NewService(st, stubWebhookSender{providerID: "req-1"}, stubEmailSender{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ProcessWebhook(context.Background(), queue.DispatchJob{AttemptID: "attempt-1", Channel: "webhook"})
	if err != nil {
		t.Fatalf("ProcessWebhook() error = %v", err)
	}
	if result.Outcome != OutcomeSent {
		t.Fatalf("result.Outcome = %v", result.Outcome)
	}
	if st.sentID == nil || *st.sentID != "req-1" {
		t.Fatalf("provider_message_id = %v, want req-1", st.sentID)
	}
}

func TestServiceProcessEmailMarksFailure(t *testing.T) {
	t.Parallel()
	to := "to@example.test"
	st := &stubStore{notification: store.Notification{RecipientEmail: &to, Variables: map[string]any{"name": "Ada"}}, template: store.Template{Name: "welcome", Body: "hello {{.name}}"}}
	svc, err := NewService(st, stubWebhookSender{}, stubEmailSender{err: errors.New("smtp down")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ProcessEmail(context.Background(), queue.DispatchJob{AttemptID: "attempt-1", Channel: "email"})
	if !IsTerminal(err) {
		t.Fatalf("ProcessEmail() error terminal = false, err=%v", err)
	}
	if result.Outcome != OutcomeFailedTerminal {
		t.Fatalf("result.Outcome = %v", result.Outcome)
	}
	if st.failedMsg != "smtp down" {
		t.Fatalf("failedMsg = %q", st.failedMsg)
	}
}

func TestServiceProcessEmailMarksSent(t *testing.T) {
	t.Parallel()
	to := "to@example.test"
	st := &stubStore{notification: store.Notification{ID: "notif-1", RecipientEmail: &to, Variables: map[string]any{"name": "Ada"}, SubmittedAt: time.Now()}, template: store.Template{Name: "welcome", Body: "hello {{.name}}"}}
	svc, err := NewService(st, stubWebhookSender{}, stubEmailSender{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ProcessEmail(context.Background(), queue.DispatchJob{AttemptID: "attempt-1", Channel: "email"})
	if err != nil {
		t.Fatalf("ProcessEmail() error = %v", err)
	}
	if result.Outcome != OutcomeSent {
		t.Fatalf("result.Outcome = %v", result.Outcome)
	}
	if st.failedMsg != "" {
		t.Fatalf("failedMsg = %q, want empty", st.failedMsg)
	}
	if st.sentID != nil {
		t.Fatalf("sentID = %v, want nil provider id for email", st.sentID)
	}
}

func TestServiceProcessEmailReturnsTransientErrorWhenLoadFails(t *testing.T) {
	t.Parallel()
	st := &stubStore{loadErr: errors.New("postgres unavailable")}
	svc, err := NewService(st, stubWebhookSender{}, stubEmailSender{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.ProcessEmail(context.Background(), queue.DispatchJob{AttemptID: "attempt-1", Channel: "email"})
	if err == nil {
		t.Fatal("ProcessEmail() error = nil, want transient error")
	}
	if IsTerminal(err) {
		t.Fatalf("ProcessEmail() error terminal = true, err=%v", err)
	}
	if st.failedMsg != "" {
		t.Fatalf("failedMsg = %q, want empty", st.failedMsg)
	}
}

func TestServiceProcessEmailReturnsTransientErrorWhenMarkAttemptFailedFails(t *testing.T) {
	t.Parallel()
	to := "to@example.test"
	st := &stubStore{notification: store.Notification{RecipientEmail: &to, Variables: map[string]any{"name": "Ada"}}, template: store.Template{Name: "welcome", Body: "hello {{.name}}"}, failedErr: errors.New("write failed")}
	svc, err := NewService(st, stubWebhookSender{}, stubEmailSender{err: errors.New("smtp down")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.ProcessEmail(context.Background(), queue.DispatchJob{AttemptID: "attempt-1", Channel: "email"})
	if err == nil {
		t.Fatal("ProcessEmail() error = nil, want transient error")
	}
	if IsTerminal(err) {
		t.Fatalf("ProcessEmail() error terminal = true, err=%v", err)
	}
}
