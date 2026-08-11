package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalConfigYAML = `database:
  host: 127.0.0.1
  user: test
  db: lingsync
accounts:
  - id: sc_us
    name: US
    app_key: key
    app_secret: secret
`

func loadRetentionConfig(t *testing.T, retentionYAML string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(minimalConfigYAML+retentionYAML), 0600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestLoadDefaultsMissingRetention(t *testing.T) {
	cfg, err := loadRetentionConfig(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Retention.TaskLogsDays != 90 || cfg.Retention.TasksDays != 365 || cfg.Retention.CleanupCron != "0 3 * * *" {
		t.Fatalf("retention defaults = %+v, want 90/365 and 03:00 cron", cfg.Retention)
	}
}

func TestLoadDefaultsEmptyCleanupCron(t *testing.T) {
	cfg, err := loadRetentionConfig(t, "\nretention:\n  task_logs_days: 90\n  tasks_days: 365\n  cleanup_cron: \"\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Retention.CleanupCron != "0 3 * * *" {
		t.Fatalf("empty cleanup cron = %q, want 03:00 default", cfg.Retention.CleanupCron)
	}
}

func TestLoadRejectsExplicitNonPositiveRetention(t *testing.T) {
	tests := []struct {
		name      string
		retention string
		field     string
	}{
		{name: "zero task logs", retention: "\nretention:\n  task_logs_days: 0\n  tasks_days: 365\n", field: "task_logs_days"},
		{name: "negative task logs", retention: "\nretention:\n  task_logs_days: -1\n  tasks_days: 365\n", field: "task_logs_days"},
		{name: "zero tasks", retention: "\nretention:\n  task_logs_days: 90\n  tasks_days: 0\n", field: "tasks_days"},
		{name: "negative tasks", retention: "\nretention:\n  task_logs_days: 90\n  tasks_days: -1\n", field: "tasks_days"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadRetentionConfig(t, tc.retention)
			if err == nil {
				t.Fatalf("explicit non-positive %s was accepted", tc.field)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("error %q does not identify %s", err, tc.field)
			}
		})
	}
}
