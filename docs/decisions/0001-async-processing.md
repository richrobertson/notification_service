
# ADR 0001: Use Asynchronous Delivery Processing

## Status
Accepted

## Context
Notification delivery depends on external systems such as email providers and webhook endpoints. These dependencies may be slow or intermittently unavailable.

If the API waits synchronously for channel delivery, submission latency becomes unpredictable and downstream failures directly degrade the client experience.

## Decision
The API will persist notification intent and acknowledge the request before downstream delivery occurs. Delivery will be performed asynchronously by dispatcher and worker processes.

## Consequences
### Positive
- lower submission latency
- improved resilience to downstream provider instability
- independent scaling of API and workers
- clearer retry and dead-letter model

### Negative
- added queue and worker complexity
- eventual consistency in delivery state
- more moving parts for local development
