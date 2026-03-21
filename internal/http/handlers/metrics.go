package handlers

import (
	"context"
	"net/http"

	"github.com/richrobertson/notification-platform/internal/pressure"
)

func Metrics(m *pressure.Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m == nil {
			writeJSON(w, http.StatusOK, map[string]any{"status": "disabled"})
			return
		}
		metrics, err := m.Metrics(context.Background())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "metrics_unavailable", "unable to collect metrics")
			return
		}
		writeJSON(w, http.StatusOK, metrics)
	}
}
