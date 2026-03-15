package notify

import (
	"net/http"
	"strings"
	"testing"
)

func TestFunctionalRequirementsOfTheMVP(t *testing.T) {
	server := newTestServer()
	handler := server.Handler()

	swaggerUI := doJSONRequest(t, handler, http.MethodGet, "/swagger", nil)
	expectStatus(t, swaggerUI, http.StatusOK, "The API should expose a Swagger web UI for exploring the REST endpoints")
	expectTrue(t, strings.Contains(swaggerUI.Body.String(), "SwaggerUIBundle"), "The Swagger web UI should include the Swagger UI bootstrap script")

	openAPI := doJSONRequest(t, handler, http.MethodGet, "/openapi.json", nil)
	expectStatus(t, openAPI, http.StatusOK, "The API should expose an OpenAPI document that powers the Swagger web UI")
	openAPIBody := decodeBody[map[string]any](t, openAPI)
	expectEqual(t, openAPIBody["openapi"].(string), "3.1.0", "The OpenAPI document should declare the expected OpenAPI version")
	paths, ok := openAPIBody["paths"].(map[string]any)
	expectTrue(t, ok, "The OpenAPI document should publish a path map for the REST API")
	_, hasNotificationsPath := paths["/v1/notifications"]
	expectTrue(t, hasNotificationsPath, "The OpenAPI document should include the notification submission endpoint")

	health := doJSONRequest(t, handler, http.MethodGet, "/v1/health", nil)
	expectStatus(t, health, http.StatusOK, "The health endpoint should confirm that the API service is alive")

	readiness := doJSONRequest(t, handler, http.MethodGet, "/v1/readiness", nil)
	expectStatus(t, readiness, http.StatusOK, "The readiness endpoint should confirm that the API service is ready to accept traffic")

	createTenant := doJSONRequest(t, handler, http.MethodPost, "/v1/tenants", map[string]any{
		"id":          "acme",
		"name":        "Acme",
		"daily_quota": 10,
	})
	expectStatus(t, createTenant, http.StatusCreated, "The MVP should allow operators to create a tenant")
	tenant := decodeBody[Tenant](t, createTenant)
	expectEqual(t, tenant.ID, "acme", "The created tenant should preserve the requested tenant identifier")
	expectEqual(t, tenant.Status, "active", "A newly created tenant should start in the active state")

	getTenant := doJSONRequest(t, handler, http.MethodGet, "/v1/tenants/acme", nil)
	expectStatus(t, getTenant, http.StatusOK, "The MVP should allow callers to read a tenant after it is created")

	createTemplate := doJSONRequest(t, handler, http.MethodPost, "/v1/templates", map[string]any{
		"id":        "password-reset",
		"tenant_id": "acme",
		"name":      "Password reset",
		"channel":   "email",
		"body":      "Hello {{ first_name }}",
	})
	expectStatus(t, createTemplate, http.StatusCreated, "The MVP should allow tenants to create a notification template")
	template := decodeBody[Template](t, createTemplate)
	expectEqual(t, template.Version, 1, "A newly created template should start at version 1")

	updateTemplate := doJSONRequest(t, handler, http.MethodPut, "/v1/templates/password-reset", map[string]any{
		"name": "Password reset v2",
		"body": "Hello {{ first_name }}, reset here: {{ reset_url }}",
	})
	expectStatus(t, updateTemplate, http.StatusOK, "The MVP should allow tenants to update an existing template")
	updatedTemplate := decodeBody[Template](t, updateTemplate)
	expectEqual(t, updatedTemplate.Version, 2, "Updating a template should increment its version")

	createNotification := doJSONRequest(t, handler, http.MethodPost, "/v1/notifications", map[string]any{
		"tenant_id":       "acme",
		"template_id":     "password-reset",
		"channels":        []string{"email", "webhook"},
		"recipient":       map[string]any{"email": "user@example.com", "url": "https://example.com/hook"},
		"variables":       map[string]any{"first_name": "Sam", "reset_url": "https://example.com/reset/abc"},
		"idempotency_key": "req-1",
	})
	expectStatus(t, createNotification, http.StatusAccepted, "The submission API should accept a notification without waiting for downstream delivery")
	notification := decodeBody[Notification](t, createNotification)
	expectEqual(t, notification.Status, "accepted", "A new notification should start in the accepted state")
	expectEqual(t, len(notification.Attempts), 2, "Fan-out across the requested channels should create one delivery attempt per channel")
	expectEqual(t, notification.Attempts[0].Status, "pending", "Each new delivery attempt should start in the pending state")

	getNotification := doJSONRequest(t, handler, http.MethodGet, "/v1/notifications/"+notification.ID, nil)
	expectStatus(t, getNotification, http.StatusOK, "The MVP should allow callers to read back a notification after submission")

	usage := doJSONRequest(t, handler, http.MethodGet, "/v1/tenants/acme/usage", nil)
	expectStatus(t, usage, http.StatusOK, "The usage endpoint should expose quota consumption for a tenant")
	usageBody := decodeBody[Usage](t, usage)
	expectEqual(t, usageBody.AcceptedNotifications, 1, "Submitting one distinct notification should increment the accepted notification count by one")
	expectEqual(t, usageBody.RemainingQuota, 9, "The remaining daily quota should decrease after a notification is accepted")

	deadLetters := doJSONRequest(t, handler, http.MethodGet, "/v1/dead-letters", nil)
	expectStatus(t, deadLetters, http.StatusOK, "The operations API should expose the dead-letter collection even when it is empty")
	deadLetterBody := decodeBody[[]DeadLetter](t, deadLetters)
	expectEqual(t, len(deadLetterBody), 0, "A clean MVP flow should start with no dead-lettered jobs")

	replay := doJSONRequest(t, handler, http.MethodPost, "/v1/notifications/"+notification.ID+"/replay", nil)
	expectStatus(t, replay, http.StatusOK, "The MVP should support replaying a stored notification")
	replayedNotification := decodeBody[Notification](t, replay)
	expectEqual(t, replayedNotification.Status, "processing", "Replaying a notification should move it back into processing")
	expectEqual(t, len(replayedNotification.Attempts), 4, "Replaying a two-channel notification should create a new attempt for each channel")
}

func TestNonFunctionalRequirementsThatTheCurrentMVPCanGuarantee(t *testing.T) {
	server := newTestServer()
	handler := server.Handler()

	createTenant := doJSONRequest(t, handler, http.MethodPost, "/v1/tenants", map[string]any{
		"id":          "acme",
		"name":        "Acme",
		"daily_quota": 2,
	})
	expectStatus(t, createTenant, http.StatusCreated, "The test setup should be able to create the first tenant")

	secondTenant := doJSONRequest(t, handler, http.MethodPost, "/v1/tenants", map[string]any{
		"id":          "globex",
		"name":        "Globex",
		"daily_quota": 2,
	})
	expectStatus(t, secondTenant, http.StatusCreated, "The test setup should be able to create the second tenant")

	acmeTemplate := doJSONRequest(t, handler, http.MethodPost, "/v1/templates", map[string]any{
		"id":        "acme-template",
		"tenant_id": "acme",
		"name":      "Acme email",
		"channel":   "email",
		"body":      "Hello from Acme",
	})
	expectStatus(t, acmeTemplate, http.StatusCreated, "The first tenant should be able to create its own template")

	globexTemplate := doJSONRequest(t, handler, http.MethodPost, "/v1/templates", map[string]any{
		"id":        "globex-template",
		"tenant_id": "globex",
		"name":      "Globex email",
		"channel":   "email",
		"body":      "Hello from Globex",
	})
	expectStatus(t, globexTemplate, http.StatusCreated, "The second tenant should be able to create its own template")

	firstSubmission := doJSONRequest(t, handler, http.MethodPost, "/v1/notifications", map[string]any{
		"tenant_id":       "acme",
		"template_id":     "acme-template",
		"channels":        []string{"email"},
		"recipient":       map[string]any{"email": "user@example.com"},
		"variables":       map[string]any{"name": "Sam"},
		"idempotency_key": "stable-key",
	})
	expectStatus(t, firstSubmission, http.StatusAccepted, "The first submission for a unique idempotency key should be accepted")
	firstNotification := decodeBody[Notification](t, firstSubmission)

	duplicateSubmission := doJSONRequest(t, handler, http.MethodPost, "/v1/notifications", map[string]any{
		"tenant_id":       "acme",
		"template_id":     "acme-template",
		"channels":        []string{"email"},
		"recipient":       map[string]any{"email": "user@example.com"},
		"variables":       map[string]any{"name": "Sam"},
		"idempotency_key": "stable-key",
	})
	expectStatus(t, duplicateSubmission, http.StatusOK, "The same idempotency key should return the original submission instead of creating a duplicate")
	duplicateNotification := decodeBody[Notification](t, duplicateSubmission)
	expectEqual(t, duplicateNotification.ID, firstNotification.ID, "Idempotent submission should return the original notification identifier")

	crossTenantAttempt := doJSONRequest(t, handler, http.MethodPost, "/v1/notifications", map[string]any{
		"tenant_id":       "globex",
		"template_id":     "acme-template",
		"channels":        []string{"email"},
		"recipient":       map[string]any{"email": "user@example.com"},
		"variables":       map[string]any{"name": "Sam"},
		"idempotency_key": "tenant-boundary-check",
	})
	expectStatus(t, crossTenantAttempt, http.StatusNotFound, "Tenant isolation should prevent one tenant from using another tenant's template")

	secondDistinctSubmission := doJSONRequest(t, handler, http.MethodPost, "/v1/notifications", map[string]any{
		"tenant_id":       "acme",
		"template_id":     "acme-template",
		"channels":        []string{"email"},
		"recipient":       map[string]any{"email": "user@example.com"},
		"variables":       map[string]any{"name": "Taylor"},
		"idempotency_key": "distinct-key",
	})
	expectStatus(t, secondDistinctSubmission, http.StatusAccepted, "A second distinct notification within quota should still be accepted")

	quotaExceeded := doJSONRequest(t, handler, http.MethodPost, "/v1/notifications", map[string]any{
		"tenant_id":       "acme",
		"template_id":     "acme-template",
		"channels":        []string{"email"},
		"recipient":       map[string]any{"email": "user@example.com"},
		"variables":       map[string]any{"name": "Jordan"},
		"idempotency_key": "quota-limit-hit",
	})
	expectStatus(t, quotaExceeded, http.StatusTooManyRequests, "Quota enforcement should reject submissions that exceed the tenant's daily limit")

	replayMissing := doJSONRequest(t, handler, http.MethodPost, "/v1/notifications/missing/replay", nil)
	expectStatus(t, replayMissing, http.StatusNotFound, "Recoverable failure handling should not hide missing notification errors during replay")
}
