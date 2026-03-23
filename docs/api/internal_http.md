# github.com/richrobertson/notification-platform/internal/http

_Generated from Go doc comments. Run `make docs-godoc` to refresh._

```text
package httpserver // import "github.com/richrobertson/notification-platform/internal/http"

Package httpserver wires the operator and application HTTP surface for the
service.

The router intentionally stays composition-oriented rather than framework heavy.
RouterDeps supplies the concrete store, queue, limiter, and health dependencies,
while the package applies cross-cutting behavior such as:

  - structured request logging
  - panic recovery
  - admin protection for operator routes
  - request size limits
  - health, readiness, and metrics aliases

The actual business handlers live in internal/http/handlers.

FUNCTIONS

func NewRouter(deps RouterDeps) http.Handler
    NewRouter builds the Stage 10 HTTP surface for the service.

    The router keeps public submission routes separate from admin-protected
    operator routes and applies the shared middleware stack used by the API
    process.


TYPES

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
    RouterDeps supplies the concrete dependencies needed to build the HTTP
    router.

```
