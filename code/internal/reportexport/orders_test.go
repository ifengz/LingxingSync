package reportexport

import (
	"strings"
	"testing"
)

func TestParseAllOrdersRequiresOfficialHeader(t *testing.T) {
	official := strings.Join(allOrdersHeader, "\t")
	data := []byte(official + "\n" + strings.Repeat("x\t", len(allOrdersHeader)-1) + "x\n")
	rows, err := ParseAllOrders(data, "", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	if _, err := ParseAllOrders([]byte(strings.Replace(official, "asin", "wrong", 1)+"\n"), "", ""); err == nil {
		t.Fatal("expected header mismatch")
	}
}

func TestParseAllOrdersAcceptsOrderReportsHeaderVariant(t *testing.T) {
	official := strings.Join(allOrdersHeaderWithItemIDAndCPF, "\t")
	data := []byte(official + "\t\n" + strings.Repeat("x\t", len(allOrdersHeaderWithItemIDAndCPF)-1) + "x\t\n")
	rows, err := ParseAllOrders(data, "", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
}
