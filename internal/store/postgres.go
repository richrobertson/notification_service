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
	ErrNotFound                 = errors.New("store: not found")
	ErrConflict                 = errors.New("store: conflict")
	ErrAttemptAlreadyFinalized  = errors.New("store: attempt already finalized")
	ErrAttemptAlreadyProcessing = errors.New("store: attempt already processing")
	ErrInvalidStateTransition   = errors.New("store: invalid state transition")
)

type Postgres struct {
	DB *sql.DB
}

func touchUpdatedAtSetClause() string {
	return "updated_at = NOW()"
}

type Tenant struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	DailyQuota int       `json:"daily_quota"`
	CreatedAt  time.Time `json:"created_at"`
}

type Template struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Channel   string    `json:"channel"`
	Version   int       `json:"version"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type Notification struct {
	ID                  string         `json:"id"`
	TenantID            string         `json:"tenant_id"`
	TemplateID          string         `json:"template_id"`
	IdempotencyKey      *string        `json:"idempotency_key"`
	Status              string         `json:"status"`
	RecipientEmail      *string        `json:"recipient_email"`
	RecipientWebhookURL *string        `json:"recipient_webhook_url"`
	Variables           map[string]any `json:"variables"`
	SubmittedAt         time.Time      `json:"submitted_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

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
	DeadLetterID       *string    `json:"dead_letter_id,omitempty"`
	ReplayOfDeadLetter *string    `json:"replay_of_dead_letter_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

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

type CreateTenantParams struct {
	ID         string
	Name       string
	DailyQuota int
}

type CreateTemplateParams struct {
	ID       string
	TenantID string
	Name     string
	Channel  string
	Version  int
	Body     string
}

type CreateNotificationParams struct {
	ID                  string
	TenantID            string
	TemplateID          string
	IdempotencyKey      *string
	RecipientEmail      *string
	RecipientWebhookURL *string
	Variables           map[string]any
}

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

type DeadLetter struct {
	ID              string     `json:"id"`
	NotificationID  string     `json:"notification_id"`
	Channel         string     `json:"channel"`
	FinalError      string     `json:"final_error"`
	DeadLetteredAt  time.Time  `json:"dead_lettered_at"`
	ReplayedAt      *time.Time `json:"replayed_at"`
	ReplayAttemptID *string    `json:"replay_attempt_id"`
}

type RetryDueAttempt struct {
	Attempt        DeliveryAttempt
	NotificationID string
	TenantID       string
}

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

func (p *Postgres) Close() error {
	if p == nil || p.DB == nil {
		return nil
	}

	if err := p.DB.Close(); err != nil {
		return fmt.Errorf("close postgres connection: %w", err)
	}

	return nil
}

func (p *Postgres) Ping(ctx context.Context) error {
	if p == nil || p.DB == nil {
		return fmt.Errorf("postgres connection is not initialized")
	}

	if err := p.DB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	return nil
}

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

func (p *Postgres) CreateNotification(ctx context.Context, params CreateNotificationParams) (Notification, error) {
	const query = `
		INSERT INTO notifications (
			id,
			tenant_id,
			template_id,
			idempotency_key,
			status,
			recipient_email,
			recipient_webhook_url,
			variables
		)
		VALUES ($1, $2, $3, $4, 'accepted', $5, $6, $7::jsonb)
		RETURNING
			id,
			tenant_id,
			template_id,
			idempotency_key,
			status,
			recipient_email,
			recipient_webhook_url,
			variables,
			submitted_at,
			updated_at
	`

	variablesJSON, err := marshalVariables(params.Variables)
	if err != nil {
		return Notification{}, fmt.Errorf("create notification: %w", err)
	}

	var notification Notification
	var rawVariables []byte
	err = p.DB.QueryRowContext(
		ctx,
		query,
		params.ID,
		params.TenantID,
		params.TemplateID,
		params.IdempotencyKey,
		params.RecipientEmail,
		params.RecipientWebhookURL,
		variablesJSON,
	).Scan(
		&notification.ID,
		&notification.TenantID,
		&notification.TemplateID,
		&notification.IdempotencyKey,
		&notification.Status,
		&notification.RecipientEmail,
		&notification.RecipientWebhookURL,
		&rawVariables,
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
			variables,
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
		&rawVariables,
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

func IsAttemptAlreadyFinalized(err error) bool {
	return errors.Is(err, ErrAttemptAlreadyFinalized)
}

func IsAttemptAlreadyProcessing(err error) bool {
	return errors.Is(err, ErrAttemptAlreadyProcessing)
}

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
	case hasDeadLettered:
		return "dead_lettered"
	case hasActive:
		return "processing"
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

func (p *Postgres) GetInitialAttemptByNotificationID(ctx context.Context, notificationID string) (DeliveryAttempt, error) {
	const query = `
		SELECT id, notification_id, channel, attempt_number, status, error_code, error_message, provider_message_id, last_error, next_retry_at, started_at, completed_at, sent_at, failed_at, dispatch_enqueued_at, enqueue_kind, created_at, updated_at
		FROM delivery_attempts
		WHERE notification_id = $1 AND enqueue_kind = 'initial'
		ORDER BY attempt_number ASC
		LIMIT 1
	`
	var attempt DeliveryAttempt
	if err := p.DB.QueryRowContext(ctx, query, notificationID).Scan(&attempt.ID, &attempt.NotificationID, &attempt.Channel, &attempt.AttemptNumber, &attempt.Status, &attempt.ErrorCode, &attempt.ErrorMessage, &attempt.ProviderMessageID, &attempt.LastError, &attempt.NextRetryAt, &attempt.StartedAt, &attempt.CompletedAt, &attempt.SentAt, &attempt.FailedAt, &attempt.DispatchEnqueuedAt, &attempt.EnqueueKind, &attempt.CreatedAt, &attempt.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeliveryAttempt{}, fmt.Errorf("get initial attempt: %w", ErrNotFound)
		}
		return DeliveryAttempt{}, fmt.Errorf("get initial attempt: %w", err)
	}
	return attempt, nil
}

func (p *Postgres) EnsureInitialAttempt(ctx context.Context, notificationID, channel, attemptID string) (DeliveryAttempt, error) {
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
	result, err := p.DB.ExecContext(ctx, insertQuery, attemptID, notificationID, channel)
	if err != nil {
		return DeliveryAttempt{}, wrapStoreError("ensure initial attempt", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return DeliveryAttempt{}, wrapStoreError("ensure initial attempt", err)
	}
	if rowsAffected > 0 {
		if err := p.RecalculateNotificationStatus(ctx, notificationID); err != nil {
			return DeliveryAttempt{}, err
		}
	}
	const selectQuery = `
		SELECT id, notification_id, channel, attempt_number, status, error_code, error_message, provider_message_id, last_error, next_retry_at, started_at, completed_at, sent_at, failed_at, dispatch_enqueued_at, enqueue_kind, created_at, updated_at
		FROM delivery_attempts
		WHERE notification_id = $1 AND channel = $2 AND attempt_number = 1
		LIMIT 1
	`
	var attempt DeliveryAttempt
	if err := p.DB.QueryRowContext(ctx, selectQuery, notificationID, channel).Scan(&attempt.ID, &attempt.NotificationID, &attempt.Channel, &attempt.AttemptNumber, &attempt.Status, &attempt.ErrorCode, &attempt.ErrorMessage, &attempt.ProviderMessageID, &attempt.LastError, &attempt.NextRetryAt, &attempt.StartedAt, &attempt.CompletedAt, &attempt.SentAt, &attempt.FailedAt, &attempt.DispatchEnqueuedAt, &attempt.EnqueueKind, &attempt.CreatedAt, &attempt.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeliveryAttempt{}, fmt.Errorf("ensure initial attempt: %w", ErrNotFound)
		}
		return DeliveryAttempt{}, fmt.Errorf("ensure initial attempt: %w", err)
	}
	return attempt, nil
}

func (p *Postgres) CreateDeliveryAttempt(ctx context.Context, params CreateDeliveryAttemptParams) (DeliveryAttempt, error) {
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
	err := p.DB.QueryRowContext(
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
	).Scan(
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
	if err != nil {
		return DeliveryAttempt{}, wrapStoreError("create delivery attempt", err)
	}

	return attempt, nil
}

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

func (p *Postgres) GetNotificationByID(ctx context.Context, id string) (Notification, error) {
	const query = `
		SELECT id, tenant_id, template_id, idempotency_key, status, recipient_email, recipient_webhook_url, variables, submitted_at, updated_at
		FROM notifications
		WHERE id = $1
	`
	var notification Notification
	var rawVariables []byte
	err := p.DB.QueryRowContext(ctx, query, id).Scan(&notification.ID, &notification.TenantID, &notification.TemplateID, &notification.IdempotencyKey, &notification.Status, &notification.RecipientEmail, &notification.RecipientWebhookURL, &rawVariables, &notification.SubmittedAt, &notification.UpdatedAt)
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

func (p *Postgres) GetDeliveryAttemptByID(ctx context.Context, id string) (DeliveryAttempt, error) {
	const query = `
		SELECT id, notification_id, channel, attempt_number, status, error_code, error_message, provider_message_id, last_error, next_retry_at, started_at, completed_at, sent_at, failed_at, dispatch_enqueued_at, enqueue_kind, created_at, updated_at
		FROM delivery_attempts
		WHERE id = $1
	`
	var attempt DeliveryAttempt
	err := p.DB.QueryRowContext(ctx, query, id).Scan(&attempt.ID, &attempt.NotificationID, &attempt.Channel, &attempt.AttemptNumber, &attempt.Status, &attempt.ErrorCode, &attempt.ErrorMessage, &attempt.ProviderMessageID, &attempt.LastError, &attempt.NextRetryAt, &attempt.StartedAt, &attempt.CompletedAt, &attempt.SentAt, &attempt.FailedAt, &attempt.DispatchEnqueuedAt, &attempt.EnqueueKind, &attempt.CreatedAt, &attempt.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeliveryAttempt{}, fmt.Errorf("get delivery attempt: %w", ErrNotFound)
		}
		return DeliveryAttempt{}, fmt.Errorf("get delivery attempt: %w", err)
	}
	return attempt, nil
}

func (p *Postgres) ListDeliveryAttemptsByNotificationID(ctx context.Context, notificationID string) ([]DeliveryAttempt, error) {
	rows, err := p.DB.QueryContext(ctx, `
		SELECT da.id, da.notification_id, da.channel, da.attempt_number, da.status, da.error_code, da.error_message, da.provider_message_id, da.last_error, da.next_retry_at, da.started_at, da.completed_at, da.sent_at, da.failed_at, da.dispatch_enqueued_at, da.enqueue_kind, dl.id, replay_dl.id, da.created_at, da.updated_at
		FROM delivery_attempts da
		LEFT JOIN dead_letters dl ON dl.notification_id = da.notification_id AND dl.channel = da.channel AND da.status = 'dead_lettered'
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
		if err := rows.Scan(&attempt.ID, &attempt.NotificationID, &attempt.Channel, &attempt.AttemptNumber, &attempt.Status, &attempt.ErrorCode, &attempt.ErrorMessage, &attempt.ProviderMessageID, &attempt.LastError, &attempt.NextRetryAt, &attempt.StartedAt, &attempt.CompletedAt, &attempt.SentAt, &attempt.FailedAt, &attempt.DispatchEnqueuedAt, &attempt.EnqueueKind, &attempt.DeadLetterID, &attempt.ReplayOfDeadLetter, &attempt.CreatedAt, &attempt.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list delivery attempts: %w", err)
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list delivery attempts: %w", err)
	}
	return attempts, nil
}

func (p *Postgres) RecalculateNotificationStatus(ctx context.Context, notificationID string) error {
	attempts, err := p.ListDeliveryAttemptsByNotificationID(ctx, notificationID)
	if err != nil {
		return fmt.Errorf("recalculate notification status: %w", err)
	}

	status := deriveNotificationStatus(attempts)
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
		return fmt.Errorf("mark attempt sent: %w", ErrInvalidStateTransition)
	}
	notificationID, err := p.notificationIDForAttempt(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("mark attempt sent: notification lookup: %w", err)
	}
	return p.RecalculateNotificationStatus(ctx, notificationID)
}

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
		return fmt.Errorf("mark attempt failed: %w", ErrInvalidStateTransition)
	}
	notificationID, err := p.notificationIDForAttempt(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("mark attempt failed: notification lookup: %w", err)
	}
	return p.RecalculateNotificationStatus(ctx, notificationID)
}

type ReplayDeadLetterResult struct {
	DeadLetter DeadLetter
	Attempt    DeliveryAttempt
}

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
		return fmt.Errorf("schedule retry: %w", ErrInvalidStateTransition)
	}
	notificationID, err := p.notificationIDForAttempt(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("schedule retry: notification lookup: %w", err)
	}
	return p.RecalculateNotificationStatus(ctx, notificationID)
}

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
		return fmt.Errorf("mark attempt dead lettered: %w", ErrInvalidStateTransition)
	}
	notificationID, err := p.notificationIDForAttempt(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("mark attempt dead lettered: notification lookup: %w", err)
	}
	return p.RecalculateNotificationStatus(ctx, notificationID)
}

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
	if err := tx.QueryRowContext(ctx, `
		SELECT id, notification_id, channel, attempt_number, status, error_code, error_message, provider_message_id, last_error, next_retry_at, started_at, completed_at, sent_at, failed_at, dispatch_enqueued_at, enqueue_kind, created_at, updated_at
		FROM delivery_attempts
		WHERE id = $1
	`, id).Scan(&attempt.ID, &attempt.NotificationID, &attempt.Channel, &attempt.AttemptNumber, &attempt.Status, &attempt.ErrorCode, &attempt.ErrorMessage, &attempt.ProviderMessageID, &attempt.LastError, &attempt.NextRetryAt, &attempt.StartedAt, &attempt.CompletedAt, &attempt.SentAt, &attempt.FailedAt, &attempt.DispatchEnqueuedAt, &attempt.EnqueueKind, &attempt.CreatedAt, &attempt.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeliveryAttempt{}, ErrNotFound
		}
		return DeliveryAttempt{}, err
	}
	return attempt, nil
}

type PendingEnqueueAttempt struct {
	Attempt      DeliveryAttempt
	TenantID     string
	DeadLetterID *string
}

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
	attempt, err := getAttemptByIDTx(ctx, tx, newAttemptID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return RetryDueAttempt{}, fmt.Errorf("ensure retry attempt: get child: %w", err)
	}
	if errors.Is(err, ErrNotFound) {
		if item.Attempt.Status != "retry_scheduled" || item.Attempt.NextRetryAt == nil || item.Attempt.NextRetryAt.After(time.Now().UTC()) {
			return RetryDueAttempt{}, fmt.Errorf("ensure retry attempt: %w", ErrNotFound)
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO delivery_attempts (id, notification_id, channel, attempt_number, status, dispatch_enqueued_at, enqueue_kind)
			VALUES ($1, $2, $3, $4, 'pending', NULL, 'retry')
			RETURNING id, notification_id, channel, attempt_number, status, error_code, error_message, provider_message_id, last_error, next_retry_at, started_at, completed_at, sent_at, failed_at, dispatch_enqueued_at, enqueue_kind, created_at, updated_at
		`, newAttemptID, item.Attempt.NotificationID, item.Attempt.Channel, item.Attempt.AttemptNumber+1).Scan(&attempt.ID, &attempt.NotificationID, &attempt.Channel, &attempt.AttemptNumber, &attempt.Status, &attempt.ErrorCode, &attempt.ErrorMessage, &attempt.ProviderMessageID, &attempt.LastError, &attempt.NextRetryAt, &attempt.StartedAt, &attempt.CompletedAt, &attempt.SentAt, &attempt.FailedAt, &attempt.DispatchEnqueuedAt, &attempt.EnqueueKind, &attempt.CreatedAt, &attempt.UpdatedAt); err != nil {
			return RetryDueAttempt{}, wrapStoreError("ensure retry attempt", err)
		}
	}
	if item.Attempt.Status == "retry_scheduled" {
		if _, err := tx.ExecContext(ctx, `UPDATE delivery_attempts SET status = 'failed', next_retry_at = NULL, updated_at = NOW() WHERE id = $1 AND status = 'retry_scheduled'`, scheduledAttemptID); err != nil {
			return RetryDueAttempt{}, fmt.Errorf("ensure retry attempt: mark prior failed: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return RetryDueAttempt{}, fmt.Errorf("ensure retry attempt: commit: %w", err)
	}
	item.Attempt = attempt
	return item, nil
}

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
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO delivery_attempts (id, notification_id, channel, attempt_number, status, dispatch_enqueued_at, enqueue_kind)
			VALUES ($1, $2, $3, $4, 'pending', NULL, 'replay')
			RETURNING id, notification_id, channel, attempt_number, status, error_code, error_message, provider_message_id, last_error, next_retry_at, started_at, completed_at, sent_at, failed_at, dispatch_enqueued_at, enqueue_kind, created_at, updated_at
		`, attemptID, dl.NotificationID, dl.Channel, attemptNumber).Scan(&attempt.ID, &attempt.NotificationID, &attempt.Channel, &attempt.AttemptNumber, &attempt.Status, &attempt.ErrorCode, &attempt.ErrorMessage, &attempt.ProviderMessageID, &attempt.LastError, &attempt.NextRetryAt, &attempt.StartedAt, &attempt.CompletedAt, &attempt.SentAt, &attempt.FailedAt, &attempt.DispatchEnqueuedAt, &attempt.EnqueueKind, &attempt.CreatedAt, &attempt.UpdatedAt); err != nil {
			return ReplayDeadLetterResult{}, wrapStoreError("ensure replay attempt", err)
		}
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
