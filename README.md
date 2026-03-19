# notification_service

## Project Overview

`notification_service` is a runnable Go foundation for a multi-tenant notification platform, now with its first synchronous database-backed API path.

The service currently provides:

- health and readiness endpoints
- tenant creation
- template creation
- notification submission
- PostgreSQL-backed persistence using `database/sql`
- OpenTelemetry bootstrap wiring for local development
- Docker Compose infrastructure for Postgres, Redis, Prometheus, Grafana, Jaeger, and the OpenTelemetry Collector

This milestone is intentionally narrow. It focuses on synchronous request handling only and stops before async delivery infrastructure.

## Current Implementation Status

Implemented today:

- `GET /v1/health`
- `GET /v1/readiness`
- `POST /v1/tenants`
- `POST /v1/templates`
- `POST /v1/notifications`
- PostgreSQL persistence using the schema in `migrations/001_init.sql`
- request logging and panic recovery middleware
- idempotent notification submission when `idempotency_key` is provided

Not implemented yet:

- async dispatch
- workers
- retries
- DLQ handling
- auth hardening
- replay flow
- usage endpoints
- dead-letter inspection endpoints
- delivery attempt processing

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

Default local configuration:

- HTTP port: `8080`
- PostgreSQL: `postgres://notification:notification@localhost:5432/notification_platform?sslmode=disable`
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
    "id": "tpl_password_reset_email_v1",
    "tenant_id": "acme",
    "name": "password-reset",
    "channel": "email",
    "version": 1,
    "body": "Hello {{.first_name}}, reset here: {{.reset_url}}"
  }'
```

### Example: Submit Notification

```bash
curl -X POST http://localhost:8080/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "id": "notif_001",
    "tenant_id": "acme",
    "template_id": "tpl_password_reset_email_v1",
    "idempotency_key": "idem-123",
    "recipient_email": "user@example.com",
    "variables": {
      "first_name": "Sam",
      "reset_url": "https://example.com/reset/abc"
    }
  }'
```

## Database Migration

The active schema is in:

- `migrations/001_init.sql`

Reset and reapply locally:

```bash
make migrate-reset
```

The current service uses the existing `tenants`, `templates`, and `notifications` tables, while the rest of the schema is reserved for later milestones.

## Current Limitations

This service currently accepts notification requests and stores them, but it does not deliver them yet.

There is no:

- background dispatch
- channel worker execution
- retry handling
- dead-letter processing
- tenant authentication or authorization
- replay API
- usage reporting

## Next Planned Work

The next milestone should focus on turning stored notifications into delivered work:

1. add delivery attempt creation
2. introduce dispatcher and worker binaries
3. connect synchronous submissions to asynchronous processing
4. add retry and DLQ handling
5. harden auth and tenant-scoped access control
