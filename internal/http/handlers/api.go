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
	CreateNotification(ctx context.Context, params store.CreateNotificationParams) (store.Notification, error)
	CreateDeliveryAttempt(ctx context.Context, params store.CreateDeliveryAttemptParams) (store.DeliveryAttempt, error)
	ListDeadLetters(ctx context.Context, limit int) ([]store.DeadLetter, error)
	GetDeadLetterByID(ctx context.Context, id string) (store.DeadLetter, error)
	FinalizeDeadLetterReplay(ctx context.Context, deadLetterID, newAttemptID string) (store.ReplayDeadLetterResult, error)
	GetNotificationByID(ctx context.Context, id string) (store.Notification, error)
}

type dispatchQueue interface {
	EnqueueDispatch(ctx context.Context, job queue.DispatchJob) error
}

type API struct {
	store apiStore
	queue dispatchQueue
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
	Variables           map[string]any `json:"variables"`
}

func NewAPI(store apiStore, redisQueue dispatchQueue) *API {
	return &API{store: store, queue: redisQueue}
}

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
		if req.IdempotencyKey != "" {
			existing, err := a.store.GetNotificationByTenantAndIdempotencyKey(r.Context(), req.TenantID, req.IdempotencyKey)
			if err == nil {
				writeJSON(w, http.StatusOK, existing)
				return
			}
			if !errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
				return
			}
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
		notification, err := a.store.CreateNotification(r.Context(), params)
		if err != nil {
			if req.IdempotencyKey != "" && store.IsConflict(err) {
				existing, lookupErr := a.store.GetNotificationByTenantAndIdempotencyKey(r.Context(), req.TenantID, req.IdempotencyKey)
				if lookupErr == nil {
					writeJSON(w, http.StatusOK, existing)
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

		attemptID := generateID("attempt")
		attempt, err := a.store.CreateDeliveryAttempt(r.Context(), store.CreateDeliveryAttemptParams{ID: attemptID, NotificationID: notification.ID, Channel: template.Channel, AttemptNumber: 1, Status: "pending"})
		if err != nil {
			slog.Default().Error("failed to create delivery attempt", slog.Any("error", err), slog.String("notification_id", notification.ID), slog.String("attempt_id", attemptID), slog.String("channel", template.Channel))
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}

		job := queue.DispatchJob{JobID: generateID("job"), NotificationID: notification.ID, AttemptID: attempt.ID, TenantID: notification.TenantID, Channel: template.Channel, CreatedAt: time.Now().UTC()}
		if err := a.queue.EnqueueDispatch(r.Context(), job); err != nil {
			slog.Default().Error("failed to enqueue dispatch job", slog.Any("error", err), slog.String("notification_id", notification.ID), slog.String("attempt_id", attempt.ID), slog.String("job_id", job.JobID), slog.String("channel", job.Channel))
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
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
		attemptID := replayAttemptID(deadLetterID)
		job := queue.DispatchJob{JobID: generateID("job"), NotificationID: deadLetter.NotificationID, AttemptID: attemptID, TenantID: notification.TenantID, Channel: deadLetter.Channel, CreatedAt: time.Now().UTC()}
		if err := a.queue.EnqueueDispatch(r.Context(), job); err != nil {
			slog.Default().Error("replay enqueue failed; dead letter not marked replayed", slog.Any("error", err), slog.String("dead_letter_id", deadLetterID), slog.String("attempt_id", attemptID), slog.String("channel", deadLetter.Channel))
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		result, err := a.store.FinalizeDeadLetterReplay(r.Context(), deadLetterID, attemptID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "dead letter not found")
				return
			}
			if errors.Is(err, store.ErrConflict) {
				writeError(w, http.StatusConflict, "conflict", "dead letter already replayed")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		writeJSON(w, http.StatusAccepted, result.Attempt)
	}
}

func replayAttemptID(deadLetterID string) string {
	return fmt.Sprintf("replay_%s", deadLetterID)
}
