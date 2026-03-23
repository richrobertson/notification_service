// Package httpserver wires the operator and application HTTP surface for the
// service.
//
// The router intentionally stays composition-oriented rather than framework
// heavy. RouterDeps supplies the concrete store, queue, limiter, and health
// dependencies, while the package applies cross-cutting behavior such as:
//
//   - structured request logging
//   - panic recovery
//   - admin protection for operator routes
//   - request size limits
//   - health, readiness, and metrics aliases
//
// The actual business handlers live in internal/http/handlers.
package httpserver
