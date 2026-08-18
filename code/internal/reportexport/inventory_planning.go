package reportexport

var fbaInventoryPlanningHeader = []string{
	"snapshot-date", "sku", "fnsku", "asin", "product-name", "condition", "available", "fc-transfer", "pending-removal-quantity",
	"inv-age-0-to-90-days", "inv-age-91-to-180-days", "inv-age-181-to-270-days", "inv-age-271-to-365-days", "inv-age-366-to-455-days", "inv-age-456-plus-days",
	"currency", "units-shipped-t7", "units-shipped-t30", "units-shipped-t60", "units-shipped-t90", "alert", "your-price", "sales-price",
	"lowest-price-new-plus-shipping", "lowest-price-used", "recommended-action", "DEPRECATED healthy-inventory-level", "recommended-sales-price",
	"recommended-sale-duration-days", "recommended-removal-quantity", "estimated-cost-savings-of-recommended-actions", "sell-through", "item-volume",
	"volume-unit-measurement", "storage-type", "storage-volume", "marketplace", "product-group", "sales-rank", "days-of-supply", "estimated-excess-quantity",
	"weeks-of-cover-t30", "weeks-of-cover-t90", "featuredoffer-price", "sales-shipped-last-7-days", "sales-shipped-last-30-days",
	"sales-shipped-last-60-days", "sales-shipped-last-90-days", "inv-age-0-to-30-days", "inv-age-31-to-60-days", "inv-age-61-to-90-days",
	"inv-age-181-to-330-days", "inv-age-331-to-365-days", "estimated-storage-cost-next-month", "inbound-quantity", "inbound-working", "inbound-shipped",
	"inbound-received", "no-sale-last-6-months", "Total Reserved Quantity", "unfulfillable-quantity", "quantity-to-be-charged-ais-181-210-days",
	"estimated-ais-181-210-days", "quantity-to-be-charged-ais-211-240-days", "estimated-ais-211-240-days", "quantity-to-be-charged-ais-241-270-days",
	"estimated-ais-241-270-days", "quantity-to-be-charged-ais-271-300-days", "estimated-ais-271-300-days", "quantity-to-be-charged-ais-301-330-days",
	"estimated-ais-301-330-days", "quantity-to-be-charged-ais-331-365-days", "estimated-ais-331-365-days", "quantity-to-be-charged-ais-366-455-days",
	"estimated-ais-366-455-days", "quantity-to-be-charged-ais-456-plus-days",
	"estimated-ais-456-plus-days", "historical-days-of-supply", "fba-minimum-inventory-level", "fba-inventory-level-health-status", "Recommended ship-in quantity",
	"Recommended ship-in date", "Last updated date for Historical Days of Supply", "Exempted from Low-Inventory-Level fee?", "Low-Inventory-Level fee applied in current week?",
	"Short term historical days of supply", "Long term historical days of supply", "Inventory age snapshot date", "Inventory Supply at FBA", "Reserved FC Processing",
	"Reserved Customer Order", "Reserved Staging", "Total Days of Supply (including units from open shipments)", "supplier", "is-seasonal-in-next-3-months",
	"season-name", "season-start-date", "season-end-date", "Fulfillment Service and Programs",
}

type FBAInventoryPlanning struct{ Values []string }

func FBAInventoryPlanningColumns() []string {
	return append([]string(nil), fbaInventoryPlanningHeader...)
}

func ParseFBAInventoryPlanning(downloaded []byte, compressionAlgorithm, contentType string) ([]FBAInventoryPlanning, error) {
	return parseFixedReportRows(downloaded, compressionAlgorithm, contentType, "FBA inventory planning", fbaInventoryPlanningHeader, func(values []string) FBAInventoryPlanning {
		return FBAInventoryPlanning{Values: values}
	})
}
