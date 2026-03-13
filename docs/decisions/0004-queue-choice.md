# ADR 0004: Start with a Simple Queue Technology for MVP

## Status
Accepted

## Context
The MVP needs asynchronous delivery, retries, and local developer ergonomics. The project should remain practical to complete while still showing strong architecture.

## Decision
The initial implementation will use Redis streams or RabbitMQ as the queueing mechanism. The exact choice can be finalized based on implementation speed and local development preference.

## Consequences
### Positive
- faster delivery of MVP
- manageable operational complexity
- easy local development

### Negative
- may not represent the final scaling choice of a large production platform
- migration to a different broker later may require abstraction or refactoring
