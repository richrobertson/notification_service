# Architecture

## Purpose

The Multi-Tenant Notification Platform provides a reusable backend service for sending notifications across channels with strong reliability, tenant isolation, and operational visibility.

The MVP supports:
- email delivery
- webhook delivery
- asynchronous processing
- idempotent submission
- retries with exponential backoff
- dead-letter handling
- observability through logs, metrics, and traces

## Problem Statement

Many teams need to send transactional notifications, but implementing channel delivery, retries, idempotency, quotas, and observability independently leads to duplicated effort and inconsistent reliability.

This platform centralizes those concerns behind a tenant-scoped API and an asynchronous processing model.

## Goals

- Expose a clean tenant-scoped API for notification submission
- Keep submission latency low through asynchronous processing
- Prevent duplicate sends through idempotency
- Isolate tenants operationally and logically
- Handle transient failures through bounded retries
- Surface terminal failures through dead-letter processing
- Make the full flow easy to debug and operate

## Non-Goals

- Rich administrative UI in the MVP
- Marketing campaign management
- Complex scheduling features in the MVP
- Full identity and user management
- Exactly-once delivery guarantees
- Broad provider support beyond initial channels

## Functional Requirements

- Create tenants
- Create and update templates
- Submit notification requests
- Deliver notifications across supported channels
- Track notification and delivery attempt status
- Replay dead-lettered notifications
- Inspect usage and dead-letter records

## Non-Functional Requirements

- Low-latency synchronous submission path
- At-least-once asynchronous processing
- Tenant isolation
- Idempotent request handling
- Recoverable failure handling
- Observable system behavior
- Secure secret and API key handling

## System Context

### Actors

- Product team or service submitting notifications
- Platform operator monitoring health and failures
- Channel worker integrating with downstream delivery providers

### External Dependencies

- PostgreSQL for persistence
- Redis or RabbitMQ for asynchronous delivery jobs
- Email provider or stub transport for MVP
- Webhook endpoints owned by downstream systems
- OpenTelemetry Collector and metrics/tracing backends

## High-Level Architecture

### 1. API Service

Responsibilities:
- authenticate incoming requests
- validate tenant access
- validate request shape
- persist notification intent
- enforce idempotency
- expose read and operational endpoints

### 2. Dispatcher

Responsibilities:
- consume newly accepted notifications
- transform a notification into channel-specific delivery jobs
- enqueue work for channel workers
- update lifecycle state

### 3. Channel Workers

Responsibilities:
- render templates with request variables
- perform channel delivery
- classify transient vs terminal failures
- schedule retries when appropriate
- write delivery results
- emit telemetry

MVP workers:
- email worker
- webhook worker

### 4. Quota and Rate-Limit Layer

Responsibilities:
- enforce daily tenant quota
- enforce submission rate limits
- reject or throttle abusive traffic

### 5. Dead-Letter Processor

Responsibilities:
- capture terminally failed jobs
- expose dead-letter records for inspection
- support replay flows

## Request Lifecycle

1. Client sends `POST /v1/notifications`
2. API authenticates the request using tenant-scoped API key
3. API validates tenant, template, and recipient payload
4. API checks idempotency key
5. API persists notification intent and initial status
6. Dispatcher creates one job per channel
7. Channel workers process queued jobs
8. Workers update attempt state and notification aggregate state
9. Failed jobs are retried with exponential backoff when eligible
10. Exhausted jobs are written to dead-letter storage
11. Operators may replay eligible dead-lettered jobs

## Why the Architecture Looks This Way

### Low-latency submission

The client-facing API should not block on external provider delivery because provider latency and intermittent failures are normal. Persisting intent and processing asynchronously reduces client-visible latency and isolates downstream instability.

### Channel isolation

Each channel has different delivery semantics and failure patterns. Separate workers keep that logic isolated and allow future scaling per channel.

### Explicit delivery attempts

Notification lifecycle and delivery attempt lifecycle are separate because a single notification may fan out into multiple channels with independent outcomes.

## Data Model

### tenants
- id
- name
- status
- daily_quota
- created_at

### api_keys
- id
- tenant_id
- key_hash
- created_at
- revoked_at

### templates
- id
- tenant_id
- name
- channel
- version
- body
- created_at

### notifications
- id
- tenant_id
- template_id
- idempotency_key
- status
- submitted_at

### delivery_attempts
- id
- notification_id
- channel
- attempt_number
- status
- error_code
- next_retry_at
- completed_at

### dead_letters
- id
- notification_id
- channel
- final_error
- dead_lettered_at

### audit_events
- id
- tenant_id
- actor
- action
- resource_type
- resource_id
- created_at

## Status Model

### Notification Status
- accepted
- processing
- partially_delivered
- delivered
- failed
- dead_lettered

### Delivery Attempt Status
- pending
- in_progress
- sent
- delivered
- failed
- retry_scheduled
- dead_lettered

## API Boundaries

### Tenant-scoped endpoints
- create tenant
- get tenant
- create template
- get template
- update template
- submit notification
- get notification
- get tenant usage

### Operational endpoints
- list dead letters
- replay dead-lettered notification
- health
- readiness

## Security Model

- API keys are scoped to tenants
- API keys are stored as hashes, not plaintext
- all reads and writes are tenant-filtered
- operational changes generate audit records
- secrets are injected through configuration, not committed to source control

## Reliability Model

### Idempotency

Clients may safely retry notification submission without creating duplicate downstream sends when the same tenant and idempotency key are reused.

### Retry policy

Transient failures are retried using bounded exponential backoff. Terminal failures or exhausted retries result in dead-lettering.

### Dead-letter handling

Dead letters are first-class operational artifacts. They are retained for inspection and may be replayed when the underlying issue is resolved.

## Observability Model

The platform emits:
- structured logs
- request and worker metrics
- distributed traces

Important telemetry dimensions:
- tenant_id
- channel
- operation
- status
- error_code

Suggested dashboards:
- notification submission rate
- success/failure rate by channel
- retry count by channel
- dead-letter count
- p95 API latency
- worker processing latency
- queue depth

## Scalability

### Horizontal scaling

- API service scales independently from workers
- each worker type can scale independently based on backlog or utilization
- queue decoupling reduces tight coupling between ingestion and delivery

### Future scaling opportunities

- separate queue per channel
- partitioning by tenant or notification class
- dedicated usage aggregation pipeline
- provider-specific worker pools

## Failure Scenarios

### Duplicate client submission
Handled through idempotency key lookup and reuse of existing notification record.

### Email provider timeout
Classified as transient failure and retried with exponential backoff.

### Invalid webhook URL
Classified as terminal failure and dead-lettered after validation or first failure depending on policy.

### Database unavailable
Readiness fails and write operations return failure rather than accepting work that cannot be persisted.

### Queue unavailable
Submission should fail fast or stop dispatching based on configuration, but the system must not silently drop accepted notifications.

## Deployment Model

### Local development
- Docker Compose for Postgres, queue, and observability stack
- API, dispatcher, and workers running locally or in Compose

### Production-style deployment
- containerized services
- Kubernetes Deployments for API and workers
- readiness and liveness probes
- autoscaling for worker deployments
- environment-based configuration

## Tradeoffs

### Why REST first instead of gRPC?
REST is easier to consume broadly, easier to demonstrate, and aligns with externally facing platform API patterns.

### Why email and webhook first?
They demonstrate the important architecture without ballooning provider integration complexity.

### Why not exactly-once delivery?
Exactly-once semantics introduce significant complexity across distributed boundaries. At-least-once delivery with idempotent submission is the practical MVP choice.

## Future Work

- SMS channel
- push/in-app channel
- scheduled delivery
- admin UI
- Terraform provisioning
- Kubernetes autoscaling policies
- richer usage analytics
- per-tenant retry policies
- webhook signing
