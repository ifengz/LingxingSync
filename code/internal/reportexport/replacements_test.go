package reportexport

import (
	"strings"
	"testing"
)

func TestParseCustomerShipmentReplacementsRequiresOfficialHeaderAndQuantity(t *testing.T) {
	data := strings.Join([]string{
		strings.Join(replacementHeader, "\t") + "\t",
		"2026-08-14T12:00:00Z\tSKU-1\tASIN-1\tFC-1\tFC-2\t2\tDAMAGED\tORDER-2\tORDER-1\t",
	}, "\n") + "\n"
	rows, err := ParseCustomerShipmentReplacements([]byte(data), "", "")
	if err != nil {
		t.Fatalf("ParseCustomerShipmentReplacements returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].Quantity != 2 || rows[0].OriginalAmazonOrderID != "ORDER-1" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestParseCustomerShipmentReplacementsRejectsUnknownHeader(t *testing.T) {
	_, err := ParseCustomerShipmentReplacements([]byte("shipment-date\tsku\n"), "", "")
	if err == nil || !strings.Contains(err.Error(), "replacement TSV header") {
		t.Fatalf("header error = %v", err)
	}
}
