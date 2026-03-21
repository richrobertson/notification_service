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
	ErrNotFound = errors.New("store: not found")
	ErrConflict = errors.New("store: conflict")
)

type Postgres struct {
	DB *sql.DB
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
	ID                string     `json:"id"`
	NotificationID    string     `json:"notification_id"`
	Channel           string     `json:"channel"`
	AttemptNumber     int        `json:"attempt_number"`
	Status            string     `json:"status"`
	ErrorCode         *string    `json:"error_code"`
	ErrorMessage      *string    `json:"error_message"`
	ProviderMessageID *string    `json:"provider_message_id"`
	LastError         *string    `json:"last_error"`
	NextRetryAt       *time.Time `json:"next_retry_at"`
	StartedAt         *time.Time `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at"`
	SentAt            *time.Time `json:"sent_at"`
	FailedAt          *time.Time `json:"failed_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
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
	ID             string
	NotificationID string
	Channel        string
	AttemptNumber  int
	Status         string
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

func (p *Postgres) CreateDeliveryAttempt(ctx context.Context, params CreateDeliveryAttemptParams) (DeliveryAttempt, error) {
	const query = `
		INSERT INTO delivery_attempts (
			id,
			notification_id,
			channel,
			attempt_number,
			status
		)
		VALUES ($1, $2, $3, $4, $5)
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
		SELECT id, notification_id, channel, attempt_number, status, error_code, error_message, provider_message_id, last_error, next_retry_at, started_at, completed_at, sent_at, failed_at, created_at, updated_at
		FROM delivery_attempts
		WHERE id = $1
	`
	var attempt DeliveryAttempt
	err := p.DB.QueryRowContext(ctx, query, id).Scan(&attempt.ID, &attempt.NotificationID, &attempt.Channel, &attempt.AttemptNumber, &attempt.Status, &attempt.ErrorCode, &attempt.ErrorMessage, &attempt.ProviderMessageID, &attempt.LastError, &attempt.NextRetryAt, &attempt.StartedAt, &attempt.CompletedAt, &attempt.SentAt, &attempt.FailedAt, &attempt.CreatedAt, &attempt.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeliveryAttempt{}, fmt.Errorf("get delivery attempt: %w", ErrNotFound)
		}
		return DeliveryAttempt{}, fmt.Errorf("get delivery attempt: %w", err)
	}
	return attempt, nil
}

func (p *Postgres) MarkAttemptSent(ctx context.Context, attemptID string, providerMessageID *string) error {
	const query = `
		UPDATE delivery_attempts
		SET status = 'sent', provider_message_id = $2, last_error = NULL, error_message = NULL, sent_at = NOW(), failed_at = NULL, completed_at = NOW()
		WHERE id = $1
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
		return fmt.Errorf("mark attempt sent: %w", ErrNotFound)
	}
	return nil
}

func (p *Postgres) MarkAttemptFailed(ctx context.Context, attemptID string, lastError string) error {
	const query = `
		UPDATE delivery_attempts
		SET status = 'failed', last_error = $2, error_message = $2, provider_message_id = NULL, failed_at = NOW(), completed_at = NOW()
		WHERE id = $1
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
		return fmt.Errorf("mark attempt failed: %w", ErrNotFound)
	}
	return nil
}
