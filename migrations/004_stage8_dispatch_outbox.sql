BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_type
        WHERE typname = 'dispatch_outbox_status'
    ) THEN
        CREATE TYPE dispatch_outbox_status AS ENUM ('pending', 'publishing', 'published');
    ELSIF NOT EXISTS (
        SELECT 1
        FROM pg_enum
        WHERE enumtypid = 'dispatch_outbox_status'::regtype
          AND enumlabel = 'publishing'
    ) THEN
        ALTER TYPE dispatch_outbox_status ADD VALUE 'publishing';
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS dispatch_outbox (
    id TEXT PRIMARY KEY,
    notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES delivery_attempts(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    channel TEXT NOT NULL CHECK (channel IN ('email', 'webhook')),
    source TEXT NOT NULL,
    status dispatch_outbox_status NOT NULL DEFAULT 'pending',
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS dispatch_outbox_attempt_id_idx
    ON dispatch_outbox (attempt_id);

CREATE INDEX IF NOT EXISTS dispatch_outbox_pending_idx
    ON dispatch_outbox (status, claimed_at, created_at)
    WHERE status IN ('pending', 'publishing');

INSERT INTO dispatch_outbox (id, notification_id, attempt_id, tenant_id, channel, source, status, claimed_at, published_at)
SELECT
    'intent-' || da.id,
    da.notification_id,
    da.id,
    n.tenant_id,
    da.channel,
    da.enqueue_kind,
    CASE
        WHEN da.dispatch_enqueued_at IS NULL THEN 'pending'::dispatch_outbox_status
        ELSE 'published'::dispatch_outbox_status
    END,
    NULL,
    da.dispatch_enqueued_at
FROM delivery_attempts da
JOIN notifications n ON n.id = da.notification_id
WHERE da.enqueue_kind IN ('initial', 'retry', 'replay')
ON CONFLICT (attempt_id) DO NOTHING;

COMMIT;
