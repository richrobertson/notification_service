# Runbook

## Local Startup

1. Start local PostgreSQL and Redis.
2. Run database migrations.
3. Start the API service.
4. Start the dispatcher.
5. Start the outbox publisher.
6. Start the channel workers.
7. Start the retry worker.
8. Start the scheduler/promoter.
9. Verify health and readiness endpoints.

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

Check PostgreSQL and Redis connectivity.

### Notification accepted but no worker activity

Check outbox publisher status, dispatcher status, Redis queue depth, and worker logs.

### Dispatch intent remains pending

Check outbox publisher logs, Redis availability, and any `last_error` value on the outbox row.

### Scheduled notification never promotes

Check `scheduled_for`, scheduler logs, and whether delivery policy pause state is blocking promotion.

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
