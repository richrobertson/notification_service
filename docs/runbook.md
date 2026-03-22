# Runbook

## Local Startup

1. Start local PostgreSQL and Redis.
2. Run database migrations.
3. Set `ADMIN_TOKEN` if you are not using the local default.
4. Start the API service.
5. Start the dispatcher.
6. Start the outbox publisher.
7. Start the channel workers.
8. Start the retry worker.
9. Start the scheduler/promoter.
10. Verify `/healthz`, `/readyz`, and `/metrics`.
11. Run `cmd/maintenance` in dry-run mode if you want to confirm retention wiring.

## Smoke Test

1. Create a tenant.
2. Create an email or webhook template.
3. Submit an immediate notification.
4. Confirm the notification, initial attempt, and dispatch intent exist.
5. Confirm the outbox publisher publishes the dispatch intent.
6. Confirm the dispatcher and channel worker consume the job.
7. Confirm the attempt reaches a terminal state or a retry state as expected.
8. Submit a future scheduled notification and confirm it stays `scheduled` until due.
9. Confirm the scheduler promotes it through the same outbox path later.

## Common Failure Cases

### API returns readiness failure

Check PostgreSQL and Redis connectivity. `/readyz` reports both dependencies explicitly.

### Notification accepted but no worker activity

Check outbox publisher status, dispatcher status, Redis queue depth, worker logs, and `/metrics`.

### Dispatch intent remains pending

Check outbox publisher logs, Redis availability, any `last_error` value on the outbox row, and `/metrics` for outbox lag/backlog.

### Retry backlog grows or retries look stuck

Check `/metrics` for `due_retry_count`, queue saturation counters, and open dead-letter counts.
Inspect retry worker logs and confirm queue soft/hard limits are not holding the system in a prolonged degraded state.

### Scheduled notification never promotes

Check `scheduled_for`, scheduler logs, `/metrics` scheduled lag, and whether delivery policy pause state is blocking promotion.

### Paused work does not resume

Inspect the relevant delivery policy scope, confirm the pause flag was cleared, then re-check `/metrics` for scheduled lag or outbox backlog in that tenant/channel scope.

### Backpressure rejects new work

Inspect `/metrics` queue depths and rejection counters.
Confirm whether the reject path is tenant rate limiting, queue soft pressure, or queue hard limit protection.

### Repeated dead-lettering

Inspect `final_error`, validate downstream endpoint or SMTP behavior, review failover settings, and replay only after the root cause is fixed.

## Replay Procedure

1. Inspect the dead-letter record.
2. Confirm the underlying issue is resolved.
3. Call the replay endpoint.
4. Confirm a replay attempt and replay dispatch intent are created.
5. Monitor the new attempt and audit trail.

## Scheduled Cancellation

1. Inspect the notification and confirm it is still future scheduled.
2. Call the cancellation endpoint before promotion.
3. Confirm the notification moves to `cancelled`.
4. Confirm no dispatch intent was published for that notification.

## Policy Pause / Resume

1. Inspect the policy scope you intend to change.
2. Pause the policy.
3. Confirm scheduled promotion or outbox publication stops for that scope.
4. Resume the policy.
5. Confirm due work starts flowing again.

## Admin Access

Use either of these headers for operator routes and metrics:

- `Authorization: Bearer <ADMIN_TOKEN>`
- `X-Admin-Token: <ADMIN_TOKEN>`

Create-notification traffic remains on the existing application path; inspection, policy, dead-letter, cancel, redrive, and metrics routes are the primary operator-protected surfaces.

## Cleanup

`cmd/maintenance` is the explicit data-retention job.

- Audit events older than `MAINTENANCE_AUDIT_RETENTION` are eligible for deletion.
- Published outbox rows older than `MAINTENANCE_OUTBOX_RETENTION` are eligible for deletion.
- Replayed dead letters older than `MAINTENANCE_DEAD_LETTER_RETENTION` are only eligible when that retention is set above `0`.
- `MAINTENANCE_DRY_RUN=true` reports what would be deleted without removing rows.
