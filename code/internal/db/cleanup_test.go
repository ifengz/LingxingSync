package db

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"
)

type cleanupExecCall struct {
	query string
	args  []any
}

type cleanupExecFake struct {
	rows  []int64
	calls []cleanupExecCall
}

func (f *cleanupExecFake) Exec(query string, args ...any) (sql.Result, error) {
	f.calls = append(f.calls, cleanupExecCall{query: query, args: args})
	rows := f.rows[len(f.calls)-1]
	return cleanupSQLResult(rows), nil
}

type cleanupSQLResult int64

func (r cleanupSQLResult) LastInsertId() (int64, error) { return 0, nil }
func (r cleanupSQLResult) RowsAffected() (int64, error) { return int64(r), nil }

func TestDeleteOldRowsUsesBoundedBatches(t *testing.T) {
	exec := &cleanupExecFake{rows: []int64{cleanupBatchSize, 7}}

	deleted, batches, err := deleteOldRows(exec.Exec, cleanupTaskLogsSQL, 90)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != int64(cleanupBatchSize+7) || batches != 2 {
		t.Fatalf("cleanup result = deleted %d batches %d", deleted, batches)
	}
	if len(exec.calls) != 2 {
		t.Fatalf("delete calls = %d, want 2", len(exec.calls))
	}
	wantArgs := []any{90, cleanupBatchSize}
	for _, call := range exec.calls {
		if call.query != cleanupTaskLogsSQL || !reflect.DeepEqual(call.args, wantArgs) {
			t.Fatalf("delete call = query %q args %#v", call.query, call.args)
		}
	}
	if !strings.Contains(cleanupTaskLogsSQL, "ORDER BY created_at, id LIMIT ?") {
		t.Fatalf("task log cleanup is not ordered and bounded: %s", cleanupTaskLogsSQL)
	}
}

func TestDeleteOldRowsFailsLoudAtBatchLimit(t *testing.T) {
	rows := make([]int64, cleanupMaxBatches)
	for i := range rows {
		rows[i] = cleanupBatchSize
	}
	exec := &cleanupExecFake{rows: rows}

	deleted, batches, err := deleteOldRows(exec.Exec, cleanupTaskLogsSQL, 90)
	if err == nil || !strings.Contains(err.Error(), "批次上限") {
		t.Fatalf("batch limit error = %v", err)
	}
	if deleted != int64(cleanupBatchSize*cleanupMaxBatches) || batches != cleanupMaxBatches {
		t.Fatalf("capped cleanup result = deleted %d batches %d", deleted, batches)
	}
	if len(exec.calls) != cleanupMaxBatches {
		t.Fatalf("delete calls = %d, want cap %d", len(exec.calls), cleanupMaxBatches)
	}
}

func TestCleanupOldRejectsNonPositiveRetention(t *testing.T) {
	for _, days := range [][2]int{{0, 365}, {-1, 365}, {90, 0}, {90, -1}} {
		if _, err := CleanupOld(nil, days[0], days[1]); err == nil {
			t.Fatalf("CleanupOld accepted taskLogsDays=%d tasksDays=%d", days[0], days[1])
		}
	}
}

func TestCleanupTaskQueryOnlyDeletesTerminalRows(t *testing.T) {
	for _, want := range []string{"status IN ('success','empty','error','cancelled')", "ORDER BY created_at, id LIMIT ?"} {
		if !strings.Contains(cleanupTasksSQL, want) {
			t.Fatalf("task cleanup query missing %q: %s", want, cleanupTasksSQL)
		}
	}
}
