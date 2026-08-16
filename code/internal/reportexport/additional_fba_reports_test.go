package reportexport

import (
	"slices"
	"strings"
	"testing"
)

var productionFBAEstimatedFeesHeader31 = []string{
	"sku", "fnsku", "asin", "product-name", "brand", "fulfilled-by", "amazon-store", "has-local-inventory", "your-price", "sales-price", "longest-side", "median-side", "shortest-side", "length-and-girth", "unit-of-dimension", "item-package-weight", "unit-of-weight", "product-size-tier", "currency", "estimated-fee-total", "estimated-referral-fee-per-unit", "estimated-variable-closing-fee", "estimated-order-handling-fee-per-order", "estimated-pick-pack-fee-per-unit", "estimated-weight-handling-fee-per-unit", "expected-fulfillment-fee-per-unit", "estimated-future-fee (Current Selling on Amazon + Future Fulfillment fees)", "estimated-future-order-handling-fee-per-order", "estimated-future-pick-pack-fee-per-unit", "estimated-future-weight-handling-fee-per-unit", "expected-future-fulfillment-fee-per-unit",
}

var expectedFBAEstimatedFeesCanonicalHeader40 = append(append([]string(nil), fbaEstimatedFeesHeader...),
	"amazon-store", "product-size-tier", "estimated-order-handling-fee-per-order", "estimated-pick-pack-fee-per-unit", "estimated-weight-handling-fee-per-unit", "expected-fulfillment-fee-per-unit", "estimated-future-fee", "estimated-future-order-handling-fee-per-order", "estimated-future-pick-pack-fee-per-unit", "estimated-future-weight-handling-fee-per-unit", "expected-future-fulfillment-fee-per-unit")

func TestParseAdditionalFBAReportsRequireOfficialHeaders(t *testing.T) {
	tests := []struct {
		name     string
		header   []string
		official string
		parse    func([]byte) (int, error)
	}{
		{"stranded", fbaStrandedInventoryHeader, "primary-action\tdate-stranded\tDate-to-take-auto-removal\tstatus-primary\tstatus-secondary\terror-message\tstranded-reason\tasin\tsku\tfnsku\tproduct-name\tcondition\tfulfilled-by\tfulfillable-qty\tyour-price\tunfulfillable-qty\treserved-quantity\tinbound-shipped-qty", func(data []byte) (int, error) {
			rows, err := ParseFBAStrandedInventory(data, "", "")
			return len(rows), err
		}},
		{"estimated fees", fbaEstimatedFeesHeader, "sku\tfnsku\tasin\tproduct-name\tproduct-group\tbrand\tfulfilled-by\thas-local-inventory\tyour-price\tsales-price\tlongest-side\tmedian-side\tshortest-side\tlength-and-girth\tunit-of-dimension\titem-package-weight\tunit-of-weight\tproduct-size-weight-band\tcurrency\testimated-fee-total\testimated-referral-fee-per-unit\testimated-variable-closing-fee\texpected-domestic-fulfilment-fee-per-unit\texpected-efn-fulfilment-fee-per-unit-uk\texpected-efn-fulfilment-fee-per-unit-de\texpected-efn-fulfilment-fee-per-unit-fr\texpected-efn-fulfilment-fee-per-unit-it\texpected-efn-fulfilment-fee-per-unit-es\texpected-efn-fulfilment-fee-per-unit-se", func(data []byte) (int, error) {
			rows, err := ParseFBAEstimatedFees(data, "", "")
			return len(rows), err
		}},
		{"inbound noncompliance", fbaInboundNoncomplianceHeader, "issue-reported-date\tshipment-creation-date\tfba-shipment-id\tfba-carton-id\tfulfillment-center-id\tsku\tfnsku\tasin\tproduct-name\tproblem-type\tproblem-quantity\texpected-quantity\treceived-quantity\tperformance-measurement-unit\tcoaching-level\tfee-type\tcurrency\tfee-total\tproblem-level\talert-status", func(data []byte) (int, error) {
			rows, err := ParseFBAInboundNoncompliance(data, "", "")
			return len(rows), err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if strings.Join(test.header, "\t") != test.official {
				t.Fatalf("header drifted from official contract: %s", strings.Join(test.header, "\t"))
			}
			data := []byte(test.official + "\n" + strings.Repeat("x\t", len(test.header)-1) + "x\n")
			rows, err := test.parse(data)
			if err != nil || rows != 1 {
				t.Fatalf("rows=%d err=%v", rows, err)
			}
		})
	}
}

func TestParseFBAEstimatedFeesAcceptsProductionNAHeaderAndMapsLongField(t *testing.T) {
	data := []byte(strings.Join(productionFBAEstimatedFeesHeader31, "\t") + "\n" + strings.Join(markerValues(productionFBAEstimatedFeesHeader31), "\t") + "\n")
	rows, err := ParseFBAEstimatedFees(data, "", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	if len(rows[0].Values) != len(expectedFBAEstimatedFeesCanonicalHeader40) {
		t.Fatalf("canonical values=%d, want %d", len(rows[0].Values), len(expectedFBAEstimatedFeesCanonicalHeader40))
	}
	for _, field := range []string{"product-size-tier", "estimated-future-fee"} {
		index := slices.Index(expectedFBAEstimatedFeesCanonicalHeader40, field)
		want := "marker-" + field
		if field == "estimated-future-fee" {
			want = "marker-estimated-future-fee (Current Selling on Amazon + Future Fulfillment fees)"
		}
		if got := rows[0].Values[index]; got != want {
			t.Fatalf("field %q = %q, want %q", field, got, want)
		}
	}
	for _, oldOnly := range []string{"product-size-weight-band", "expected-domestic-fulfilment-fee-per-unit", "expected-efn-fulfilment-fee-per-unit-uk"} {
		if got := rows[0].Values[slices.Index(expectedFBAEstimatedFeesCanonicalHeader40, oldOnly)]; got != "" {
			t.Fatalf("old-only field %q was merged: %q", oldOnly, got)
		}
	}
}

func TestParseFBAEstimatedFeesKeepsOldHeaderAndFillsNewFieldsEmpty(t *testing.T) {
	data := []byte(strings.Join(fbaEstimatedFeesHeader, "\t") + "\n" + strings.Join(markerValues(fbaEstimatedFeesHeader), "\t") + "\n")
	rows, err := ParseFBAEstimatedFees(data, "", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	if len(rows[0].Values) != len(expectedFBAEstimatedFeesCanonicalHeader40) {
		t.Fatalf("canonical values=%d, want %d", len(rows[0].Values), len(expectedFBAEstimatedFeesCanonicalHeader40))
	}
	for _, field := range expectedFBAEstimatedFeesCanonicalHeader40[len(fbaEstimatedFeesHeader):] {
		if got := rows[0].Values[slices.Index(expectedFBAEstimatedFeesCanonicalHeader40, field)]; got != "" {
			t.Fatalf("missing old-header field %q = %q", field, got)
		}
	}
}

func TestParseFBAEstimatedFeesRejectsUnknownProductionHeader(t *testing.T) {
	header := append([]string(nil), productionFBAEstimatedFeesHeader31...)
	header[len(header)-1] = "unknown-production-field"
	if _, err := ParseFBAEstimatedFees([]byte(strings.Join(header, "\t")+"\n"), "", ""); err == nil {
		t.Fatal("unknown estimated fees header was accepted")
	}
}

func markerValues(header []string) []string {
	values := make([]string, len(header))
	for i, name := range header {
		values[i] = "marker-" + name
	}
	return values
}
