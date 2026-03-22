package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestStage9DeliveryControlsMigrationDefinesSchedulerAndPolicyIndexes(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../../migrations/005_stage9_delivery_controls.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(contents)
	for _, snippet := range []string{
		"CREATE INDEX IF NOT EXISTS notifications_scheduled_pending_idx",
		"WHERE scheduled_for IS NOT NULL AND promoted_at IS NULL AND cancelled_at IS NULL",
		"CREATE INDEX IF NOT EXISTS delivery_policies_scope_updated_idx",
		"ON delivery_policies (tenant_id, channel, updated_at DESC)",
	} {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("migration missing %q", snippet)
		}
	}
}
