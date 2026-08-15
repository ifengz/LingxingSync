package reportexport

import (
	"strings"
	"testing"
)

func TestParseFixedFeeReportsRequireOfficialHeaders(t *testing.T) {
	tests := []struct {
		name   string
		header []string
		parse  func([]byte) (int, error)
	}{
		{"storage", fbaStorageFeeChargesHeader, func(data []byte) (int, error) {
			rows, err := ParseFBAStorageFeeCharges(data, "", "")
			return len(rows), err
		}},
		{"overage", fbaOverageFeeChargesHeader, func(data []byte) (int, error) {
			rows, err := ParseFBAOverageFeeCharges(data, "", "")
			return len(rows), err
		}},
		{"longterm", fbaLongtermStorageFeeChargesHeader, func(data []byte) (int, error) {
			rows, err := ParseFBALongtermStorageFeeCharges(data, "", "")
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
