package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestStage10OperationalIndexesMigrationDefinesMetricsIndexes(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../../migrations/006_stage10_operational_indexes.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(contents)
	for _, snippet := range []string{
		"CREATE INDEX IF NOT EXISTS dispatch_outbox_status_created_at_idx",
		"ON dispatch_outbox (status, created_at)",
		"CREATE INDEX IF NOT EXISTS dead_letters_replayed_at_idx",
		"WHERE replayed_at IS NOT NULL",
		"CREATE INDEX IF NOT EXISTS audit_events_action_created_at_idx",
		"ON audit_events (action, created_at DESC)",
	} {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("migration missing %q", snippet)
		}
	}
}
