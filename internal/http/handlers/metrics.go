package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/richrobertson/notification-platform/internal/pressure"
	"github.com/richrobertson/notification-platform/internal/store"
)

type OperationalMetricsProvider interface {
	CollectOperationalMetrics(ctx context.Context, now time.Time) (store.OperationalMetrics, error)
}

type metricsResponse struct {
	Status      string                    `json:"status"`
	Pressure    *pressure.MetricsSnapshot `json:"pressure,omitempty"`
	Operational *store.OperationalMetrics `json:"operational,omitempty"`
}

func Metrics(m *pressure.Monitor, provider OperationalMetricsProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := metricsResponse{Status: "ok"}

		if m != nil {
			metrics, err := m.Metrics(r.Context())
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "metrics_unavailable", "unable to collect pressure metrics")
				return
			}
			response.Pressure = &metrics
		}

		if provider != nil {
			metrics, err := provider.CollectOperationalMetrics(r.Context(), time.Now().UTC())
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "metrics_unavailable", "unable to collect operational metrics")
				return
			}
			response.Operational = &metrics
		}

		if response.Pressure == nil && response.Operational == nil {
			response.Status = "disabled"
		}
		writeJSON(w, http.StatusOK, response)
	}
}
