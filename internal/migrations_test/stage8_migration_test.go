package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestStage8DispatchOutboxMigrationDefinesPortableOutboxDDL(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../../migrations/004_stage8_dispatch_outbox.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(contents)
	for _, snippet := range []string{
		"CREATE TABLE IF NOT EXISTS dispatch_outbox",
		"notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE",
		"attempt_id TEXT NOT NULL REFERENCES delivery_attempts(id) ON DELETE CASCADE",
		"channel TEXT NOT NULL CHECK (channel IN ('email', 'webhook'))",
		"ALTER TYPE dispatch_outbox_status ADD VALUE 'publishing'",
		"INSERT INTO dispatch_outbox",
		"ON CONFLICT (attempt_id) DO NOTHING",
	} {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("migration missing %q", snippet)
		}
	}

	if strings.Contains(sql, "channel_type") {
		t.Fatal("migration should not depend on channel_type")
	}
}
