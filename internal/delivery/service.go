package delivery

import (
	"context"
	"fmt"
	"strings"

	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type NotificationStore interface {
	LoadDeliveryJob(ctx context.Context, notificationID, attemptID string) (store.Notification, store.Template, store.DeliveryAttempt, error)
	MarkAttemptSent(ctx context.Context, attemptID string, providerMessageID *string) error
	MarkAttemptFailed(ctx context.Context, attemptID string, lastError string) error
}

type webhookSender interface {
	Send(ctx context.Context, req WebhookRequest) (string, error)
}

type emailSender interface {
	Send(ctx context.Context, req EmailRequest) error
}

type Service struct {
	store         NotificationStore
	webhookSender webhookSender
	emailSender   emailSender
	sentCounter   metric.Int64Counter
	failCounter   metric.Int64Counter
}

func NewService(store NotificationStore, webhookSender webhookSender, emailSender emailSender) (*Service, error) {
	meter := otel.Meter("notification-platform/delivery")
	sentCounter, err := meter.Int64Counter("deliveries_sent_total")
	if err != nil {
		return nil, fmt.Errorf("create sent counter: %w", err)
	}
	failCounter, err := meter.Int64Counter("deliveries_failed_total")
	if err != nil {
		return nil, fmt.Errorf("create failed counter: %w", err)
	}
	return &Service{store: store, webhookSender: webhookSender, emailSender: emailSender, sentCounter: sentCounter, failCounter: failCounter}, nil
}

func (s *Service) ProcessWebhook(ctx context.Context, job queue.DispatchJob) error {
	ctx, span := otel.Tracer("notification-platform/delivery").Start(ctx, "delivery.process_webhook")
	defer span.End()
	span.SetAttributes(attribute.String("job.id", job.JobID), attribute.String("notification.id", job.NotificationID), attribute.String("attempt.id", job.AttemptID))

	notification, template, _, err := s.store.LoadDeliveryJob(ctx, job.NotificationID, job.AttemptID)
	if err != nil {
		return err
	}
	if notification.RecipientWebhookURL == nil || strings.TrimSpace(*notification.RecipientWebhookURL) == "" {
		return s.failAttempt(ctx, job.AttemptID, job.Channel, "recipient_webhook_url is required")
	}
	body, err := RenderTemplate(template.Body, notification.Variables)
	if err != nil {
		return s.failAttempt(ctx, job.AttemptID, job.Channel, err.Error())
	}
	providerID, err := s.webhookSender.Send(ctx, WebhookRequest{URL: *notification.RecipientWebhookURL, Body: body})
	if err != nil {
		return s.failAttempt(ctx, job.AttemptID, job.Channel, err.Error())
	}
	var providerPtr *string
	if providerID != "" {
		providerPtr = &providerID
	}
	if err := s.store.MarkAttemptSent(ctx, job.AttemptID, providerPtr); err != nil {
		return err
	}
	s.recordSent(ctx, job.Channel)
	return nil
}

func (s *Service) ProcessEmail(ctx context.Context, job queue.DispatchJob) error {
	ctx, span := otel.Tracer("notification-platform/delivery").Start(ctx, "delivery.process_email")
	defer span.End()
	span.SetAttributes(attribute.String("job.id", job.JobID), attribute.String("notification.id", job.NotificationID), attribute.String("attempt.id", job.AttemptID))

	notification, template, _, err := s.store.LoadDeliveryJob(ctx, job.NotificationID, job.AttemptID)
	if err != nil {
		return err
	}
	if notification.RecipientEmail == nil || strings.TrimSpace(*notification.RecipientEmail) == "" {
		return s.failAttempt(ctx, job.AttemptID, job.Channel, "recipient_email is required")
	}
	body, err := RenderTemplate(template.Body, notification.Variables)
	if err != nil {
		return s.failAttempt(ctx, job.AttemptID, job.Channel, err.Error())
	}
	subject := strings.TrimSpace(template.Name)
	if subject == "" {
		subject = fmt.Sprintf("notification %s", notification.ID)
	}
	if err := s.emailSender.Send(ctx, EmailRequest{To: *notification.RecipientEmail, Subject: subject, Body: body}); err != nil {
		return s.failAttempt(ctx, job.AttemptID, job.Channel, err.Error())
	}
	if err := s.store.MarkAttemptSent(ctx, job.AttemptID, nil); err != nil {
		return err
	}
	s.recordSent(ctx, job.Channel)
	return nil
}

func (s *Service) failAttempt(ctx context.Context, attemptID, channel, msg string) error {
	if err := s.store.MarkAttemptFailed(ctx, attemptID, msg); err != nil {
		return err
	}
	s.failCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("channel", channel)))
	return nil
}

func (s *Service) recordSent(ctx context.Context, channel string) {
	s.sentCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("channel", channel)))
}
