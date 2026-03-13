# ADR 0002: Enforce Idempotent Submission with Tenant-Scoped Keys

## Status
Accepted

## Context
Clients may retry requests due to timeouts, network interruptions, or uncertainty about whether a previous request succeeded.

Without idempotency, retries could cause duplicate notifications and duplicate downstream sends.

## Decision
The platform will support an idempotency key on notification submission. Idempotency keys are tenant-scoped. Reuse of the same key for the same tenant returns the previously accepted notification rather than creating a new one.

## Consequences
### Positive
- safer client retry behavior
- duplicate sends reduced significantly
- cleaner API contract

### Negative
- requires storage and lookup logic
- request semantics must be documented clearly
- conflicting reuse patterns must be handled predictably
