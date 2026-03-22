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

type DependencyCheck struct {
	Name string
	Ping func(context.Context) error
}

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

func Readiness(checks ...DependencyCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		status := readinessResponse{
			Status:       "ready",
			Dependencies: make([]dependencyStatus, 0, len(checks)),
		}
		for _, check := range checks {
			dependency := dependencyStatus{Name: check.Name, Status: "ok"}
			if check.Ping != nil {
				if err := check.Ping(ctx); err != nil {
					dependency.Status = "down"
					status.Status = "not_ready"
				}
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
