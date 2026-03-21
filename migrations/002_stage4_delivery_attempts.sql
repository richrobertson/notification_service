BEGIN;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

ALTER TABLE delivery_attempts
    ADD COLUMN IF NOT EXISTS provider_message_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS last_error TEXT NULL,
    ADD COLUMN IF NOT EXISTS sent_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS failed_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

DROP TRIGGER IF EXISTS delivery_attempts_set_updated_at ON delivery_attempts;

CREATE TRIGGER delivery_attempts_set_updated_at
BEFORE UPDATE ON delivery_attempts
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

COMMIT;
