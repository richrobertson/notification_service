# ADR 0005: Use Tenant-Scoped API Keys for MVP Authentication

## Status
Accepted

## Context
The platform needs a simple and credible authentication mechanism without over-expanding into full user identity and OAuth flows during the MVP.

## Decision
The platform will authenticate requests using tenant-scoped API keys provided through an HTTP header. Keys will be stored hashed.

## Consequences
### Positive
- simple to implement
- realistic for service-to-service usage
- easy to explain in interviews

### Negative
- not as feature-rich as OAuth-based approaches
- key lifecycle management remains intentionally basic in MVP