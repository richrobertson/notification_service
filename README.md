# notification_service

## Project Overview

`notification_service` is a runnable Go notification platform foundation. Stage 5 keeps the Stage 4 Redis-list + PostgreSQL architecture, and adds bounded retries, durable dead-letter persistence, operator replay, and best-effort recovery of stranded reserved jobs.

The service now provides:

- health and readiness endpoints
- tenant creation
- template creation
- notification submission
- PostgreSQL-backed persistence using `database/sql`
- Redis-backed dispatch queues using Redis lists
- a standalone dispatcher process that routes generic dispatch jobs to channel-specific queues
- standalone webhook and email workers that consume channel-specific queues
- a standalone retry worker that polls PostgreSQL for due retries and republishes them to the dispatch queue
- real webhook HTTP POST delivery
- real SMTP-based email delivery
- bounded retry scheduling for transient delivery failures
- durable PostgreSQL dead-letter persistence after retry exhaustion
- dead-letter list/get/replay API endpoints
- automated best-effort recovery that drains stranded `*:processing` queues back to their source queues

## Stage 5 Status

Implemented in this stage:

- `GET /v1/health`
- `GET /v1/readiness`
- `POST /v1/tenants`
- `POST /v1/templates`
- `POST /v1/notifications`
- `GET /v1/dead-letters`
- `GET /v1/dead-letters/{id}`
- `POST /v1/dead-letters/{id}/replay`
- PostgreSQL persistence for notifications, delivery attempts, and dead letters
- Redis-backed dispatch queue `notify:dispatch`
- dispatcher routing to `notify:dispatch:webhook` and `notify:dispatch:email`
- channel worker reserve/ack flow using per-channel `*:processing` queues
- retry scheduling using `delivery_attempts.status = retry_scheduled` plus `next_retry_at`
- retry execution via `cmd/retry_worker`
- durable dead-lettering using the existing `dead_letters` table
- operator replay that creates a fresh attempt row and republishes a new dispatch job
- startup and periodic recovery of stranded jobs in:
  - `notify:dispatch:processing`
  - `notify:dispatch:webhook:processing`
  - `notify:dispatch:email:processing`

## Stage 5 Delivery Semantics

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
2. a worker marks it `in_progress`
3. on transient failure, the same attempt is marked `retry_scheduled` with `next_retry_at`
4. when the retry worker picks up a due retry, it first creates the next attempt durably in PostgreSQL with enqueue still pending
5. the retry worker then enqueues Redis work only for that already-durable attempt and marks it enqueued after success
6. if Redis is unavailable, the attempt remains pending enqueue in PostgreSQL and is retried later
7. once the retry budget is exhausted, the final attempt is marked `dead_lettered` and a `dead_letters` row is inserted

Replay uses the same model: the replay attempt is created durably first, the dead letter is only marked replayed after enqueue succeeds, and failed enqueue work remains recoverable from PostgreSQL. This keeps the history inspectable without introducing a full generalized outbox or lease framework.

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

## Default Local Configuration

- HTTP port: `8080`
- PostgreSQL: `postgres://notification:notification@localhost:5432/notification_platform?sslmode=disable`
- Redis address: `localhost:6379`
- webhook timeout: `5s`
- retry max attempts: `3`
- retry base delay: `5s`
- retry max delay: `1m`
- retry worker poll interval: `2s`
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
- `PROCESSING_RECOVERY_INTERVAL`

## Remaining Intentional Limitations After Stage 5

Stage 5 is deliberately modest. The service still does **not** provide:

- a transactional outbox pattern for PostgreSQL + Redis atomicity
- exactly-once delivery semantics
- provider failover
- advanced scheduling beyond bounded retry delays
- rate limiting / quota enforcement beyond whatever already exists in the broader codebase
- an operator UI
- distributed coordination or leader election for recovery/retry workers

Those remain future milestones.
