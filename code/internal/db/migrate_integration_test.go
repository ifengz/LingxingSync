package db

import (
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// LINGXING_MIGRATION_TEST_DSN points at a disposable or already-migrated local
// database. It is intentionally opt-in so ordinary unit tests never mutate a
// developer database.
func TestRunMigrationsIsRepeatableAgainstExistingSchema(t *testing.T) {
	dsn := os.Getenv("LINGXING_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set LINGXING_MIGRATION_TEST_DSN to run the local migration integration test")
	}

	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatalf("connect migration test database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	for attempt := 1; attempt <= 2; attempt++ {
		if err := RunMigrations(db, "../../migrations"); err != nil {
			t.Fatalf("RunMigrations attempt %d: %v", attempt, err)
		}
	}
}
