package reportexport

const (
	FBARecommendedRemovalReportType = "GET_FBA_RECOMMENDED_REMOVAL_DATA"
	FBARemovalOrderReportType       = "GET_FBA_FULFILLMENT_REMOVAL_ORDER_DETAIL_DATA"
	FBARemovalShipmentReportType    = "GET_FBA_FULFILLMENT_REMOVAL_SHIPMENT_DETAIL_DATA"
)

var fbaRecommendedRemovalHeader = []string{
	"snapshot-date", "sku", "fnsku", "asin", "product-name", "condition", "sellable-quantity", "sellable-271-365-days", "sellable-365+-days", "sellable-removal-quantity", "unsellable-quantity", "unsellable-0-7-days", "unsellable-8-60-days", "unsellable-61-90-days", "sellable-121-180-days", "sellable-181-270-days",
}

var fbaRemovalOrderHeader = []string{
	"request-date", "order-id", "order-type", "service-speed", "order-status", "last-updated-date", "sku", "fnsku", "disposition", "requested-quantity", "cancelled-quantity", "disposed-quantity", "shipped-quantity", "in-process-quantity", "removal-fee", "currency",
}

var fbaRemovalShipmentHeader = []string{
	"request-date", "order-id", "shipment-date", "sku", "fnsku", "disposition", "shipped-quantity", "carrier", "tracking-number", "removal-order-type",
}

type FBARecommendedRemoval struct{ Values []string }
type FBARemovalOrder struct{ Values []string }
type FBARemovalShipment struct{ Values []string }

func ParseFBARecommendedRemoval(downloaded []byte, compressionAlgorithm, contentType string) ([]FBARecommendedRemoval, error) {
	return parseFixedReportRows(downloaded, compressionAlgorithm, contentType, "FBA recommended removal", fbaRecommendedRemovalHeader, func(values []string) FBARecommendedRemoval { return FBARecommendedRemoval{Values: values} })
}

func ParseFBARemovalOrder(downloaded []byte, compressionAlgorithm, contentType string) ([]FBARemovalOrder, error) {
	return parseFixedReportRows(downloaded, compressionAlgorithm, contentType, "FBA removal order", fbaRemovalOrderHeader, func(values []string) FBARemovalOrder { return FBARemovalOrder{Values: values} })
}

func ParseFBARemovalShipment(downloaded []byte, compressionAlgorithm, contentType string) ([]FBARemovalShipment, error) {
	return parseFixedReportRows(downloaded, compressionAlgorithm, contentType, "FBA removal shipment", fbaRemovalShipmentHeader, func(values []string) FBARemovalShipment { return FBARemovalShipment{Values: values} })
}
