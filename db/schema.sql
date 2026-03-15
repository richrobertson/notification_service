CREATE TYPE tenant_status AS ENUM ('active', 'disabled');
CREATE TYPE notification_status AS ENUM (
    'accepted',
    'processing',
    'partially_delivered',
    'delivered',
    'failed',
    'dead_lettered'
);
CREATE TYPE delivery_attempt_status AS ENUM (
    'pending',
    'in_progress',
    'sent',
    'delivered',
    'failed',
    'retry_scheduled',
    'dead_lettered'
);
CREATE TYPE channel_type AS ENUM ('email', 'webhook');

CREATE TABLE tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    status tenant_status NOT NULL DEFAULT 'active',
    daily_quota INTEGER NOT NULL CHECK (daily_quota > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE api_keys (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX api_keys_active_key_hash_idx
    ON api_keys (key_hash)
    WHERE revoked_at IS NULL;

CREATE TABLE templates (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    channel channel_type NOT NULL,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX templates_tenant_name_channel_idx
    ON templates (tenant_id, name, channel);

CREATE TABLE notifications (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    template_id TEXT NOT NULL REFERENCES templates(id) ON DELETE RESTRICT,
    idempotency_key TEXT NOT NULL,
    status notification_status NOT NULL,
    channels channel_type[] NOT NULL,
    recipient JSONB NOT NULL,
    variables JSONB NOT NULL DEFAULT '{}'::jsonb,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX notifications_tenant_idempotency_idx
    ON notifications (tenant_id, idempotency_key);

CREATE INDEX notifications_tenant_submitted_at_idx
    ON notifications (tenant_id, submitted_at DESC);

CREATE INDEX notifications_status_idx
    ON notifications (status);

CREATE TABLE delivery_attempts (
    id UUID PRIMARY KEY,
    notification_id UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    channel channel_type NOT NULL,
    attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
    status delivery_attempt_status NOT NULL,
    error_code TEXT,
    next_retry_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (notification_id, channel, attempt_number)
);

CREATE INDEX delivery_attempts_notification_idx
    ON delivery_attempts (notification_id);

CREATE INDEX delivery_attempts_retry_idx
    ON delivery_attempts (status, next_retry_at)
    WHERE status = 'retry_scheduled';

CREATE TABLE dead_letters (
    id UUID PRIMARY KEY,
    notification_id UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    channel channel_type NOT NULL,
    final_error TEXT NOT NULL,
    dead_lettered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    replayed_at TIMESTAMPTZ
);

CREATE INDEX dead_letters_notification_idx
    ON dead_letters (notification_id);

CREATE INDEX dead_letters_open_idx
    ON dead_letters (dead_lettered_at DESC)
    WHERE replayed_at IS NULL;

CREATE TABLE audit_events (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_events_tenant_created_at_idx
    ON audit_events (tenant_id, created_at DESC);
