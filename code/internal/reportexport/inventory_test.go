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
	if rows[0].AFNFCTransferQuantity != "" || rows[0].AFNOnhandBuyableQuantity != "" || rows[0].Store != "" {
		t.Fatalf("EU variant must leave production-only fields empty: %#v", rows[0])
	}
}

func TestParseFBAInventoryAcceptsProductionTwentyFourColumnHeader(t *testing.T) {
	productionHeader := append(append([]string(nil), fbaInventoryHeader...), "afn-fc-transfer-quantity", "afn-onhand-buyable-quantity", "store")
	data := strings.Join([]string{
		strings.Join(productionHeader, "\t"),
		"SKU-1\tFNSKU-1\tASIN-1\tWidget\tNew\t12.50\tyes\t5\tyes\t2\t10\t1\t3\t14\t0.25\t4\t5\t6\t0\t1\tyes\t4\t5\tSTORE-1",
	}, "\n") + "\n"
	rows, err := ParseFBAInventory([]byte(data), "", "")
	if err != nil {
		t.Fatalf("production 24-column variant returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].AFNFCTransferQuantity != "4" || rows[0].AFNOnhandBuyableQuantity != "5" || rows[0].Store != "STORE-1" {
		t.Fatalf("production fields = %#v", rows)
	}
}

func TestParseFBAInventoryAcceptsProduction24ColumnContract(t *testing.T) {
	header := "sku\tfnsku\tasin\tproduct-name\tcondition\tyour-price\tmfn-listing-exists\tmfn-fulfillable-quantity\tafn-listing-exists\tafn-warehouse-quantity\tafn-fulfillable-quantity\tafn-unsellable-quantity\tafn-reserved-quantity\tafn-total-quantity\tper-unit-volume\tafn-inbound-working-quantity\tafn-inbound-shipped-quantity\tafn-inbound-receiving-quantity\tafn-researching-quantity\tafn-reserved-future-supply\tafn-future-supply-buyable\tafn-fc-transfer-quantity\tafn-onhand-buyable-quantity\tstore"
	row := "SKU-PROD\tFNSKU-PROD\tASIN-PROD\tProduction Widget\tNew\t19.99\tyes\t1\tyes\t2\t3\t4\t5\t14\t0.25\t6\t7\t8\t9\t10\tyes\t11\t12\tNA"
	rows, err := ParseFBAInventory([]byte(header+"\n"+row+"\n"), "", "")
	if err != nil {
		t.Fatalf("ParseFBAInventory returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %#v", rows)
	}
	got := rows[0]
	if got.AFNFCTransferQuantity != "11" || got.AFNOnhandBuyableQuantity != "12" || got.Store != "NA" {
		t.Fatalf("production fields = %#v", got)
	}
}

func TestParseFBAInventoryRejectsUnknown24ColumnContract(t *testing.T) {
	header := "sku\tfnsku\tasin\tproduct-name\tcondition\tyour-price\tmfn-listing-exists\tmfn-fulfillable-quantity\tafn-listing-exists\tafn-warehouse-quantity\tafn-fulfillable-quantity\tafn-unsellable-quantity\tafn-reserved-quantity\tafn-total-quantity\tper-unit-volume\tafn-inbound-working-quantity\tafn-inbound-shipped-quantity\tafn-inbound-receiving-quantity\tafn-researching-quantity\tafn-reserved-future-supply\tafn-future-supply-buyable\tunknown-quantity\tafn-onhand-buyable-quantity\tstore"
	_, err := ParseFBAInventory([]byte(header+"\n"), "", "")
	if err == nil || !strings.Contains(err.Error(), "FBA inventory TSV header") {
		t.Fatalf("header error = %v", err)
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
