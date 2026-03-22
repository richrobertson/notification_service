package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	handlers "github.com/richrobertson/notification-platform/internal/http/handlers"
	"github.com/richrobertson/notification-platform/internal/pressure"
	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

type RouterDeps struct {
	AppName string
	DBPing  func(context.Context) error
	Store   *store.Postgres
	Queue   *queue.RedisQueue
	Monitor *pressure.Monitor
	Limiter handlers.TenantRateLimiter
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func NewRouter(deps RouterDeps) http.Handler {
	mux := http.NewServeMux()
	api := handlers.NewAPI(deps.Store, deps.Queue, deps.Limiter, deps.Monitor)
	mux.Handle("GET /v1/health", handlers.Health())
	mux.Handle("GET /v1/readiness", handlers.Readiness(deps.DBPing))
	mux.Handle("GET /v1/metrics", handlers.Metrics(deps.Monitor))
	mux.Handle("POST /v1/tenants", api.CreateTenant())
	mux.Handle("POST /v1/templates", api.CreateTemplate())
	mux.Handle("POST /v1/notifications", api.CreateNotification())
	mux.Handle("GET /v1/notifications/{id}", api.GetNotification())
	mux.Handle("GET /v1/notifications/{id}/attempts", api.ListNotificationAttempts())
	mux.Handle("POST /v1/notifications/{id}/cancel", api.CancelNotification())
	mux.Handle("POST /v1/notifications/{id}/redrive", api.RedriveNotification())
	mux.Handle("GET /v1/attempts/{id}", api.GetAttempt())
	mux.Handle("GET /v1/policies", api.ListPolicies())
	mux.Handle("POST /v1/policies", api.UpsertPolicy())
	mux.Handle("GET /v1/policies/{id}", api.GetPolicy())
	mux.Handle("POST /v1/policies/{id}/pause", api.PausePolicy())
	mux.Handle("POST /v1/policies/{id}/resume", api.ResumePolicy())
	mux.Handle("GET /v1/dead-letters", api.ListDeadLetters())
	mux.Handle("GET /v1/dead-letters/{id}", api.GetDeadLetter())
	mux.Handle("POST /v1/dead-letters/{id}/replay", api.ReplayDeadLetter())

	var handler http.Handler = mux
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
		logger.Info("http request", slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.Int("status_code", recorder.statusCode), slog.Duration("duration", time.Since(start)))
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
