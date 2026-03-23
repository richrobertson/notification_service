# Notification Service Roadmap

## Current status

The repository is currently **through Stage 10**, with earlier follow-up fixes and polish already merged.

- Stages 3 through 10 are complete
- Stage 8 dispatch outbox durability is in place
- Stage 9 advanced delivery controls are in place
- Stage 10 production / platform polish is in place
- The next major milestone is a later expansion step rather than another foundational runtime milestone

---

## Done

### Stage 3 — Async Dispatch Foundation
- Redis-backed dispatch queue
- Dispatcher process for channel routing
- Initial async notification execution model

### Stage 4 — Real Delivery Workers
- Webhook and email delivery workers
- Reliable reserve / process / ack flow
- Hardened worker execution model

### Stage 4 Follow-up — Worker Hardening
- Safe and repeatable migration behavior
- Reserve / ack / requeue worker semantics
- Better distinction between terminal and transient failures

### Stage 5 — Retry, Dead-Letter, Replay
- Bounded retry scheduling
- Durable dead-letter persistence
- Replay API for failed deliveries
- Recovery of stranded `*:processing` queue jobs

### Stage 6 — Correctness Under Duplication
- Attempt-level idempotency
- Duplicate suppression for queue and worker execution
- Monotonic delivery attempt state transitions
- Notification and attempt inspection APIs
- Audit trail for delivery lifecycle events

### Stage 6 Follow-up — Polish
- Improved notification status freshness
- Correct handling of pending initial and replay attempts in inspection paths
- Better operator visibility into in-flight work

### Stage 7 — Backpressure, Rate Limiting, and Tenant Isolation
- Tenant-aware API rate limiting
- Queue-depth-based backpressure
- Lightweight tenant fairness in workers
- Worker concurrency controls
- Overload visibility through metrics and pressure reporting

### Stage 8 — Stronger Dispatch Durability
- Narrow Postgres-backed dispatch outbox
- Dedicated outbox publisher worker
- Postgres as authoritative source for dispatch publication
- Reduced reliance on scattered enqueue-recovery paths

### Stage 9 — Advanced Delivery Controls
- Scheduled delivery with durable promotion via `cmd/scheduler`
- Delivery policy controls for tenant/channel pause, replay, scheduling, failover, and retry overrides
- Manual cancellation of future scheduled notifications
- Manual redrive of eligible deferred notification work
- Narrow provider failover for webhook and secondary SMTP delivery
- Audit visibility for failover and operator delivery-control actions

### Stage 10 — Production / Platform Polish
- Health, readiness, and richer metrics endpoints
- Admin token protection for operator routes
- Config validation and bounded HTTP request handling
- Graceful shutdown across API and worker processes
- Retention-driven maintenance cleanup and updated runbooks

---

## In progress

- No separate major milestone is currently in progress
- The system is in a **stabilized post-Stage-10 state**
- The next milestone is a later expansion step, not a missing Stage 10 follow-up

---

## Next

### Later platform expansion

- Additional channels such as SMS
- Richer provider routing and operational controls
- More complete admin and operator experiences
- Deeper tenant/platform policy and quota controls

---

## Later

- Multi-region and more advanced deployment models
- Broader provider ecosystem work if the design still benefits from it
- More complete admin/operator tooling beyond API and runbooks

---

## Notes

- The roadmap is intentionally **pragmatic and incremental**
- The system already prioritizes:
  - durability
  - correctness
  - operator visibility
  - survivability under load
  - production/runtime safety
- The design continues to favor:
  - simple, explicit components
  - Postgres as source of truth
  - Redis as execution transport
  - at-least-once delivery with strong deduplication and durable publication
