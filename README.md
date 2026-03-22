# notification_service

## Project Overview

`notification_service` is a runnable Go notification platform foundation. The repository is currently through Stage 9, with PostgreSQL as the authoritative source of truth for durable dispatch publication, Redis serving as the execution transport, and explicit delivery policy controls layered on top.

See [docs/roadmap.md](docs/roadmap.md) for current milestone status and next steps.

The service now provides:

- health and readiness endpoints
- tenant creation
- template creation
- notification submission
- PostgreSQL-backed persistence using `database/sql`
- Redis-backed dispatch queues using Redis lists
- a standalone dispatcher process that routes generic dispatch jobs to channel-specific queues
- standalone webhook and email workers that consume channel-specific queues
- a standalone retry worker that polls PostgreSQL for due retries and creates durable retry dispatch intents
- a standalone outbox publisher that polls PostgreSQL for pending dispatch intents and publishes them to Redis
- real webhook HTTP POST delivery
- real SMTP-based email delivery
- bounded retry scheduling for transient delivery failures
- durable PostgreSQL dead-letter persistence after retry exhaustion
- dead-letter list/get/replay API endpoints
- notification and attempt inspection API endpoints
- attempt-level duplicate-job suppression using PostgreSQL state guards
- notification status rollups derived from durable attempt history
- audit events for major lifecycle transitions
- automated best-effort recovery that drains stranded `*:processing` queues back to their source queues

## Current Milestone Status

The repository is in a stabilized post-Stage-9 state. Stages 3 through 9 are complete, and the next major milestone is Stage 10.

Current baseline includes:

- `GET /v1/health`
- `GET /v1/readiness`
- `POST /v1/tenants`
- `POST /v1/templates`
- `POST /v1/notifications`
- `GET /v1/notifications/{id}`
- `GET /v1/notifications/{id}/attempts`
- `GET /v1/attempts/{id}`
- `GET /v1/dead-letters`
- `GET /v1/dead-letters/{id}`
- `POST /v1/dead-letters/{id}/replay`
- PostgreSQL persistence for notifications, delivery attempts, and dead letters
- Redis-backed dispatch queue `notify:dispatch`
- dispatcher routing to `notify:dispatch:webhook` and `notify:dispatch:email`
- channel worker reserve/ack flow using per-channel `*:processing` queues
- retry scheduling using `delivery_attempts.status = retry_scheduled` plus `next_retry_at`
- retry execution via `cmd/retry_worker`
- dispatch publication via `cmd/outbox_publisher`
- durable dead-lettering using the existing `dead_letters` table
- operator replay that reuses a durable replay attempt identity and creates a durable replay dispatch intent
- compare-and-set attempt activation so only one worker can move `pending -> in_progress`
- duplicate-job suppression when a queued attempt is already `in_progress`, `sent`, `failed`, `retry_scheduled`, or `dead_lettered`
- monotonic attempt transitions guarded in SQL (`in_progress -> sent|failed|retry_scheduled|dead_lettered`)
- webhook/email correlation headers (`Idempotency-Key`, `X-Notification-Attempt-ID`, `X-Notification-ID`, deterministic email `Message-ID`)
- notification rollups (`accepted`, `processing`, `delivered`, `partially_delivered`, `failed`, `dead_lettered`)
- dispatch intent durability in PostgreSQL via `dispatch_outbox`
- audit events for notification acceptance, dispatch intent creation/publication, retry scheduling, dead-lettering, replay, and duplicate suppression
- startup and periodic recovery of stranded jobs in:
  - `notify:dispatch:processing`
  - `notify:dispatch:webhook:processing`
  - `notify:dispatch:email:processing`
- future scheduled notification creation and durable promotion via `cmd/scheduler`
- delivery policy controls for tenant/channel pause, replay gating, scheduling gating, failover enablement, and retry overrides
- manual cancellation for scheduled notifications before promotion
- manual redrive of not-yet-promoted notification work
- narrow provider failover for webhook and SMTP delivery, with audit visibility

## Delivery Semantics

The delivery service now uses a small explicit error model:

- success: mark attempt `sent`
- terminal failure: mark attempt `failed`
- transient retryable failure before retry exhaustion: mark attempt `retry_scheduled` and set `next_retry_at`
- transient retryable failure after retry exhaustion: mark attempt `dead_lettered` and insert a durable dead-letter record

Retry policy is intentionally simple and configurable:

- `RETRY_MAX_ATTEMPTS`
- `RETRY_BASE_DELAY`
- `RETRY_MAX_DELAY`
- `RETRY_EXPONENTIAL_BACKOFF`
- `RETRY_JITTER`
- `RETRY_WORKER_POLL_INTERVAL`
- `PROCESSING_RECOVERY_INTERVAL`

The current retry model is:

1. the active attempt starts as `pending`
2. exactly one worker may mark it `in_progress` via a compare-and-set update
3. on transient failure, the same attempt is marked `retry_scheduled` with `next_retry_at`
4. when API, retry, or replay creates publishable work, it writes the attempt plus one `dispatch_outbox` intent in the same PostgreSQL transaction
5. the outbox publisher later enqueues Redis work only for that already-durable intent and marks it published after success
6. if Redis is unavailable at outbox publish time, the intent remains pending in PostgreSQL and is retried later
7. once the retry budget is exhausted, the final attempt is marked `dead_lettered` and a `dead_letters` row is inserted

Replay uses the same model: the replay attempt is created durably first, the dead letter is only marked replayed after the outbox publisher successfully enqueues Redis work, and failed publish work remains recoverable from PostgreSQL. Initial API attempts and retry attempts now follow the same path, so Redis is treated as execution transport rather than the source of truth for "what still needs dispatch publication." Duplicate Redis jobs are still possible underneath, but Stage 6 attempt-level idempotency and duplicate suppression still protect execution.

## Stage 9 Delivery Controls

- Notifications may be created with `scheduled_for`; they stay durable in PostgreSQL and are promoted later by `cmd/scheduler`.
- Delivery policies are durable and explicit, with predictable tenant/channel override behavior on top of system defaults.
- Policy controls include pause/resume, scheduling enablement, replay enablement, failover enablement, and retry overrides.
- Paused work is not discarded; scheduled promotion and outbox publication both respect paused policy state.
- Manual operator controls now distinguish replay of dead letters from redrive of scheduled or deferred notification work.
- Webhook delivery supports primary plus secondary endpoint fallback when failover is enabled and the failure is retryable.
- Email delivery supports primary plus secondary SMTP fallback when failover is enabled and the failure is retryable.
- Failover remains visible through audit events and attempt inspection rather than being hidden behind a generalized provider abstraction.

## Dispatch Outbox Lite

`dispatch_outbox` is intentionally narrow. It is not a generalized event bus, not CDC, and not WAL streaming.

- One publishable attempt gets at most one live dispatch intent.
- Initial submission, retry creation, and replay creation all write their dispatch intent transactionally with the durable attempt state.
- The outbox publisher polls pending intents, pushes `notify:dispatch`, and only then marks the intent `published`.
- If Redis is down when the outbox publisher tries to publish, the intent stays `pending` with `last_error` for inspection and later recovery.
- The API still depends on Redis for rate limiting and queue-pressure checks, so a Redis outage can still block new submissions even though already-created dispatch intents remain durable in PostgreSQL.
- Existing attempt inspection still shows pending durable work through `status = pending` plus `dispatch_enqueued_at = null`.

## Duplicate Suppression Model

Stage 6 still uses **at-least-once queueing** underneath. It does **not** claim true exactly-once delivery across Redis, process crashes, PostgreSQL, and downstream providers.

What it now does provide:

- submission idempotency at the notification API layer
- attempt-level compare-and-set activation (`pending -> in_progress`)
- suppression of duplicate jobs once an attempt is already active or already terminal/superseded
- monotonic terminal updates so a duplicate worker cannot turn `sent -> failed` or `dead_lettered -> sent`
- downstream correlation headers so operators and receivers can identify repeated deliveries more easily

The goal is an **exactly-once illusion** for operators and API users, while remaining honest that the underlying queueing model is still at-least-once.

## Queue Recovery Behavior

Stage 5 adds bounded automated recovery for stranded reserved jobs.

At dispatcher/worker startup, and periodically afterward, the process drains each known Redis processing queue back into its source queue using best-effort `RPOPLPUSH` recovery. This is operationally honest but intentionally limited:

- recovery is best-effort, not exactly-once
- duplicate delivery is still possible if a process crashes after an external provider call but before durable state/ack cleanup
- recovery is FIFO/LIFO-consistent-enough for Redis list queues, not a strict ordering guarantee

## Local Development

Start local infrastructure:

```bash
make dev-up
```

Apply the database migrations:

```bash
make migrate-up
```

Run the API service:

```bash
make run-api
```

Run the dispatcher:

```bash
make run-dispatcher
```

Run the webhook worker:

```bash
make run-webhook-worker
```

Run the email worker:

```bash
make run-email-worker
```

Run the retry worker:

```bash
go run ./cmd/retry_worker
```

Run the outbox publisher:

```bash
go run ./cmd/outbox_publisher
```

Run the scheduler/promoter:

```bash
go run ./cmd/scheduler
```

## Default Local Configuration

- HTTP port: `8080`
- PostgreSQL: `postgres://notification:notification@localhost:5432/notification_platform?sslmode=disable`
- Redis address: `localhost:6379`
- webhook timeout: `5s`
- retry max attempts: `3`
- retry base delay: `5s`
- retry max delay: `1m`
- retry worker poll interval: `2s`
- outbox publisher poll interval: `2s`
- scheduler poll interval: `2s`
- processing recovery interval: `30s`
- Mailpit SMTP host/port: `localhost:1025`

## Environment Variables

Existing:

- `WEBHOOK_TIMEOUT`
- `QUEUE_BLOCK_TIMEOUT`
- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USERNAME`
- `SMTP_PASSWORD`
- `SMTP_FROM`
- `SMTP_USE_TLS`
- `SMTP_STARTTLS`
- `SMTP_INSECURE_SKIP_VERIFY`

New in Stage 5:

- `RETRY_MAX_ATTEMPTS`
- `RETRY_BASE_DELAY`
- `RETRY_MAX_DELAY`
- `RETRY_EXPONENTIAL_BACKOFF`
- `RETRY_JITTER`
- `RETRY_WORKER_POLL_INTERVAL`
- `OUTBOX_POLL_INTERVAL`
- `SCHEDULER_POLL_INTERVAL`
- `PROCESSING_RECOVERY_INTERVAL`
- `SMTP_SECONDARY_HOST`
- `SMTP_SECONDARY_PORT`
- `SMTP_SECONDARY_USERNAME`
- `SMTP_SECONDARY_PASSWORD`
- `SMTP_SECONDARY_FROM`
- `SMTP_SECONDARY_USE_TLS`
- `SMTP_SECONDARY_STARTTLS`
- `SMTP_SECONDARY_INSECURE_SKIP_VERIFY`

## Remaining Intentional Limitations

The roadmap remains deliberately pragmatic. The service still does **not** provide:

- exactly-once delivery semantics
- a full generalized outbox or event platform
- a generalized workflow or orchestration engine
- CDC, WAL tailing, or Debezium-style publication
- advanced cross-region or multi-region replication guarantees
- generalized duplicate suppression across every possible crash boundary
- fully pluggable provider ecosystems or marketplace-style provider routing
- rate limiting / quota enforcement beyond whatever already exists in the broader codebase
- an admin UI or operator console
- distributed coordination or leader election for recovery/retry/outbox workers

Those remain future milestones, with the next major gap centered on production/platform polish rather than basic durability, policy control, or survivability under load.

## Stage 7: Backpressure, rate limiting, and tenant isolation

Stage 7 adds graceful degradation controls while keeping the existing Redis + Postgres architecture.

### Overload behavior
- The API now applies a per-tenant Redis-backed fixed-window rate limit and returns `429 Too Many Requests` with `Retry-After` when exceeded.
- The API checks queue depth on the dispatch queue and channel queues before accepting new notifications or replay requests.
- Queue soft limits emit warnings; queue hard limits reject new work early with explicit overload responses instead of letting Redis queues grow without bound.

### Fairness model
- Workers use a bounded fair scheduler that rotates buffered jobs by tenant instead of draining one tenant indefinitely.
- Each worker has configurable total concurrency and a configurable per-tenant in-flight cap so a single tenant cannot monopolize SMTP or webhook capacity.
- This is lightweight fairness, not strict QoS: ordering is still best-effort and delivery remains at-least-once.

### Retry behavior under pressure
- Retry scheduling stretches when channel queue depth is already above the soft limit.
- The retry worker skips enqueue bursts when queues are already saturated, reducing retry storms during incidents.

### Visibility
- `/v1/metrics` returns queue depth, soft/hard limits, rate-limited totals, rejected totals, worker saturation counts, and tenant throttling counts.
- Logs now distinguish rate limiting, soft pressure warnings, hard queue rejection, and retry deferrals.

### New environment variables
- `API_RATE_LIMIT_PER_SECOND`
- `API_RATE_LIMIT_WINDOW`
- `QUEUE_SOFT_LIMIT`
- `QUEUE_HARD_LIMIT`
- `BACKPRESSURE_RETRY_AFTER`
- `EMAIL_WORKER_CONCURRENCY`
- `WEBHOOK_WORKER_CONCURRENCY`
- `PER_TENANT_WORKER_BURST`
- `PER_TENANT_MAX_IN_FLIGHT`
- `RETRY_PRESSURE_MULTIPLIER`
- `RETRY_PRESSURE_MIN_DELAY`

### Remaining limitations
- Fairness is bounded and opportunistic rather than strict weighted fair queuing.
- Rate limiting is fixed-window Redis counter based, so it is simple rather than perfectly smooth.
- The system still provides at-least-once delivery with no strict SLA or strict per-tenant QoS guarantees.
