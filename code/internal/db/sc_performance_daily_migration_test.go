package db

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestSCPerformanceDailyMigrationCopiesRawColumnsAndAddsDateKey(t *testing.T) {
	oldSQL, err := os.ReadFile("../../migrations/012_add_ls_sc_performance.sql")
	if err != nil {
		t.Fatal(err)
	}
	dailySQL, err := os.ReadFile("../../migrations/034_add_ls_sc_performance_daily.sql")
	if err != nil {
		t.Fatal(err)
	}
	column := regexp.MustCompile(`^    ([A-Za-z][A-Za-z0-9_]*)\s+`)
	for _, line := range strings.Split(string(oldSQL), "\n") {
		match := column.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		name := match[1]
		if name == "PRIMARY" || name == "INDEX" || name == "synced_at" {
			continue
		}
		if !strings.Contains(string(dailySQL), line) {
			t.Fatalf("daily migration missing or changed 012 column declaration: %s", line)
		}
	}
	for _, want := range []string{"CREATE TABLE IF NOT EXISTS ls_sc_performance_daily", "business_date DATE NOT NULL", "PRIMARY KEY (account_id, sid, asin, business_date)"} {
		if !strings.Contains(string(dailySQL), want) {
			t.Fatalf("daily migration missing %q", want)
		}
	}
}
