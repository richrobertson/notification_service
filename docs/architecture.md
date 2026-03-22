# Architecture

## Purpose

The notification platform provides a tenant-scoped backend for durable asynchronous delivery across email and webhook channels, with explicit retry, scheduling, failover, and operator control behavior.

## Current Capabilities

- email delivery
- webhook delivery
- asynchronous processing through Redis queues
- idempotent submission
- bounded retries and dead-letter handling
- Postgres-backed dispatch publication through a narrow outbox
- future scheduled delivery with durable promotion
- tenant/channel delivery policy controls
- audit, logs, metrics, and traces for operational visibility

## Goals

- keep the submission path fast and durable
- make PostgreSQL the source of truth for notification and dispatch state
- preserve at-least-once execution while suppressing duplicate work safely
- keep tenant behavior explicit through policy instead of hidden worker heuristics
- make failures inspectable, replayable, and operationally honest

## Non-Goals

- exactly-once delivery guarantees
- generalized workflow orchestration
- broad provider marketplaces or pluggable ecosystems
- complex campaign management
- rich administrative UI

## Core Components

### API service

Responsibilities:
- authenticate and validate requests
- enforce idempotency
- resolve delivery policy
- persist notification and attempt state
- expose inspection and operator endpoints

### Outbox publisher

Responsibilities:
- poll PostgreSQL for pending dispatch intents
- publish Redis `notify:dispatch` jobs
- mark intents published only after successful enqueue
- leave failed publication work recoverable in PostgreSQL

### Dispatcher

Responsibilities:
- consume generic dispatch jobs
- route them to channel-specific Redis queues
- keep Redis as execution transport rather than durable source of truth

### Channel workers

Responsibilities:
- reserve and process channel jobs
- render templates and call downstream providers
- classify retryable versus terminal failures
- apply narrow failover behavior when policy allows it
- finalize durable attempt state

### Retry worker

Responsibilities:
- poll PostgreSQL for due retry attempts
- create durable retry dispatch intents
- reuse the same outbox publication path as initial delivery

### Scheduler/promoter

Responsibilities:
- poll PostgreSQL for due scheduled notifications
- promote eligible work into the outbox path
- respect paused policy state before promotion

## Data Model

### notifications

- durable notification intent
- aggregate notification status
- scheduled delivery metadata through `scheduled_for`, `promoted_at`, and `cancelled_at`
- recipient details and template linkage

### delivery_attempts

- per-channel attempt records
- monotonic attempt lifecycle
- retry scheduling through `next_retry_at`
- provider metadata through `provider_message_id`, `provider_used`, and `failover_used`

### dispatch_outbox

- one durable dispatch intent per publishable attempt
- pending/publishing/published state
- recoverable publication failures through `last_error`

### delivery_policies

- tenant/channel scoped controls
- pause/resume state
- failover, scheduling, and replay gating
- retry override settings

### dead_letters

- durable record of exhausted attempts
- replay linkage and operator inspection support

### audit_events

- tenant-scoped lifecycle and operator actions
- duplicate suppression, replay, failover, and policy events where tenant-scoped audit is valid

## Request Lifecycle

1. Client sends `POST /v1/notifications`.
2. API authenticates, validates, and checks idempotency.
3. API resolves tenant/channel delivery policy.
4. API writes notification state and initial attempt state in PostgreSQL.
5. Immediate work gets a dispatch intent in `dispatch_outbox`; future scheduled work stays durable until due.
6. The scheduler promotes due scheduled work by creating the dispatch intent later.
7. The outbox publisher publishes Redis `notify:dispatch` jobs from pending intents.
8. The dispatcher routes generic jobs to `notify:dispatch:webhook` or `notify:dispatch:email`.
9. Channel workers process jobs, finalize attempt state, and schedule retry or dead-letter outcomes as needed.
10. Retry and replay flows create durable attempts plus dispatch intents before Redis publication.

## Reliability Model

- PostgreSQL is authoritative for notification, attempt, retry, scheduling, and dispatch-publication state.
- Redis is execution transport, not the source of truth for what still needs to be dispatched.
- Queueing remains at-least-once.
- Attempt activation and terminalization are guarded in SQL to suppress duplicate worker execution.
- Duplicate Redis jobs are tolerated underneath the Stage 6 attempt-level protections.

## Operator Controls

- inspect notifications and attempts
- inspect and replay dead letters
- cancel future scheduled notifications before promotion
- redrive eligible deferred work
- list, inspect, pause, resume, and update delivery policies

## Remaining Gaps

- no exactly-once semantics
- no multi-region active/active coordination
- no generalized event bus or CDC pipeline
- no full admin UI
- no broad provider routing marketplace
