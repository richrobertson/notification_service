# notification_service

## Project Overview

`notification_service` is a runnable Go foundation for a multi-tenant notification platform. Stage 4 adds real channel delivery workers while deliberately keeping delivery guarantees simple and limited. This hardening patch makes the current worker flow more operationally honest without expanding into Stage 5 retry or replay features.

The service currently provides:

- health and readiness endpoints
- tenant creation
- template creation
- notification submission
- PostgreSQL-backed persistence using `database/sql`
- Redis-backed dispatch queues using Redis lists
- a standalone dispatcher process that routes generic dispatch jobs to channel-specific queues
- standalone webhook and email workers that consume channel-specific queues
- real webhook HTTP POST delivery
- real SMTP-based email delivery
- OpenTelemetry bootstrap wiring for local development
- Docker Compose infrastructure for Postgres, Redis, Prometheus, Grafana, Jaeger, the OpenTelemetry Collector, and Mailpit for local SMTP capture

## Stage 4 Status

Implemented in this stage:

- `GET /v1/health`
- `GET /v1/readiness`
- `POST /v1/tenants`
- `POST /v1/templates`
- `POST /v1/notifications`
- PostgreSQL persistence for notifications
- PostgreSQL persistence for the initial `delivery_attempts` row with `attempt_number = 1` and `status = pending`
- generic dispatch job enqueue to Redis queue `notify:dispatch`
- dispatcher consumption from `notify:dispatch`
- dispatcher routing to channel queues:
  - `notify:dispatch:webhook`
  - `notify:dispatch:email`
- webhook worker consumption from `notify:dispatch:webhook` with reserve/ack via `notify:dispatch:webhook:processing`
- email worker consumption from `notify:dispatch:email` with reserve/ack via `notify:dispatch:email:processing`
- template rendering with deterministic missing-variable failures
- delivery attempt lifecycle transitions from `pending` to `in_progress` to terminal `sent` or `failed`
- storage of delivery timestamps, provider message id, and concise delivery errors
- request logging and panic recovery middleware
- idempotent notification submission when `idempotency_key` is provided

Not implemented yet:

- retries
- DLQ handling
- replay flow
- scheduling
- auth hardening
- usage endpoints
- dead-letter inspection endpoints
- transactional outbox protection for DB + queue atomicity
- production-grade delivery guarantees such as retries, exactly-once execution, or provider failover

## Stage 4 Architecture Summary

Current request flow:

1. Client calls `POST /v1/notifications`
2. API validates tenant, template, and recipient fields
3. API persists the notification row in PostgreSQL
4. API inserts the initial `delivery_attempts` row with `pending`
5. API enqueues a small JSON dispatch job onto `notify:dispatch`
6. `cmd/dispatcher` blocks on `notify:dispatch`
7. Dispatcher republishes the same job to either `notify:dispatch:webhook` or `notify:dispatch:email`
8. A channel worker loads the notification, template, and delivery attempt from PostgreSQL
9. The worker renders the outbound payload and performs real delivery
10. The worker marks the attempt `in_progress` once work begins
11. On durable success the worker ACKs the reserved Redis job and marks `delivery_attempts` as `sent`
12. On durable terminal failure the worker ACKs the reserved Redis job and marks `delivery_attempts` as `failed`
13. On transient persistence or processing interruption the worker requeues the reserved job instead of silently losing it

Important honesty notes:

- The API still does **not** push directly to channel queues.
- Workers currently process one job at a time in a simple loop.
- Workers now reserve jobs into per-channel processing queues before attempting delivery so transient DB/update failures do not silently drop work.
- Malformed queue jobs can still strand a reserved entry in the processing queue and require operator cleanup; there is no automated recovery sweeper yet.
- PostgreSQL writes and Redis enqueue are **not** yet coordinated with an outbox pattern, so DB/queue atomicity is not yet hardened.
- Reserved in-flight jobs are safer than destructive pops, but a worker crash can still leave a job sitting in `*:processing` until an operator or a future recovery loop moves it back.
- Delivery completion is tracked on `delivery_attempts`; broader notification rollup state is intentionally still simple.


## Stage 4 Hardening Guarantees

What Stage 4 now guarantees:

- a worker does not remove a channel job from Redis until it has durably recorded either `sent` or terminal `failed`, or explicitly requeued the job after a transient processing failure
- attempts move to `in_progress` when a worker begins meaningful work
- terminal failures such as missing recipients, template rendering errors, webhook failures, and SMTP delivery failures are recorded as `failed`
- transient infrastructure failures such as PostgreSQL load/update failures cause the job to remain recoverable instead of being silently lost
- fresh database migration from scratch does not rely on an externally pre-created `set_updated_at()` helper

What Stage 4 still does **not** guarantee:

- no retries, retry scheduling, DLQ routing, replay APIs, or provider failover yet
- no automated recovery of jobs left behind in `*:processing` after a worker crash
- no outbox-style atomicity between PostgreSQL writes and Redis enqueue
- no exactly-once delivery semantics

These remain future Stage 5 concerns.

## Current Endpoints

- `GET /v1/health`
- `GET /v1/readiness`
- `POST /v1/tenants`
- `POST /v1/templates`
- `POST /v1/notifications`

## Current Queue Design

Redis list queues used in Stage 4:

- generic dispatch queue:
  - `notify:dispatch`
- channel queues:
  - `notify:dispatch:webhook`
  - `notify:dispatch:email`
- processing queues used by workers while a job is reserved:
  - `notify:dispatch:webhook:processing`
  - `notify:dispatch:email:processing`

Dispatch job envelope fields:

- `job_id`
- `notification_id`
- `attempt_id`
- `tenant_id`
- `channel`
- `created_at`

The job is intentionally small so workers load the full records from PostgreSQL.

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

Run the dispatcher in a second terminal:

```bash
make run-dispatcher
```

Run the webhook worker in a third terminal:

```bash
make run-webhook-worker
```

Run the email worker in a fourth terminal:

```bash
make run-email-worker
```

Default local configuration:

- HTTP port: `8080`
- PostgreSQL: `postgres://notification:notification@localhost:5432/notification_platform?sslmode=disable`
- Redis address: `localhost:6379`
- Redis password: empty
- Redis DB: `0`
- OTLP endpoint: `localhost:4317`
- webhook timeout: `5s`
- Mailpit SMTP host/port: `localhost:1025`
- Mailpit UI: `http://localhost:8025`

New environment variables:

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

Useful local endpoints:

- API health: `http://localhost:8080/v1/health`
- API readiness: `http://localhost:8080/v1/readiness`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000`
- Jaeger: `http://localhost:16686`
- Mailpit UI: `http://localhost:8025`

Run checks:

```bash
make fmt
make lint
make test
```

### Example: Create Webhook Template

```bash
curl -X POST http://localhost:8080/v1/templates   -H "Content-Type: application/json"   -d '{
    "id": "tpl_password_reset_webhook_v1",
    "tenant_id": "acme",
    "name": "password-reset",
    "channel": "webhook",
    "version": 1,
    "body": "{"event":"password_reset","url":"{{.reset_url}}"}"
  }'
```

### Example: Test Webhook Delivery Locally

Start a simple local receiver:

```bash
python -m http.server 18080
```

Then submit a webhook notification pointing at a real POST-capable endpoint you control locally, for example a request bin or a small mock server. If you use a custom local handler, the webhook worker will mark the attempt `sent` on any `2xx` response and `failed` on non-`2xx` or network errors.

### Example: Create Email Template

```bash
curl -X POST http://localhost:8080/v1/templates   -H "Content-Type: application/json"   -d '{
    "id": "tpl_welcome_email_v1",
    "tenant_id": "acme",
    "name": "welcome-email",
    "channel": "email",
    "version": 1,
    "body": "Hello {{.first_name}}, welcome to Acme."
  }'
```

### Example: Submit Email Notification

```bash
curl -X POST http://localhost:8080/v1/notifications   -H "Content-Type: application/json"   -d '{
    "id": "notif_email_001",
    "tenant_id": "acme",
    "template_id": "tpl_welcome_email_v1",
    "recipient_email": "user@example.test",
    "variables": {
      "first_name": "Ada"
    }
  }'
```

Mailpit will capture local emails so you can inspect them at `http://localhost:8025`.

## Database Migrations

Active migrations:

- `migrations/001_init.sql`
- `migrations/002_stage4_delivery_attempts.sql`
