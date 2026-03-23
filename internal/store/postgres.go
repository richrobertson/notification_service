package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

var (
	// ErrNotFound reports that the requested durable record does not exist.
	ErrNotFound = errors.New("store: not found")
	// ErrConflict reports that a uniqueness or ownership invariant was violated.
	ErrConflict = errors.New("store: conflict")
	// ErrAttemptAlreadyFinalized reports that an attempt can no longer move into a
	// new active state.
	ErrAttemptAlreadyFinalized = errors.New("store: attempt already finalized")
	// ErrAttemptAlreadyProcessing reports that another worker already owns the
	// active execution of an attempt.
	ErrAttemptAlreadyProcessing = errors.New("store: attempt already processing")
	// ErrInvalidStateTransition reports that the requested transition would break
	// the durable workflow rules.
	ErrInvalidStateTransition = errors.New("store: invalid state transition")
)

// Postgres wraps the shared sql.DB used by the durable store layer.
type Postgres struct {
	DB *sql.DB
}

func touchUpdatedAtSetClause() string {
	return "updated_at = NOW()"
}

// Tenant is the durable tenant record stored in Postgres.
type Tenant struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	DailyQuota int       `json:"daily_quota"`
	CreatedAt  time.Time `json:"created_at"`
}

// Template is the durable template record stored in Postgres.
type Template struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Channel   string    `json:"channel"`
	Version   int       `json:"version"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Notification is the durable notification record exposed by the API and
// operator inspection endpoints.
type Notification struct {
	ID                  string         `json:"id"`
	TenantID            string         `json:"tenant_id"`
	TemplateID          string         `json:"template_id"`
	IdempotencyKey      *string        `json:"idempotency_key"`
	Status              string         `json:"status"`
	RecipientEmail      *string        `json:"recipient_email"`
	RecipientWebhookURL *string        `json:"recipient_webhook_url"`
	SecondaryWebhookURL *string        `json:"secondary_webhook_url,omitempty"`
	Variables           map[string]any `json:"variables"`
	ScheduledFor        *time.Time     `json:"scheduled_for,omitempty"`
	PromotedAt          *time.Time     `json:"promoted_at,omitempty"`
	CancelledAt         *time.Time     `json:"cancelled_at,omitempty"`
	SubmittedAt         time.Time      `json:"submitted_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// DeliveryAttempt is the durable execution record for one delivery try.
type DeliveryAttempt struct {
	ID                 string     `json:"id"`
	NotificationID     string     `json:"notification_id"`
	Channel            string     `json:"channel"`
	AttemptNumber      int        `json:"attempt_number"`
	Status             string     `json:"status"`
	ErrorCode          *string    `json:"error_code"`
	ErrorMessage       *string    `json:"error_message"`
	ProviderMessageID  *string    `json:"provider_message_id"`
	LastError          *string    `json:"last_error"`
	NextRetryAt        *time.Time `json:"next_retry_at"`
	StartedAt          *time.Time `json:"started_at"`
	CompletedAt        *time.Time `json:"completed_at"`
	SentAt             *time.Time `json:"sent_at"`
	FailedAt           *time.Time `json:"failed_at"`
	DispatchEnqueuedAt *time.Time `json:"dispatch_enqueued_at"`
	EnqueueKind        string     `json:"enqueue_kind"`
	ProviderUsed       *string    `json:"provider_used,omitempty"`
	FailoverUsed       bool       `json:"failover_used"`
	DeadLetterID       *string    `json:"dead_letter_id,omitempty"`
	ReplayOfDeadLetter *string    `json:"replay_of_dead_letter_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// DispatchIntent is the Stage 8 outbox record that tracks whether an attempt
// still needs Redis publication.
type DispatchIntent struct {
	ID             string     `json:"id"`
	NotificationID string     `json:"notification_id"`
	AttemptID      string     `json:"attempt_id"`
	TenantID       string     `json:"tenant_id"`
	Channel        string     `json:"channel"`
	Source         string     `json:"source"`
	Status         string     `json:"status"`
	LastError      *string    `json:"last_error"`
	CreatedAt      time.Time  `json:"created_at"`
	ClaimedAt      *time.Time `json:"claimed_at"`
	PublishedAt    *time.Time `json:"published_at"`
}

// AuditEvent is the durable audit-trail record for operator and runtime
// actions.
type AuditEvent struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
}

// CreateTenantParams holds the input for CreateTenant.
type CreateTenantParams struct {
	ID         string
	Name       string
	DailyQuota int
}

// CreateTemplateParams holds the input for CreateTemplate.
type CreateTemplateParams struct {
	ID       string
	TenantID string
	Name     string
	Channel  string
	Version  int
	Body     string
}

// CreateNotificationParams holds the durable notification fields created before
// initial dispatch behavior is applied.
type CreateNotificationParams struct {
	ID                  string
	TenantID            string
	TemplateID          string
	IdempotencyKey      *string
	RecipientEmail      *string
	RecipientWebhookURL *string
	SecondaryWebhookURL *string
	Variables           map[string]any
	ScheduledFor        *time.Time
}

// CreateDeliveryAttemptParams holds the input for CreateDeliveryAttempt.
type CreateDeliveryAttemptParams struct {
	ID                 string
	NotificationID     string
	Channel            string
	AttemptNumber      int
	Status             string
	NextRetryAt        *time.Time
	LastError          *string
	DispatchEnqueuedAt *time.Time
	EnqueueKind        string
}

// CreateDispatchIntentParams holds the input for CreateDispatchIntent.
type CreateDispatchIntentParams struct {
	ID             string
	NotificationID string
	AttemptID      string
	TenantID       string
	Channel        string
	Source         string
	Status         string
	PublishedAt    *time.Time
}

// CreateNotificationDispatchParams combines notification, initial attempt, and
// initial dispatch-intent creation.
type CreateNotificationDispatchParams struct {
	Notification CreateNotificationParams
	Channel      string
	AttemptID    string
	IntentID     string
}

// DeliveryPolicy stores one policy row that may contribute to tenant/channel
// policy resolution.
type DeliveryPolicy struct {
	ID                    string    `json:"id"`
	TenantID              *string   `json:"tenant_id,omitempty"`
	Channel               *string   `json:"channel,omitempty"`
	Paused                *bool     `json:"paused,omitempty"`
	FailoverEnabled       *bool     `json:"failover_enabled,omitempty"`
	SchedulingEnabled     *bool     `json:"scheduling_enabled,omitempty"`
	ReplayAllowed         *bool     `json:"replay_allowed,omitempty"`
	MaxAttemptsOverride   *int      `json:"max_attempts_override,omitempty"`
	RetryBaseDelaySeconds *int      `json:"retry_base_delay_seconds,omitempty"`
	RetryMaxDelaySeconds  *int      `json:"retry_max_delay_seconds,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// ResolvedDeliveryPolicy is the final policy view after precedence and override
// rules have been applied.
type ResolvedDeliveryPolicy struct {
	TenantID              string `json:"tenant_id"`
	Channel               string `json:"channel"`
	Paused                bool   `json:"paused"`
	FailoverEnabled       bool   `json:"failover_enabled"`
	SchedulingEnabled     bool   `json:"scheduling_enabled"`
	ReplayAllowed         bool   `json:"replay_allowed"`
	MaxAttemptsOverride   *int   `json:"max_attempts_override,omitempty"`
	RetryBaseDelaySeconds *int   `json:"retry_base_delay_seconds,omitempty"`
	RetryMaxDelaySeconds  *int   `json:"retry_max_delay_seconds,omitempty"`
}

// UpsertDeliveryPolicyParams holds the mutable policy fields for upsert.
type UpsertDeliveryPolicyParams struct {
	ID                    string
	TenantID              *string
	Channel               *string
	Paused                *bool
	FailoverEnabled       *bool
	SchedulingEnabled     *bool
	ReplayAllowed         *bool
	MaxAttemptsOverride   *int
	RetryBaseDelaySeconds *int
	RetryMaxDelaySeconds  *int
}

// PromotedScheduledNotification is the result of turning scheduled work into an
// active dispatch intent.
type PromotedScheduledNotification struct {
	Notification Notification
	Attempt      DeliveryAttempt
	Intent       DispatchIntent
}

// DeadLetter is the durable terminal record for a permanently failed attempt.
type DeadLetter struct {
	ID              string     `json:"id"`
	NotificationID  string     `json:"notification_id"`
	Channel         string     `json:"channel"`
	FinalError      string     `json:"final_error"`
	DeadLetteredAt  time.Time  `json:"dead_lettered_at"`
	ReplayedAt      *time.Time `json:"replayed_at"`
	ReplayAttemptID *string    `json:"replay_attempt_id"`
}

// RetryDueAttempt is the retry scheduler's durable input row.
type RetryDueAttempt struct {
	Attempt        DeliveryAttempt
	NotificationID string
	TenantID       string
}

// PendingDispatchIntent is the outbox publisher's durable input row.
type PendingDispatchIntent struct {
	Intent       DispatchIntent
	DeadLetterID *string
}

// OperationalMetrics is the Stage 10 store-backed metrics snapshot returned by
// the metrics endpoint.
type OperationalMetrics struct {
	OutboxPendingCount        int       `json:"outbox_pending_count"`
	OutboxPublishingCount     int       `json:"outbox_publishing_count"`
	OutboxOldestLagSeconds    int64     `json:"outbox_oldest_lag_seconds"`
	DueRetryCount             int       `json:"due_retry_count"`
	OpenDeadLetterCount       int       `json:"open_dead_letter_count"`
	DuplicateSuppressionCount int       `json:"duplicate_suppression_count"`
	RetryScheduledCount       int       `json:"retry_scheduled_count"`
	DeadLetteredCount         int       `json:"dead_lettered_count"`
	ScheduledPendingCount     int       `json:"scheduled_pending_count"`
	ScheduledDueCount         int       `json:"scheduled_due_count"`
	ScheduledOldestLagSeconds int64     `json:"scheduled_oldest_lag_seconds"`
	CollectedAt               time.Time `json:"collected_at"`
}

// CleanupParams controls one explicit maintenance pass.
type CleanupParams struct {
	Now                 time.Time
	AuditRetention      time.Duration
	OutboxRetention     time.Duration
	DeadLetterRetention time.Duration
	DryRun              bool
}

// CleanupResult reports what a maintenance pass deleted, or would have deleted
// in dry-run mode.
type CleanupResult struct {
	DryRun                 bool      `json:"dry_run"`
	AuditEventsDeleted     int64     `json:"audit_events_deleted"`
	PublishedOutboxDeleted int64     `json:"published_outbox_deleted"`
	DeadLettersDeleted     int64     `json:"dead_letters_deleted"`
	ExecutedAt             time.Time `json:"executed_at"`
}

// NewPostgres opens the durable Postgres connection used by the runtime.
func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &Postgres{DB: db}, nil
}

// Close releases the underlying database connection pool.
func (p *Postgres) Close() error {
	if p == nil || p.DB == nil {
		return nil
	}

	if err := p.DB.Close(); err != nil {
		return fmt.Errorf("close postgres connection: %w", err)
	}

	return nil
}

// Ping verifies that Postgres is reachable.
func (p *Postgres) Ping(ctx context.Context) error {
	if p == nil || p.DB == nil {
		return fmt.Errorf("postgres connection is not initialized")
	}

	if err := p.DB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	return nil
}

// CreateTenant inserts a new tenant row.
func (p *Postgres) CreateTenant(ctx context.Context, params CreateTenantParams) (Tenant, error) {
	const query = `
		INSERT INTO tenants (id, name, daily_quota)
		VALUES ($1, $2, $3)
		RETURNING id, name, status, daily_quota, created_at
	`

	var tenant Tenant
	err := p.DB.QueryRowContext(ctx, query, params.ID, params.Name, params.DailyQuota).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Status,
		&tenant.DailyQuota,
		&tenant.CreatedAt,
	)
	if err != nil {
		return Tenant{}, wrapStoreError("create tenant", err)
	}

	return tenant, nil
}

// GetTenantByID returns one tenant by ID.
func (p *Postgres) GetTenantByID(ctx context.Context, id string) (Tenant, error) {
	const query = `
		SELECT id, name, status, daily_quota, created_at
		FROM tenants
		WHERE id = $1
	`

	var tenant Tenant
	err := p.DB.QueryRowContext(ctx, query, id).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Status,
		&tenant.DailyQuota,
		&tenant.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Tenant{}, fmt.Errorf("get tenant: %w", ErrNotFound)
		}
		return Tenant{}, fmt.Errorf("get tenant: %w", err)
	}

	return tenant, nil
}

// CreateTemplate inserts a new template row.
func (p *Postgres) CreateTemplate(ctx context.Context, params CreateTemplateParams) (Template, error) {
	const query = `
		INSERT INTO templates (id, tenant_id, name, channel, version, body)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, name, channel, version, body, created_at
	`

	var template Template
	err := p.DB.QueryRowContext(
		ctx,
		query,
		params.ID,
		params.TenantID,
		params.Name,
		params.Channel,
		params.Version,
		params.Body,
	).Scan(
		&template.ID,
		&template.TenantID,
		&template.Name,
		&template.Channel,
		&template.Version,
		&template.Body,
		&template.CreatedAt,
	)
	if err != nil {
		return Template{}, wrapStoreError("create template", err)
	}

	return template, nil
}

// GetTemplateByID returns one template by ID.
func (p *Postgres) GetTemplateByID(ctx context.Context, id string) (Template, error) {
	const query = `
		SELECT id, tenant_id, name, channel, version, body, created_at
		FROM templates
		WHERE id = $1
	`

	var template Template
	err := p.DB.QueryRowContext(ctx, query, id).Scan(
		&template.ID,
		&template.TenantID,
		&template.Name,
		&template.Channel,
		&template.Version,
		&template.Body,
		&template.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Template{}, fmt.Errorf("get template: %w", ErrNotFound)
		}
		return Template{}, fmt.Errorf("get template: %w", err)
	}

	return template, nil
}

// CreateNotification inserts a notification row without creating an initial
// attempt or dispatch intent.
func (p *Postgres) CreateNotification(ctx context.Context, params CreateNotificationParams) (Notification, error) {
	notification, err := createNotificationTx(ctx, p.DB, params)
	if err != nil {
		return Notification{}, err
	}
	return notification, nil
}

func createNotificationTx(ctx context.Context, querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, params CreateNotificationParams) (Notification, error) {
	status := "accepted"
	if params.ScheduledFor != nil && params.ScheduledFor.After(time.Now().UTC()) {
		status = "scheduled"
	}
	const query = `
		INSERT INTO notifications (
			id,
			tenant_id,
			template_id,
			idempotency_key,
			status,
			recipient_email,
			recipient_webhook_url,
			secondary_webhook_url,
			variables,
			scheduled_for
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10)
		RETURNING
			id,
			tenant_id,
			template_id,
			idempotency_key,
			status,
			recipient_email,
			recipient_webhook_url,
			secondary_webhook_url,
			variables,
			scheduled_for,
			promoted_at,
			cancelled_at,
			submitted_at,
			updated_at
	`

	variablesJSON, err := marshalVariables(params.Variables)
	if err != nil {
		return Notification{}, fmt.Errorf("create notification: %w", err)
	}

	var notification Notification
	var rawVariables []byte
	err = querier.QueryRowContext(
		ctx,
		query,
		params.ID,
		params.TenantID,
		params.TemplateID,
		params.IdempotencyKey,
		status,
		params.RecipientEmail,
		params.RecipientWebhookURL,
		params.SecondaryWebhookURL,
		variablesJSON,
		params.ScheduledFor,
	).Scan(
		&notification.ID,
		&notification.TenantID,
		&notification.TemplateID,
		&notification.IdempotencyKey,
		&notification.Status,
		&notification.RecipientEmail,
		&notification.RecipientWebhookURL,
		&notification.SecondaryWebhookURL,
		&rawVariables,
		&notification.ScheduledFor,
		&notification.PromotedAt,
		&notification.CancelledAt,
		&notification.SubmittedAt,
		&notification.UpdatedAt,
	)
	if err != nil {
		return Notification{}, wrapStoreError("create notification", err)
	}

	notification.Variables, err = unmarshalVariables(rawVariables)
	if err != nil {
		return Notification{}, fmt.Errorf("create notification: %w", err)
	}

	return notification, nil
}

// CreateNotificationWithInitialDispatch atomically creates the notification and
// its initial attempt, plus an initial dispatch intent when the work is
// immediately publishable.
func (p *Postgres) CreateNotificationWithInitialDispatch(ctx context.Context, params CreateNotificationDispatchParams) (Notification, DeliveryAttempt, DispatchIntent, error) {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return Notification{}, DeliveryAttempt{}, DispatchIntent{}, fmt.Errorf("create notification with initial dispatch: begin tx: %w", err)
	}
	defer tx.Rollback()

	notification, err := createNotificationTx(ctx, tx, params.Notification)
	if err != nil {
		return Notification{}, DeliveryAttempt{}, DispatchIntent{}, fmt.Errorf("create notification with initial dispatch: %w", err)
	}

	attempt, err := createDeliveryAttemptTx(ctx, tx, CreateDeliveryAttemptParams{
		ID:             params.AttemptID,
		NotificationID: notification.ID,
		Channel:        params.Channel,
		AttemptNumber:  1,
		Status:         "pending",
		EnqueueKind:    "initial",
	})
	if err != nil {
		return Notification{}, DeliveryAttempt{}, DispatchIntent{}, fmt.Errorf("create notification with initial dispatch: create delivery attempt: %w", err)
	}

	var intent DispatchIntent
	if notification.ScheduledFor == nil || !notification.ScheduledFor.After(time.Now().UTC()) {
		intent, err = createDispatchIntentTx(ctx, tx, CreateDispatchIntentParams{
			ID:             params.IntentID,
			NotificationID: notification.ID,
			AttemptID:      attempt.ID,
			TenantID:       notification.TenantID,
			Channel:        params.Channel,
			Source:         "initial",
		})
		if err != nil {
			return Notification{}, DeliveryAttempt{}, DispatchIntent{}, fmt.Errorf("create notification with initial dispatch: create dispatch intent: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Notification{}, DeliveryAttempt{}, DispatchIntent{}, fmt.Errorf("create notification with initial dispatch: commit: %w", err)
	}
	if intent.ID != "" {
		if err := p.RecalculateNotificationStatus(ctx, notification.ID); err != nil {
			return Notification{}, DeliveryAttempt{}, DispatchIntent{}, err
		}
	}
	notification, err = p.GetNotificationByID(ctx, notification.ID)
	if err != nil {
		return Notification{}, DeliveryAttempt{}, DispatchIntent{}, fmt.Errorf("create notification with initial dispatch: reload notification: %w", err)
	}

	return notification, attempt, intent, nil
}

// GetNotificationByTenantAndIdempotencyKey returns the durable notification for
// a tenant-scoped idempotency key.
func (p *Postgres) GetNotificationByTenantAndIdempotencyKey(ctx context.Context, tenantID, idempotencyKey string) (Notification, error) {
	const query = `
		SELECT
			id,
			tenant_id,
			template_id,
			idempotency_key,
			status,
			recipient_email,
			recipient_webhook_url,
			secondary_webhook_url,
			variables,
			scheduled_for,
			promoted_at,
			cancelled_at,
			submitted_at,
			updated_at
		FROM notifications
		WHERE tenant_id = $1 AND idempotency_key = $2
	`

	var notification Notification
	var rawVariables []byte
	err := p.DB.QueryRowContext(ctx, query, tenantID, idempotencyKey).Scan(
		&notification.ID,
		&notification.TenantID,
		&notification.TemplateID,
		&notification.IdempotencyKey,
		&notification.Status,
		&notification.RecipientEmail,
		&notification.RecipientWebhookURL,
		&notification.SecondaryWebhookURL,
		&rawVariables,
		&notification.ScheduledFor,
		&notification.PromotedAt,
		&notification.CancelledAt,
		&notification.SubmittedAt,
		&notification.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Notification{}, fmt.Errorf("get notification by idempotency key: %w", ErrNotFound)
		}
		return Notification{}, fmt.Errorf("get notification by idempotency key: %w", err)
	}

	notification.Variables, err = unmarshalVariables(rawVariables)
	if err != nil {
		return Notification{}, fmt.Errorf("get notification by idempotency key: %w", err)
	}

	return notification, nil
}

// IsConflict reports whether an error should be treated as a durable conflict.
func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}

func wrapStoreError(operation string, err error) error {
	if isUniqueViolation(err) {
		return fmt.Errorf("%s: %w", operation, ErrConflict)
	}

	return fmt.Errorf("%s: %w", operation, err)
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

func marshalVariables(variables map[string]any) ([]byte, error) {
	if variables == nil {
		variables = map[string]any{}
	}

	payload, err := json.Marshal(variables)
	if err != nil {
		return nil, fmt.Errorf("marshal variables: %w", err)
	}

	return payload, nil
}

func unmarshalVariables(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}

	var variables map[string]any
	if err := json.Unmarshal(raw, &variables); err != nil {
		return nil, fmt.Errorf("unmarshal variables: %w", err)
	}
	if variables == nil {
		return map[string]any{}, nil
	}

	return variables, nil
}

func scanDeliveryAttempt(scanner interface {
	Scan(dest ...any) error
}, attempt *DeliveryAttempt) error {
	return scanner.Scan(
		&attempt.ID,
		&attempt.NotificationID,
		&attempt.Channel,
		&attempt.AttemptNumber,
		&attempt.Status,
		&attempt.ErrorCode,
		&attempt.ErrorMessage,
		&attempt.ProviderMessageID,
		&attempt.LastError,
		&attempt.NextRetryAt,
		&attempt.StartedAt,
		&attempt.CompletedAt,
		&attempt.SentAt,
		&attempt.FailedAt,
		&attempt.DispatchEnqueuedAt,
		&attempt.EnqueueKind,
		&attempt.CreatedAt,
		&attempt.UpdatedAt,
	)
}

func scanDispatchIntent(scanner interface {
	Scan(dest ...any) error
}, intent *DispatchIntent) error {
	return scanner.Scan(
		&intent.ID,
		&intent.NotificationID,
		&intent.AttemptID,
		&intent.TenantID,
		&intent.Channel,
		&intent.Source,
		&intent.Status,
		&intent.LastError,
		&intent.CreatedAt,
		&intent.ClaimedAt,
		&intent.PublishedAt,
	)
}

func scanDeliveryPolicy(scanner interface {
	Scan(dest ...any) error
}, policy *DeliveryPolicy) error {
	var tenantID, channel sql.NullString
	var paused, failoverEnabled, schedulingEnabled, replayAllowed sql.NullBool
	var maxAttempts, retryBaseDelay, retryMaxDelay sql.NullInt64
	if err := scanner.Scan(
		&policy.ID,
		&tenantID,
		&channel,
		&paused,
		&failoverEnabled,
		&schedulingEnabled,
		&replayAllowed,
		&maxAttempts,
		&retryBaseDelay,
		&retryMaxDelay,
		&policy.CreatedAt,
		&policy.UpdatedAt,
	); err != nil {
		return err
	}
	policy.TenantID = nullStringPtr(tenantID)
	policy.Channel = nullStringPtr(channel)
	policy.Paused = nullBoolPtr(paused)
	policy.FailoverEnabled = nullBoolPtr(failoverEnabled)
	policy.SchedulingEnabled = nullBoolPtr(schedulingEnabled)
	policy.ReplayAllowed = nullBoolPtr(replayAllowed)
	policy.MaxAttemptsOverride = nullIntPtr(maxAttempts)
	policy.RetryBaseDelaySeconds = nullIntPtr(retryBaseDelay)
	policy.RetryMaxDelaySeconds = nullIntPtr(retryMaxDelay)
	return nil
}

func nullStringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func nullBoolPtr(v sql.NullBool) *bool {
	if !v.Valid {
		return nil
	}
	b := v.Bool
	return &b
}

func nullIntPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

func nullableBool(v *bool) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func applyPolicyRow(resolved *ResolvedDeliveryPolicy, row DeliveryPolicy) {
	if row.Paused != nil {
		resolved.Paused = *row.Paused
	}
	if row.FailoverEnabled != nil {
		resolved.FailoverEnabled = *row.FailoverEnabled
	}
	if row.SchedulingEnabled != nil {
		resolved.SchedulingEnabled = *row.SchedulingEnabled
	}
	if row.ReplayAllowed != nil {
		resolved.ReplayAllowed = *row.ReplayAllowed
	}
	if row.MaxAttemptsOverride != nil {
		value := *row.MaxAttemptsOverride
		resolved.MaxAttemptsOverride = &value
	}
	if row.RetryBaseDelaySeconds != nil {
		value := *row.RetryBaseDelaySeconds
		resolved.RetryBaseDelaySeconds = &value
	}
	if row.RetryMaxDelaySeconds != nil {
		value := *row.RetryMaxDelaySeconds
		resolved.RetryMaxDelaySeconds = &value
	}
}

// IsAttemptAlreadyFinalized reports whether an error means the attempt is
// already terminal.
func IsAttemptAlreadyFinalized(err error) bool {
	return errors.Is(err, ErrAttemptAlreadyFinalized)
}

// IsAttemptAlreadyProcessing reports whether another worker already owns the
// attempt.
func IsAttemptAlreadyProcessing(err error) bool {
	return errors.Is(err, ErrAttemptAlreadyProcessing)
}

// IsInvalidStateTransition reports whether the requested durable transition was
// rejected by workflow rules.
func IsInvalidStateTransition(err error) bool {
	return errors.Is(err, ErrInvalidStateTransition)
}

func isAttemptTerminalState(status string) bool {
	switch status {
	case "sent", "failed", "retry_scheduled", "dead_lettered":
		return true
	default:
		return false
	}
}

func deriveNotificationStatus(attempts []DeliveryAttempt) string {
	if len(attempts) == 0 {
		return "accepted"
	}

	hasSent := false
	hasDeadLettered := false
	hasFailed := false
	hasActive := false
	for _, attempt := range attempts {
		switch attempt.Status {
		case "sent":
			hasSent = true
		case "dead_lettered":
			hasDeadLettered = true
		case "failed":
			hasFailed = true
		case "pending", "in_progress", "retry_scheduled":
			hasActive = true
		}
	}

	switch {
	case hasSent && (hasDeadLettered || hasFailed || hasActive):
		return "partially_delivered"
	case hasSent:
		return "delivered"
	case hasActive:
		return "processing"
	case hasDeadLettered:
		return "dead_lettered"
	case hasFailed:
		return "failed"
	default:
		return "accepted"
	}
}

func (p *Postgres) notificationIDForAttempt(ctx context.Context, attemptID string) (string, error) {
	var notificationID string
	if err := p.DB.QueryRowContext(ctx, `SELECT notification_id FROM delivery_attempts WHERE id = $1`, attemptID).Scan(&notificationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return notificationID, nil
}

// GetInitialAttemptByNotificationID returns the initial attempt for a
// notification.
func (p *Postgres) GetInitialAttemptByNotificationID(ctx context.Context, notificationID string) (DeliveryAttempt, error) {
	const query = `
		SELECT id, notification_id, channel, attempt_number, status, error_code, error_message, provider_message_id, last_error, next_retry_at, started_at, completed_at, sent_at, failed_at, dispatch_enqueued_at, enqueue_kind, created_at, updated_at
		FROM delivery_attempts
		WHERE notification_id = $1 AND enqueue_kind = 'initial'
		ORDER BY attempt_number ASC
		LIMIT 1
	`
	var attempt DeliveryAttempt
	if err := scanDeliveryAttempt(p.DB.QueryRowContext(ctx, query, notificationID), &attempt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeliveryAttempt{}, fmt.Errorf("get initial attempt: %w", ErrNotFound)
		}
		return DeliveryAttempt{}, fmt.Errorf("get initial attempt: %w", err)
	}
	return attempt, nil
}

// EnsureInitialAttempt repairs or recreates the initial attempt and dispatch
// intent for idempotent request recovery paths.
func (p *Postgres) EnsureInitialAttempt(ctx context.Context, notificationID, channel, attemptID, intentID string) (DeliveryAttempt, DispatchIntent, error) {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return DeliveryAttempt{}, DispatchIntent{}, fmt.Errorf("ensure initial attempt: begin tx: %w", err)
	}
	defer tx.Rollback()

	var tenantID string
	if err := tx.QueryRowContext(ctx, `SELECT tenant_id FROM notifications WHERE id = $1`, notificationID).Scan(&tenantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeliveryAttempt{}, DispatchIntent{}, fmt.Errorf("ensure initial attempt: %w", ErrNotFound)
		}
		return DeliveryAttempt{}, DispatchIntent{}, fmt.Errorf("ensure initial attempt: tenant lookup: %w", err)
	}

	const insertQuery = `
		INSERT INTO delivery_attempts (
			id,
			notification_id,
			channel,
			attempt_number,
			status,
			dispatch_enqueued_at,
			enqueue_kind
		)
		VALUES ($1, $2, $3, 1, 'pending', NULL, 'initial')
		ON CONFLICT (notification_id, channel, attempt_number) DO NOTHING
	`
	if _, err := tx.ExecContext(ctx, insertQuery, attemptID, notificationID, channel); err != nil {
		return DeliveryAttempt{}, DispatchIntent{}, wrapStoreError("ensure initial attempt", err)
	}
	const selectQuery = `
		SELECT id, notification_id, channel, attempt_number, status, error_code, error_message, provider_message_id, last_error, next_retry_at, started_at, completed_at, sent_at, failed_at, dispatch_enqueued_at, enqueue_kind, created_at, updated_at
		FROM delivery_attempts
		WHERE notification_id = $1 AND channel = $2 AND attempt_number = 1
		LIMIT 1
	`
	var attempt DeliveryAttempt
	if err := scanDeliveryAttempt(tx.QueryRowContext(ctx, selectQuery, notificationID, channel), &attempt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeliveryAttempt{}, DispatchIntent{}, fmt.Errorf("ensure initial attempt: %w", ErrNotFound)
		}
		return DeliveryAttempt{}, DispatchIntent{}, fmt.Errorf("ensure initial attempt: %w", err)
	}
	intent, err := createDispatchIntentTx(ctx, tx, CreateDispatchIntentParams{
		ID:             intentID,
		NotificationID: attempt.NotificationID,
		AttemptID:      attempt.ID,
		TenantID:       tenantID,
		Channel:        attempt.Channel,
		Source:         "initial",
		Status:         publishedIntentStatus(attempt.DispatchEnqueuedAt),
		PublishedAt:    attempt.DispatchEnqueuedAt,
	})
	if err != nil {
		return DeliveryAttempt{}, DispatchIntent{}, wrapStoreError("ensure initial attempt: create dispatch intent", err)
	}
	if err := tx.Commit(); err != nil {
		return DeliveryAttempt{}, DispatchIntent{}, fmt.Errorf("ensure initial attempt: commit: %w", err)
	}
	if err := p.RecalculateNotificationStatus(ctx, notificationID); err != nil {
		return DeliveryAttempt{}, DispatchIntent{}, err
	}
	return attempt, intent, nil
}

// CreateDeliveryAttempt inserts a delivery attempt row.
func (p *Postgres) CreateDeliveryAttempt(ctx context.Context, params CreateDeliveryAttemptParams) (DeliveryAttempt, error) {
	attempt, err := createDeliveryAttemptTx(ctx, p.DB, params)
	if err != nil {
		return DeliveryAttempt{}, err
	}

	return attempt, nil
}

func createDeliveryAttemptTx(ctx context.Context, querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, params CreateDeliveryAttemptParams) (DeliveryAttempt, error) {
	const query = `
		INSERT INTO delivery_attempts (
			id,
			notification_id,
			channel,
			attempt_number,
			status,
			next_retry_at,
			last_error,
			dispatch_enqueued_at,
			enqueue_kind
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING
			id,
			notification_id,
			channel,
			attempt_number,
			status,
			error_code,
			error_message,
			provider_message_id,
			last_error,
			next_retry_at,
			started_at,
			completed_at,
			sent_at,
			failed_at,
			dispatch_enqueued_at,
			enqueue_kind,
			created_at,
			updated_at
	`

	var attempt DeliveryAttempt
	err := scanDeliveryAttempt(querier.QueryRowContext(
		ctx,
		query,
		params.ID,
		params.NotificationID,
		params.Channel,
		params.AttemptNumber,
		params.Status,
		params.NextRetryAt,
		params.LastError,
		params.DispatchEnqueuedAt,
		params.EnqueueKind,
	), &attempt)
	if err != nil {
		return DeliveryAttempt{}, wrapStoreError("create delivery attempt", err)
	}

	return attempt, nil
}

// CreateDispatchIntent inserts a Stage 8 outbox row.
func (p *Postgres) CreateDispatchIntent(ctx context.Context, params CreateDispatchIntentParams) (DispatchIntent, error) {
	intent, err := createDispatchIntentTx(ctx, p.DB, params)
	if err != nil {
		return DispatchIntent{}, err
	}
	return intent, nil
}

func createDispatchIntentTx(ctx context.Context, querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, params CreateDispatchIntentParams) (DispatchIntent, error) {
	const insertQuery = `
		INSERT INTO dispatch_outbox (
			id,
			notification_id,
			attempt_id,
			tenant_id,
			channel,
			source,
			status,
			published_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (attempt_id) DO NOTHING
		RETURNING id, notification_id, attempt_id, tenant_id, channel, source, status, last_error, created_at, claimed_at, published_at
	`
	const selectQuery = `
		SELECT id, notification_id, attempt_id, tenant_id, channel, source, status, last_error, created_at, claimed_at, published_at
		FROM dispatch_outbox
		WHERE attempt_id = $1
	`

	var intent DispatchIntent
	if err := scanDispatchIntent(querier.QueryRowContext(
		ctx,
		insertQuery,
		params.ID,
		params.NotificationID,
		params.AttemptID,
		params.TenantID,
		params.Channel,
		params.Source,
		dispatchIntentStatusOrDefault(params.Status),
		params.PublishedAt,
	), &intent); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return DispatchIntent{}, wrapStoreError("create dispatch intent", err)
		}
		if err := scanDispatchIntent(querier.QueryRowContext(ctx, selectQuery, params.AttemptID), &intent); err != nil {
			return DispatchIntent{}, wrapStoreError("create dispatch intent", err)
		}
	}
	return intent, nil
}

func dispatchIntentStatusOrDefault(status string) string {
	if status == "" {
		return "pending"
	}
	return status
}

func publishedIntentStatus(publishedAt *time.Time) string {
	if publishedAt != nil {
		return "published"
	}
	return "pending"
}

// LoadDeliveryJob loads the durable notification, template, and attempt needed
// by a channel worker.
func (p *Postgres) LoadDeliveryJob(ctx context.Context, notificationID, attemptID string) (Notification, Template, DeliveryAttempt, error) {
	notification, err := p.GetNotificationByID(ctx, notificationID)
	if err != nil {
		return Notification{}, Template{}, DeliveryAttempt{}, err
	}
	template, err := p.GetTemplateByID(ctx, notification.TemplateID)
	if err != nil {
		return Notification{}, Template{}, DeliveryAttempt{}, err
	}
	attempt, err := p.GetDeliveryAttemptByID(ctx, attemptID)
	if err != nil {
		return Notification{}, Template{}, DeliveryAttempt{}, err
	}
	if attempt.NotificationID != notificationID {
		return Notification{}, Template{}, DeliveryAttempt{}, fmt.Errorf("load delivery job: attempt %s does not belong to notification %s", attemptID, notificationID)
	}
	return notification, template, attempt, nil
}

// GetNotificationByID returns one notification by ID.
func (p *Postgres) GetNotificationByID(ctx context.Context, id string) (Notification, error) {
	const query = `
		SELECT id, tenant_id, template_id, idempotency_key, status, recipient_email, recipient_webhook_url, secondary_webhook_url, variables, scheduled_for, promoted_at, cancelled_at, submitted_at, updated_at
		FROM notifications
		WHERE id = $1
	`
	var notification Notification
	var rawVariables []byte
	err := p.DB.QueryRowContext(ctx, query, id).Scan(&notification.ID, &notification.TenantID, &notification.TemplateID, &notification.IdempotencyKey, &notification.Status, &notification.RecipientEmail, &notification.RecipientWebhookURL, &notification.SecondaryWebhookURL, &rawVariables, &notification.ScheduledFor, &notification.PromotedAt, &notification.CancelledAt, &notification.SubmittedAt, &notification.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Notification{}, fmt.Errorf("get notification: %w", ErrNotFound)
		}
		return Notification{}, fmt.Errorf("get notification: %w", err)
	}
	notification.Variables, err = unmarshalVariables(rawVariables)
	if err != nil {
		return Notification{}, fmt.Errorf("get notification: %w", err)
	}
	return notification, nil
}

// GetDeliveryAttemptByID returns one attempt by ID.
func (p *Postgres) GetDeliveryAttemptByID(ctx context.Context, id string) (DeliveryAttempt, error) {
	const query = `
		SELECT id, notification_id, channel, attempt_number, status, error_code, error_message, provider_message_id, last_error, next_retry_at, started_at, completed_at, sent_at, failed_at, dispatch_enqueued_at, enqueue_kind, provider_used, failover_used, created_at, updated_at
		FROM delivery_attempts
		WHERE id = $1
	`
	var attempt DeliveryAttempt
	err := p.DB.QueryRowContext(ctx, query, id).Scan(&attempt.ID, &attempt.NotificationID, &attempt.Channel, &attempt.AttemptNumber, &attempt.Status, &attempt.ErrorCode, &attempt.ErrorMessage, &attempt.ProviderMessageID, &attempt.LastError, &attempt.NextRetryAt, &attempt.StartedAt, &attempt.CompletedAt, &attempt.SentAt, &attempt.FailedAt, &attempt.DispatchEnqueuedAt, &attempt.EnqueueKind, &attempt.ProviderUsed, &attempt.FailoverUsed, &attempt.CreatedAt, &attempt.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeliveryAttempt{}, fmt.Errorf("get delivery attempt: %w", ErrNotFound)
		}
		return DeliveryAttempt{}, fmt.Errorf("get delivery attempt: %w", err)
	}
	return attempt, nil
}

// ListDeliveryAttemptsByNotificationID returns all attempts for a notification
// in inspection-friendly order.
func (p *Postgres) ListDeliveryAttemptsByNotificationID(ctx context.Context, notificationID string) ([]DeliveryAttempt, error) {
	rows, err := p.DB.QueryContext(ctx, `
		WITH dead_lettered_attempts AS (
			SELECT
				id,
				notification_id,
				channel,
				ROW_NUMBER() OVER (PARTITION BY notification_id, channel ORDER BY attempt_number ASC, created_at ASC, id ASC) AS seq
			FROM delivery_attempts
			WHERE notification_id = $1 AND status = 'dead_lettered'
		),
		dead_letter_rows AS (
			SELECT
				id,
				notification_id,
				channel,
				ROW_NUMBER() OVER (PARTITION BY notification_id, channel ORDER BY dead_lettered_at ASC, id ASC) AS seq
			FROM dead_letters
			WHERE notification_id = $1
		)
		SELECT da.id, da.notification_id, da.channel, da.attempt_number, da.status, da.error_code, da.error_message, da.provider_message_id, da.last_error, da.next_retry_at, da.started_at, da.completed_at, da.sent_at, da.failed_at, da.dispatch_enqueued_at, da.enqueue_kind, da.provider_used, da.failover_used, dlr.id, replay_dl.id, da.created_at, da.updated_at
		FROM delivery_attempts da
		LEFT JOIN dead_lettered_attempts dla ON dla.id = da.id
		LEFT JOIN dead_letter_rows dlr ON dlr.notification_id = da.notification_id AND dlr.channel = da.channel AND dlr.seq = dla.seq
		LEFT JOIN dead_letters replay_dl ON replay_dl.replay_attempt_id = da.id
		WHERE da.notification_id = $1
		ORDER BY da.attempt_number ASC, da.created_at ASC
	`, notificationID)
	if err != nil {
		return nil, fmt.Errorf("list delivery attempts: %w", err)
	}
	defer rows.Close()

	var attempts []DeliveryAttempt
	for rows.Next() {
		var attempt DeliveryAttempt
		if err := rows.Scan(&attempt.ID, &attempt.NotificationID, &attempt.Channel, &attempt.AttemptNumber, &attempt.Status, &attempt.ErrorCode, &attempt.ErrorMessage, &attempt.ProviderMessageID, &attempt.LastError, &attempt.NextRetryAt, &attempt.StartedAt, &attempt.CompletedAt, &attempt.SentAt, &attempt.FailedAt, &attempt.DispatchEnqueuedAt, &attempt.EnqueueKind, &attempt.ProviderUsed, &attempt.FailoverUsed, &attempt.DeadLetterID, &attempt.ReplayOfDeadLetter, &attempt.CreatedAt, &attempt.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list delivery attempts: %w", err)
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list delivery attempts: %w", err)
	}
	return attempts, nil
}

// RecalculateNotificationStatus recomputes the notification rollup from its
// current attempt state.
func (p *Postgres) RecalculateNotificationStatus(ctx context.Context, notificationID string) error {
	var scheduledFor, promotedAt, cancelledAt *time.Time
	if err := p.DB.QueryRowContext(ctx, `
		SELECT scheduled_for, promoted_at, cancelled_at
		FROM notifications
		WHERE id = $1
	`, notificationID).Scan(&scheduledFor, &promotedAt, &cancelledAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("recalculate notification status: %w", ErrNotFound)
		}
		return fmt.Errorf("recalculate notification status: %w", err)
	}

	attempts, err := p.ListDeliveryAttemptsByNotificationID(ctx, notificationID)
	if err != nil {
		return fmt.Errorf("recalculate notification status: %w", err)
	}

	status := deriveNotificationStatus(attempts)
	if cancelledAt != nil {
		status = "cancelled"
	} else if scheduledFor != nil && promotedAt == nil && scheduledFor.After(time.Now().UTC()) {
		status = "scheduled"
	}
	result, err := p.DB.ExecContext(ctx, `
		UPDATE notifications
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`, notificationID, status)
	if err != nil {
		return fmt.Errorf("recalculate notification status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("recalculate notification status: rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("recalculate notification status: %w", ErrNotFound)
	}
	return nil
}

// MarkAttemptInProgress atomically transitions an attempt from pending to
// in_progress.
func (p *Postgres) MarkAttemptInProgress(ctx context.Context, attemptID string) error {
	const query = `
		UPDATE delivery_attempts
		SET status = 'in_progress', started_at = COALESCE(started_at, NOW()), completed_at = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`
	result, err := p.DB.ExecContext(ctx, query, attemptID)
	if err != nil {
		return fmt.Errorf("mark attempt in progress: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark attempt in progress: rows affected: %w", err)
	}
	if rows == 0 {
		attempt, loadErr := p.GetDeliveryAttemptByID(ctx, attemptID)
		if loadErr != nil {
			return fmt.Errorf("mark attempt in progress: %w", loadErr)
		}
		switch {
		case attempt.Status == "in_progress":
			return fmt.Errorf("mark attempt in progress: %w", ErrAttemptAlreadyProcessing)
		case isAttemptTerminalState(attempt.Status):
			return fmt.Errorf("mark attempt in progress: %w", ErrAttemptAlreadyFinalized)
		default:
			return fmt.Errorf("mark attempt in progress: %w", ErrInvalidStateTransition)
		}
	}
	notificationID, err := p.notificationIDForAttempt(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("mark attempt in progress: notification lookup: %w", err)
	}
	if err := p.RecalculateNotificationStatus(ctx, notificationID); err != nil {
		return fmt.Errorf("mark attempt in progress: recalculate notification status: %w", err)
	}
	return nil
}

// MarkAttemptSent finalizes an attempt as sent.
func (p *Postgres) MarkAttemptSent(ctx context.Context, attemptID string, providerMessageID *string) error {
	const query = `
		UPDATE delivery_attempts
		SET status = 'sent', provider_message_id = $2, last_error = NULL, error_message = NULL, sent_at = NOW(), failed_at = NULL, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'in_progress'
	`
	result, err := p.DB.ExecContext(ctx, query, attemptID, providerMessageID)
	if err != nil {
		return fmt.Errorf("mark attempt sent: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark attempt sent: rows affected: %w", err)
	}
	if rows == 0 {
		attempt, loadErr := p.GetDeliveryAttemptByID(ctx, attemptID)
		if loadErr != nil {
			return fmt.Errorf("mark attempt sent: %w", loadErr)
		}
		if isAttemptTerminalState(attempt.Status) {
			return fmt.Errorf("mark attempt sent: %w", ErrAttemptAlreadyFinalized)
		}
		return fmt.Errorf("mark attempt sent: %w", ErrInvalidStateTransition)
	}
	notificationID, err := p.notificationIDForAttempt(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("mark attempt sent: notification lookup: %w", err)
	}
	return p.RecalculateNotificationStatus(ctx, notificationID)
}

// MarkAttemptFailed finalizes an attempt as failed without retry scheduling.
func (p *Postgres) MarkAttemptFailed(ctx context.Context, attemptID string, lastError string) error {
	const query = `
		UPDATE delivery_attempts
		SET status = 'failed', last_error = $2, error_message = $2, provider_message_id = NULL, failed_at = NOW(), completed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'in_progress'
	`
	result, err := p.DB.ExecContext(ctx, query, attemptID, lastError)
	if err != nil {
		return fmt.Errorf("mark attempt failed: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark attempt failed: rows affected: %w", err)
	}
	if rows == 0 {
		attempt, loadErr := p.GetDeliveryAttemptByID(ctx, attemptID)
		if loadErr != nil {
			return fmt.Errorf("mark attempt failed: %w", loadErr)
		}
		if isAttemptTerminalState(attempt.Status) {
			return fmt.Errorf("mark attempt failed: %w", ErrAttemptAlreadyFinalized)
		}
		return fmt.Errorf("mark attempt failed: %w", ErrInvalidStateTransition)
	}
	notificationID, err := p.notificationIDForAttempt(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("mark attempt failed: notification lookup: %w", err)
	}
	return p.RecalculateNotificationStatus(ctx, notificationID)
}

// ReplayDeadLetterResult returns the durable rows produced by a replay path.
type ReplayDeadLetterResult struct {
	DeadLetter DeadLetter
	Attempt    DeliveryAttempt
}

// ScheduleRetry moves an attempt into retry_scheduled and stores the next retry
// timestamp.
func (p *Postgres) ScheduleRetry(ctx context.Context, attemptID, lastError string, nextRetryAt time.Time) error {
	const query = `
		UPDATE delivery_attempts
		SET status = 'retry_scheduled', last_error = $2, error_message = $2, next_retry_at = $3, provider_message_id = NULL, failed_at = NOW(), completed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'in_progress'
	`
	result, err := p.DB.ExecContext(ctx, query, attemptID, lastError, nextRetryAt)
	if err != nil {
		return fmt.Errorf("schedule retry: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("schedule retry: rows affected: %w", err)
	}
	if rows == 0 {
		attempt, loadErr := p.GetDeliveryAttemptByID(ctx, attemptID)
		if loadErr != nil {
			return fmt.Errorf("schedule retry: %w", loadErr)
		}
		if isAttemptTerminalState(attempt.Status) {
			return fmt.Errorf("schedule retry: %w", ErrAttemptAlreadyFinalized)
		}
		return fmt.Errorf("schedule retry: %w", ErrInvalidStateTransition)
	}
	notificationID, err := p.notificationIDForAttempt(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("schedule retry: notification lookup: %w", err)
	}
	return p.RecalculateNotificationStatus(ctx, notificationID)
}

// MarkAttemptDeadLettered finalizes an attempt as dead-lettered.
func (p *Postgres) MarkAttemptDeadLettered(ctx context.Context, attemptID, lastError string) error {
	const query = `
		UPDATE delivery_attempts
		SET status = 'dead_lettered', last_error = $2, error_message = $2, next_retry_at = NULL, provider_message_id = NULL, failed_at = NOW(), completed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'in_progress'
	`
	result, err := p.DB.ExecContext(ctx, query, attemptID, lastError)
	if err != nil {
		return fmt.Errorf("mark attempt dead lettered: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark attempt dead lettered: rows affected: %w", err)
	}
	if rows == 0 {
		attempt, loadErr := p.GetDeliveryAttemptByID(ctx, attemptID)
		if loadErr != nil {
			return fmt.Errorf("mark attempt dead lettered: %w", loadErr)
		}
		if isAttemptTerminalState(attempt.Status) {
			return fmt.Errorf("mark attempt dead lettered: %w", ErrAttemptAlreadyFinalized)
		}
		return fmt.Errorf("mark attempt dead lettered: %w", ErrInvalidStateTransition)
	}
	notificationID, err := p.notificationIDForAttempt(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("mark attempt dead lettered: notification lookup: %w", err)
	}
	return p.RecalculateNotificationStatus(ctx, notificationID)
}

// InsertDeadLetter creates a durable dead-letter row.
func (p *Postgres) InsertDeadLetter(ctx context.Context, id, notificationID, channel, finalError string) (DeadLetter, error) {
	const query = `
		INSERT INTO dead_letters (id, notification_id, channel, final_error)
		VALUES ($1, $2, $3, $4)
		RETURNING id, notification_id, channel, final_error, dead_lettered_at, replayed_at, replay_attempt_id
	`
	var deadLetter DeadLetter
	if err := p.DB.QueryRowContext(ctx, query, id, notificationID, channel, finalError).Scan(
		&deadLetter.ID,
		&deadLetter.NotificationID,
		&deadLetter.Channel,
		&deadLetter.FinalError,
		&deadLetter.DeadLetteredAt,
		&deadLetter.ReplayedAt,
		&deadLetter.ReplayAttemptID,
	); err != nil {
		return DeadLetter{}, wrapStoreError("insert dead letter", err)
	}
	return deadLetter, nil
}

// ListDeadLetters returns recent dead letters for operator inspection.
func (p *Postgres) ListDeadLetters(ctx context.Context, limit int) ([]DeadLetter, error) {
	if limit <= 0 {
		limit = 100
	}
	const query = `
		SELECT id, notification_id, channel, final_error, dead_lettered_at, replayed_at, replay_attempt_id
		FROM dead_letters
		ORDER BY dead_lettered_at DESC
		LIMIT $1
	`
	rows, err := p.DB.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list dead letters: %w", err)
	}
	defer rows.Close()
	var deadLetters []DeadLetter
	for rows.Next() {
		var dl DeadLetter
		if err := rows.Scan(&dl.ID, &dl.NotificationID, &dl.Channel, &dl.FinalError, &dl.DeadLetteredAt, &dl.ReplayedAt, &dl.ReplayAttemptID); err != nil {
			return nil, fmt.Errorf("list dead letters: %w", err)
		}
		deadLetters = append(deadLetters, dl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list dead letters: %w", err)
	}
	return deadLetters, nil
}

// GetDeadLetterByID returns one dead-letter row by ID.
func (p *Postgres) GetDeadLetterByID(ctx context.Context, id string) (DeadLetter, error) {
	const query = `
		SELECT id, notification_id, channel, final_error, dead_lettered_at, replayed_at, replay_attempt_id
		FROM dead_letters
		WHERE id = $1
	`
	var dl DeadLetter
	if err := p.DB.QueryRowContext(ctx, query, id).Scan(&dl.ID, &dl.NotificationID, &dl.Channel, &dl.FinalError, &dl.DeadLetteredAt, &dl.ReplayedAt, &dl.ReplayAttemptID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeadLetter{}, fmt.Errorf("get dead letter: %w", ErrNotFound)
		}
		return DeadLetter{}, fmt.Errorf("get dead letter: %w", err)
	}
	return dl, nil
}

// FinalizeDeadLetterReplay is the older replay path that recreates work and
// marks the dead letter replayed in one transaction.
func (p *Postgres) FinalizeDeadLetterReplay(ctx context.Context, deadLetterID, newAttemptID string) (ReplayDeadLetterResult, error) {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return ReplayDeadLetterResult{}, fmt.Errorf("finalize dead letter replay: begin tx: %w", err)
	}
	defer tx.Rollback()

	const lockQuery = `
		SELECT id, notification_id, channel, final_error, dead_lettered_at, replayed_at, replay_attempt_id
		FROM dead_letters
		WHERE id = $1
		FOR UPDATE
	`
	var dl DeadLetter
	if err := tx.QueryRowContext(ctx, lockQuery, deadLetterID).Scan(&dl.ID, &dl.NotificationID, &dl.Channel, &dl.FinalError, &dl.DeadLetteredAt, &dl.ReplayedAt, &dl.ReplayAttemptID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ReplayDeadLetterResult{}, fmt.Errorf("finalize dead letter replay: %w", ErrNotFound)
		}
		return ReplayDeadLetterResult{}, fmt.Errorf("finalize dead letter replay: %w", err)
	}

	attempt, err := getAttemptByIDTx(ctx, tx, newAttemptID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return ReplayDeadLetterResult{}, fmt.Errorf("finalize dead letter replay: get attempt: %w", err)
	}
	if errors.Is(err, ErrNotFound) {
		var attemptNumber int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(attempt_number), 0) + 1 FROM delivery_attempts WHERE notification_id = $1 AND channel = $2`, dl.NotificationID, dl.Channel).Scan(&attemptNumber); err != nil {
			return ReplayDeadLetterResult{}, fmt.Errorf("finalize dead letter replay: next attempt number: %w", err)
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO delivery_attempts (id, notification_id, channel, attempt_number, status)
			VALUES ($1, $2, $3, $4, 'pending')
			RETURNING id, notification_id, channel, attempt_number, status, error_code, error_message, provider_message_id, last_error, next_retry_at, started_at, completed_at, sent_at, failed_at, created_at, updated_at
		`, newAttemptID, dl.NotificationID, dl.Channel, attemptNumber).Scan(
			&attempt.ID, &attempt.NotificationID, &attempt.Channel, &attempt.AttemptNumber, &attempt.Status, &attempt.ErrorCode, &attempt.ErrorMessage, &attempt.ProviderMessageID, &attempt.LastError, &attempt.NextRetryAt, &attempt.StartedAt, &attempt.CompletedAt, &attempt.SentAt, &attempt.FailedAt, &attempt.DispatchEnqueuedAt, &attempt.EnqueueKind, &attempt.CreatedAt, &attempt.UpdatedAt,
		); err != nil {
			return ReplayDeadLetterResult{}, wrapStoreError("finalize dead letter replay", err)
		}
	}
	if dl.ReplayedAt == nil {
		replayedAt := time.Now().UTC()
		if _, err := tx.ExecContext(ctx, `UPDATE dead_letters SET replayed_at = $2 WHERE id = $1 AND replayed_at IS NULL`, deadLetterID, replayedAt); err != nil {
			return ReplayDeadLetterResult{}, fmt.Errorf("finalize dead letter replay: mark replayed: %w", err)
		}
		dl.ReplayedAt = &replayedAt
	}
	if err := tx.Commit(); err != nil {
		return ReplayDeadLetterResult{}, fmt.Errorf("finalize dead letter replay: commit: %w", err)
	}
	return ReplayDeadLetterResult{DeadLetter: dl, Attempt: attempt}, nil
}

// ListDueRetryAttempts returns retry-scheduled attempts whose retry time is due.
func (p *Postgres) ListDueRetryAttempts(ctx context.Context, limit int) ([]RetryDueAttempt, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := p.DB.QueryContext(ctx, `
		SELECT da.id, da.notification_id, da.channel, da.attempt_number, da.status, da.error_code, da.error_message, da.provider_message_id, da.last_error, da.next_retry_at, da.started_at, da.completed_at, da.sent_at, da.failed_at, da.created_at, da.updated_at, n.tenant_id
		FROM delivery_attempts da
		JOIN notifications n ON n.id = da.notification_id
		WHERE da.status = 'retry_scheduled' AND da.next_retry_at <= NOW()
		ORDER BY da.next_retry_at ASC, da.created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list due retry attempts: %w", err)
	}
	defer rows.Close()
	var items []RetryDueAttempt
	for rows.Next() {
		var item RetryDueAttempt
		if err := rows.Scan(&item.Attempt.ID, &item.Attempt.NotificationID, &item.Attempt.Channel, &item.Attempt.AttemptNumber, &item.Attempt.Status, &item.Attempt.ErrorCode, &item.Attempt.ErrorMessage, &item.Attempt.ProviderMessageID, &item.Attempt.LastError, &item.Attempt.NextRetryAt, &item.Attempt.StartedAt, &item.Attempt.CompletedAt, &item.Attempt.SentAt, &item.Attempt.FailedAt, &item.Attempt.CreatedAt, &item.Attempt.UpdatedAt, &item.TenantID); err != nil {
			return nil, fmt.Errorf("list due retry attempts: %w", err)
		}
		item.NotificationID = item.Attempt.NotificationID
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list due retry attempts: %w", err)
	}
	return items, nil
}

// FinalizeRetryDispatch is the older retry path that creates retry work in one
// transaction.
func (p *Postgres) FinalizeRetryDispatch(ctx context.Context, scheduledAttemptID, newAttemptID string) (RetryDueAttempt, error) {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return RetryDueAttempt{}, fmt.Errorf("finalize retry dispatch: begin tx: %w", err)
	}
	defer tx.Rollback()
	var item RetryDueAttempt
	if err := tx.QueryRowContext(ctx, `
		SELECT da.id, da.notification_id, da.channel, da.attempt_number, da.status, da.error_code, da.error_message, da.provider_message_id, da.last_error, da.next_retry_at, da.started_at, da.completed_at, da.sent_at, da.failed_at, da.created_at, da.updated_at, n.tenant_id
		FROM delivery_attempts da
		JOIN notifications n ON n.id = da.notification_id
		WHERE da.id = $1
		FOR UPDATE
	`, scheduledAttemptID).Scan(&item.Attempt.ID, &item.Attempt.NotificationID, &item.Attempt.Channel, &item.Attempt.AttemptNumber, &item.Attempt.Status, &item.Attempt.ErrorCode, &item.Attempt.ErrorMessage, &item.Attempt.ProviderMessageID, &item.Attempt.LastError, &item.Attempt.NextRetryAt, &item.Attempt.StartedAt, &item.Attempt.CompletedAt, &item.Attempt.SentAt, &item.Attempt.FailedAt, &item.Attempt.CreatedAt, &item.Attempt.UpdatedAt, &item.TenantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RetryDueAttempt{}, fmt.Errorf("finalize retry dispatch: %w", ErrNotFound)
		}
		return RetryDueAttempt{}, fmt.Errorf("finalize retry dispatch: %w", err)
	}
	item.NotificationID = item.Attempt.NotificationID
	attempt, err := getAttemptByIDTx(ctx, tx, newAttemptID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return RetryDueAttempt{}, fmt.Errorf("finalize retry dispatch: get attempt: %w", err)
	}
	if errors.Is(err, ErrNotFound) {
		if item.Attempt.Status != "retry_scheduled" || item.Attempt.NextRetryAt == nil || item.Attempt.NextRetryAt.After(time.Now().UTC()) {
			return RetryDueAttempt{}, fmt.Errorf("finalize retry dispatch: %w", ErrNotFound)
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO delivery_attempts (id, notification_id, channel, attempt_number, status)
			VALUES ($1, $2, $3, $4, 'pending')
			RETURNING id, notification_id, channel, attempt_number, status, error_code, error_message, provider_message_id, last_error, next_retry_at, started_at, completed_at, sent_at, failed_at, created_at, updated_at
		`, newAttemptID, item.Attempt.NotificationID, item.Attempt.Channel, item.Attempt.AttemptNumber+1).Scan(
			&attempt.ID, &attempt.NotificationID, &attempt.Channel, &attempt.AttemptNumber, &attempt.Status, &attempt.ErrorCode, &attempt.ErrorMessage, &attempt.ProviderMessageID, &attempt.LastError, &attempt.NextRetryAt, &attempt.StartedAt, &attempt.CompletedAt, &attempt.SentAt, &attempt.FailedAt, &attempt.DispatchEnqueuedAt, &attempt.EnqueueKind, &attempt.CreatedAt, &attempt.UpdatedAt,
		); err != nil {
			return RetryDueAttempt{}, wrapStoreError("finalize retry dispatch", err)
		}
	}
	if item.Attempt.Status == "retry_scheduled" {
		if _, err := tx.ExecContext(ctx, `UPDATE delivery_attempts SET status = 'failed', next_retry_at = NULL, updated_at = NOW() WHERE id = $1 AND status = 'retry_scheduled'`, scheduledAttemptID); err != nil {
			return RetryDueAttempt{}, fmt.Errorf("finalize retry dispatch: mark prior failed: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return RetryDueAttempt{}, fmt.Errorf("finalize retry dispatch: commit: %w", err)
	}
	item.Attempt = attempt
	return item, nil
}

func getAttemptByIDTx(ctx context.Context, tx *sql.Tx, id string) (DeliveryAttempt, error) {
	var attempt DeliveryAttempt
	if err := scanDeliveryAttempt(tx.QueryRowContext(ctx, `
		SELECT id, notification_id, channel, attempt_number, status, error_code, error_message, provider_message_id, last_error, next_retry_at, started_at, completed_at, sent_at, failed_at, dispatch_enqueued_at, enqueue_kind, created_at, updated_at
		FROM delivery_attempts
		WHERE id = $1
	`, id), &attempt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeliveryAttempt{}, ErrNotFound
		}
		return DeliveryAttempt{}, err
	}
	return attempt, nil
}

// PendingEnqueueAttempt exposes the older compatibility view of attempts that
// still need queue publication.
type PendingEnqueueAttempt struct {
	Attempt      DeliveryAttempt
	TenantID     string
	DeadLetterID *string
}

// EnsureRetryAttempt repairs or recreates the retry attempt and its dispatch
// intent.
func (p *Postgres) EnsureRetryAttempt(ctx context.Context, scheduledAttemptID, newAttemptID string) (RetryDueAttempt, error) {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return RetryDueAttempt{}, fmt.Errorf("ensure retry attempt: begin tx: %w", err)
	}
	defer tx.Rollback()
	var item RetryDueAttempt
	if err := tx.QueryRowContext(ctx, `
		SELECT da.id, da.notification_id, da.channel, da.attempt_number, da.status, da.error_code, da.error_message, da.provider_message_id, da.last_error, da.next_retry_at, da.started_at, da.completed_at, da.sent_at, da.failed_at, da.dispatch_enqueued_at, da.enqueue_kind, da.created_at, da.updated_at, n.tenant_id
		FROM delivery_attempts da
		JOIN notifications n ON n.id = da.notification_id
		WHERE da.id = $1
		FOR UPDATE
	`, scheduledAttemptID).Scan(&item.Attempt.ID, &item.Attempt.NotificationID, &item.Attempt.Channel, &item.Attempt.AttemptNumber, &item.Attempt.Status, &item.Attempt.ErrorCode, &item.Attempt.ErrorMessage, &item.Attempt.ProviderMessageID, &item.Attempt.LastError, &item.Attempt.NextRetryAt, &item.Attempt.StartedAt, &item.Attempt.CompletedAt, &item.Attempt.SentAt, &item.Attempt.FailedAt, &item.Attempt.DispatchEnqueuedAt, &item.Attempt.EnqueueKind, &item.Attempt.CreatedAt, &item.Attempt.UpdatedAt, &item.TenantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RetryDueAttempt{}, fmt.Errorf("ensure retry attempt: %w", ErrNotFound)
		}
		return RetryDueAttempt{}, fmt.Errorf("ensure retry attempt: %w", err)
	}
	item.NotificationID = item.Attempt.NotificationID
	createdNewAttempt := false
	attempt, err := getAttemptByIDTx(ctx, tx, newAttemptID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return RetryDueAttempt{}, fmt.Errorf("ensure retry attempt: get child: %w", err)
	}
	if errors.Is(err, ErrNotFound) {
		if item.Attempt.Status != "retry_scheduled" || item.Attempt.NextRetryAt == nil || item.Attempt.NextRetryAt.After(time.Now().UTC()) {
			return RetryDueAttempt{}, fmt.Errorf("ensure retry attempt: %w", ErrNotFound)
		}
		attempt, err = createDeliveryAttemptTx(ctx, tx, CreateDeliveryAttemptParams{
			ID:             newAttemptID,
			NotificationID: item.Attempt.NotificationID,
			Channel:        item.Attempt.Channel,
			AttemptNumber:  item.Attempt.AttemptNumber + 1,
			Status:         "pending",
			EnqueueKind:    "retry",
		})
		if err != nil {
			return RetryDueAttempt{}, fmt.Errorf("ensure retry attempt: %w", err)
		}
		createdNewAttempt = true
	}
	if _, err := createDispatchIntentTx(ctx, tx, CreateDispatchIntentParams{
		ID:             "intent-" + attempt.ID,
		NotificationID: attempt.NotificationID,
		AttemptID:      attempt.ID,
		TenantID:       item.TenantID,
		Channel:        attempt.Channel,
		Source:         "retry",
		Status:         publishedIntentStatus(attempt.DispatchEnqueuedAt),
		PublishedAt:    attempt.DispatchEnqueuedAt,
	}); err != nil {
		return RetryDueAttempt{}, fmt.Errorf("ensure retry attempt: %w", err)
	}
	if item.Attempt.Status == "retry_scheduled" {
		if _, err := tx.ExecContext(ctx, `UPDATE delivery_attempts SET status = 'failed', next_retry_at = NULL, updated_at = NOW() WHERE id = $1 AND status = 'retry_scheduled'`, scheduledAttemptID); err != nil {
			return RetryDueAttempt{}, fmt.Errorf("ensure retry attempt: mark prior failed: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return RetryDueAttempt{}, fmt.Errorf("ensure retry attempt: commit: %w", err)
	}
	if createdNewAttempt {
		if err := p.RecalculateNotificationStatus(ctx, item.NotificationID); err != nil {
			return RetryDueAttempt{}, err
		}
	}
	item.Attempt = attempt
	return item, nil
}

// EnsureReplayAttempt repairs or recreates the replay attempt and its dispatch
// intent.
func (p *Postgres) EnsureReplayAttempt(ctx context.Context, deadLetterID, newAttemptID string) (ReplayDeadLetterResult, error) {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return ReplayDeadLetterResult{}, fmt.Errorf("ensure replay attempt: begin tx: %w", err)
	}
	defer tx.Rollback()
	var dl DeadLetter
	if err := tx.QueryRowContext(ctx, `
		SELECT id, notification_id, channel, final_error, dead_lettered_at, replayed_at, replay_attempt_id
		FROM dead_letters
		WHERE id = $1
		FOR UPDATE
	`, deadLetterID).Scan(&dl.ID, &dl.NotificationID, &dl.Channel, &dl.FinalError, &dl.DeadLetteredAt, &dl.ReplayedAt, &dl.ReplayAttemptID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ReplayDeadLetterResult{}, fmt.Errorf("ensure replay attempt: %w", ErrNotFound)
		}
		return ReplayDeadLetterResult{}, fmt.Errorf("ensure replay attempt: %w", err)
	}
	if dl.ReplayedAt != nil && dl.ReplayAttemptID != nil {
		attempt, err := getAttemptByIDTx(ctx, tx, *dl.ReplayAttemptID)
		if err != nil {
			return ReplayDeadLetterResult{}, fmt.Errorf("ensure replay attempt: existing replay attempt: %w", err)
		}
		return ReplayDeadLetterResult{DeadLetter: dl, Attempt: attempt}, nil
	}
	attemptID := newAttemptID
	if dl.ReplayAttemptID != nil {
		attemptID = *dl.ReplayAttemptID
	}
	attempt, err := getAttemptByIDTx(ctx, tx, attemptID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return ReplayDeadLetterResult{}, fmt.Errorf("ensure replay attempt: get attempt: %w", err)
	}
	if errors.Is(err, ErrNotFound) {
		var attemptNumber int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(attempt_number), 0) + 1 FROM delivery_attempts WHERE notification_id = $1 AND channel = $2`, dl.NotificationID, dl.Channel).Scan(&attemptNumber); err != nil {
			return ReplayDeadLetterResult{}, fmt.Errorf("ensure replay attempt: next attempt number: %w", err)
		}
		attempt, err = createDeliveryAttemptTx(ctx, tx, CreateDeliveryAttemptParams{
			ID:             attemptID,
			NotificationID: dl.NotificationID,
			Channel:        dl.Channel,
			AttemptNumber:  attemptNumber,
			Status:         "pending",
			EnqueueKind:    "replay",
		})
		if err != nil {
			return ReplayDeadLetterResult{}, fmt.Errorf("ensure replay attempt: %w", err)
		}
	}
	var tenantID string
	if err := tx.QueryRowContext(ctx, `SELECT tenant_id FROM notifications WHERE id = $1`, dl.NotificationID).Scan(&tenantID); err != nil {
		return ReplayDeadLetterResult{}, fmt.Errorf("ensure replay attempt: tenant lookup: %w", err)
	}
	if _, err := createDispatchIntentTx(ctx, tx, CreateDispatchIntentParams{
		ID:             "intent-" + attempt.ID,
		NotificationID: attempt.NotificationID,
		AttemptID:      attempt.ID,
		TenantID:       tenantID,
		Channel:        attempt.Channel,
		Source:         "replay",
		Status:         publishedIntentStatus(attempt.DispatchEnqueuedAt),
		PublishedAt:    attempt.DispatchEnqueuedAt,
	}); err != nil {
		return ReplayDeadLetterResult{}, fmt.Errorf("ensure replay attempt: %w", err)
	}
	if dl.ReplayAttemptID == nil {
		if _, err := tx.ExecContext(ctx, `UPDATE dead_letters SET replay_attempt_id = $2 WHERE id = $1 AND replay_attempt_id IS NULL`, deadLetterID, attempt.ID); err != nil {
			return ReplayDeadLetterResult{}, fmt.Errorf("ensure replay attempt: set replay attempt id: %w", err)
		}
		dl.ReplayAttemptID = &attempt.ID
	}
	if err := tx.Commit(); err != nil {
		return ReplayDeadLetterResult{}, fmt.Errorf("ensure replay attempt: commit: %w", err)
	}
	if err := p.RecalculateNotificationStatus(ctx, dl.NotificationID); err != nil {
		return ReplayDeadLetterResult{}, err
	}
	return ReplayDeadLetterResult{DeadLetter: dl, Attempt: attempt}, nil
}

// ClaimPendingDispatchIntents leases a batch of outbox rows to one publisher.
func (p *Postgres) ClaimPendingDispatchIntents(ctx context.Context, limit int, staleAfter time.Duration) ([]PendingDispatchIntent, error) {
	if limit <= 0 {
		limit = 100
	}
	if staleAfter <= 0 {
		return nil, fmt.Errorf("claim pending dispatch intents: staleAfter must be positive")
	}
	staleAfterSeconds := int(staleAfter / time.Second)
	if staleAfterSeconds == 0 {
		staleAfterSeconds = 1
	}
	rows, err := p.DB.QueryContext(ctx, `
		WITH claimed AS (
			UPDATE dispatch_outbox AS o
			SET status = 'publishing', claimed_at = NOW()
			WHERE o.id IN (
				SELECT i.id
				FROM dispatch_outbox i
				WHERE (
					i.status = 'pending'
					OR (i.status = 'publishing' AND i.claimed_at IS NOT NULL AND i.claimed_at <= NOW() - ($2 * INTERVAL '1 second'))
				)
				  AND COALESCE(
					(
						SELECT p_tc.paused
						FROM delivery_policies p_tc
						WHERE p_tc.tenant_id = i.tenant_id AND p_tc.channel = i.channel
						ORDER BY p_tc.updated_at DESC
						LIMIT 1
					),
					(
						SELECT p_t.paused
						FROM delivery_policies p_t
						WHERE p_t.tenant_id = i.tenant_id AND p_t.channel IS NULL
						ORDER BY p_t.updated_at DESC
						LIMIT 1
					),
					(
						SELECT p_gc.paused
						FROM delivery_policies p_gc
						WHERE p_gc.tenant_id IS NULL AND p_gc.channel = i.channel
						ORDER BY p_gc.updated_at DESC
						LIMIT 1
					),
					(
						SELECT p_g.paused
						FROM delivery_policies p_g
						WHERE p_g.tenant_id IS NULL AND p_g.channel IS NULL
						ORDER BY p_g.updated_at DESC
						LIMIT 1
					),
					false
				) = false
				ORDER BY i.created_at ASC
				FOR UPDATE SKIP LOCKED
				LIMIT $1
			)
			RETURNING o.id, o.notification_id, o.attempt_id, o.tenant_id, o.channel, o.source, o.status, o.last_error, o.created_at, o.claimed_at, o.published_at
		)
		SELECT c.id, c.notification_id, c.attempt_id, c.tenant_id, c.channel, c.source, c.status, c.last_error, c.created_at, c.claimed_at, c.published_at, dl.id
		FROM claimed c
		LEFT JOIN dead_letters dl ON dl.replay_attempt_id = c.attempt_id
		ORDER BY c.created_at ASC
	`, limit, staleAfterSeconds)
	if err != nil {
		return nil, fmt.Errorf("claim pending dispatch intents: %w", err)
	}
	defer rows.Close()

	var intents []PendingDispatchIntent
	for rows.Next() {
		var item PendingDispatchIntent
		if err := rows.Scan(
			&item.Intent.ID,
			&item.Intent.NotificationID,
			&item.Intent.AttemptID,
			&item.Intent.TenantID,
			&item.Intent.Channel,
			&item.Intent.Source,
			&item.Intent.Status,
			&item.Intent.LastError,
			&item.Intent.CreatedAt,
			&item.Intent.ClaimedAt,
			&item.Intent.PublishedAt,
			&item.DeadLetterID,
		); err != nil {
			return nil, fmt.Errorf("claim pending dispatch intents: %w", err)
		}
		intents = append(intents, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim pending dispatch intents: %w", err)
	}
	return intents, nil
}

// MarkDispatchIntentPublished finalizes a claimed dispatch intent as published.
func (p *Postgres) MarkDispatchIntentPublished(ctx context.Context, intentID string, claimedAt time.Time) error {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mark dispatch intent published: begin tx: %w", err)
	}
	defer tx.Rollback()

	var intent DispatchIntent
	if err := scanDispatchIntent(tx.QueryRowContext(ctx, `
		UPDATE dispatch_outbox
		SET status = 'published', claimed_at = NULL, published_at = COALESCE(published_at, NOW()), last_error = NULL
		WHERE id = $1 AND status = 'publishing' AND claimed_at = $2
		RETURNING id, notification_id, attempt_id, tenant_id, channel, source, status, last_error, created_at, claimed_at, published_at
	`, intentID, claimedAt), &intent); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if err := scanDispatchIntent(tx.QueryRowContext(ctx, `
				SELECT id, notification_id, attempt_id, tenant_id, channel, source, status, last_error, created_at, claimed_at, published_at
				FROM dispatch_outbox
				WHERE id = $1
			`, intentID), &intent); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("mark dispatch intent published: %w", ErrNotFound)
				}
				return fmt.Errorf("mark dispatch intent published: %w", err)
			}
			if intent.Status == "published" {
				if err := tx.Commit(); err != nil {
					return fmt.Errorf("mark dispatch intent published: commit: %w", err)
				}
				return nil
			}
			return fmt.Errorf("mark dispatch intent published: %w", ErrInvalidStateTransition)
		}
		return fmt.Errorf("mark dispatch intent published: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE delivery_attempts
		SET dispatch_enqueued_at = COALESCE(dispatch_enqueued_at, NOW()), updated_at = NOW()
		WHERE id = $1
	`, intent.AttemptID); err != nil {
		return fmt.Errorf("mark dispatch intent published: mark attempt enqueued: %w", err)
	}

	if intent.Source == "replay" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE dead_letters
			SET replayed_at = COALESCE(replayed_at, NOW())
			WHERE replay_attempt_id = $1
		`, intent.AttemptID); err != nil {
			return fmt.Errorf("mark dispatch intent published: mark replayed: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("mark dispatch intent published: commit: %w", err)
	}
	return nil
}

// RecordDispatchIntentError releases a claimed intent back to pending with the
// latest publish failure recorded.
func (p *Postgres) RecordDispatchIntentError(ctx context.Context, intentID string, claimedAt time.Time, lastError string) error {
	result, err := p.DB.ExecContext(ctx, `
		UPDATE dispatch_outbox
		SET status = 'pending', claimed_at = NULL, last_error = $2
		WHERE id = $1 AND status = 'publishing' AND claimed_at = $3
	`, intentID, lastError, claimedAt)
	if err != nil {
		return fmt.Errorf("record dispatch intent error: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("record dispatch intent error: rows affected: %w", err)
	}
	if rows == 0 {
		var status string
		if err := p.DB.QueryRowContext(ctx, `
			SELECT status
			FROM dispatch_outbox
			WHERE id = $1
		`, intentID).Scan(&status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("record dispatch intent error: %w", ErrNotFound)
			}
			return fmt.Errorf("record dispatch intent error: %w", err)
		}
		if status == "published" {
			return nil
		}
		return fmt.Errorf("record dispatch intent error: %w", ErrInvalidStateTransition)
	}
	return nil
}

// ListAttemptsPendingEnqueue exposes the older pre-outbox repair surface for
// compatibility and migration-safe inspection.
func (p *Postgres) ListAttemptsPendingEnqueue(ctx context.Context, limit int) ([]PendingEnqueueAttempt, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.DB.QueryContext(ctx, `
		SELECT da.id, da.notification_id, da.channel, da.attempt_number, da.status, da.error_code, da.error_message, da.provider_message_id, da.last_error, da.next_retry_at, da.started_at, da.completed_at, da.sent_at, da.failed_at, da.dispatch_enqueued_at, da.enqueue_kind, da.created_at, da.updated_at, n.tenant_id, dl.id
		FROM delivery_attempts da
		JOIN notifications n ON n.id = da.notification_id
		LEFT JOIN dead_letters dl ON dl.replay_attempt_id = da.id
		WHERE da.status = 'pending' AND da.dispatch_enqueued_at IS NULL AND da.enqueue_kind IN ('initial', 'retry', 'replay')
		ORDER BY da.created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list attempts pending enqueue: %w", err)
	}
	defer rows.Close()
	var items []PendingEnqueueAttempt
	for rows.Next() {
		var item PendingEnqueueAttempt
		if err := rows.Scan(&item.Attempt.ID, &item.Attempt.NotificationID, &item.Attempt.Channel, &item.Attempt.AttemptNumber, &item.Attempt.Status, &item.Attempt.ErrorCode, &item.Attempt.ErrorMessage, &item.Attempt.ProviderMessageID, &item.Attempt.LastError, &item.Attempt.NextRetryAt, &item.Attempt.StartedAt, &item.Attempt.CompletedAt, &item.Attempt.SentAt, &item.Attempt.FailedAt, &item.Attempt.DispatchEnqueuedAt, &item.Attempt.EnqueueKind, &item.Attempt.CreatedAt, &item.Attempt.UpdatedAt, &item.TenantID, &item.DeadLetterID); err != nil {
			return nil, fmt.Errorf("list attempts pending enqueue: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list attempts pending enqueue: %w", err)
	}
	return items, nil
}

// MarkAttemptEnqueued records that an attempt has been published to Redis.
func (p *Postgres) MarkAttemptEnqueued(ctx context.Context, attemptID string) error {
	result, err := p.DB.ExecContext(ctx, `UPDATE delivery_attempts SET dispatch_enqueued_at = COALESCE(dispatch_enqueued_at, NOW()), updated_at = NOW() WHERE id = $1`, attemptID)
	if err != nil {
		return fmt.Errorf("mark attempt enqueued: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark attempt enqueued: rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("mark attempt enqueued: %w", ErrNotFound)
	}
	return nil
}

// FinalizeReplayEnqueue is the older replay compatibility path that marks work
// as enqueued and replayed together.
func (p *Postgres) FinalizeReplayEnqueue(ctx context.Context, deadLetterID, attemptID string) error {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("finalize replay enqueue: begin tx: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE delivery_attempts SET dispatch_enqueued_at = COALESCE(dispatch_enqueued_at, NOW()), updated_at = NOW() WHERE id = $1`, attemptID); err != nil {
		return fmt.Errorf("finalize replay enqueue: mark attempt enqueued: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE dead_letters SET replayed_at = COALESCE(replayed_at, NOW()) WHERE id = $1 AND replay_attempt_id = $2`, deadLetterID, attemptID); err != nil {
		return fmt.Errorf("finalize replay enqueue: mark dead letter replayed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("finalize replay enqueue: commit: %w", err)
	}
	return nil
}

// UpdateAttemptProvider stores which provider handled an attempt and whether
// failover was used.
func (p *Postgres) UpdateAttemptProvider(ctx context.Context, attemptID, provider string, failoverUsed bool) error {
	result, err := p.DB.ExecContext(ctx, `
		UPDATE delivery_attempts
		SET provider_used = $2, failover_used = $3, updated_at = NOW()
		WHERE id = $1
	`, attemptID, provider, failoverUsed)
	if err != nil {
		return fmt.Errorf("update attempt provider: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update attempt provider: rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("update attempt provider: %w", ErrNotFound)
	}
	return nil
}

// ListDeliveryPolicies returns all durable policy rows.
func (p *Postgres) ListDeliveryPolicies(ctx context.Context) ([]DeliveryPolicy, error) {
	rows, err := p.DB.QueryContext(ctx, `
		SELECT id, tenant_id, channel, paused, failover_enabled, scheduling_enabled, replay_allowed, max_attempts_override, retry_base_delay_seconds, retry_max_delay_seconds, created_at, updated_at
		FROM delivery_policies
		ORDER BY tenant_id NULLS FIRST, channel NULLS FIRST, created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list delivery policies: %w", err)
	}
	defer rows.Close()
	var policies []DeliveryPolicy
	for rows.Next() {
		var policy DeliveryPolicy
		if err := scanDeliveryPolicy(rows, &policy); err != nil {
			return nil, fmt.Errorf("list delivery policies: %w", err)
		}
		policies = append(policies, policy)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list delivery policies: %w", err)
	}
	return policies, nil
}

// GetDeliveryPolicyByID returns one policy row by ID.
func (p *Postgres) GetDeliveryPolicyByID(ctx context.Context, id string) (DeliveryPolicy, error) {
	var policy DeliveryPolicy
	if err := scanDeliveryPolicy(p.DB.QueryRowContext(ctx, `
		SELECT id, tenant_id, channel, paused, failover_enabled, scheduling_enabled, replay_allowed, max_attempts_override, retry_base_delay_seconds, retry_max_delay_seconds, created_at, updated_at
		FROM delivery_policies
		WHERE id = $1
	`, id), &policy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeliveryPolicy{}, fmt.Errorf("get delivery policy: %w", ErrNotFound)
		}
		return DeliveryPolicy{}, fmt.Errorf("get delivery policy: %w", err)
	}
	return policy, nil
}

// UpsertDeliveryPolicy inserts or updates one delivery policy row.
func (p *Postgres) UpsertDeliveryPolicy(ctx context.Context, params UpsertDeliveryPolicyParams) (DeliveryPolicy, error) {
	var policy DeliveryPolicy
	if err := scanDeliveryPolicy(p.DB.QueryRowContext(ctx, `
		INSERT INTO delivery_policies (
			id, tenant_id, channel, paused, failover_enabled, scheduling_enabled, replay_allowed, max_attempts_override, retry_base_delay_seconds, retry_max_delay_seconds
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			tenant_id = EXCLUDED.tenant_id,
			channel = EXCLUDED.channel,
			paused = EXCLUDED.paused,
			failover_enabled = EXCLUDED.failover_enabled,
			scheduling_enabled = EXCLUDED.scheduling_enabled,
			replay_allowed = EXCLUDED.replay_allowed,
			max_attempts_override = EXCLUDED.max_attempts_override,
			retry_base_delay_seconds = EXCLUDED.retry_base_delay_seconds,
			retry_max_delay_seconds = EXCLUDED.retry_max_delay_seconds,
			updated_at = NOW()
		RETURNING id, tenant_id, channel, paused, failover_enabled, scheduling_enabled, replay_allowed, max_attempts_override, retry_base_delay_seconds, retry_max_delay_seconds, created_at, updated_at
	`, params.ID, params.TenantID, params.Channel, nullableBool(params.Paused), nullableBool(params.FailoverEnabled), nullableBool(params.SchedulingEnabled), nullableBool(params.ReplayAllowed), nullableInt(params.MaxAttemptsOverride), nullableInt(params.RetryBaseDelaySeconds), nullableInt(params.RetryMaxDelaySeconds)), &policy); err != nil {
		return DeliveryPolicy{}, wrapStoreError("upsert delivery policy", err)
	}
	return policy, nil
}

// SetDeliveryPolicyPaused updates the paused flag for a policy row.
func (p *Postgres) SetDeliveryPolicyPaused(ctx context.Context, id string, paused bool) (DeliveryPolicy, error) {
	var policy DeliveryPolicy
	if err := scanDeliveryPolicy(p.DB.QueryRowContext(ctx, `
		UPDATE delivery_policies
		SET paused = $2, updated_at = NOW()
		WHERE id = $1
		RETURNING id, tenant_id, channel, paused, failover_enabled, scheduling_enabled, replay_allowed, max_attempts_override, retry_base_delay_seconds, retry_max_delay_seconds, created_at, updated_at
	`, id, paused), &policy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeliveryPolicy{}, fmt.Errorf("set delivery policy paused: %w", ErrNotFound)
		}
		return DeliveryPolicy{}, fmt.Errorf("set delivery policy paused: %w", err)
	}
	return policy, nil
}

// ResolveDeliveryPolicy applies the tenant/channel policy precedence rules and
// returns the effective policy.
func (p *Postgres) ResolveDeliveryPolicy(ctx context.Context, tenantID, channel string) (ResolvedDeliveryPolicy, error) {
	rows, err := p.DB.QueryContext(ctx, `
		SELECT id, tenant_id, channel, paused, failover_enabled, scheduling_enabled, replay_allowed, max_attempts_override, retry_base_delay_seconds, retry_max_delay_seconds, created_at, updated_at
		FROM delivery_policies
		WHERE (tenant_id = $1 OR tenant_id IS NULL)
		  AND (channel = $2 OR channel IS NULL)
		ORDER BY
			CASE
				WHEN tenant_id = $1 AND channel = $2 THEN 1
				WHEN tenant_id = $1 AND channel IS NULL THEN 2
				WHEN tenant_id IS NULL AND channel = $2 THEN 3
				ELSE 4
			END,
			updated_at DESC,
			created_at DESC
	`, tenantID, channel)
	if err != nil {
		return ResolvedDeliveryPolicy{}, fmt.Errorf("resolve delivery policy: %w", err)
	}
	defer rows.Close()

	resolved := ResolvedDeliveryPolicy{
		TenantID:          tenantID,
		Channel:           channel,
		Paused:            false,
		FailoverEnabled:   false,
		SchedulingEnabled: true,
		ReplayAllowed:     true,
	}
	var ordered []DeliveryPolicy
	for rows.Next() {
		var policy DeliveryPolicy
		if err := scanDeliveryPolicy(rows, &policy); err != nil {
			return ResolvedDeliveryPolicy{}, fmt.Errorf("resolve delivery policy: %w", err)
		}
		ordered = append(ordered, policy)
	}
	if err := rows.Err(); err != nil {
		return ResolvedDeliveryPolicy{}, fmt.Errorf("resolve delivery policy: %w", err)
	}
	for i := len(ordered) - 1; i >= 0; i-- {
		applyPolicyRow(&resolved, ordered[i])
	}
	return resolved, nil
}

// PromoteDueScheduledNotifications turns due scheduled notifications into live
// dispatch intents.
func (p *Postgres) PromoteDueScheduledNotifications(ctx context.Context, limit int, now time.Time) ([]PromotedScheduledNotification, error) {
	if limit <= 0 {
		limit = 100
	}
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("promote scheduled notifications: begin tx: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT n.id, n.tenant_id, n.template_id, n.idempotency_key, n.status, n.recipient_email, n.recipient_webhook_url, n.secondary_webhook_url, n.variables, n.scheduled_for, n.promoted_at, n.cancelled_at, n.submitted_at, n.updated_at,
		       da.id, da.notification_id, da.channel, da.attempt_number, da.status, da.error_code, da.error_message, da.provider_message_id, da.last_error, da.next_retry_at, da.started_at, da.completed_at, da.sent_at, da.failed_at, da.dispatch_enqueued_at, da.enqueue_kind, da.created_at, da.updated_at
		FROM notifications n
		JOIN delivery_attempts da ON da.notification_id = n.id AND da.enqueue_kind = 'initial' AND da.attempt_number = 1
		WHERE n.scheduled_for IS NOT NULL
		  AND n.scheduled_for <= $2
		  AND n.promoted_at IS NULL
		  AND n.cancelled_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM dispatch_outbox o WHERE o.attempt_id = da.id)
		  AND COALESCE(
			(
				SELECT p_tc.paused
				FROM delivery_policies p_tc
				WHERE p_tc.tenant_id = n.tenant_id AND p_tc.channel = da.channel
				ORDER BY p_tc.updated_at DESC
				LIMIT 1
			),
			(
				SELECT p_t.paused
				FROM delivery_policies p_t
				WHERE p_t.tenant_id = n.tenant_id AND p_t.channel IS NULL
				ORDER BY p_t.updated_at DESC
				LIMIT 1
			),
			(
				SELECT p_gc.paused
				FROM delivery_policies p_gc
				WHERE p_gc.tenant_id IS NULL AND p_gc.channel = da.channel
				ORDER BY p_gc.updated_at DESC
				LIMIT 1
			),
			(
				SELECT p_g.paused
				FROM delivery_policies p_g
				WHERE p_g.tenant_id IS NULL AND p_g.channel IS NULL
				ORDER BY p_g.updated_at DESC
				LIMIT 1
			),
			false
		  ) = false
		ORDER BY n.scheduled_for ASC, n.submitted_at ASC
		FOR UPDATE OF n, da SKIP LOCKED
		LIMIT $1
	`, limit, now)
	if err != nil {
		return nil, fmt.Errorf("promote scheduled notifications: %w", err)
	}
	defer rows.Close()

	var promoted []PromotedScheduledNotification
	for rows.Next() {
		var item PromotedScheduledNotification
		var rawVariables []byte
		if err := rows.Scan(
			&item.Notification.ID, &item.Notification.TenantID, &item.Notification.TemplateID, &item.Notification.IdempotencyKey, &item.Notification.Status, &item.Notification.RecipientEmail, &item.Notification.RecipientWebhookURL, &item.Notification.SecondaryWebhookURL, &rawVariables, &item.Notification.ScheduledFor, &item.Notification.PromotedAt, &item.Notification.CancelledAt, &item.Notification.SubmittedAt, &item.Notification.UpdatedAt,
			&item.Attempt.ID, &item.Attempt.NotificationID, &item.Attempt.Channel, &item.Attempt.AttemptNumber, &item.Attempt.Status, &item.Attempt.ErrorCode, &item.Attempt.ErrorMessage, &item.Attempt.ProviderMessageID, &item.Attempt.LastError, &item.Attempt.NextRetryAt, &item.Attempt.StartedAt, &item.Attempt.CompletedAt, &item.Attempt.SentAt, &item.Attempt.FailedAt, &item.Attempt.DispatchEnqueuedAt, &item.Attempt.EnqueueKind, &item.Attempt.CreatedAt, &item.Attempt.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("promote scheduled notifications: %w", err)
		}
		item.Notification.Variables, err = unmarshalVariables(rawVariables)
		if err != nil {
			return nil, fmt.Errorf("promote scheduled notifications: %w", err)
		}
		item.Intent, err = createDispatchIntentTx(ctx, tx, CreateDispatchIntentParams{
			ID:             "intent-" + item.Attempt.ID,
			NotificationID: item.Notification.ID,
			AttemptID:      item.Attempt.ID,
			TenantID:       item.Notification.TenantID,
			Channel:        item.Attempt.Channel,
			Source:         "scheduled",
		})
		if err != nil {
			return nil, fmt.Errorf("promote scheduled notifications: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE notifications
			SET promoted_at = COALESCE(promoted_at, $2), updated_at = NOW()
			WHERE id = $1
		`, item.Notification.ID, now); err != nil {
			return nil, fmt.Errorf("promote scheduled notifications: mark promoted: %w", err)
		}
		item.Notification.PromotedAt = &now
		promoted = append(promoted, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("promote scheduled notifications: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("promote scheduled notifications: commit: %w", err)
	}
	for _, item := range promoted {
		if err := p.RecalculateNotificationStatus(ctx, item.Notification.ID); err != nil {
			return nil, err
		}
	}
	return promoted, nil
}

// CancelScheduledNotification cancels still-future scheduled work that has not
// yet been promoted.
func (p *Postgres) CancelScheduledNotification(ctx context.Context, notificationID string) (Notification, error) {
	result, err := p.DB.ExecContext(ctx, `
		UPDATE notifications
		SET status = 'cancelled', cancelled_at = NOW(), updated_at = NOW()
		WHERE id = $1
		  AND scheduled_for > NOW()
		  AND status = 'scheduled'
		  AND promoted_at IS NULL
		  AND cancelled_at IS NULL
		  AND NOT EXISTS (
			SELECT 1
			FROM dispatch_outbox o
			WHERE o.notification_id = notifications.id
		  )
	`, notificationID)
	if err != nil {
		return Notification{}, fmt.Errorf("cancel scheduled notification: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Notification{}, fmt.Errorf("cancel scheduled notification: rows affected: %w", err)
	}
	if rows == 0 {
		var exists bool
		if err := p.DB.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM notifications WHERE id = $1)
		`, notificationID).Scan(&exists); err != nil {
			return Notification{}, fmt.Errorf("cancel scheduled notification: check existence: %w", err)
		}
		if !exists {
			return Notification{}, fmt.Errorf("cancel scheduled notification: %w", ErrNotFound)
		}
		return Notification{}, fmt.Errorf("cancel scheduled notification: %w", ErrInvalidStateTransition)
	}
	return p.GetNotificationByID(ctx, notificationID)
}

// RedriveNotification manually promotes eligible scheduled work into the outbox
// path.
func (p *Postgres) RedriveNotification(ctx context.Context, notificationID string) (PromotedScheduledNotification, error) {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return PromotedScheduledNotification{}, fmt.Errorf("redrive notification: begin tx: %w", err)
	}
	defer tx.Rollback()

	var item PromotedScheduledNotification
	var rawVariables []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT n.id, n.tenant_id, n.template_id, n.idempotency_key, n.status, n.recipient_email, n.recipient_webhook_url, n.secondary_webhook_url, n.variables, n.scheduled_for, n.promoted_at, n.cancelled_at, n.submitted_at, n.updated_at,
		       da.id, da.notification_id, da.channel, da.attempt_number, da.status, da.error_code, da.error_message, da.provider_message_id, da.last_error, da.next_retry_at, da.started_at, da.completed_at, da.sent_at, da.failed_at, da.dispatch_enqueued_at, da.enqueue_kind, da.created_at, da.updated_at
		FROM notifications n
		JOIN delivery_attempts da ON da.notification_id = n.id AND da.enqueue_kind = 'initial' AND da.attempt_number = 1
		WHERE n.id = $1
		  AND n.promoted_at IS NULL
		  AND n.cancelled_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM dispatch_outbox o WHERE o.attempt_id = da.id)
		FOR UPDATE OF n, da
	`, notificationID).Scan(
		&item.Notification.ID, &item.Notification.TenantID, &item.Notification.TemplateID, &item.Notification.IdempotencyKey, &item.Notification.Status, &item.Notification.RecipientEmail, &item.Notification.RecipientWebhookURL, &item.Notification.SecondaryWebhookURL, &rawVariables, &item.Notification.ScheduledFor, &item.Notification.PromotedAt, &item.Notification.CancelledAt, &item.Notification.SubmittedAt, &item.Notification.UpdatedAt,
		&item.Attempt.ID, &item.Attempt.NotificationID, &item.Attempt.Channel, &item.Attempt.AttemptNumber, &item.Attempt.Status, &item.Attempt.ErrorCode, &item.Attempt.ErrorMessage, &item.Attempt.ProviderMessageID, &item.Attempt.LastError, &item.Attempt.NextRetryAt, &item.Attempt.StartedAt, &item.Attempt.CompletedAt, &item.Attempt.SentAt, &item.Attempt.FailedAt, &item.Attempt.DispatchEnqueuedAt, &item.Attempt.EnqueueKind, &item.Attempt.CreatedAt, &item.Attempt.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			var exists bool
			if err := p.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM notifications WHERE id = $1)`, notificationID).Scan(&exists); err != nil {
				return PromotedScheduledNotification{}, fmt.Errorf("redrive notification: check existence: %w", err)
			}
			if !exists {
				return PromotedScheduledNotification{}, fmt.Errorf("redrive notification: %w", ErrNotFound)
			}
			return PromotedScheduledNotification{}, fmt.Errorf("redrive notification: %w", ErrInvalidStateTransition)
		}
		return PromotedScheduledNotification{}, fmt.Errorf("redrive notification: %w", err)
	}
	item.Notification.Variables, err = unmarshalVariables(rawVariables)
	if err != nil {
		return PromotedScheduledNotification{}, fmt.Errorf("redrive notification: %w", err)
	}
	if item.Notification.CancelledAt != nil {
		return PromotedScheduledNotification{}, fmt.Errorf("redrive notification: %w", ErrInvalidStateTransition)
	}
	policy, err := p.ResolveDeliveryPolicy(ctx, item.Notification.TenantID, item.Attempt.Channel)
	if err != nil {
		return PromotedScheduledNotification{}, err
	}
	if policy.Paused {
		return PromotedScheduledNotification{}, fmt.Errorf("redrive notification: %w", ErrInvalidStateTransition)
	}
	item.Intent, err = createDispatchIntentTx(ctx, tx, CreateDispatchIntentParams{
		ID:             "intent-" + item.Attempt.ID,
		NotificationID: item.Notification.ID,
		AttemptID:      item.Attempt.ID,
		TenantID:       item.Notification.TenantID,
		Channel:        item.Attempt.Channel,
		Source:         "manual_redrive",
	})
	if err != nil {
		return PromotedScheduledNotification{}, fmt.Errorf("redrive notification: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		UPDATE notifications
		SET promoted_at = COALESCE(promoted_at, $2), updated_at = NOW()
		WHERE id = $1
	`, item.Notification.ID, now); err != nil {
		return PromotedScheduledNotification{}, fmt.Errorf("redrive notification: mark promoted: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return PromotedScheduledNotification{}, fmt.Errorf("redrive notification: commit: %w", err)
	}
	item.Notification.PromotedAt = &now
	if err := p.RecalculateNotificationStatus(ctx, item.Notification.ID); err != nil {
		return PromotedScheduledNotification{}, err
	}
	return item, nil
}

// RecordAuditEvent inserts one durable audit event.
func (p *Postgres) RecordAuditEvent(ctx context.Context, id, tenantID, actor, action, resourceType, resourceID string, metadata map[string]any) error {
	rawMetadata, err := marshalVariables(metadata)
	if err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	if _, err := p.DB.ExecContext(ctx, `
		INSERT INTO audit_events (id, tenant_id, actor, action, resource_type, resource_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
	`, id, tenantID, actor, action, resourceType, resourceID, rawMetadata); err != nil {
		return wrapStoreError("record audit event", err)
	}
	return nil
}

// CollectOperationalMetrics returns the Stage 10 store-backed metrics snapshot
// used by the metrics endpoint.
func (p *Postgres) CollectOperationalMetrics(ctx context.Context, now time.Time) (OperationalMetrics, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	metrics := OperationalMetrics{CollectedAt: now}
	if err := p.DB.QueryRowContext(ctx, `
		SELECT
			COALESCE(COUNT(*) FILTER (WHERE status = 'pending'), 0),
			COALESCE(COUNT(*) FILTER (WHERE status = 'publishing'), 0),
			COALESCE(MAX(EXTRACT(EPOCH FROM ($1 - created_at))) FILTER (WHERE status = 'pending'), 0)
		FROM dispatch_outbox
	`, now).Scan(&metrics.OutboxPendingCount, &metrics.OutboxPublishingCount, &metrics.OutboxOldestLagSeconds); err != nil {
		return OperationalMetrics{}, fmt.Errorf("collect operational metrics: outbox: %w", err)
	}
	if err := p.DB.QueryRowContext(ctx, `
		SELECT
			COALESCE(COUNT(*) FILTER (WHERE status = 'retry_scheduled' AND next_retry_at <= $1), 0),
			COALESCE(COUNT(*) FILTER (WHERE status = 'retry_scheduled'), 0)
		FROM delivery_attempts
	`, now).Scan(&metrics.DueRetryCount, &metrics.RetryScheduledCount); err != nil {
		return OperationalMetrics{}, fmt.Errorf("collect operational metrics: retries: %w", err)
	}
	if err := p.DB.QueryRowContext(ctx, `
		SELECT COALESCE(COUNT(*), 0)
		FROM dead_letters
		WHERE replayed_at IS NULL
	`).Scan(&metrics.OpenDeadLetterCount); err != nil {
		return OperationalMetrics{}, fmt.Errorf("collect operational metrics: dead letters: %w", err)
	}
	if err := p.DB.QueryRowContext(ctx, `
		SELECT
			COALESCE(COUNT(*) FILTER (WHERE scheduled_for IS NOT NULL AND promoted_at IS NULL AND cancelled_at IS NULL), 0),
			COALESCE(COUNT(*) FILTER (WHERE scheduled_for IS NOT NULL AND scheduled_for <= $1 AND promoted_at IS NULL AND cancelled_at IS NULL), 0),
			COALESCE(MAX(EXTRACT(EPOCH FROM ($1 - scheduled_for))) FILTER (WHERE scheduled_for IS NOT NULL AND scheduled_for <= $1 AND promoted_at IS NULL AND cancelled_at IS NULL), 0)
		FROM notifications
	`, now).Scan(&metrics.ScheduledPendingCount, &metrics.ScheduledDueCount, &metrics.ScheduledOldestLagSeconds); err != nil {
		return OperationalMetrics{}, fmt.Errorf("collect operational metrics: scheduled: %w", err)
	}
	if err := p.DB.QueryRowContext(ctx, `
		SELECT
			COALESCE(COUNT(*) FILTER (WHERE action = 'duplicate_job_suppressed'), 0),
			COALESCE(COUNT(*) FILTER (WHERE action = 'dead_lettered'), 0)
		FROM audit_events
	`).Scan(&metrics.DuplicateSuppressionCount, &metrics.DeadLetteredCount); err != nil {
		return OperationalMetrics{}, fmt.Errorf("collect operational metrics: audits: %w", err)
	}
	return metrics, nil
}

// RunMaintenance performs one explicit retention-driven cleanup pass.
func (p *Postgres) RunMaintenance(ctx context.Context, params CleanupParams) (CleanupResult, error) {
	if params.Now.IsZero() {
		params.Now = time.Now().UTC()
	}
	if params.AuditRetention <= 0 {
		return CleanupResult{}, fmt.Errorf("run maintenance: audit retention must be positive")
	}
	if params.OutboxRetention <= 0 {
		return CleanupResult{}, fmt.Errorf("run maintenance: outbox retention must be positive")
	}
	if params.DeadLetterRetention < 0 {
		return CleanupResult{}, fmt.Errorf("run maintenance: dead letter retention must be >= 0")
	}

	result := CleanupResult{DryRun: params.DryRun, ExecutedAt: params.Now}
	auditCutoff := params.Now.Add(-params.AuditRetention)
	outboxCutoff := params.Now.Add(-params.OutboxRetention)
	deadLetterCutoff := params.Now.Add(-params.DeadLetterRetention)

	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("run maintenance: begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM audit_events
		WHERE created_at < $1
	`, auditCutoff).Scan(&result.AuditEventsDeleted); err != nil {
		return CleanupResult{}, fmt.Errorf("run maintenance: audit count: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM dispatch_outbox
		WHERE status = 'published' AND published_at IS NOT NULL AND published_at < $1
	`, outboxCutoff).Scan(&result.PublishedOutboxDeleted); err != nil {
		return CleanupResult{}, fmt.Errorf("run maintenance: outbox count: %w", err)
	}
	if params.DeadLetterRetention > 0 {
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM dead_letters
			WHERE replayed_at IS NOT NULL AND replayed_at < $1
		`, deadLetterCutoff).Scan(&result.DeadLettersDeleted); err != nil {
			return CleanupResult{}, fmt.Errorf("run maintenance: dead letter count: %w", err)
		}
	}

	if params.DryRun {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			return CleanupResult{}, fmt.Errorf("run maintenance: rollback dry run: %w", err)
		}
		return result, nil
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM audit_events WHERE created_at < $1`, auditCutoff); err != nil {
		return CleanupResult{}, fmt.Errorf("run maintenance: delete audit events: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM dispatch_outbox WHERE status = 'published' AND published_at IS NOT NULL AND published_at < $1`, outboxCutoff); err != nil {
		return CleanupResult{}, fmt.Errorf("run maintenance: delete outbox rows: %w", err)
	}
	if params.DeadLetterRetention > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM dead_letters WHERE replayed_at IS NOT NULL AND replayed_at < $1`, deadLetterCutoff); err != nil {
			return CleanupResult{}, fmt.Errorf("run maintenance: delete dead letters: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return CleanupResult{}, fmt.Errorf("run maintenance: commit: %w", err)
	}
	return result, nil
}
