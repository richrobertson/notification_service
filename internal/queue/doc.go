// Package queue provides the Redis-backed execution transport used by the
// dispatcher, channel workers, and Stage 7 pressure controls.
//
// The package exposes a small set of concepts:
//
//   - DispatchJob is the durable execution payload placed on Redis lists
//   - RedisQueue handles enqueue, reserve, ack, and recovery operations
//   - PressureSnapshot summarizes queue depth for backpressure decisions
//   - TenantRateLimiter uses Redis counters for fixed-window API throttling
//
// The transport is intentionally at-least-once. Reserve/ack semantics and
// processing-queue recovery exist to make failures inspectable and recoverable,
// not to promise exactly-once routing.
package queue
