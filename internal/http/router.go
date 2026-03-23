package httpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	handlers "github.com/richrobertson/notification-platform/internal/http/handlers"
	"github.com/richrobertson/notification-platform/internal/pressure"
	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

// RouterDeps supplies the concrete dependencies needed to build the HTTP
// router.
type RouterDeps struct {
	AppName             string
	AdminToken          string
	MaxRequestBodyBytes int64
	DBPing              func(context.Context) error
	RedisPing           func(context.Context) error
	Store               *store.Postgres
	Queue               *queue.RedisQueue
	Monitor             *pressure.Monitor
	Limiter             handlers.TenantRateLimiter
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

// NewRouter builds the Stage 10 HTTP surface for the service.
//
// The router keeps public submission routes separate from admin-protected
// operator routes and applies the shared middleware stack used by the API
// process.
func NewRouter(deps RouterDeps) http.Handler {
	mux := http.NewServeMux()
	api := handlers.NewAPI(deps.Store, deps.Queue, deps.Limiter, deps.Monitor)
	var metricsProvider handlers.OperationalMetricsProvider
	if deps.Store != nil {
		metricsProvider = deps.Store
	}

	readiness := handlers.Readiness(
		handlers.DependencyCheck{Name: "postgres", Ping: deps.DBPing},
		handlers.DependencyCheck{Name: "redis", Ping: deps.RedisPing},
	)

	mux.Handle("GET /healthz", handlers.Health(deps.AppName))
	mux.Handle("GET /readyz", readiness)
	mux.Handle("GET /v1/health", handlers.Health(deps.AppName))
	mux.Handle("GET /v1/readiness", readiness)

	admin := adminMiddleware(deps.AdminToken)
	mux.Handle("GET /metrics", admin(handlers.Metrics(deps.Monitor, metricsProvider)))
	mux.Handle("GET /v1/metrics", admin(handlers.Metrics(deps.Monitor, metricsProvider)))
	mux.Handle("POST /v1/tenants", admin(api.CreateTenant()))
	mux.Handle("POST /v1/templates", admin(api.CreateTemplate()))
	mux.Handle("POST /v1/notifications", api.CreateNotification())
	mux.Handle("GET /v1/notifications/{id}", admin(api.GetNotification()))
	mux.Handle("GET /v1/notifications/{id}/attempts", admin(api.ListNotificationAttempts()))
	mux.Handle("POST /v1/notifications/{id}/cancel", admin(api.CancelNotification()))
	mux.Handle("POST /v1/notifications/{id}/redrive", admin(api.RedriveNotification()))
	mux.Handle("GET /v1/attempts/{id}", admin(api.GetAttempt()))
	mux.Handle("GET /v1/policies", admin(api.ListPolicies()))
	mux.Handle("POST /v1/policies", admin(api.UpsertPolicy()))
	mux.Handle("GET /v1/policies/{id}", admin(api.GetPolicy()))
	mux.Handle("POST /v1/policies/{id}/pause", admin(api.PausePolicy()))
	mux.Handle("POST /v1/policies/{id}/resume", admin(api.ResumePolicy()))
	mux.Handle("GET /v1/dead-letters", admin(api.ListDeadLetters()))
	mux.Handle("GET /v1/dead-letters/{id}", admin(api.GetDeadLetter()))
	mux.Handle("POST /v1/dead-letters/{id}/replay", admin(api.ReplayDeadLetter()))

	var handler http.Handler = mux
	handler = requestBodyLimitMiddleware(deps.MaxRequestBodyBytes)(handler)
	handler = recoveryMiddleware(handler)
	handler = loggingMiddleware(handler)

	return handler
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func loggingMiddleware(next http.Handler) http.Handler {
	logger := slog.Default()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r)

		attrs := []any{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status_code", recorder.statusCode),
			slog.Duration("duration", time.Since(start)),
		}
		if id := r.PathValue("id"); id != "" {
			attrs = append(attrs, slog.String("resource_id", id))
		}
		if tenantID := r.URL.Query().Get("tenant_id"); tenantID != "" {
			attrs = append(attrs, slog.String("tenant_id", tenantID))
		}
		logger.Info("http request", attrs...)
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() == nil {
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "internal_error", "message": "internal server error"})
		}()
		next.ServeHTTP(w, r)
	})
}

func requestBodyLimitMiddleware(limit int64) func(http.Handler) http.Handler {
	if limit <= 0 {
		limit = 1 << 20
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch) {
				if r.ContentLength > limit && r.ContentLength > 0 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusRequestEntityTooLarge)
					_ = json.NewEncoder(w).Encode(map[string]string{"code": "request_too_large", "message": "request body exceeds configured limit"})
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func adminMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.TrimSpace(token) == "" {
				next.ServeHTTP(w, r)
				return
			}
			provided := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
			if provided == "" {
				if auth := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(auth, "Bearer ") {
					provided = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
				}
			}
			if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": "unauthorized", "message": "admin credentials required"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
