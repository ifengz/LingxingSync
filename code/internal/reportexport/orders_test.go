package reportexport

import (
	"strings"
	"testing"
)

const productionAllOrdersHeader34 = "amazon-order-id\tmerchant-order-id\tpurchase-date\tlast-updated-date\torder-status\tfulfillment-channel\tsales-channel\torder-channel\tship-service-level\tproduct-name\tsku\tasin\titem-status\tquantity\tcurrency\titem-price\titem-tax\tshipping-price\tshipping-tax\tgift-wrap-price\tgift-wrap-tax\titem-promotion-discount\tship-promotion-discount\tship-city\tship-state\tship-postal-code\tship-country\tpromotion-ids\tcpf\tis-business-order\tpurchase-order-number\tprice-designation\tsignature-confirmation-recommended\torder-item-id"

func TestParseAllOrdersRequiresOfficialHeader(t *testing.T) {
	official := strings.Join(allOrdersHeader, "\t")
	data := []byte(official + "\n" + strings.Repeat("x\t", len(allOrdersHeader)-1) + "x\n")
	rows, err := ParseAllOrders(data, "", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	if len(rows[0].Values) != 34 || rows[0].Values[28] != "" || rows[0].Values[32] != "" || rows[0].Values[33] != "" {
		t.Fatalf("31-column row was not normalized: values=%#v", rows[0].Values)
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
	if len(rows[0].Values) != 34 || rows[0].Values[5] != "x" || rows[0].Values[28] != "x" || rows[0].Values[32] != "" || rows[0].Values[33] != "x" {
		t.Fatalf("33-column row was not normalized: values=%#v", rows[0].Values)
	}
}

func TestParseAllOrdersAcceptsProductionThirtyFourColumnHeader(t *testing.T) {
	row := "amazon-1\tmerchant-1\t2026-08-14T00:00:00Z\t2026-08-14T01:00:00Z\tShipped\tAmazon\tAmazon.com\tchannel\tStandard\tProduct\tSKU-1\tASIN-1\tShipped\t2\tUSD\t10\t1\t2\t0\t0\t0\t0\t0\tCity\tState\t12345\tUS\tpromo\tcpf-value\ttrue\tpo-1\tBusinessPrice\ttrue\titem-1"
	rows, err := ParseAllOrders([]byte(productionAllOrdersHeader34+"\n"+row+"\n"), "", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	values := rows[0].Values
	if len(values) != 34 || values[5] != "Amazon" || values[28] != "cpf-value" || values[32] != "true" || values[33] != "item-1" {
		t.Fatalf("34-column row was not preserved in canonical order: %#v", values)
	}

	unknown := strings.Replace(productionAllOrdersHeader34, "signature-confirmation-recommended", "unknown-production-field", 1)
	if _, err := ParseAllOrders([]byte(unknown+"\n"+row+"\n"), "", ""); err == nil {
		t.Fatal("expected unknown 34-column header to fail loud")
	}
}
