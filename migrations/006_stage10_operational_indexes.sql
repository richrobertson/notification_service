BEGIN;

CREATE INDEX IF NOT EXISTS dispatch_outbox_status_created_at_idx
    ON dispatch_outbox (status, created_at);

CREATE INDEX IF NOT EXISTS dead_letters_replayed_at_idx
    ON dead_letters (replayed_at)
    WHERE replayed_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS audit_events_action_created_at_idx
    ON audit_events (action, created_at DESC);

COMMIT;
