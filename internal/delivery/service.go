package delivery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type NotificationStore interface {
	LoadDeliveryJob(ctx context.Context, notificationID, attemptID string) (store.Notification, store.Template, store.DeliveryAttempt, error)
	GetDeliveryAttemptByID(ctx context.Context, attemptID string) (store.DeliveryAttempt, error)
	ResolveDeliveryPolicy(ctx context.Context, tenantID, channel string) (store.ResolvedDeliveryPolicy, error)
	UpdateAttemptProvider(ctx context.Context, attemptID, provider string, failoverUsed bool) error
	MarkAttemptInProgress(ctx context.Context, attemptID string) error
	MarkAttemptSent(ctx context.Context, attemptID string, providerMessageID *string) error
	MarkAttemptFailed(ctx context.Context, attemptID string, lastError string) error
	ScheduleRetry(ctx context.Context, attemptID, lastError string, nextRetryAt time.Time) error
	MarkAttemptDeadLettered(ctx context.Context, attemptID, lastError string) error
	InsertDeadLetter(ctx context.Context, id, notificationID, channel, finalError string) (store.DeadLetter, error)
	RecordAuditEvent(ctx context.Context, id, tenantID, actor, action, resourceType, resourceID string, metadata map[string]any) error
}

type RetryPolicy struct {
	MaxAttempts        int
	BaseDelay          time.Duration
	MaxDelay           time.Duration
	ExponentialBackoff bool
	Jitter             time.Duration
	Now                func() time.Time
	IDGenerator        func() string
	RandSource         *rand.Rand
	PressureMultiplier int
	PressureMinDelay   time.Duration
	QueueDepth         func(channel string) int
	QueueSoftLimit     int
}

type webhookSender interface {
	Send(ctx context.Context, req WebhookRequest) (string, error)
}
type emailSender interface {
	Send(ctx context.Context, req EmailRequest) error
}

type Outcome int

const (
	OutcomeSent Outcome = iota
	OutcomeFailedTerminal
	OutcomeRetryScheduled
	OutcomeDeadLettered
	OutcomeDuplicateSuppressed
)

type Result struct {
	Outcome     Outcome
	NextRetryAt *time.Time
	DeadLetter  *store.DeadLetter
	Message     string
}

type TerminalError struct{ Err error }

func (e *TerminalError) Error() string { return e.Err.Error() }
func (e *TerminalError) Unwrap() error { return e.Err }

type RetryableError struct{ Err error }

func (e *RetryableError) Error() string { return e.Err.Error() }
func (e *RetryableError) Unwrap() error { return e.Err }

func IsTerminal(err error) bool  { var target *TerminalError; return errors.As(err, &target) }
func IsRetryable(err error) bool { var target *RetryableError; return errors.As(err, &target) }
func terminalErrorf(format string, args ...any) error {
	return &TerminalError{Err: fmt.Errorf(format, args...)}
}
func retryableErrorf(format string, args ...any) error {
	return &RetryableError{Err: fmt.Errorf(format, args...)}
}

func MaybeRetryable(err error) error {
	if err == nil || IsTerminal(err) || IsRetryable(err) {
		return err
	}
	return &RetryableError{Err: err}
}

type Service struct {
	store         NotificationStore
	webhookSender webhookSender
	webhookBackup webhookSender
	emailSender   emailSender
	emailBackup   emailSender
	policy        RetryPolicy
	sentCounter   metric.Int64Counter
	failCounter   metric.Int64Counter
}

func NewService(store NotificationStore, webhookSender webhookSender, webhookBackup webhookSender, emailSender emailSender, emailBackup emailSender, policy RetryPolicy) (*Service, error) {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 3
	}
	if policy.BaseDelay <= 0 {
		policy.BaseDelay = 5 * time.Second
	}
	if policy.MaxDelay <= 0 {
		policy.MaxDelay = time.Minute
	}
	if policy.Now == nil {
		policy.Now = func() time.Time { return time.Now().UTC() }
	}
	if policy.IDGenerator == nil {
		policy.IDGenerator = func() string { return fmt.Sprintf("dead-%d", time.Now().UnixNano()) }
	}
	if policy.RandSource == nil {
		policy.RandSource = rand.New(rand.NewSource(1))
	}
	if policy.PressureMultiplier <= 0 {
		policy.PressureMultiplier = 1
	}
	meter := otel.Meter("notification-platform/delivery")
	sentCounter, err := meter.Int64Counter("deliveries_sent_total")
	if err != nil {
		return nil, fmt.Errorf("create sent counter: %w", err)
	}
	failCounter, err := meter.Int64Counter("deliveries_failed_total")
	if err != nil {
		return nil, fmt.Errorf("create failed counter: %w", err)
	}
	return &Service{store: store, webhookSender: webhookSender, webhookBackup: webhookBackup, emailSender: emailSender, emailBackup: emailBackup, policy: policy, sentCounter: sentCounter, failCounter: failCounter}, nil
}

func (s *Service) ProcessWebhook(ctx context.Context, job queue.DispatchJob) (Result, error) {
	return s.process(ctx, job, func(ctx context.Context, notification store.Notification, template store.Template, policy store.ResolvedDeliveryPolicy) (*string, string, bool, error) {
		if notification.RecipientWebhookURL == nil || strings.TrimSpace(*notification.RecipientWebhookURL) == "" {
			return nil, "", false, terminalErrorf("recipient_webhook_url is required")
		}
		body, err := RenderTemplate(template.Body, notification.Variables)
		if err != nil {
			return nil, "", false, &TerminalError{Err: err}
		}
		providerID, err := s.webhookSender.Send(ctx, WebhookRequest{URL: *notification.RecipientWebhookURL, Body: body, AttemptID: job.AttemptID, NotificationID: notification.ID})
		if err != nil {
			if IsRetryable(MaybeRetryable(err)) && policy.FailoverEnabled && s.webhookBackup != nil && notification.SecondaryWebhookURL != nil && strings.TrimSpace(*notification.SecondaryWebhookURL) != "" {
				backupProviderID, backupErr := s.webhookBackup.Send(ctx, WebhookRequest{URL: *notification.SecondaryWebhookURL, Body: body, AttemptID: job.AttemptID, NotificationID: notification.ID})
				if backupErr == nil {
					return stringPtrOrNil(backupProviderID), "webhook_secondary", true, nil
				}
			}
			return nil, "webhook_primary", false, MaybeRetryable(err)
		}
		if providerID == "" {
			return nil, "webhook_primary", false, nil
		}
		return &providerID, "webhook_primary", false, nil
	})
}

func (s *Service) ProcessEmail(ctx context.Context, job queue.DispatchJob) (Result, error) {
	return s.process(ctx, job, func(ctx context.Context, notification store.Notification, template store.Template, policy store.ResolvedDeliveryPolicy) (*string, string, bool, error) {
		if notification.RecipientEmail == nil || strings.TrimSpace(*notification.RecipientEmail) == "" {
			return nil, "", false, terminalErrorf("recipient_email is required")
		}
		body, err := RenderTemplate(template.Body, notification.Variables)
		if err != nil {
			return nil, "", false, &TerminalError{Err: err}
		}
		subject := strings.TrimSpace(template.Name)
		if subject == "" {
			subject = fmt.Sprintf("notification %s", notification.ID)
		}
		if err := s.emailSender.Send(ctx, EmailRequest{To: *notification.RecipientEmail, Subject: subject, Body: body, AttemptID: job.AttemptID, NotificationID: notification.ID}); err != nil {
			if IsRetryable(MaybeRetryable(err)) && policy.FailoverEnabled && s.emailBackup != nil {
				if backupErr := s.emailBackup.Send(ctx, EmailRequest{To: *notification.RecipientEmail, Subject: subject, Body: body, AttemptID: job.AttemptID, NotificationID: notification.ID}); backupErr == nil {
					return nil, "smtp_secondary", true, nil
				}
			}
			return nil, "smtp_primary", false, MaybeRetryable(err)
		}
		return nil, "smtp_primary", false, nil
	})
}

func (s *Service) process(ctx context.Context, job queue.DispatchJob, sender func(context.Context, store.Notification, store.Template, store.ResolvedDeliveryPolicy) (*string, string, bool, error)) (Result, error) {
	ctx, span := otel.Tracer("notification-platform/delivery").Start(ctx, "delivery.process")
	defer span.End()
	span.SetAttributes(attribute.String("job.id", job.JobID), attribute.String("notification.id", job.NotificationID), attribute.String("attempt.id", job.AttemptID), attribute.String("channel", job.Channel))

	if err := s.store.MarkAttemptInProgress(ctx, job.AttemptID); err != nil {
		if store.IsAttemptAlreadyFinalized(err) || store.IsAttemptAlreadyProcessing(err) {
			attempt, loadErr := s.store.GetDeliveryAttemptByID(ctx, job.AttemptID)
			if loadErr != nil {
				return Result{}, fmt.Errorf("load duplicate attempt: %w", loadErr)
			}
			msg := fmt.Sprintf("duplicate job suppressed for attempt %s in state %s", job.AttemptID, attempt.Status)
			slog.Default().Info(msg, slog.String("attempt_id", job.AttemptID), slog.String("notification_id", job.NotificationID), slog.String("channel", job.Channel), slog.String("status", attempt.Status))
			_ = s.store.RecordAuditEvent(ctx, s.policy.IDGenerator(), job.TenantID, "worker", "duplicate_job_suppressed", "delivery_attempt", job.AttemptID, map[string]any{"notification_id": job.NotificationID, "channel": job.Channel, "status": attempt.Status})
			return Result{Outcome: OutcomeDuplicateSuppressed, Message: msg}, nil
		}
		return Result{}, fmt.Errorf("mark attempt in progress: %w", err)
	}
	notification, template, attempt, err := s.store.LoadDeliveryJob(ctx, job.NotificationID, job.AttemptID)
	if err != nil {
		return Result{}, err
	}
	resolvedPolicy, err := s.store.ResolveDeliveryPolicy(ctx, notification.TenantID, job.Channel)
	if err != nil {
		return Result{}, err
	}
	providerID, providerName, failoverUsed, err := sender(ctx, notification, template, resolvedPolicy)
	if err == nil {
		if err := s.store.MarkAttemptSent(ctx, job.AttemptID, providerID); err != nil {
			return Result{}, err
		}
		if providerName != "" {
			if updateErr := s.store.UpdateAttemptProvider(ctx, job.AttemptID, providerName, failoverUsed); updateErr != nil {
				slog.Default().Warn("failed to update attempt provider metadata", slog.String("attempt_id", job.AttemptID), slog.String("notification_id", job.NotificationID), slog.String("channel", job.Channel), slog.String("provider_name", providerName), slog.Bool("failover_used", failoverUsed), slog.Any("error", updateErr))
			}
		}
		if failoverUsed {
			_ = s.store.RecordAuditEvent(ctx, s.policy.IDGenerator(), job.TenantID, "worker", "provider_failover_used", "delivery_attempt", job.AttemptID, map[string]any{"notification_id": job.NotificationID, "channel": job.Channel, "provider": providerName})
		}
		_ = s.store.RecordAuditEvent(ctx, s.policy.IDGenerator(), job.TenantID, "worker", "attempt_sent", "delivery_attempt", job.AttemptID, map[string]any{"notification_id": job.NotificationID, "channel": job.Channel})
		s.recordSent(ctx, job.Channel)
		return Result{Outcome: OutcomeSent}, nil
	}
	if IsRetryable(err) {
		return s.handleRetryable(ctx, notification, attempt, job.Channel, resolvedPolicy, err)
	}
	return s.failTerminal(ctx, notification.TenantID, attempt.ID, job.Channel, err)
}

func (s *Service) handleRetryable(ctx context.Context, notification store.Notification, attempt store.DeliveryAttempt, channel string, resolvedPolicy store.ResolvedDeliveryPolicy, cause error) (Result, error) {
	maxAttempts := s.policy.MaxAttempts
	if resolvedPolicy.MaxAttemptsOverride != nil && *resolvedPolicy.MaxAttemptsOverride > 0 {
		maxAttempts = *resolvedPolicy.MaxAttemptsOverride
	}
	if attempt.AttemptNumber >= maxAttempts {
		if err := s.store.MarkAttemptDeadLettered(ctx, attempt.ID, cause.Error()); err != nil {
			return Result{}, err
		}
		dl, err := s.store.InsertDeadLetter(ctx, s.policy.IDGenerator(), notification.ID, channel, cause.Error())
		if err != nil {
			return Result{}, err
		}
		_ = s.store.RecordAuditEvent(ctx, s.policy.IDGenerator(), notification.TenantID, "worker", "dead_lettered", "delivery_attempt", attempt.ID, map[string]any{"notification_id": notification.ID, "dead_letter_id": dl.ID, "channel": channel, "error": cause.Error()})
		s.failCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("channel", channel), attribute.String("final_state", "dead_lettered")))
		return Result{Outcome: OutcomeDeadLettered, DeadLetter: &dl}, cause
	}
	nextRetryAt := s.nextRetryAt(channel, attempt.AttemptNumber, resolvedPolicy)
	if err := s.store.ScheduleRetry(ctx, attempt.ID, cause.Error(), nextRetryAt); err != nil {
		return Result{}, err
	}
	_ = s.store.RecordAuditEvent(ctx, s.policy.IDGenerator(), notification.TenantID, "worker", "retry_scheduled", "delivery_attempt", attempt.ID, map[string]any{"notification_id": notification.ID, "channel": channel, "next_retry_at": nextRetryAt.Format(time.RFC3339Nano), "error": cause.Error()})
	s.failCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("channel", channel), attribute.String("final_state", "retry_scheduled")))
	return Result{Outcome: OutcomeRetryScheduled, NextRetryAt: &nextRetryAt}, cause
}

func (s *Service) failTerminal(ctx context.Context, tenantID, attemptID, channel string, cause error) (Result, error) {
	if err := s.store.MarkAttemptFailed(ctx, attemptID, cause.Error()); err != nil {
		return Result{}, err
	}
	attempt, attemptErr := s.store.GetDeliveryAttemptByID(ctx, attemptID)
	if attemptErr == nil {
		_ = s.store.RecordAuditEvent(ctx, s.policy.IDGenerator(), tenantID, "worker", "attempt_failed", "delivery_attempt", attemptID, map[string]any{"notification_id": attempt.NotificationID, "channel": channel, "error": cause.Error()})
	}
	s.failCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("channel", channel), attribute.String("final_state", "failed")))
	return Result{Outcome: OutcomeFailedTerminal}, cause
}

func (s *Service) nextRetryAt(channel string, attemptNumber int, resolvedPolicy store.ResolvedDeliveryPolicy) time.Time {
	delay := s.policy.BaseDelay
	maxDelay := s.policy.MaxDelay
	if resolvedPolicy.RetryBaseDelaySeconds != nil && *resolvedPolicy.RetryBaseDelaySeconds > 0 {
		delay = time.Duration(*resolvedPolicy.RetryBaseDelaySeconds) * time.Second
	}
	if resolvedPolicy.RetryMaxDelaySeconds != nil && *resolvedPolicy.RetryMaxDelaySeconds > 0 {
		maxDelay = time.Duration(*resolvedPolicy.RetryMaxDelaySeconds) * time.Second
	}
	if s.policy.ExponentialBackoff {
		for i := 1; i < attemptNumber; i++ {
			delay *= 2
			if delay >= maxDelay {
				delay = maxDelay
				break
			}
		}
	}
	if delay > s.policy.MaxDelay {
		delay = s.policy.MaxDelay
	}
	if s.policy.QueueDepth != nil && s.policy.QueueSoftLimit > 0 {
		if depth := s.policy.QueueDepth(channel); depth >= s.policy.QueueSoftLimit {
			for i := 1; i < s.policy.PressureMultiplier; i++ {
				delay *= 2
				if delay >= s.policy.MaxDelay {
					delay = s.policy.MaxDelay
					break
				}
			}
			if delay < s.policy.PressureMinDelay {
				delay = s.policy.PressureMinDelay
			}
		}
	}
	if s.policy.Jitter > 0 {
		delta := time.Duration(s.policy.RandSource.Int63n(int64(s.policy.Jitter)*2+1)) - s.policy.Jitter
		delay += delta
		if delay < time.Second {
			delay = time.Second
		}
	}
	return s.policy.Now().Add(delay)
}

func stringPtrOrNil(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}

func (s *Service) recordSent(ctx context.Context, channel string) {
	s.sentCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("channel", channel)))
}
