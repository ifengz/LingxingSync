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

func TestParseFBAInventoryAcceptsOfficialEUQuantityVariant(t *testing.T) {
	data := strings.Join([]string{
		strings.Join(fbaInventoryEUHeader, "\t"),
		"SKU-1\tFNSKU-1\tASIN-1\tWidget\tNew\t12.50\tyes\t5\tyes\t2\t10\t1\t3\t14\t0.25\t4\t5\t6\t0\t1\tyes\t7\t3\t",
	}, "\n") + "\n"
	rows, err := ParseFBAInventory([]byte(data), "", "")
	if err != nil {
		t.Fatalf("EU quantity variant returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].AFNFulfillableQuantityLocal != "7" || rows[0].AFNFulfillableQuantityRemote != "3" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestParseReservedInventoryRequiresExactOfficialHeader(t *testing.T) {
	data := "sku\tfnsku\tasin\tproduct-name\treserved_qty\treserved_customerorders\treserved_fc-transfers\treserved_fc-processing\t\nSKU-1\tFNSKU-1\tASIN-1\tWidget\t8\t2\t3\t3\t\n"
	rows, err := ParseReservedInventory([]byte(data), "", "")
	if err != nil {
		t.Fatalf("ParseReservedInventory returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].ReservedQty != 8 || rows[0].ReservedFCTransfers != 3 || rows[0].ReservedFCProcessing != 3 {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestParseReservedInventoryAcceptsProductionNineColumnHeader(t *testing.T) {
	data := "sku\tfnsku\tasin\tproduct-name\treserved_qty\treserved_customerorders\treserved_fc-processing\treserved_staging\tprogram\nSKU-1\tFNSKU-1\tASIN-1\tWidget\t8\t2\t3\t1\tFBA\n"
	rows, err := ParseReservedInventory([]byte(data), "", "")
	if err != nil {
		t.Fatalf("ParseReservedInventory returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].ReservedStaging != 1 || rows[0].ReservedStagingRaw != "1" || rows[0].Program != "FBA" {
		t.Fatalf("parsed production reserved row = %#v", rows)
	}
	if rows[0].ReservedFCTransfersRaw != "" {
		t.Fatalf("production variant must not invent FC transfers: %#v", rows[0])
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

func TestParseInventoryAcceptsTrailingTabWithoutAddingAField(t *testing.T) {
	data := "seller-sku\tfulfillment-channel-sku\tasin\tcondition-type\tWarehouse-Condition-code\tQuantity Available\t\nSKU-1\tFC-SKU-1\tASIN-1\tNew\tSELLABLE\t17\t\n"
	rows, err := ParseAFNInventory([]byte(data), "", "")
	if err != nil {
		t.Fatalf("trailing tab should be ignored: %v", err)
	}
	if len(rows) != 1 || rows[0].QuantityAvailable != 17 {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestInventoryReportTypesAreDistinct(t *testing.T) {
	if FBAInventoryReportType == ReservedInventoryReportType || FBAInventoryReportType == AFNInventoryReportType || ReservedInventoryReportType == AFNInventoryReportType {
		t.Fatal("inventory report types must remain distinct")
	}
}

func TestParseFBAAllInventoryUsesArchivedReportTypeContract(t *testing.T) {
	data := strings.Join([]string{
		"sku\tfnsku\tasin\tproduct-name\tcondition\tyour-price\tmfn-listing-exists\tmfn-fulfillable-quantity\tafn-listing-exists\tafn-warehouse-quantity\tafn-fulfillable-quantity\tafn-unsellable-quantity\tafn-reserved-quantity\tafn-total-quantity\tper-unit-volume\tafn-inbound-working-quantity\tafn-inbound-shipped-quantity\tafn-inbound-receiving-quantity\tafn-researching-quantity\tafn-reserved-future-supply\tafn-future-supply-buyable",
		"SKU-ARCHIVED\tFNSKU-ARCHIVED\tASIN-ARCHIVED\tArchived Widget\tNew\t9.99\tyes\t1\tyes\t2\t3\t4\t5\t14\t0.25\t6\t7\t8\t0\t1\tyes",
	}, "\n") + "\n"
	rows, err := ParseFBAAllInventory([]byte(data), "", "")
	if err != nil {
		t.Fatalf("ParseFBAAllInventory returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].SKU != "SKU-ARCHIVED" || rows[0].AFNTotalQuantity != 14 {
		t.Fatalf("rows = %#v", rows)
	}
}
