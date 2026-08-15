package reportexport

import (
	"strings"
	"testing"
)

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
