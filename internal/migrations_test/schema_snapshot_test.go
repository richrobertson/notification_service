package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaSnapshotDefinesNotificationsUpdatedAtTrigger(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../../db/schema.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(contents)
	for _, snippet := range []string{
		"CREATE TRIGGER notifications_set_updated_at",
		"BEFORE UPDATE ON notifications",
		"EXECUTE FUNCTION set_updated_at();",
	} {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("schema snapshot missing %q", snippet)
		}
	}
}
