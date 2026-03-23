# github.com/richrobertson/notification-platform/internal/delivery

_Generated from Go doc comments. Run `make docs-godoc` to refresh._

```text
package delivery // import "github.com/richrobertson/notification-platform/internal/delivery"

Package delivery contains the delivery-time execution logic for channel workers.

It sits between queue jobs and durable store updates:

  - queue.DispatchJob identifies the notification attempt to process
  - store.Postgres provides durable attempt and dead-letter state changes
  - webhook and SMTP senders perform the external side effects

The package is deliberately honest about its guarantees. Delivery is still
at-least-once underneath, but the service layers attempt-state guards, duplicate
suppression, retry scheduling, dead-lettering, and audited failover behavior on
top.

New contributors should read Service first. It is the main orchestration entry
point and shows the end-to-end processing contract that workers rely on.

FUNCTIONS

func IsRetryable(err error) bool
    IsRetryable reports whether the error was classified as retryable.

func IsTerminal(err error) bool
    IsTerminal reports whether the error was classified as a terminal delivery
    failure.

func MaybeRetryable(err error) error
    MaybeRetryable wraps an ordinary error as retryable unless it has already
    been classified.

func RenderTemplate(body string, variables map[string]any) (string, error)
    RenderTemplate applies `{{variable}}` substitutions to a template body.

    The renderer is intentionally small and explicit. It is suitable for message
    bodies and operator-visible examples, not for generalized templating logic.

func NewOptionalSecondaryEmailSender(cfg config.Config) emailSender
    NewOptionalSecondaryEmailSender returns the configured secondary sender when
    one is available, or nil when failover is disabled by configuration.


TYPES

type EmailRequest struct {
	To             string
	Subject        string
	Body           string
	AttemptID      string
	NotificationID string
}
    EmailRequest is the rendered email payload passed to an SMTP sender.

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
    NotificationStore captures the durable state transitions the delivery
    service depends on while processing one attempt.

type Outcome int
    Outcome classifies the durable result of processing one queue job.

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
    RetryPolicy controls how retryable failures are converted into retry
    schedules.

type RetryableError struct{ Err error }
    RetryableError marks a failure as retryable.

func (e *RetryableError) Error() string

func (e *RetryableError) Unwrap() error

type SMTPSender struct {
	// Has unexported fields.
}
    SMTPSender delivers email through one configured SMTP endpoint.

func NewSMTPSender(cfg config.Config) *SMTPSender
    NewSMTPSender constructs the primary SMTP sender from process configuration.

func NewSecondarySMTPSender(cfg config.Config) *SMTPSender
    NewSecondarySMTPSender constructs the secondary SMTP sender used for
    failover.

func (s *SMTPSender) Send(ctx context.Context, req EmailRequest) error
    Send writes one email message to the configured SMTP endpoint.

type Service struct {
	// Has unexported fields.
}
    Service coordinates one delivery attempt from queue job to durable outcome.

func NewService(store NotificationStore, webhookSender webhookSender, webhookBackup webhookSender, emailSender emailSender, emailBackup emailSender, policy RetryPolicy) (*Service, error)
    NewService constructs a delivery Service with retry policy defaults and
    telemetry counters.

func (s *Service) ProcessEmail(ctx context.Context, job queue.DispatchJob) (Result, error)
    ProcessEmail processes one email dispatch job.

func (s *Service) ProcessWebhook(ctx context.Context, job queue.DispatchJob) (Result, error)
    ProcessWebhook processes one webhook dispatch job.

type TerminalError struct{ Err error }
    TerminalError marks a failure as non-retryable.

func (e *TerminalError) Error() string

func (e *TerminalError) Unwrap() error

type WebhookRequest struct {
	URL            string
	Body           string
	AttemptID      string
	NotificationID string
}
    WebhookRequest is the rendered outbound webhook payload.

type WebhookSender struct {
	// Has unexported fields.
}
    WebhookSender delivers rendered webhook bodies over HTTP.

func NewWebhookSender(timeout time.Duration) *WebhookSender
    NewWebhookSender constructs an HTTP webhook sender with the given timeout.

func (s *WebhookSender) Send(ctx context.Context, req WebhookRequest) (string, error)
    Send delivers one webhook request and returns the provider-facing message ID
    when the downstream endpoint supplies one.

```
