package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

type apiStore interface {
	CreateTenant(ctx context.Context, params store.CreateTenantParams) (store.Tenant, error)
	GetTenantByID(ctx context.Context, id string) (store.Tenant, error)
	CreateTemplate(ctx context.Context, params store.CreateTemplateParams) (store.Template, error)
	GetTemplateByID(ctx context.Context, id string) (store.Template, error)
	GetNotificationByTenantAndIdempotencyKey(ctx context.Context, tenantID, idempotencyKey string) (store.Notification, error)
	GetInitialAttemptByNotificationID(ctx context.Context, notificationID string) (store.DeliveryAttempt, error)
	EnsureInitialAttempt(ctx context.Context, notificationID, channel, attemptID, intentID string) (store.DeliveryAttempt, store.DispatchIntent, error)
	CreateNotification(ctx context.Context, params store.CreateNotificationParams) (store.Notification, error)
	CreateNotificationWithInitialDispatch(ctx context.Context, params store.CreateNotificationDispatchParams) (store.Notification, store.DeliveryAttempt, store.DispatchIntent, error)
	CancelScheduledNotification(ctx context.Context, notificationID string) (store.Notification, error)
	RedriveNotification(ctx context.Context, notificationID string) (store.PromotedScheduledNotification, error)
	ResolveDeliveryPolicy(ctx context.Context, tenantID, channel string) (store.ResolvedDeliveryPolicy, error)
	ListDeliveryPolicies(ctx context.Context) ([]store.DeliveryPolicy, error)
	GetDeliveryPolicyByID(ctx context.Context, id string) (store.DeliveryPolicy, error)
	UpsertDeliveryPolicy(ctx context.Context, params store.UpsertDeliveryPolicyParams) (store.DeliveryPolicy, error)
	SetDeliveryPolicyPaused(ctx context.Context, id string, paused bool) (store.DeliveryPolicy, error)
	CreateDeliveryAttempt(ctx context.Context, params store.CreateDeliveryAttemptParams) (store.DeliveryAttempt, error)
	ListDeadLetters(ctx context.Context, limit int) ([]store.DeadLetter, error)
	GetDeadLetterByID(ctx context.Context, id string) (store.DeadLetter, error)
	EnsureReplayAttempt(ctx context.Context, deadLetterID, newAttemptID string) (store.ReplayDeadLetterResult, error)
	RecalculateNotificationStatus(ctx context.Context, notificationID string) error
	GetNotificationByID(ctx context.Context, id string) (store.Notification, error)
	GetDeliveryAttemptByID(ctx context.Context, id string) (store.DeliveryAttempt, error)
	ListDeliveryAttemptsByNotificationID(ctx context.Context, notificationID string) ([]store.DeliveryAttempt, error)
	RecordAuditEvent(ctx context.Context, id, tenantID, actor, action, resourceType, resourceID string, metadata map[string]any) error
}

type dispatchQueue interface {
	EnqueueDispatch(ctx context.Context, job queue.DispatchJob) error
}

// TenantRateLimiter is the API's tenant-aware request-throttling contract.
type TenantRateLimiter interface {
	Allow(ctx context.Context, tenantID string) (bool, time.Duration, error)
}

// PressureMonitor is the subset of queue-pressure behavior the API uses for
// Stage 7 overload protection.
type PressureMonitor interface {
	Snapshot(ctx context.Context) (queue.PressureSnapshot, error)
	IncRateLimited(tenantID string)
	IncRejected(reason, tenantID string)
}

// API bundles the store, queue, rate limiter, and pressure monitor used by the
// HTTP handlers.
type API struct {
	store   apiStore
	queue   dispatchQueue
	limiter TenantRateLimiter
	monitor PressureMonitor
}

type notificationInspectionResponse struct {
	Notification store.Notification      `json:"notification"`
	Attempts     []store.DeliveryAttempt `json:"attempts"`
}

type createTenantRequest struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	DailyQuota int    `json:"daily_quota"`
}

type createTemplateRequest struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Channel  string `json:"channel"`
	Version  int    `json:"version"`
	Body     string `json:"body"`
}

type createNotificationRequest struct {
	ID                  string         `json:"id"`
	TenantID            string         `json:"tenant_id"`
	TemplateID          string         `json:"template_id"`
	IdempotencyKey      string         `json:"idempotency_key"`
	RecipientEmail      string         `json:"recipient_email"`
	RecipientWebhookURL string         `json:"recipient_webhook_url"`
	SecondaryWebhookURL string         `json:"secondary_webhook_url"`
	ScheduledFor        *time.Time     `json:"scheduled_for"`
	Variables           map[string]any `json:"variables"`
}

type upsertPolicyRequest struct {
	ID                    string  `json:"id"`
	TenantID              *string `json:"tenant_id"`
	Channel               *string `json:"channel"`
	Paused                *bool   `json:"paused"`
	FailoverEnabled       *bool   `json:"failover_enabled"`
	SchedulingEnabled     *bool   `json:"scheduling_enabled"`
	ReplayAllowed         *bool   `json:"replay_allowed"`
	MaxAttemptsOverride   *int    `json:"max_attempts_override"`
	RetryBaseDelaySeconds *int    `json:"retry_base_delay_seconds"`
	RetryMaxDelaySeconds  *int    `json:"retry_max_delay_seconds"`
}

// NewAPI constructs the concrete Stage 7+ HTTP handler set.
func NewAPI(store apiStore, redisQueue dispatchQueue, limiter TenantRateLimiter, monitor PressureMonitor) *API {
	return &API{store: store, queue: redisQueue, limiter: limiter, monitor: monitor}
}

func (a *API) recordAudit(ctx context.Context, tenantID, actor, action, resourceType, resourceID string, metadata map[string]any) {
	if err := a.store.RecordAuditEvent(ctx, generateID("audit"), tenantID, actor, action, resourceType, resourceID, metadata); err != nil {
		slog.Default().Warn("failed to record audit event", slog.Any("error", err), slog.String("action", action), slog.String("resource_id", resourceID))
	}
}

func (a *API) recordPolicyAudit(ctx context.Context, policy store.DeliveryPolicy, action string) {
	if policy.TenantID == nil || strings.TrimSpace(*policy.TenantID) == "" {
		slog.Default().Info("skipping tenant-scoped audit for global delivery policy action", slog.String("action", action), slog.String("policy_id", policy.ID))
		return
	}
	a.recordAudit(ctx, *policy.TenantID, "api", action, "delivery_policy", policy.ID, map[string]any{})
}

func (a *API) enforceRateLimit(ctx context.Context, w http.ResponseWriter, tenantID string) bool {
	if a.limiter == nil || tenantID == "" {
		return true
	}
	allowed, retryAfter, err := a.limiter.Allow(ctx, tenantID)
	if err != nil {
		slog.Default().Error("rate limiter unavailable", slog.Any("error", err), slog.String("tenant_id", tenantID))
		writeError(w, http.StatusServiceUnavailable, "pressure_unavailable", "unable to evaluate request pressure")
		return false
	}
	if allowed {
		return true
	}
	if a.monitor != nil {
		a.monitor.IncRateLimited(tenantID)
	}
	w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
	slog.Default().Warn("request rate limited", slog.String("tenant_id", tenantID), slog.Duration("retry_after", retryAfter))
	writeError(w, http.StatusTooManyRequests, "rate_limited", "tenant request rate exceeded")
	return false
}

func (a *API) enforceBackpressure(ctx context.Context, w http.ResponseWriter, tenantID string) bool {
	if a.monitor == nil {
		return true
	}
	snapshot, err := a.monitor.Snapshot(ctx)
	if err != nil {
		slog.Default().Error("queue pressure snapshot failed", slog.Any("error", err))
		writeError(w, http.StatusServiceUnavailable, "pressure_unavailable", "unable to evaluate queue pressure")
		return false
	}
	if !snapshot.AcceptingWrites() {
		if a.monitor != nil {
			a.monitor.IncRejected("queue_hard_limit", tenantID)
		}
		code := http.StatusServiceUnavailable
		if snapshot.AnyHardLimited() {
			code = http.StatusTooManyRequests
		}
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", snapshot.RetryAfter.Seconds()))
		slog.Default().Warn("request rejected due to queue pressure", slog.String("tenant_id", tenantID), slog.Any("depths", snapshot.Depths), slog.Int("soft_limit", snapshot.SoftLimit), slog.Int("hard_limit", snapshot.HardLimit))
		writeError(w, code, "queue_overloaded", "notification service is saturated; retry later")
		return false
	}
	if snapshot.AnySoftLimited() {
		slog.Default().Warn("queue pressure soft limit reached", slog.String("tenant_id", tenantID), slog.Any("depths", snapshot.Depths), slog.Int("soft_limit", snapshot.SoftLimit))
	}
	return true
}

func (a *API) ensureInitialAttempt(ctx context.Context, w http.ResponseWriter, existing store.Notification) {
	template, err := a.store.GetTemplateByID(ctx, existing.TemplateID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	slog.Default().Info("idempotent retry recovering existing notification using stored template channel", slog.String("notification_id", existing.ID), slog.String("template_id", existing.TemplateID), slog.String("channel", template.Channel))
	attempt, _, attemptErr := a.store.EnsureInitialAttempt(ctx, existing.ID, template.Channel, generateID("attempt"), generateID("intent"))
	if attemptErr != nil {
		if errors.Is(attemptErr, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "notification not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if attempt.DispatchEnqueuedAt != nil {
		writeJSON(w, http.StatusOK, existing)
		return
	}
	slog.Default().Info("idempotent retry found existing notification with durable pending dispatch", slog.String("notification_id", existing.ID), slog.String("attempt_id", attempt.ID), slog.String("channel", attempt.Channel))
	a.recordAudit(ctx, existing.TenantID, "api", "dispatch_pending", "delivery_attempt", attempt.ID, map[string]any{"notification_id": existing.ID, "channel": attempt.Channel, "source": "initial"})
	writeJSON(w, http.StatusAccepted, existing)
}

// CreateTenant handles `POST /v1/tenants`.
func (a *API) CreateTenant() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createTenantRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("invalid request body: %v", err))
			return
		}
		if strings.TrimSpace(req.ID) == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "id is required")
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "name is required")
			return
		}
		if req.DailyQuota <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "daily_quota must be greater than 0")
			return
		}
		tenant, err := a.store.CreateTenant(r.Context(), store.CreateTenantParams{ID: req.ID, Name: req.Name, DailyQuota: req.DailyQuota})
		if err != nil {
			if store.IsConflict(err) {
				writeError(w, http.StatusConflict, "conflict", "tenant already exists")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		writeJSON(w, http.StatusCreated, tenant)
	}
}

// CreateTemplate handles `POST /v1/templates`.
func (a *API) CreateTemplate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createTemplateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("invalid request body: %v", err))
			return
		}
		if strings.TrimSpace(req.ID) == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "id is required")
			return
		}
		if strings.TrimSpace(req.TenantID) == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "tenant_id is required")
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "name is required")
			return
		}
		if req.Channel != "email" && req.Channel != "webhook" {
			writeError(w, http.StatusBadRequest, "bad_request", "channel must be one of: email, webhook")
			return
		}
		if req.Version <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "version must be greater than 0")
			return
		}
		if strings.TrimSpace(req.Body) == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "body is required")
			return
		}
		if _, err := a.store.GetTenantByID(r.Context(), req.TenantID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "tenant not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		template, err := a.store.CreateTemplate(r.Context(), store.CreateTemplateParams{ID: req.ID, TenantID: req.TenantID, Name: req.Name, Channel: req.Channel, Version: req.Version, Body: req.Body})
		if err != nil {
			if store.IsConflict(err) {
				writeError(w, http.StatusConflict, "conflict", "template already exists")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		writeJSON(w, http.StatusCreated, template)
	}
}

// CreateNotification handles `POST /v1/notifications`.
//
// The handler applies Stage 7 rate limiting and backpressure checks, Stage 6
// idempotency repair behavior, and Stage 8/9 durable dispatch or scheduling
// behavior.
func (a *API) CreateNotification() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createNotificationRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("invalid request body: %v", err))
			return
		}
		if strings.TrimSpace(req.ID) == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "id is required")
			return
		}
		if strings.TrimSpace(req.TenantID) == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "tenant_id is required")
			return
		}
		if strings.TrimSpace(req.TemplateID) == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "template_id is required")
			return
		}
		if !a.enforceRateLimit(r.Context(), w, req.TenantID) {
			return
		}
		if !a.enforceBackpressure(r.Context(), w, req.TenantID) {
			return
		}
		if _, err := a.store.GetTenantByID(r.Context(), req.TenantID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "tenant not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		template, err := a.store.GetTemplateByID(r.Context(), req.TemplateID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "template not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		if template.TenantID != req.TenantID {
			writeError(w, http.StatusBadRequest, "bad_request", "template does not belong to tenant")
			return
		}
		recipientEmail := strings.TrimSpace(req.RecipientEmail)
		recipientWebhookURL := strings.TrimSpace(req.RecipientWebhookURL)
		switch template.Channel {
		case "email":
			if recipientEmail == "" {
				writeError(w, http.StatusBadRequest, "bad_request", "recipient_email is required for email templates")
				return
			}
		case "webhook":
			if recipientWebhookURL == "" {
				writeError(w, http.StatusBadRequest, "bad_request", "recipient_webhook_url is required for webhook templates")
				return
			}
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		if req.IdempotencyKey != "" {
			existing, err := a.store.GetNotificationByTenantAndIdempotencyKey(r.Context(), req.TenantID, req.IdempotencyKey)
			if err == nil {
				a.ensureInitialAttempt(r.Context(), w, existing)
				return
			}
			if !errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
				return
			}
		}
		policy, err := a.store.ResolveDeliveryPolicy(r.Context(), req.TenantID, template.Channel)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		if req.ScheduledFor != nil && !policy.SchedulingEnabled {
			writeError(w, http.StatusConflict, "conflict", "scheduled delivery is disabled for this tenant/channel")
			return
		}
		params := store.CreateNotificationParams{ID: req.ID, TenantID: req.TenantID, TemplateID: req.TemplateID, Variables: req.Variables}
		if req.IdempotencyKey != "" {
			params.IdempotencyKey = &req.IdempotencyKey
		}
		if recipientEmail != "" {
			params.RecipientEmail = &recipientEmail
		}
		if recipientWebhookURL != "" {
			params.RecipientWebhookURL = &recipientWebhookURL
		}
		if secondaryWebhookURL := strings.TrimSpace(req.SecondaryWebhookURL); secondaryWebhookURL != "" {
			params.SecondaryWebhookURL = &secondaryWebhookURL
		}
		if req.ScheduledFor != nil {
			params.ScheduledFor = req.ScheduledFor
		}
		notification, attempt, intent, err := a.store.CreateNotificationWithInitialDispatch(r.Context(), store.CreateNotificationDispatchParams{
			Notification: params,
			Channel:      template.Channel,
			AttemptID:    generateID("attempt"),
			IntentID:     generateID("intent"),
		})
		if err != nil {
			if req.IdempotencyKey != "" && store.IsConflict(err) {
				existing, lookupErr := a.store.GetNotificationByTenantAndIdempotencyKey(r.Context(), req.TenantID, req.IdempotencyKey)
				if lookupErr == nil {
					a.ensureInitialAttempt(r.Context(), w, existing)
					return
				}
			}
			if store.IsConflict(err) {
				writeError(w, http.StatusConflict, "conflict", "notification already exists")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		a.recordAudit(r.Context(), notification.TenantID, "api", "notification_accepted", "notification", notification.ID, map[string]any{"template_id": notification.TemplateID})
		if intent.ID != "" {
			a.recordAudit(r.Context(), notification.TenantID, "api", "dispatch_intent_created", "dispatch_intent", intent.ID, map[string]any{"notification_id": notification.ID, "channel": template.Channel, "source": "initial", "attempt_id": attempt.ID})
		} else if notification.ScheduledFor != nil {
			a.recordAudit(r.Context(), notification.TenantID, "api", "notification_scheduled", "notification", notification.ID, map[string]any{"template_id": notification.TemplateID, "channel": template.Channel, "attempt_id": attempt.ID, "scheduled_for": notification.ScheduledFor.Format(time.RFC3339Nano)})
		}

		writeJSON(w, http.StatusAccepted, notification)
	}
}

func generateID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(buf))
}

// ListDeadLetters handles `GET /v1/dead-letters`.
func (a *API) ListDeadLetters() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deadLetters, err := a.store.ListDeadLetters(r.Context(), 100)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		writeJSON(w, http.StatusOK, deadLetters)
	}
}

// GetDeadLetter handles `GET /v1/dead-letters/{id}`.
func (a *API) GetDeadLetter() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deadLetter, err := a.store.GetDeadLetterByID(r.Context(), r.PathValue("id"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "dead letter not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		writeJSON(w, http.StatusOK, deadLetter)
	}
}

// ReplayDeadLetter handles `POST /v1/dead-letters/{id}/replay`.
func (a *API) ReplayDeadLetter() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deadLetterID := r.PathValue("id")
		deadLetter, err := a.store.GetDeadLetterByID(r.Context(), deadLetterID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "dead letter not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		if deadLetter.ReplayedAt != nil {
			writeError(w, http.StatusConflict, "conflict", "dead letter already replayed")
			return
		}
		notification, err := a.store.GetNotificationByID(r.Context(), deadLetter.NotificationID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		if !a.enforceBackpressure(r.Context(), w, notification.TenantID) {
			return
		}
		policy, err := a.store.ResolveDeliveryPolicy(r.Context(), notification.TenantID, deadLetter.Channel)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		if !policy.ReplayAllowed {
			writeError(w, http.StatusConflict, "conflict", "replay is disabled for this tenant/channel")
			return
		}
		a.recordAudit(r.Context(), notification.TenantID, "api", "replay_requested", "dead_letter", deadLetterID, map[string]any{"notification_id": deadLetter.NotificationID, "channel": deadLetter.Channel})
		attemptID := replayAttemptID(deadLetterID)
		result, err := a.store.EnsureReplayAttempt(r.Context(), deadLetterID, attemptID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "dead letter not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		intentID := "intent-" + result.Attempt.ID
		a.recordAudit(r.Context(), notification.TenantID, "api", "dispatch_intent_created", "dispatch_intent", intentID, map[string]any{"notification_id": result.Attempt.NotificationID, "dead_letter_id": deadLetterID, "channel": result.Attempt.Channel, "source": "replay", "attempt_id": result.Attempt.ID})
		writeJSON(w, http.StatusAccepted, result.Attempt)
	}
}

// GetNotification handles `GET /v1/notifications/{id}`.
func (a *API) GetNotification() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		notification, err := a.store.GetNotificationByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "notification not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		attempts, err := a.store.ListDeliveryAttemptsByNotificationID(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		writeJSON(w, http.StatusOK, notificationInspectionResponse{Notification: notification, Attempts: attempts})
	}
}

// ListNotificationAttempts handles `GET /v1/notifications/{id}/attempts`.
func (a *API) ListNotificationAttempts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		attempts, err := a.store.ListDeliveryAttemptsByNotificationID(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		writeJSON(w, http.StatusOK, attempts)
	}
}

// GetAttempt handles `GET /v1/attempts/{id}`.
func (a *API) GetAttempt() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		attempt, err := a.store.GetDeliveryAttemptByID(r.Context(), r.PathValue("id"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "attempt not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		writeJSON(w, http.StatusOK, attempt)
	}
}

// CancelNotification handles `POST /v1/notifications/{id}/cancel`.
func (a *API) CancelNotification() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		notification, err := a.store.CancelScheduledNotification(r.Context(), r.PathValue("id"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "notification not found")
				return
			}
			if errors.Is(err, store.ErrInvalidStateTransition) {
				writeError(w, http.StatusConflict, "conflict", "notification cannot be cancelled")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		a.recordAudit(r.Context(), notification.TenantID, "api", "scheduled_notification_cancelled", "notification", notification.ID, map[string]any{})
		writeJSON(w, http.StatusOK, notification)
	}
}

// RedriveNotification handles `POST /v1/notifications/{id}/redrive`.
func (a *API) RedriveNotification() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := a.store.RedriveNotification(r.Context(), r.PathValue("id"))
		if err != nil {
			switch {
			case errors.Is(err, store.ErrNotFound):
				writeError(w, http.StatusNotFound, "not_found", "notification not found")
			case errors.Is(err, store.ErrInvalidStateTransition):
				writeError(w, http.StatusConflict, "conflict", "notification cannot be re-driven")
			default:
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}
		a.recordAudit(r.Context(), result.Notification.TenantID, "api", "manual_redrive_requested", "notification", result.Notification.ID, map[string]any{"attempt_id": result.Attempt.ID, "channel": result.Attempt.Channel})
		a.recordAudit(r.Context(), result.Notification.TenantID, "api", "dispatch_intent_created", "dispatch_intent", result.Intent.ID, map[string]any{"notification_id": result.Notification.ID, "channel": result.Attempt.Channel, "source": result.Intent.Source, "attempt_id": result.Attempt.ID})
		writeJSON(w, http.StatusAccepted, result.Notification)
	}
}

// ListPolicies handles `GET /v1/policies`.
func (a *API) ListPolicies() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		policies, err := a.store.ListDeliveryPolicies(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		writeJSON(w, http.StatusOK, policies)
	}
}

// GetPolicy handles `GET /v1/policies/{id}`.
func (a *API) GetPolicy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		policy, err := a.store.GetDeliveryPolicyByID(r.Context(), r.PathValue("id"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "policy not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		writeJSON(w, http.StatusOK, policy)
	}
}

// UpsertPolicy handles `POST /v1/policies`.
func (a *API) UpsertPolicy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req upsertPolicyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("invalid request body: %v", err))
			return
		}
		if strings.TrimSpace(req.ID) == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "id is required")
			return
		}
		if req.Channel != nil && *req.Channel != "email" && *req.Channel != "webhook" {
			writeError(w, http.StatusBadRequest, "bad_request", "channel must be one of: email, webhook")
			return
		}
		for _, value := range []*int{req.MaxAttemptsOverride, req.RetryBaseDelaySeconds, req.RetryMaxDelaySeconds} {
			if value != nil && *value <= 0 {
				writeError(w, http.StatusBadRequest, "bad_request", "numeric policy overrides must be greater than 0")
				return
			}
		}
		if req.TenantID != nil {
			trimmedTenantID := strings.TrimSpace(*req.TenantID)
			if trimmedTenantID == "" {
				req.TenantID = nil
			} else if _, err := a.store.GetTenantByID(r.Context(), trimmedTenantID); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					writeError(w, http.StatusNotFound, "not_found", "tenant not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
				return
			} else {
				req.TenantID = &trimmedTenantID
			}
		}
		policy, err := a.store.UpsertDeliveryPolicy(r.Context(), store.UpsertDeliveryPolicyParams{
			ID:                    req.ID,
			TenantID:              req.TenantID,
			Channel:               req.Channel,
			Paused:                req.Paused,
			FailoverEnabled:       req.FailoverEnabled,
			SchedulingEnabled:     req.SchedulingEnabled,
			ReplayAllowed:         req.ReplayAllowed,
			MaxAttemptsOverride:   req.MaxAttemptsOverride,
			RetryBaseDelaySeconds: req.RetryBaseDelaySeconds,
			RetryMaxDelaySeconds:  req.RetryMaxDelaySeconds,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		a.recordPolicyAudit(r.Context(), policy, "delivery_policy_updated")
		writeJSON(w, http.StatusOK, policy)
	}
}

// PausePolicy handles `POST /v1/policies/{id}/pause`.
func (a *API) PausePolicy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		policy, err := a.store.SetDeliveryPolicyPaused(r.Context(), r.PathValue("id"), true)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "policy not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		a.recordPolicyAudit(r.Context(), policy, "delivery_paused")
		writeJSON(w, http.StatusOK, policy)
	}
}

// ResumePolicy handles `POST /v1/policies/{id}/resume`.
func (a *API) ResumePolicy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		policy, err := a.store.SetDeliveryPolicyPaused(r.Context(), r.PathValue("id"), false)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "policy not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		a.recordPolicyAudit(r.Context(), policy, "delivery_resumed")
		writeJSON(w, http.StatusOK, policy)
	}
}

func replayAttemptID(deadLetterID string) string {
	return fmt.Sprintf("replay_%s", deadLetterID)
}
