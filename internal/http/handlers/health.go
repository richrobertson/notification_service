package handlers

import (
	"context"
	"net/http"
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

// DependencyCheck describes one dependency readiness probe.
type DependencyCheck struct {
	Name string
	Ping func(context.Context) error
}

// Health returns a simple liveness handler for the current service name.
func Health(serviceName string) http.HandlerFunc {
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

// Readiness returns a readiness handler that evaluates the supplied dependency
// checks and reports whether the process can currently do useful work.
func Readiness(checks ...DependencyCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		status := readinessResponse{
			Status:       "ready",
			Dependencies: make([]dependencyStatus, 0, len(checks)),
		}
		for _, check := range checks {
			dependency := dependencyStatus{Name: check.Name}
			if check.Ping == nil {
				dependency.Status = "unknown"
				status.Status = "not_ready"
			} else if err := check.Ping(ctx); err != nil {
				dependency.Status = "down"
				status.Status = "not_ready"
			} else {
				dependency.Status = "ok"
			}
			status.Dependencies = append(status.Dependencies, dependency)
		}

		if status.Status != "ready" {
			writeJSON(w, http.StatusServiceUnavailable, status)
			return
		}
		writeJSON(w, http.StatusOK, status)
	}
}
