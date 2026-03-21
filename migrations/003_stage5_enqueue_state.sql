BEGIN;

ALTER TABLE delivery_attempts
    ADD COLUMN IF NOT EXISTS dispatch_enqueued_at TIMESTAMPTZ NULL;

ALTER TABLE dead_letters
    ADD COLUMN IF NOT EXISTS replay_attempt_id TEXT NULL;

CREATE INDEX IF NOT EXISTS delivery_attempts_dispatch_pending_idx
    ON delivery_attempts (status, created_at)
    WHERE dispatch_enqueued_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS dead_letters_replay_attempt_id_idx
    ON dead_letters (replay_attempt_id)
    WHERE replay_attempt_id IS NOT NULL;

COMMIT;
