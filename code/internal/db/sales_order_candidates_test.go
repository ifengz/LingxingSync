package db

import "testing"

func TestQueryRecentSalesOrderCandidatesRejectsInvalidWindowBeforeQuery(t *testing.T) {
	if _, err := QueryRecentSalesOrderCandidates(nil, "sc_us_1", "2026-08-11", "2026-08-10"); err == nil {
		t.Fatal("reversed candidate window must fail before querying the database")
	}
	if _, err := QueryRecentSalesOrderCandidates(nil, "sc_us_1", "not-a-date", "2026-08-10"); err == nil {
		t.Fatal("invalid candidate window must fail before querying the database")
	}
}
