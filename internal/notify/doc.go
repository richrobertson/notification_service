// Package notify contains the earlier in-memory MVP service and HTTP server.
//
// The newer Stage 4+ production-oriented path lives under the store, queue,
// delivery, and httpserver packages. This package remains useful for lightweight
// local examples, openapi shaping, and historical integration tests that model
// the original service contract without Postgres or Redis.
//
// Maintainers should treat this package as the simplest conceptual entry point,
// not the source of truth for the durable runtime architecture.
package notify
