# github.com/richrobertson/notification-platform/internal/http/handlers

_Generated from Go doc comments. Run `make docs-godoc` to refresh._

```text
package handlers // import "github.com/richrobertson/notification-platform/internal/http/handlers"

Package handlers implements the concrete HTTP handlers used by the Stage 7+ API.

The package is organized around practical operator-facing workflows:

  - notification submission and inspection
  - dead-letter replay and redrive
  - delivery policy management
  - health, readiness, and metrics exposure

The handlers are intentionally thin wrappers over the store and queue-facing
interfaces so tests can focus on business behavior instead of transport
plumbing.

FUNCTIONS

func Health(serviceName string) http.HandlerFunc
    Health returns a simple liveness handler for the current service name.

func Metrics(m *pressure.Monitor, provider OperationalMetricsProvider) http.HandlerFunc
    Metrics returns the JSON metrics handler used by `/metrics` and
    `/v1/metrics`.

    It combines the in-process pressure monitor with optional durable metrics
    from the store layer so operators can inspect both queue pressure and
    durable backlog state from one endpoint.

func Readiness(checks ...DependencyCheck) http.HandlerFunc
    Readiness returns a readiness handler that evaluates the supplied dependency
    checks and reports whether the process can currently do useful work.


TYPES

type API struct {
	// Has unexported fields.
}
    API bundles the store, queue, rate limiter, and pressure monitor used by the
    HTTP handlers.

func NewAPI(store apiStore, redisQueue dispatchQueue, limiter TenantRateLimiter, monitor PressureMonitor) *API
    NewAPI constructs the concrete Stage 7+ HTTP handler set.

func (a *API) CancelNotification() http.HandlerFunc
    CancelNotification handles `POST /v1/notifications/{id}/cancel`.

func (a *API) CreateNotification() http.HandlerFunc
    CreateNotification handles `POST /v1/notifications`.

    The handler applies Stage 7 rate limiting and backpressure checks, Stage 6
    idempotency repair behavior, and Stage 8/9 durable dispatch or scheduling
    behavior.

func (a *API) CreateTemplate() http.HandlerFunc
    CreateTemplate handles `POST /v1/templates`.

func (a *API) CreateTenant() http.HandlerFunc
    CreateTenant handles `POST /v1/tenants`.

func (a *API) GetAttempt() http.HandlerFunc
    GetAttempt handles `GET /v1/attempts/{id}`.

func (a *API) GetDeadLetter() http.HandlerFunc
    GetDeadLetter handles `GET /v1/dead-letters/{id}`.

func (a *API) GetNotification() http.HandlerFunc
    GetNotification handles `GET /v1/notifications/{id}`.

func (a *API) GetPolicy() http.HandlerFunc
    GetPolicy handles `GET /v1/policies/{id}`.

func (a *API) ListDeadLetters() http.HandlerFunc
    ListDeadLetters handles `GET /v1/dead-letters`.

func (a *API) ListNotificationAttempts() http.HandlerFunc
    ListNotificationAttempts handles `GET /v1/notifications/{id}/attempts`.

func (a *API) ListPolicies() http.HandlerFunc
    ListPolicies handles `GET /v1/policies`.

func (a *API) PausePolicy() http.HandlerFunc
    PausePolicy handles `POST /v1/policies/{id}/pause`.

func (a *API) RedriveNotification() http.HandlerFunc
    RedriveNotification handles `POST /v1/notifications/{id}/redrive`.

func (a *API) ReplayDeadLetter() http.HandlerFunc
    ReplayDeadLetter handles `POST /v1/dead-letters/{id}/replay`.

func (a *API) ResumePolicy() http.HandlerFunc
    ResumePolicy handles `POST /v1/policies/{id}/resume`.

func (a *API) UpsertPolicy() http.HandlerFunc
    UpsertPolicy handles `POST /v1/policies`.

type DependencyCheck struct {
	Name string
	Ping func(context.Context) error
}

type OperationalMetricsProvider interface {
	CollectOperationalMetrics(ctx context.Context, now time.Time) (store.OperationalMetrics, error)
}
    OperationalMetricsProvider supplies the store-backed metrics used by the
    metrics endpoint.

type PressureMonitor interface {
	Snapshot(ctx context.Context) (queue.PressureSnapshot, error)
	IncRateLimited(tenantID string)
	IncRejected(reason, tenantID string)
}
    PressureMonitor is the subset of queue-pressure behavior the API uses for
    Stage 7 overload protection.

type TenantRateLimiter interface {
	Allow(ctx context.Context, tenantID string) (bool, time.Duration, error)
}
    TenantRateLimiter is the API's tenant-aware request-throttling contract.

```
