package reportexport

import (
	"strings"
	"testing"
)

func TestParseAdditionalFBAReportsRequireOfficialHeaders(t *testing.T) {
	tests := []struct {
		name   string
		header []string
		parse  func([]byte) (int, error)
	}{
		{"stranded", fbaStrandedInventoryHeader, func(data []byte) (int, error) {
			rows, err := ParseFBAStrandedInventory(data, "", "")
			return len(rows), err
		}},
		{"estimated fees", fbaEstimatedFeesHeader, func(data []byte) (int, error) {
			rows, err := ParseFBAEstimatedFees(data, "", "")
			return len(rows), err
		}},
		{"inbound noncompliance", fbaInboundNoncomplianceHeader, func(data []byte) (int, error) {
			rows, err := ParseFBAInboundNoncompliance(data, "", "")
			return len(rows), err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := []byte(strings.Join(test.header, "\t") + "\n" + strings.Repeat("x\t", len(test.header)-1) + "x\n")
			rows, err := test.parse(data)
			if err != nil || rows != 1 {
				t.Fatalf("rows=%d err=%v", rows, err)
			}
		})
	}
}
