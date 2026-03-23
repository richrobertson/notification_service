// Package store contains the Postgres-backed durable model for the
// notification_service runtime.
//
// It is the system's authoritative state layer for tenants, templates,
// notifications, delivery attempts, dead letters, policies, scheduled work,
// dispatch intents, audit events, and maintenance visibility.
//
// The exported Postgres methods are intentionally explicit instead of generic.
// That keeps the invariants near the queries that enforce them, which is
// especially important for:
//
//   - monotonic attempt state transitions
//   - idempotent repair paths
//   - Stage 8 dispatch outbox publication
//   - Stage 9 policy and scheduling controls
//   - Stage 10 operational metrics and cleanup flows
//
// New contributors should read the exported Postgres methods as the durable
// workflow contract for the rest of the system.
package store
