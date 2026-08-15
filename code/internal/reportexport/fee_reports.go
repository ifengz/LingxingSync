package reportexport

const (
	FBAStorageFeeChargesReportType         = "GET_FBA_STORAGE_FEE_CHARGES_DATA"
	FBAOverageFeeChargesReportType         = "GET_FBA_OVERAGE_FEE_CHARGES_DATA"
	FBALongtermStorageFeeChargesReportType = "GET_FBA_FULFILLMENT_LONGTERM_STORAGE_FEE_CHARGES_DATA"
)

var fbaStorageFeeChargesHeader = []string{
	"asin", "fnsku", "product_name", "fulfillment_center", "country_code", "longest_side", "median_side", "shortest_side",
	"measurement_units", "weight", "weight_units", "item_volume", "volume_units", "product_size_tier", "average_quantity_on_hand",
	"average_quantity_pending_removal", "estimated_total_item_volume", "month_of_charge", "storage_rate", "currency",
	"estimated_monthly_storage_fee", "dangerous_goods_storage_type", "eligible_for_inventory_discount", "qualifies_for_inventory_discount",
	"total_incentive_fee_amount", "breakdown_incentive_fee_amount", "average_quantity_customer_orders",
}

var fbaOverageFeeChargesHeader = []string{
	"charged_date", "country_code", "storage_type", "charge_rate", "storage_usage_volume", "storage_limit_volume", "overage_volume", "volume_unit", "charged_fee_amount", "currency_code",
}

var fbaLongtermStorageFeeChargesHeader = []string{
	"snapshot-date", "sku", "fnsku", "asin", "product-name", "condition", "per-unit-volume", "currency", "volume-unit", "country", "qty-charged", "amount-charged", "surcharge-age-tier", "rate-surcharge",
}

type FBAStorageFeeCharges struct{ Values []string }
type FBAOverageFeeCharges struct{ Values []string }
type FBALongtermStorageFeeCharges struct{ Values []string }

func ParseFBAStorageFeeCharges(downloaded []byte, compressionAlgorithm, contentType string) ([]FBAStorageFeeCharges, error) {
	return parseFixedReportRows(downloaded, compressionAlgorithm, contentType, "FBA storage fee charges", fbaStorageFeeChargesHeader, func(values []string) FBAStorageFeeCharges { return FBAStorageFeeCharges{Values: values} })
}

func ParseFBAOverageFeeCharges(downloaded []byte, compressionAlgorithm, contentType string) ([]FBAOverageFeeCharges, error) {
	return parseFixedReportRows(downloaded, compressionAlgorithm, contentType, "FBA overage fee charges", fbaOverageFeeChargesHeader, func(values []string) FBAOverageFeeCharges { return FBAOverageFeeCharges{Values: values} })
}

func ParseFBALongtermStorageFeeCharges(downloaded []byte, compressionAlgorithm, contentType string) ([]FBALongtermStorageFeeCharges, error) {
	return parseFixedReportRows(downloaded, compressionAlgorithm, contentType, "FBA longterm storage fee charges", fbaLongtermStorageFeeChargesHeader, func(values []string) FBALongtermStorageFeeCharges {
		return FBALongtermStorageFeeCharges{Values: values}
	})
}

func parseFixedReportRows[T any](downloaded []byte, compressionAlgorithm, contentType, name string, header []string, build func([]string) T) ([]T, error) {
	records, err := readExactTSV(downloaded, compressionAlgorithm, contentType, name, header)
	if err != nil {
		return nil, err
	}
	rows := make([]T, 0, len(records))
	for _, record := range records {
		rows = append(rows, build(record))
	}
	return rows, nil
}
