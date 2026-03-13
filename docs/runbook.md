# Runbook

## Local Startup

1. Start dependencies using Docker Compose
2. Run database migrations
3. Start the API service
4. Start dispatcher
5. Start workers
5. Verify health and readiness endpoints

## Smoke Test

1. Create a tenant
2. Create a template
3. Submit a notification
4. Confirm notification record exists
5. Confirm worker consumes job
6. Confirm delivery attempt status updates

## Common Failure Cases

### API returns readiness failure
Check database and queue connectivity.

### Notification accepted but no worker activity
Check dispatcher status, queue connectivity, and worker logs.

### Repeated dead-lettering
Inspect final_error, validate downstream endpoint behavior, and replay once root cause is resolved.

## Replay Procedure

1. Inspect dead-letter entry
2. Confirm root cause is fixed
3. Call replay endpoint
4. Monitor new delivery attempts and telemetry