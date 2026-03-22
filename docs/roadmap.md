# Notification Service Roadmap

## Current status

The repository is currently **through Stage 8**, with earlier Stage 6 follow-up fixes and polish already merged.

- Stages 3 through 8 are complete
- Stage 6 correctness and inspection improvements are in place
- Stage 7 overload protection and tenant isolation are in place
- Stage 8 dispatch outbox durability is in place
- The next major milestone is **Stage 9**

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

---

## In progress

- No separate major milestone is currently in progress
- The system is in a **stabilized post–Stage 8 state**
- The next milestone is clearly **Stage 9**

---

## Next

### Stage 9 — Advanced Delivery Controls

Focus: **richer delivery policy and platform sophistication**

- Provider failover strategies
- Richer scheduling and delivery policies
- Per-channel controls and tuning
- Stronger tenant and platform controls
- More advanced operational handling around delivery behavior

---

## Later

### Stage 10 — Production / Platform Polish
- Security hardening
- Expanded observability and metrics
- Admin and operator workflows
- Deployment and operational maturity

---

## Notes

- The roadmap is intentionally **pragmatic and incremental**
- The system already prioritizes:
  - durability
  - correctness
  - operator visibility
  - survivability under load
- The primary remaining gap is **advanced delivery policy and platform sophistication**
- The design continues to favor:
  - simple, explicit components
  - Postgres as source of truth
  - Redis as execution transport
  - at-least-once delivery with strong deduplication and durable publication