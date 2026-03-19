BEGIN;

CREATE TABLE tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    daily_quota INTEGER NOT NULL CHECK (daily_quota > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT tenants_status_check CHECK (status IN ('active', 'suspended'))
);

CREATE TABLE api_keys (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ NULL,
    CONSTRAINT api_keys_tenant_key_hash_key UNIQUE (tenant_id, key_hash)
);

CREATE TABLE templates (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    channel TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT templates_channel_check CHECK (channel IN ('email', 'webhook')),
    CONSTRAINT templates_tenant_name_channel_version_key UNIQUE (tenant_id, name, channel, version)
);

CREATE TABLE notifications (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    template_id TEXT NOT NULL REFERENCES templates(id) ON DELETE RESTRICT,
    idempotency_key TEXT NULL,
    status TEXT NOT NULL,
    recipient_email TEXT NULL,
    recipient_webhook_url TEXT NULL,
    variables JSONB NOT NULL DEFAULT '{}'::jsonb,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notifications_status_check CHECK (
        status IN (
            'accepted',
            'processing',
            'partially_delivered',
            'delivered',
            'failed',
            'dead_lettered'
        )
    ),
    CONSTRAINT notifications_recipient_check CHECK (
        recipient_email IS NOT NULL OR recipient_webhook_url IS NOT NULL
    )
);

CREATE TABLE delivery_attempts (
    id TEXT PRIMARY KEY,
    notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,
    attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
    status TEXT NOT NULL,
    error_code TEXT NULL,
    error_message TEXT NULL,
    next_retry_at TIMESTAMPTZ NULL,
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT delivery_attempts_channel_check CHECK (channel IN ('email', 'webhook')),
    CONSTRAINT delivery_attempts_status_check CHECK (
        status IN (
            'pending',
            'in_progress',
            'sent',
            'delivered',
            'failed',
            'retry_scheduled',
            'dead_lettered'
        )
    ),
    CONSTRAINT delivery_attempts_notification_channel_attempt_key UNIQUE (notification_id, channel, attempt_number)
);

CREATE TABLE dead_letters (
    id TEXT PRIMARY KEY,
    notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,
    final_error TEXT NOT NULL,
    dead_lettered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    replayed_at TIMESTAMPTZ NULL,
    CONSTRAINT dead_letters_channel_check CHECK (channel IN ('email', 'webhook'))
);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX api_keys_tenant_id_idx
    ON api_keys (tenant_id);

CREATE INDEX api_keys_tenant_id_revoked_at_idx
    ON api_keys (tenant_id, revoked_at);

CREATE INDEX templates_tenant_id_idx
    ON templates (tenant_id);

CREATE INDEX templates_tenant_name_channel_idx
    ON templates (tenant_id, name, channel);

CREATE INDEX notifications_tenant_id_idx
    ON notifications (tenant_id);

CREATE INDEX notifications_template_id_idx
    ON notifications (template_id);

CREATE INDEX notifications_status_idx
    ON notifications (status);

CREATE INDEX notifications_submitted_at_desc_idx
    ON notifications (submitted_at DESC);

CREATE UNIQUE INDEX notifications_tenant_idempotency_key_active_idx
    ON notifications (tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX delivery_attempts_notification_id_idx
    ON delivery_attempts (notification_id);

CREATE INDEX delivery_attempts_channel_status_idx
    ON delivery_attempts (channel, status);

CREATE INDEX delivery_attempts_next_retry_at_idx
    ON delivery_attempts (next_retry_at)
    WHERE next_retry_at IS NOT NULL;

CREATE INDEX dead_letters_tenant_id_idx
    ON dead_letters (tenant_id);

CREATE INDEX dead_letters_notification_id_idx
    ON dead_letters (notification_id);

CREATE INDEX dead_letters_active_idx
    ON dead_letters (tenant_id, dead_lettered_at DESC)
    WHERE replayed_at IS NULL;

CREATE INDEX audit_events_tenant_id_idx
    ON audit_events (tenant_id);

CREATE INDEX audit_events_created_at_desc_idx
    ON audit_events (created_at DESC);

CREATE FUNCTION set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

CREATE TRIGGER notifications_set_updated_at
BEFORE UPDATE ON notifications
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

COMMIT;
