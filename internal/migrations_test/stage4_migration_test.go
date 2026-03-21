package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestStage4DeliveryAttemptsMigrationDefinesSafeUpdatedAtTrigger(t *testing.T) {
	t.Parallel()
	contents, err := os.ReadFile("../../migrations/002_stage4_delivery_attempts.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	sql := string(contents)
	for _, snippet := range []string{
		"CREATE OR REPLACE FUNCTION set_updated_at()",
		"DROP TRIGGER IF EXISTS delivery_attempts_set_updated_at ON delivery_attempts;",
		"CREATE TRIGGER delivery_attempts_set_updated_at",
	} {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("migration missing %q", snippet)
		}
	}
}
