package reportexport

import "fmt"

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

const (
	fbaEstimatedFeesFutureFeeHeader = "estimated-future-fee (Current Selling on Amazon + Future Fulfillment fees)"
	fbaEstimatedFeesFutureFeeColumn = "estimated-future-fee"
)

var fbaEstimatedFeesProductionHeader = []string{
	"sku", "fnsku", "asin", "amazon-store", "product-name", "product-group", "brand", "fulfilled-by", "your-price", "sales-price", "longest-side", "median-side", "shortest-side", "length-and-girth", "unit-of-dimension", "item-package-weight", "unit-of-weight", "product-size-tier", "currency", "estimated-fee-total", "estimated-referral-fee-per-unit", "estimated-variable-closing-fee", "estimated-order-handling-fee-per-order", "estimated-pick-pack-fee-per-unit", "estimated-weight-handling-fee-per-unit", "expected-fulfillment-fee-per-unit", fbaEstimatedFeesFutureFeeHeader, "estimated-future-order-handling-fee-per-order", "estimated-future-pick-pack-fee-per-unit", "estimated-future-weight-handling-fee-per-unit", "expected-future-fulfillment-fee-per-unit",
}

var fbaEstimatedFeesCanonicalHeader = append(append([]string(nil), fbaEstimatedFeesHeader...),
	"amazon-store", "product-size-tier", "estimated-order-handling-fee-per-order", "estimated-pick-pack-fee-per-unit", "estimated-weight-handling-fee-per-unit", "expected-fulfillment-fee-per-unit", fbaEstimatedFeesFutureFeeColumn, "estimated-future-order-handling-fee-per-order", "estimated-future-pick-pack-fee-per-unit", "estimated-future-weight-handling-fee-per-unit", "expected-future-fulfillment-fee-per-unit")

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
	records, header, err := readExactTSVVariants(downloaded, compressionAlgorithm, contentType, "FBA estimated fees", [][]string{fbaEstimatedFeesHeader, fbaEstimatedFeesProductionHeader})
	if err != nil {
		return nil, err
	}
	canonicalIndex := make(map[string]int, len(fbaEstimatedFeesCanonicalHeader))
	for i, name := range fbaEstimatedFeesCanonicalHeader {
		canonicalIndex[name] = i
	}
	rows := make([]FBAEstimatedFees, 0, len(records))
	for _, record := range records {
		values := make([]string, len(fbaEstimatedFeesCanonicalHeader))
		for i, name := range header {
			if name == fbaEstimatedFeesFutureFeeHeader {
				name = fbaEstimatedFeesFutureFeeColumn
			}
			index, ok := canonicalIndex[name]
			if !ok {
				return nil, fmt.Errorf("FBA estimated fees canonical column %q is missing", name)
			}
			values[index] = record[i]
		}
		rows = append(rows, FBAEstimatedFees{Values: values})
	}
	return rows, nil
}

func ParseFBAInboundNoncompliance(downloaded []byte, compressionAlgorithm, contentType string) ([]FBAInboundNoncompliance, error) {
	return parseFixedReportRows(downloaded, compressionAlgorithm, contentType, "FBA inbound noncompliance", fbaInboundNoncomplianceHeader, func(values []string) FBAInboundNoncompliance { return FBAInboundNoncompliance{Values: values} })
}
