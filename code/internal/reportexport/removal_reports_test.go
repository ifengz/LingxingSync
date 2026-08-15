package reportexport

import (
	"strings"
	"testing"
)

func TestParseRemovalReportsRequireExactOfficialHeaders(t *testing.T) {
	tests := []struct {
		name     string
		header   []string
		official string
		parse    func([]byte) (int, error)
	}{
		{"recommended", fbaRecommendedRemovalHeader, "snapshot-date\tsku\tfnsku\tasin\tproduct-name\tcondition\tsellable-quantity\tsellable-271-365-days\tsellable-365+-days\tsellable-removal-quantity\tunsellable-quantity\tunsellable-0-7-days\tunsellable-8-60-days\tunsellable-61-90-days\tsellable-121-180-days\tsellable-181-270-days", func(data []byte) (int, error) {
			rows, err := ParseFBARecommendedRemoval(data, "", "")
			return len(rows), err
		}},
		{"order", fbaRemovalOrderHeader, "request-date\torder-id\torder-type\tservice-speed\torder-status\tlast-updated-date\tsku\tfnsku\tdisposition\trequested-quantity\tcancelled-quantity\tdisposed-quantity\tshipped-quantity\tin-process-quantity\tremoval-fee\tcurrency", func(data []byte) (int, error) { rows, err := ParseFBARemovalOrder(data, "", ""); return len(rows), err }},
		{"shipment", fbaRemovalShipmentHeader, "request-date\torder-id\tshipment-date\tsku\tfnsku\tdisposition\tshipped-quantity\tcarrier\ttracking-number\tremoval-order-type", func(data []byte) (int, error) {
			rows, err := ParseFBARemovalShipment(data, "", "")
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
			bad := append(append([]string(nil), test.header...), "unknown-column")
			if _, err := test.parse([]byte(strings.Join(bad, "\t") + "\n")); err == nil {
				t.Fatal("expected unknown-column failure")
			}
		})
	}
}
