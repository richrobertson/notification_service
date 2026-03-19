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
