BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
        WHERE enumtypid = 'notification_status'::regtype
          AND enumlabel = 'scheduled'
    ) THEN
        ALTER TYPE notification_status ADD VALUE 'scheduled';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
        WHERE enumtypid = 'notification_status'::regtype
          AND enumlabel = 'cancelled'
    ) THEN
        ALTER TYPE notification_status ADD VALUE 'cancelled';
    END IF;
END$$;

ALTER TABLE notifications
    ADD COLUMN IF NOT EXISTS secondary_webhook_url TEXT,
    ADD COLUMN IF NOT EXISTS scheduled_for TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS promoted_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE delivery_attempts
    ADD COLUMN IF NOT EXISTS provider_used TEXT,
    ADD COLUMN IF NOT EXISTS failover_used BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS delivery_policies (
    id TEXT PRIMARY KEY,
    tenant_id TEXT REFERENCES tenants(id) ON DELETE CASCADE,
    channel TEXT CHECK (channel IN ('email', 'webhook')),
    paused BOOLEAN,
    failover_enabled BOOLEAN,
    scheduling_enabled BOOLEAN,
    replay_allowed BOOLEAN,
    max_attempts_override INTEGER CHECK (max_attempts_override IS NULL OR max_attempts_override > 0),
    retry_base_delay_seconds INTEGER CHECK (retry_base_delay_seconds IS NULL OR retry_base_delay_seconds > 0),
    retry_max_delay_seconds INTEGER CHECK (retry_max_delay_seconds IS NULL OR retry_max_delay_seconds > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS delivery_policies_scope_idx
    ON delivery_policies (tenant_id, channel);

COMMIT;
