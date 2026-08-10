package db

import (
	"os"
	"strings"
	"testing"
)

func TestTaskLogRetentionIndexMigrationIsIdempotent(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/029_add_sync_task_logs_created_at_index.sql")
	if err != nil {
		t.Fatalf("read retention index migration: %v", err)
	}
	sqlText := strings.ToUpper(strings.Join(strings.Fields(string(raw)), " "))
	for _, want := range []string{
		"INFORMATION_SCHEMA.STATISTICS",
		"TABLE_NAME = 'SYNC_TASK_LOGS'",
		"COLUMN_NAME = 'CREATED_AT'",
		"SEQ_IN_INDEX = 1",
		"ALTER TABLE SYNC_TASK_LOGS ADD INDEX IDX_CREATED_AT (CREATED_AT)",
		"PREPARE",
		"EXECUTE",
		"DEALLOCATE PREPARE",
	} {
		if !strings.Contains(sqlText, want) {
			t.Fatalf("retention index migration missing %q", want)
		}
	}
	for _, destructive := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sqlText, destructive) {
			t.Fatalf("retention index migration contains destructive statement %q", destructive)
		}
	}
}
