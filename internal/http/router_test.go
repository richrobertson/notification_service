package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthz(t *testing.T) {
	router := NewRouter(RouterDeps{AppName: "notification-service"})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestReadyzChecksDependencies(t *testing.T) {
	router := NewRouter(RouterDeps{
		AppName: "notification-service",
		DBPing:  func(context.Context) error { return nil },
		RedisPing: func(context.Context) error {
			return context.DeadlineExceeded
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestAdminMiddlewareProtectsOperatorEndpoints(t *testing.T) {
	router := NewRouter(RouterDeps{AppName: "notification-service", AdminToken: "secret"})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestAdminMiddlewareAllowsBearerToken(t *testing.T) {
	router := NewRouter(RouterDeps{AppName: "notification-service", AdminToken: "secret"})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestRequestBodyLimitMiddlewareRejectsLargeBodies(t *testing.T) {
	router := NewRouter(RouterDeps{AppName: "notification-service", MaxRequestBodyBytes: 4})
	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", strings.NewReader(`{"id":"way-too-large"}`))
	req.ContentLength = int64(len(`{"id":"way-too-large"}`))
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}
