package reportexport

import (
	"strings"
	"testing"
)

func TestParseFBAInventoryRequiresExactOfficialHeaderAndQuantities(t *testing.T) {
	data := strings.Join([]string{
		"sku\tfnsku\tasin\tproduct-name\tcondition\tyour-price\tmfn-listing-exists\tmfn-fulfillable-quantity\tafn-listing-exists\tafn-warehouse-quantity\tafn-fulfillable-quantity\tafn-unsellable-quantity\tafn-reserved-quantity\tafn-total-quantity\tper-unit-volume\tafn-inbound-working-quantity\tafn-inbound-shipped-quantity\tafn-inbound-receiving-quantity\tafn-researching-quantity\tafn-reserved-future-supply\tafn-future-supply-buyable",
		"SKU-1\tFNSKU-1\tASIN-1\tWidget\tNew\t12.50\tyes\t5\tyes\t2\t10\t1\t3\t14\t0.25\t4\t5\t6\t0\t1\tyes",
	}, "\n") + "\n"
	rows, err := ParseFBAInventory([]byte(data), "", "")
	if err != nil {
		t.Fatalf("ParseFBAInventory returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].AFNFulfillableQuantity != 10 || rows[0].AFNReservedQuantity != 3 {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestParseReservedInventoryRequiresExactOfficialHeader(t *testing.T) {
	data := "sku\tfnsku\tasin\tproduct-name\treserved_qty\treserved_customerorders\treserved_fc-processing\nSKU-1\tFNSKU-1\tASIN-1\tWidget\t8\t2\t6\n"
	rows, err := ParseReservedInventory([]byte(data), "", "")
	if err != nil {
		t.Fatalf("ParseReservedInventory returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].ReservedQty != 8 || rows[0].ReservedFCProcessing != 6 {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestParseAFNInventoryPreservesOfficialCaseAndSpaces(t *testing.T) {
	data := "seller-sku\tfulfillment-channel-sku\tasin\tcondition-type\tWarehouse-Condition-code\tQuantity Available\nSKU-1\tFC-SKU-1\tASIN-1\tNew\tSELLABLE\t17\n"
	rows, err := ParseAFNInventory([]byte(data), "", "")
	if err != nil {
		t.Fatalf("ParseAFNInventory returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].QuantityAvailable != 17 || rows[0].WarehouseConditionCode != "SELLABLE" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestParseInventoryRejectsWrongHeaderAndMalformedQuantity(t *testing.T) {
	_, err := ParseReservedInventory([]byte("sku\tfnsku\n"), "", "")
	if err == nil || !strings.Contains(err.Error(), "reserved inventory TSV header") {
		t.Fatalf("header error = %v", err)
	}
	data := "seller-sku\tfulfillment-channel-sku\tasin\tcondition-type\tWarehouse-Condition-code\tQuantity Available\nSKU-1\tFC-SKU-1\tASIN-1\tNew\tSELLABLE\tn/a\n"
	_, err = ParseAFNInventory([]byte(data), "", "")
	if err == nil || !strings.Contains(err.Error(), "quantity available") {
		t.Fatalf("quantity error = %v", err)
	}
}

func TestInventoryReportTypesAreDistinct(t *testing.T) {
	if FBAInventoryReportType == ReservedInventoryReportType || FBAInventoryReportType == AFNInventoryReportType || ReservedInventoryReportType == AFNInventoryReportType {
		t.Fatal("inventory report types must remain distinct")
	}
}
