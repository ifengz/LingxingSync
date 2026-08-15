package reportexport

import (
	"strings"
	"testing"
)

func TestParseAFNInventoryByCountryRequiresOfficialHeader(t *testing.T) {
	data := strings.Join(afnInventoryByCountryHeader, "\t") + "\nSKU-1\tFC-SKU-1\tASIN-1\tNew\tDE\t17\n"
	rows, err := ParseAFNInventoryByCountry([]byte(data), "", "")
	if err != nil {
		t.Fatalf("ParseAFNInventoryByCountry returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].Country != "DE" || rows[0].QuantityForLocalFulfillment != 17 {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestParseAFNInventoryByCountryRejectsUnknownHeader(t *testing.T) {
	_, err := ParseAFNInventoryByCountry([]byte("seller-sku\twrong\n"), "", "")
	if err == nil || !strings.Contains(err.Error(), "AFN inventory by country TSV header") {
		t.Fatalf("header error = %v", err)
	}
}
