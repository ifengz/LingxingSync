package reportexport

const AllOrdersReportType = "GET_FLAT_FILE_ALL_ORDERS_DATA_BY_ORDER_DATE_GENERAL"

var allOrdersHeader = []string{
	"amazon-order-id", "merchant-order-id", "purchase-date", "last-updated-date", "order-status", "fulfillment-channel", "sales-channel", "order-channel", "ship-service-level", "product-name", "sku", "asin", "item-status", "quantity", "currency", "item-price", "item-tax", "shipping-price", "shipping-tax", "gift-wrap-price", "gift-wrap-tax", "item-promotion-discount", "ship-promotion-discount", "ship-city", "ship-state", "ship-postal-code", "ship-country", "promotion-ids", "is-business-order", "purchase-order-number", "price-designation",
}

type AllOrder struct{ Values []string }

func ParseAllOrders(downloaded []byte, compressionAlgorithm, contentType string) ([]AllOrder, error) {
	return parseFixedReportRows(downloaded, compressionAlgorithm, contentType, "Amazon all orders", allOrdersHeader, func(values []string) AllOrder { return AllOrder{Values: values} })
}
