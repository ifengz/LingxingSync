package worker

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/db"
)

func TestRunCleanupLogsOneSuccessSummary(t *testing.T) {
	oldWriter, oldFlags, oldPrefix := log.Writer(), log.Flags(), log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	})
	var output bytes.Buffer
	log.SetOutput(&output)
	log.SetFlags(0)
	log.SetPrefix("")

	calls := 0
	s := &Scheduler{
		cfg: &config.Config{Retention: config.Retention{TaskLogsDays: 90, TasksDays: 365}},
		dbx: &sqlx.DB{},
		cleanupOld: func(_ *sqlx.DB, taskLogsDays, tasksDays int) (db.CleanupResult, error) {
			calls++
			if taskLogsDays != 90 || tasksDays != 365 {
				t.Fatalf("retention args = %d/%d", taskLogsDays, tasksDays)
			}
			return db.CleanupResult{
				TaskLogsDeleted: 1234,
				TaskLogsBatches: 2,
				TasksDeleted:    56,
				TasksBatches:    1,
			}, nil
		},
	}

	s.runCleanup(context.Background())

	if calls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", calls)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("success log lines = %d: %q", len(lines), output.String())
	}
	for _, want := range []string{
		"CleanupOld 完成",
		"task_logs_deleted=1234",
		"task_logs_batches=2",
		"tasks_deleted=56",
		"tasks_batches=1",
	} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("success summary missing %q: %s", want, lines[0])
		}
	}
}

func TestRunCleanupUsesDefaultCleanerWhenNotInjected(t *testing.T) {
	oldWriter, oldFlags, oldPrefix := log.Writer(), log.Flags(), log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	})
	var output bytes.Buffer
	log.SetOutput(&output)
	log.SetFlags(0)
	log.SetPrefix("")

	s := &Scheduler{
		cfg: &config.Config{Retention: config.Retention{TaskLogsDays: 0, TasksDays: 365}},
		dbx: &sqlx.DB{},
	}
	s.runCleanup(context.Background())

	if !strings.Contains(output.String(), "taskLogsDays 必须 > 0") {
		t.Fatalf("default cleaner was not called: %s", output.String())
	}
}
