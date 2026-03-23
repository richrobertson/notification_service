// Package handlers implements the concrete HTTP handlers used by the Stage 7+
// API.
//
// The package is organized around practical operator-facing workflows:
//
//   - notification submission and inspection
//   - dead-letter replay and redrive
//   - delivery policy management
//   - health, readiness, and metrics exposure
//
// The handlers are intentionally thin wrappers over the store and queue-facing
// interfaces so tests can focus on business behavior instead of transport
// plumbing.
package handlers
