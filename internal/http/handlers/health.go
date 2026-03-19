package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

const defaultServiceName = "notification-platform-api"

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

type dependencyStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type readinessResponse struct {
	Status       string             `json:"status"`
	Dependencies []dependencyStatus `json:"dependencies"`
}

func Health() http.HandlerFunc {
	serviceName := os.Getenv("APP_NAME")
	if serviceName == "" {
		serviceName = defaultServiceName
	}

	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{
			Status:  "ok",
			Service: serviceName,
		})
	}
}

func Readiness(dbPing func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		status := readinessResponse{
			Status: "ready",
			Dependencies: []dependencyStatus{
				{Name: "postgres", Status: "ok"},
			},
		}

		if err := dbPing(ctx); err != nil {
			status.Status = "not_ready"
			status.Dependencies[0].Status = "down"
			writeJSON(w, http.StatusServiceUnavailable, status)
			return
		}

		writeJSON(w, http.StatusOK, status)
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
