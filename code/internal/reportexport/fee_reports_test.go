package reportexport

import (
	"slices"
	"strings"
	"testing"
)

var productionFBAStorageFeeChargesHeader34 = []string{
	"sku", "fnsku", "product_name", "fulfillment_center", "country_code", "longest_side", "median_side", "shortest_side",
	"measurement_units", "weight", "weight_units", "item_volume", "volume_units", "product_size_tier", "average_quantity_on_hand",
	"average_quantity_pending_removal", "estimated_total_item_volume", "month_of_charge", "storage_utilization_ratio",
	"storage_utilization_ratio_units", "base_rate", "utilization_surcharge_rate", "avg_qty_for_sus", "est_vol_for_sus",
	"est_base_msf", "est_sus", "currency", "estimated_monthly_storage_fee", "dangerous_goods_storage_type",
	"eligible_for_inventory_discount", "qualifies_for_inventory_discount", "total_incentive_fee_amount",
	"breakdown_incentive_fee_amount", "average_quantity_customer_orders",
}

var expectedFBAStorageFeeChargesCanonicalHeader36 = []string{
	"asin", "fnsku", "product_name", "fulfillment_center", "country_code", "longest_side", "median_side", "shortest_side",
	"measurement_units", "weight", "weight_units", "item_volume", "volume_units", "product_size_tier", "average_quantity_on_hand",
	"average_quantity_pending_removal", "estimated_total_item_volume", "month_of_charge", "storage_rate", "currency",
	"estimated_monthly_storage_fee", "dangerous_goods_storage_type", "eligible_for_inventory_discount", "qualifies_for_inventory_discount",
	"total_incentive_fee_amount", "breakdown_incentive_fee_amount", "average_quantity_customer_orders", "sku",
	"storage_utilization_ratio", "storage_utilization_ratio_units", "base_rate", "utilization_surcharge_rate", "avg_qty_for_sus",
	"est_vol_for_sus", "est_base_msf", "est_sus",
}

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

func TestParseFBAStorageFeeChargesAcceptsProductionThirtyFourColumnHeader(t *testing.T) {
	data := []byte(strings.Join(productionFBAStorageFeeChargesHeader34, "\t") + "\n" + strings.Join(feeMarkerValues(productionFBAStorageFeeChargesHeader34), "\t") + "\n")
	rows, err := ParseFBAStorageFeeCharges(data, "", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	values := rows[0].Values
	if len(values) != 36 {
		t.Fatalf("canonical values=%d, want 36", len(values))
	}
	for _, field := range []string{"sku", "storage_utilization_ratio", "est_sus"} {
		if got := values[slices.Index(expectedFBAStorageFeeChargesCanonicalHeader36, field)]; got != "marker-"+field {
			t.Fatalf("production field %q = %q", field, got)
		}
	}
	for _, oldOnly := range []string{"asin", "storage_rate"} {
		if got := values[slices.Index(expectedFBAStorageFeeChargesCanonicalHeader36, oldOnly)]; got != "" {
			t.Fatalf("old-only field %q was inferred: %q", oldOnly, got)
		}
	}
}

func TestParseFBAStorageFeeChargesKeepsLegacyHeaderAndLeavesProductionFieldsEmpty(t *testing.T) {
	data := []byte(strings.Join(fbaStorageFeeChargesHeader, "\t") + "\n" + strings.Join(feeMarkerValues(fbaStorageFeeChargesHeader), "\t") + "\n")
	rows, err := ParseFBAStorageFeeCharges(data, "", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	values := rows[0].Values
	if len(values) != 36 || values[0] != "marker-asin" || values[18] != "marker-storage_rate" {
		t.Fatalf("legacy row was not normalized: %#v", values)
	}
	for _, newOnly := range expectedFBAStorageFeeChargesCanonicalHeader36[27:] {
		if got := values[slices.Index(expectedFBAStorageFeeChargesCanonicalHeader36, newOnly)]; got != "" {
			t.Fatalf("production-only field %q = %q", newOnly, got)
		}
	}
}

func TestParseFBAStorageFeeChargesRejectsUnknownProductionHeader(t *testing.T) {
	header := append([]string(nil), productionFBAStorageFeeChargesHeader34...)
	header[30] = "unknown-production-field"
	if _, err := ParseFBAStorageFeeCharges([]byte(strings.Join(header, "\t")+"\n"), "", ""); err == nil {
		t.Fatal("unknown storage fee header was accepted")
	}
}

func feeMarkerValues(header []string) []string {
	values := make([]string, len(header))
	for i, field := range header {
		values[i] = "marker-" + field
	}
	return values
}
