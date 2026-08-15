package reportexport

import (
	"strings"
	"testing"
)

func TestParseFBAReimbursementsRequiresOfficialHeader(t *testing.T) {
	values := []string{"2026-08-14", "R-1", "C-1", "O-1", "DAMAGED", "SKU-1", "FNSKU-1", "ASIN-1", "Widget", "New", "USD", "2.00", "4.00", "1", "1", "2", "R-0", "INVENTORY"}
	data := strings.Join(reimbursementHeader, "\t") + "\t\n" + strings.Join(values, "\t") + "\t\n"
	rows, err := ParseFBAReimbursements([]byte(data), "", "")
	if err != nil {
		t.Fatalf("ParseFBAReimbursements returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].ReimbursementID != "R-1" || rows[0].QuantityReimbursedTotal != "2" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestParseFBAReimbursementsRejectsUnknownHeader(t *testing.T) {
	_, err := ParseFBAReimbursements([]byte("approval-date\treimbursement-id\n"), "", "")
	if err == nil || !strings.Contains(err.Error(), "reimbursements TSV header") {
		t.Fatalf("header error = %v", err)
	}
}
