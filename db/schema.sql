CREATE TYPE tenant_status AS ENUM ('active', 'disabled');
CREATE TYPE notification_status AS ENUM (
    'accepted',
    'scheduled',
    'processing',
    'partially_delivered',
    'delivered',
    'failed',
    'dead_lettered',
    'cancelled'
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
CREATE TYPE dispatch_outbox_status AS ENUM ('pending', 'publishing', 'published');

CREATE TABLE tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    status tenant_status NOT NULL DEFAULT 'active',
    daily_quota INTEGER NOT NULL CHECK (daily_quota > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE api_keys (
    id TEXT PRIMARY KEY,
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
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    template_id TEXT NOT NULL REFERENCES templates(id) ON DELETE RESTRICT,
    idempotency_key TEXT NOT NULL,
    status notification_status NOT NULL,
    recipient_email TEXT,
    recipient_webhook_url TEXT,
    secondary_webhook_url TEXT,
    variables JSONB NOT NULL DEFAULT '{}'::jsonb,
    scheduled_for TIMESTAMPTZ,
    promoted_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX notifications_tenant_idempotency_idx
    ON notifications (tenant_id, idempotency_key);

CREATE INDEX notifications_tenant_submitted_at_idx
    ON notifications (tenant_id, submitted_at DESC);

CREATE INDEX notifications_status_idx
    ON notifications (status);

CREATE INDEX notifications_scheduled_pending_idx
    ON notifications (scheduled_for)
    WHERE scheduled_for IS NOT NULL AND promoted_at IS NULL AND cancelled_at IS NULL;

CREATE TABLE delivery_attempts (
    id TEXT PRIMARY KEY,
    notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    channel channel_type NOT NULL,
    attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
    status delivery_attempt_status NOT NULL,
    error_code TEXT,
    error_message TEXT,
    provider_message_id TEXT,
    last_error TEXT,
    next_retry_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    dispatch_enqueued_at TIMESTAMPTZ,
    enqueue_kind TEXT NOT NULL DEFAULT 'initial',
    provider_used TEXT,
    failover_used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (notification_id, channel, attempt_number)
);

CREATE INDEX delivery_attempts_notification_idx
    ON delivery_attempts (notification_id);

CREATE INDEX delivery_attempts_retry_idx
    ON delivery_attempts (status, next_retry_at)
    WHERE status = 'retry_scheduled';

CREATE INDEX delivery_attempts_dispatch_pending_idx
    ON delivery_attempts (status, created_at)
    WHERE dispatch_enqueued_at IS NULL AND enqueue_kind IN ('initial', 'retry', 'replay');

CREATE TABLE dispatch_outbox (
    id TEXT PRIMARY KEY,
    notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES delivery_attempts(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    channel channel_type NOT NULL,
    source TEXT NOT NULL,
    status dispatch_outbox_status NOT NULL DEFAULT 'pending',
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    UNIQUE (attempt_id)
);

CREATE INDEX dispatch_outbox_pending_idx
    ON dispatch_outbox (status, claimed_at, created_at)
    WHERE status IN ('pending', 'publishing');

CREATE TABLE delivery_policies (
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

CREATE INDEX delivery_policies_scope_idx
    ON delivery_policies (tenant_id, channel);

CREATE INDEX delivery_policies_scope_updated_idx
    ON delivery_policies (tenant_id, channel, updated_at DESC);

CREATE TABLE dead_letters (
    id TEXT PRIMARY KEY,
    notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    channel channel_type NOT NULL,
    final_error TEXT NOT NULL,
    dead_lettered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    replayed_at TIMESTAMPTZ,
    replay_attempt_id TEXT
);

CREATE INDEX dead_letters_notification_idx
    ON dead_letters (notification_id);

CREATE INDEX dead_letters_open_idx
    ON dead_letters (dead_lettered_at DESC)
    WHERE replayed_at IS NULL;

CREATE UNIQUE INDEX dead_letters_replay_attempt_id_idx
    ON dead_letters (replay_attempt_id)
    WHERE replay_attempt_id IS NOT NULL;

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

CREATE INDEX audit_events_tenant_created_at_idx
    ON audit_events (tenant_id, created_at DESC);


CREATE FUNCTION set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

CREATE TRIGGER delivery_attempts_set_updated_at
BEFORE UPDATE ON delivery_attempts
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER notifications_set_updated_at
BEFORE UPDATE ON notifications
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
