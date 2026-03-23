# github.com/richrobertson/notification-platform/internal/notify

_Generated from Go doc comments. Run `make docs-godoc` to refresh._

```text
package notify // import "github.com/richrobertson/notification-platform/internal/notify"

Package notify contains the earlier in-memory MVP service and HTTP server.

The newer Stage 4+ production-oriented path lives under the store, queue,
delivery, and httpserver packages. This package remains useful for lightweight
local examples, openapi shaping, and historical integration tests that model the
original service contract without Postgres or Redis.

Maintainers should treat this package as the simplest conceptual entry point,
not the source of truth for the durable runtime architecture.

TYPES

type CreateNotificationInput struct {
	TenantID       string         `json:"tenant_id"`
	TemplateID     string         `json:"template_id"`
	Channels       []string       `json:"channels"`
	Recipient      map[string]any `json:"recipient"`
	Variables      map[string]any `json:"variables"`
	IdempotencyKey string         `json:"idempotency_key"`
}
    CreateNotificationInput is the request payload for MVP notification
    submission.

type CreateTemplateInput struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Channel  string `json:"channel"`
	Body     string `json:"body"`
}
    CreateTemplateInput is the request payload for template creation.

type CreateTenantInput struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	DailyQuota int    `json:"daily_quota"`
}
    CreateTenantInput is the request payload for tenant creation.

type DeadLetter struct {
	ID             string    `json:"id"`
	NotificationID string    `json:"notification_id"`
	Channel        string    `json:"channel"`
	FinalError     string    `json:"final_error"`
	DeadLetteredAt time.Time `json:"dead_lettered_at"`
}
    DeadLetter is the in-memory representation of a failed notification attempt.

type DeliveryAttempt struct {
	ID             string     `json:"id"`
	NotificationID string     `json:"notification_id"`
	Channel        string     `json:"channel"`
	AttemptNumber  int        `json:"attempt_number"`
	Status         string     `json:"status"`
	ErrorCode      *string    `json:"error_code"`
	NextRetryAt    *time.Time `json:"next_retry_at"`
	CompletedAt    *time.Time `json:"completed_at"`
}
    DeliveryAttempt is the simplified in-memory attempt record used by the MVP
    service.

type Notification struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id"`
	TemplateID     string            `json:"template_id"`
	Channels       []string          `json:"channels"`
	Recipient      map[string]any    `json:"recipient"`
	Variables      map[string]any    `json:"variables"`
	IdempotencyKey string            `json:"idempotency_key"`
	Status         string            `json:"status"`
	SubmittedAt    time.Time         `json:"submitted_at"`
	Attempts       []DeliveryAttempt `json:"attempts"`
}
    Notification is the in-memory notification model exposed by the MVP service.

type Server struct {
	// Has unexported fields.
}

func NewServer(service *Service) *Server
    NewServer constructs the HTTP server wrapper for the in-memory MVP service.

func (s *Server) Handler() http.Handler
    Handler returns the configured HTTP handler tree.

type Service struct {
	// Has unexported fields.
}
    Service is the in-memory MVP implementation of the notification API.

func NewService() *Service
    NewService creates a new in-memory notify Service.

func (s *Service) CreateNotification(input CreateNotificationInput) (Notification, bool, error)
    CreateNotification creates a new notification and returns whether it was
    deduplicated by idempotency key.

func (s *Service) CreateTemplate(input CreateTemplateInput) (Template, error)
    CreateTemplate stores a new template for a tenant.

func (s *Service) CreateTenant(input CreateTenantInput) (Tenant, error)
    CreateTenant stores a new tenant in the in-memory service.

func (s *Service) DeadLetters() []DeadLetter
    DeadLetters returns the current in-memory dead-letter set.

func (s *Service) GetNotification(id string) (Notification, error)
    GetNotification returns one notification by ID.

func (s *Service) GetTemplate(id string) (Template, error)
    GetTemplate returns one template by ID.

func (s *Service) GetTenant(id string) (Tenant, error)
    GetTenant returns one tenant by ID.

func (s *Service) ReplayNotification(id string) (Notification, error)
    ReplayNotification resets a failed notification so it can be attempted again
    in the MVP service.

func (s *Service) UpdateTemplate(id string, input UpdateTemplateInput) (Template, error)
    UpdateTemplate replaces the mutable parts of an existing template.

func (s *Service) Usage(tenantID string) (Usage, error)
    Usage returns the current-day usage counters for one tenant.

type Template struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Channel   string    `json:"channel"`
	Version   int       `json:"version"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
    Template is the in-memory MVP representation of a message template.

type Tenant struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	DailyQuota int       `json:"daily_quota"`
	CreatedAt  time.Time `json:"created_at"`
}
    Tenant is the in-memory tenant record used by the MVP service.

type UpdateTemplateInput struct {
	Name string `json:"name"`
	Body string `json:"body"`
}
    UpdateTemplateInput is the request payload for template updates.

type Usage struct {
	TenantID              string `json:"tenant_id"`
	Date                  string `json:"date"`
	AcceptedNotifications int    `json:"accepted_notifications"`
	DailyQuota            int    `json:"daily_quota"`
	RemainingQuota        int    `json:"remaining_quota"`
}
    Usage reports a tenant's accepted-notification count for the current day.

```
