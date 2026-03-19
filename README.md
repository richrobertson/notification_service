# notification_service

## Project Overview

`notification_service` is a runnable Go foundation for a multi-tenant notification platform. Stage 3 adds the first durable async dispatch foundation while intentionally stopping before any real delivery execution.

The service currently provides:

- health and readiness endpoints
- tenant creation
- template creation
- notification submission
- PostgreSQL-backed persistence using `database/sql`
- Redis-backed dispatch queues using Redis lists
- a standalone dispatcher process that routes generic dispatch jobs to channel-specific queues
- OpenTelemetry bootstrap wiring for local development
- Docker Compose infrastructure for Postgres, Redis, Prometheus, Grafana, Jaeger, and the OpenTelemetry Collector

## Stage 3 Status

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
- request logging and panic recovery middleware
- idempotent notification submission when `idempotency_key` is provided

Not implemented yet:

- webhook execution
- email sending
- workers that consume channel queues
- retries
- DLQ handling
- replay flow
- scheduling
- auth hardening
- usage endpoints
- dead-letter inspection endpoints
- transactional outbox protection for DB + queue atomicity

## Stage 3 Architecture Summary

Current request flow:

1. Client calls `POST /v1/notifications`
2. API validates tenant, template, and recipient fields
3. API persists the notification row in PostgreSQL
4. API inserts the initial `delivery_attempts` row with `pending`
5. API enqueues a small JSON dispatch job onto `notify:dispatch`
6. `cmd/dispatcher` blocks on `notify:dispatch`
7. Dispatcher republishes the same job to either `notify:dispatch:webhook` or `notify:dispatch:email`

Important honesty notes:

- The API does **not** push directly to channel queues in this stage.
- No worker consumes the channel-specific queues yet.
- No delivery execution occurs yet.
- The design is intentionally simple and demoable for now.
- PostgreSQL writes and Redis enqueue are **not** yet coordinated with an outbox pattern, so DB/queue atomicity is not yet hardened.

## Current Endpoints

- `GET /v1/health`
- `GET /v1/readiness`
- `POST /v1/tenants`
- `POST /v1/templates`
- `POST /v1/notifications`

### `POST /v1/tenants`

Creates a tenant record.

Required fields:

- `id`
- `name`
- `daily_quota`

### `POST /v1/templates`

Creates a template for an existing tenant.

Required fields:

- `id`
- `tenant_id`
- `name`
- `channel`
- `version`
- `body`

Supported channels:

- `email`
- `webhook`

### `POST /v1/notifications`

Creates a notification submission for an existing tenant and template.

Required fields:

- `id`
- `tenant_id`
- `template_id`

Optional fields:

- `idempotency_key`
- `recipient_email`
- `recipient_webhook_url`
- `variables`

The required recipient field depends on the template channel:

- email templates require `recipient_email`
- webhook templates require `recipient_webhook_url`

Submission now also creates the first delivery attempt and enqueues one generic dispatch job.

## Current Queue Design

Redis list queues used in Stage 3:

- generic dispatch queue:
  - `notify:dispatch`
- channel queues:
  - `notify:dispatch:webhook`
  - `notify:dispatch:email`

Dispatch job envelope fields:

- `job_id`
- `notification_id`
- `attempt_id`
- `tenant_id`
- `channel`
- `created_at`

The job is intentionally small so later workers can load full records from PostgreSQL.

## Local Development

Start local infrastructure:

```bash
make dev-up
```

Apply the database migration:

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

Default local configuration:

- HTTP port: `8080`
- PostgreSQL: `postgres://notification:notification@localhost:5432/notification_platform?sslmode=disable`
- Redis address: `localhost:6379`
- Redis password: empty
- Redis DB: `0`
- OTLP endpoint: `localhost:4317`

Useful local endpoints:

- API health: `http://localhost:8080/v1/health`
- API readiness: `http://localhost:8080/v1/readiness`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000`
- Jaeger: `http://localhost:16686`

Run checks:

```bash
make fmt
make lint
make test
```

### Example: Create Tenant

```bash
curl -X POST http://localhost:8080/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "id": "acme",
    "name": "Acme Corp",
    "daily_quota": 1000
  }'
```

### Example: Create Template

```bash
curl -X POST http://localhost:8080/v1/templates \
  -H "Content-Type: application/json" \
  -d '{
    "id": "tpl_password_reset_webhook_v1",
    "tenant_id": "acme",
    "name": "password-reset",
    "channel": "webhook",
    "version": 1,
    "body": "{\"event\":\"password_reset\",\"url\":\"{{.reset_url}}\"}"
  }'
```

### Example: Submit Notification

```bash
curl -X POST http://localhost:8080/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "id": "notif_001",
    "tenant_id": "acme",
    "template_id": "tpl_password_reset_webhook_v1",
    "idempotency_key": "idem-123",
    "recipient_webhook_url": "https://example.com/hooks/password-reset",
    "variables": {
      "reset_url": "https://example.com/reset/abc"
    }
  }'
```

After submission you should be able to observe:

- a row in `notifications`
- an initial row in `delivery_attempts` with `attempt_number = 1` and `status = pending`
- a JSON job pushed to `notify:dispatch`
- the dispatcher moving that job to `notify:dispatch:webhook`

## Database Migration

The active schema is in:

- `migrations/001_init.sql`

Reset and reapply locally:

```bash
make migrate-reset
```

The current service actively uses `tenants`, `templates`, `notifications`, and `delivery_attempts`. Other schema objects remain reserved for later milestones.

## Current Limitations

This service currently accepts notification requests, stores them, creates an initial delivery attempt, and routes dispatch jobs. It still does not perform delivery.

There is no:

- webhook execution
- email sending
- worker execution on channel queues
- retry handling
- dead-letter processing
- tenant authentication or authorization hardening
- replay API
- usage reporting
- transactional outbox for DB + queue consistency

## Next Planned Work

The next milestone should focus on adding workers that consume the channel-specific queues and execute delivery while preserving the intentionally small job envelope introduced here.
