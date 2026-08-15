package reportexport

import (
	"strings"
	"testing"
)

const officialFulfilledShipmentsHeader = "amazon-order-id\tmerchant-order-id\tshipment-id\tshipment-item-id\tamazon-order-item-id\tmerchant-order-item-id\tpurchase-date\tpayments-date\tshipment-date\treporting-date\tbuyer-email\tbuyer-name\tbuyer-phone-number\tsku\tproduct-name\tquantity-shipped\tcurrency\titem-price\titem-tax\tshipping-price\tshipping-tax\tgift-wrap-price\tgift-wrap-tax\tship-service-level\trecipient-name\tship-address-1\tship-address-2\tship-address-3\tship-city\tship-state\tship-postal-code\tship-country\tship-phone-number\tbill-address-1\tbill-address-2\tbill-address-3\tbill-city\tbill-state\tbill-postal-code\tbill-country\titem-promotion-discount\tship-promotion-discount\tcarrier\ttracking-number\testimated-arrival-date\tfulfillment-center-id\tfulfillment-channel\tsales-channel"

func TestParseFulfilledShipmentsRequiresOfficialFortyEightColumnHeader(t *testing.T) {
	values := []string{
		"ORDER-1", "MERCHANT-1", "SHIPMENT-1", "SHIPMENT-ITEM-1", "ORDER-ITEM-1", "MERCHANT-ITEM-1",
		"2026-08-14T01:02:03Z", "2026-08-14T01:03:03Z", "2026-08-14T03:00:00Z", "2026-08-14T03:30:00Z",
		"buyer@example.test", "Buyer Name", "+1-555-0100", "SKU-1", "Widget", "2", "USD", "25.00", "2.00", "3.00", "0.30", "1.00", "0.10", "Expedited",
		"Recipient Name", "1 Main St", "Unit 2", "Attn Receiving", "Seattle", "WA", "98101", "US", "+1-555-0101",
		"9 Billing Ave", "Floor 3", "Accounts Payable", "Seattle", "WA", "98102", "US",
		"5.00", "1.00", "UPS", "1Z999", "2026-08-16T00:00:00Z", "BFI4", "AFN", "Amazon.com",
	}
	rows, err := ParseFulfilledShipments([]byte(officialFulfilledShipmentsHeader+"\n"+strings.Join(values, "\t")+"\n"), "", "text/tab-separated-values; charset=utf-8")
	if err != nil {
		t.Fatalf("ParseFulfilledShipments returned error: %v", err)
	}
	if len(rows) != 1 || len(rows[0].Values) != 48 {
		t.Fatalf("rows = %#v", rows)
	}
	if rows[0].Values[10] != "buyer@example.test" || rows[0].Values[24] != "Recipient Name" || rows[0].Values[33] != "9 Billing Ave" || rows[0].Values[47] != "Amazon.com" {
		t.Fatalf("raw PII/report values were not preserved: %#v", rows[0].Values)
	}
}

func TestParseFulfilledShipmentsFailsLoudOnUnknownHeader(t *testing.T) {
	header := strings.Replace(officialFulfilledShipmentsHeader, "tracking-number", "tracking-id", 1)
	if _, err := ParseFulfilledShipments([]byte(header+"\n"), "", ""); err == nil || !strings.Contains(err.Error(), "header") {
		t.Fatalf("unknown header error = %v", err)
	}
}
