package reportexport

const (
	FBAStrandedInventoryReportType    = "GET_STRANDED_INVENTORY_UI_DATA"
	FBAEstimatedFeesReportType        = "GET_FBA_ESTIMATED_FBA_FEES_TXT_DATA"
	FBAInboundNoncomplianceReportType = "GET_FBA_FULFILLMENT_INBOUND_NONCOMPLIANCE_DATA"
)

var fbaStrandedInventoryHeader = []string{
	"primary-action", "date-stranded", "Date-to-take-auto-removal", "status-primary", "status-secondary", "error-message", "stranded-reason", "asin", "sku", "fnsku", "product-name", "condition", "fulfilled-by", "fulfillable-qty", "your-price", "unfulfillable-qty", "reserved-quantity", "inbound-shipped-qty",
}

var fbaEstimatedFeesHeader = []string{
	"sku", "fnsku", "asin", "product-name", "product-group", "brand", "fulfilled-by", "has-local-inventory", "your-price", "sales-price", "longest-side", "median-side", "shortest-side", "length-and-girth", "unit-of-dimension", "item-package-weight", "unit-of-weight", "product-size-weight-band", "currency", "estimated-fee-total", "estimated-referral-fee-per-unit", "estimated-variable-closing-fee", "expected-domestic-fulfilment-fee-per-unit", "expected-efn-fulfilment-fee-per-unit-uk", "expected-efn-fulfilment-fee-per-unit-de", "expected-efn-fulfilment-fee-per-unit-fr", "expected-efn-fulfilment-fee-per-unit-it", "expected-efn-fulfilment-fee-per-unit-es", "expected-efn-fulfilment-fee-per-unit-se",
}

var fbaInboundNoncomplianceHeader = []string{
	"issue-reported-date", "shipment-creation-date", "fba-shipment-id", "fba-carton-id", "fulfillment-center-id", "sku", "fnsku", "asin", "product-name", "problem-type", "problem-quantity", "expected-quantity", "received-quantity", "performance-measurement-unit", "coaching-level", "fee-type", "currency", "fee-total", "problem-level", "alert-status",
}

type FBAStrandedInventory struct{ Values []string }
type FBAEstimatedFees struct{ Values []string }
type FBAInboundNoncompliance struct{ Values []string }

func ParseFBAStrandedInventory(downloaded []byte, compressionAlgorithm, contentType string) ([]FBAStrandedInventory, error) {
	return parseFixedReportRows(downloaded, compressionAlgorithm, contentType, "FBA stranded inventory", fbaStrandedInventoryHeader, func(values []string) FBAStrandedInventory { return FBAStrandedInventory{Values: values} })
}

func ParseFBAEstimatedFees(downloaded []byte, compressionAlgorithm, contentType string) ([]FBAEstimatedFees, error) {
	return parseFixedReportRows(downloaded, compressionAlgorithm, contentType, "FBA estimated fees", fbaEstimatedFeesHeader, func(values []string) FBAEstimatedFees { return FBAEstimatedFees{Values: values} })
}

func ParseFBAInboundNoncompliance(downloaded []byte, compressionAlgorithm, contentType string) ([]FBAInboundNoncompliance, error) {
	return parseFixedReportRows(downloaded, compressionAlgorithm, contentType, "FBA inbound noncompliance", fbaInboundNoncomplianceHeader, func(values []string) FBAInboundNoncompliance { return FBAInboundNoncompliance{Values: values} })
}
