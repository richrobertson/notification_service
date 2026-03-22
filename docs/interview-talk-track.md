
# Interview Talk Track

## Problem
Teams often need a reusable, reliable way to send transactional notifications without each team rebuilding delivery, retries, observability, and quota controls.

## Constraints
The system needed low submission latency, tenant isolation, idempotent submission, asynchronous delivery, bounded retries, and strong debuggability.

## Design
I built a tenant-scoped REST API backed by persistent notification intent, asynchronous job dispatch, channel-specific workers, explicit retry and dead-letter handling, and end-to-end observability.

## Tradeoffs
I chose REST for broad usability and faster implementation, asynchronous processing to isolate downstream provider latency, and a simple queue technology for MVP speed. I limited the first version to email and webhook to keep the architecture deep without letting integrations sprawl.

## Reliability Story
The system supports idempotent submission, bounded exponential backoff, dead-letter replay, and a Postgres-backed outbox for durable dispatch publication, which makes failures visible and recoverable instead of hidden or silently dropped.

## Operational Story
I instrumented the request path and worker pipeline with structured logs, metrics, and traces so it is easy to debug issues like queue backlog, retry storms, scheduled promotion lag, paused policy scopes, or provider-specific failures.

## What I’d Add Next
I would add SMS, richer operator workflows, and deeper provider routing controls, but only after the current delivery policy and observability loop proved stable.
