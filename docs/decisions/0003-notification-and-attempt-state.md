
# ADR 0003: Separate Notification State from Delivery Attempt State

## Status
Accepted

## Context
A notification may target multiple delivery channels. One channel may succeed while another fails or retries.

A single flat status field is not expressive enough for operational debugging or accurate lifecycle tracking.

## Decision
The data model will distinguish between aggregate notification status and per-channel delivery attempt status.

## Consequences
### Positive
- clearer operational visibility
- better support for partial success cases
- easier replay and retry handling

### Negative
- more complex data model
- more complex state transition logic

