package listingdaily

import "fmt"

type FieldDiff struct {
	Key      Key
	Field    string
	Database any
	Report   any
}

// Reconciliation intentionally contains only the three contractually allowed
// outputs: database missing, report missing, and mismatched fields on one key.
type Reconciliation struct {
	MissingInDB     []Key
	MissingInReport []Key
	FieldDiffs      []FieldDiff
}

func Reconcile(database, report []Metric) (Reconciliation, error) {
	return ReconcileFields(database, report, knownMetricFieldNames())
}

func ReconcileFields(database, report []Metric, fields []string) (Reconciliation, error) {
	dbRows, err := indexMetrics(database)
	if err != nil {
		return Reconciliation{}, fmt.Errorf("listing daily: database input: %w", err)
	}
	reportRows, err := indexMetrics(report)
	if err != nil {
		return Reconciliation{}, fmt.Errorf("listing daily: report input: %w", err)
	}
	result := Reconciliation{}
	for id, reportRow := range reportRows {
		databaseRow, ok := dbRows[id]
		if !ok {
			result.MissingInDB = append(result.MissingInDB, reportRow.Key)
			continue
		}
		result.FieldDiffs = append(result.FieldDiffs, diffValuesForFields(databaseRow, reportRow, fields)...)
	}
	for id, databaseRow := range dbRows {
		if _, ok := reportRows[id]; !ok {
			result.MissingInReport = append(result.MissingInReport, databaseRow.Key)
		}
	}
	return result, nil
}

func indexMetrics(rows []Metric) (map[string]Metric, error) {
	indexed := make(map[string]Metric, len(rows))
	for _, row := range rows {
		if err := validateInput(Input{Key: row.Key, Scope: row.Scope, Values: row.Values}); err != nil {
			return nil, err
		}
		id := keyID(row.Key, row.Scope)
		if _, exists := indexed[id]; exists {
			return nil, fmt.Errorf("duplicate primary key %s", id)
		}
		indexed[id] = row
	}
	return indexed, nil
}

func diffValues(database, report Metric) []FieldDiff {
	return diffValuesForFields(database, report, knownMetricFieldNames())
}

func diffValuesForFields(database, report Metric, fields []string) []FieldDiff {
	result := make([]FieldDiff, 0, len(fields))
	for _, field := range fields {
		databaseValue, reportValue := metricField(database.Values, field), metricField(report.Values, field)
		if sameKnownValue(databaseValue, reportValue) {
			continue
		}
		result = append(result, FieldDiff{Key: database.Key, Field: field, Database: databaseValue, Report: reportValue})
	}
	return result
}

func knownMetricFieldNames() []string {
	return []string{"sales_units", "sales_amount", "returns_qty", "inventory_sellable", "inventory_inbound", "inventory_reserved", "inventory_unfulfillable", "inventory_local_warehouse", "inventory_unhealthy_units", "inventory_aged90_sellable_units", "inventory_sell_through_rate", "inventory_receive_fill_rate", "inventory_vendor_confirmation_rate", "inventory_avg_lead_time_days", "inventory_sellable_cost", "inventory_unfulfillable_cost", "inventory_aged90_cost", "inventory_unhealthy_cost", "inventory_inbound_cost", "inventory_currency", "inventory_inbound_receiving", "inventory_inbound_shipped", "inventory_inbound_working", "inventory_reserved_customer_orders", "inventory_reserved_fc_processing", "inventory_reserved_fc_transfers", "sessions_desktop", "sessions_mobile", "sessions_total", "review_count", "rating", "sp_spend", "sp_sales", "sp_orders", "sp_impressions", "sp_clicks", "sd_spend", "sd_sales", "sd_orders", "sd_impressions", "sd_clicks", "hsa_spend", "hsa_sales", "hsa_orders", "hsa_impressions", "hsa_clicks", "sb_spend", "sb_sales", "sb_orders", "sb_impressions", "sb_clicks"}
}

// CustomerReturnsSchemaRequirements is the exact listing-daily schema touched
// after a formal Customer Returns report has been downloaded and retained.
func CustomerReturnsSchemaRequirements() map[string][]string {
	metricColumns := []string{"listing_dimension_id", "business_date"}
	for _, field := range knownMetricFieldNames() {
		metricColumns = append(metricColumns, field, field+"_source")
	}
	metricColumns = append(metricColumns, "is_provisional", "is_verified", "verified_fields", "report_verified_at")
	return map[string][]string{
		"listing_dimensions": {
			"id", "store_id", "channel", "identity_scope", "identity_key", "asin", "sku",
		},
		"listing_daily_metrics": metricColumns,
		"listing_daily_reconciliations": {
			"report_audit_id", "report_task_id", "business_date", "status",
			"missing_in_db", "missing_in_report", "field_diffs", "error_message", "updated_at",
		},
	}
}

func metricField(values Values, field string) any {
	switch field {
	case "sales_units":
		return values.SalesUnits
	case "sales_amount":
		return values.SalesAmount
	case "returns_qty":
		return values.ReturnsQty
	case "inventory_sellable":
		return values.InventorySellable
	case "inventory_inbound":
		return values.InventoryInbound
	case "inventory_reserved":
		return values.InventoryReserved
	case "inventory_unfulfillable":
		return values.InventoryUnfulfillable
	case "inventory_local_warehouse":
		return values.InventoryLocalWarehouse
	case "inventory_unhealthy_units":
		return values.InventoryUnhealthyUnits
	case "inventory_aged90_sellable_units":
		return values.InventoryAged90SellableUnits
	case "inventory_sell_through_rate":
		return values.InventorySellThroughRate
	case "inventory_receive_fill_rate":
		return values.InventoryReceiveFillRate
	case "inventory_vendor_confirmation_rate":
		return values.InventoryVendorConfirmationRate
	case "inventory_avg_lead_time_days":
		return values.InventoryAvgLeadTimeDays
	case "inventory_sellable_cost":
		return values.InventorySellableCost
	case "inventory_unfulfillable_cost":
		return values.InventoryUnfulfillableCost
	case "inventory_aged90_cost":
		return values.InventoryAged90Cost
	case "inventory_unhealthy_cost":
		return values.InventoryUnhealthyCost
	case "inventory_inbound_cost":
		return values.InventoryInboundCost
	case "inventory_currency":
		return values.InventoryCurrency
	case "inventory_inbound_receiving":
		return values.InventoryInboundReceiving
	case "inventory_inbound_shipped":
		return values.InventoryInboundShipped
	case "inventory_inbound_working":
		return values.InventoryInboundWorking
	case "inventory_reserved_customer_orders":
		return values.InventoryReservedCustomerOrders
	case "inventory_reserved_fc_processing":
		return values.InventoryReservedFCProcessing
	case "inventory_reserved_fc_transfers":
		return values.InventoryReservedFCTransfers
	case "sessions_desktop":
		return values.SessionsDesktop
	case "sessions_mobile":
		return values.SessionsMobile
	case "sessions_total":
		return values.SessionsTotal
	case "review_count":
		return values.ReviewCount
	case "rating":
		return values.Rating
	case "sp_spend":
		return values.SPSpend
	case "sp_sales":
		return values.SPSales
	case "sp_orders":
		return values.SPOrders
	case "sp_impressions":
		return values.SPImpressions
	case "sp_clicks":
		return values.SPClicks
	case "sd_spend":
		return values.SDSpend
	case "sd_sales":
		return values.SDSales
	case "sd_orders":
		return values.SDOrders
	case "sd_impressions":
		return values.SDImpressions
	case "sd_clicks":
		return values.SDClicks
	case "hsa_spend":
		return values.HSASpend
	case "hsa_sales":
		return values.HSASales
	case "hsa_orders":
		return values.HSAOrders
	case "hsa_impressions":
		return values.HSAImpressions
	case "hsa_clicks":
		return values.HSAClicks
	case "sb_spend":
		return values.SBSpend
	case "sb_sales":
		return values.SBSales
	case "sb_orders":
		return values.SBOrders
	case "sb_impressions":
		return values.SBImpressions
	case "sb_clicks":
		return values.SBClicks
	default:
		return nil
	}
}

func sameKnownValue(database, report any) bool {
	if report == nil {
		return true
	}
	switch reportValue := report.(type) {
	case *int64:
		if reportValue == nil {
			return true
		}
		databaseValue, ok := database.(*int64)
		return ok && databaseValue != nil && *databaseValue == *reportValue
	case *float64:
		if reportValue == nil {
			return true
		}
		databaseValue, ok := database.(*float64)
		return ok && databaseValue != nil && *databaseValue == *reportValue
	case *string:
		if reportValue == nil {
			return true
		}
		databaseValue, ok := database.(*string)
		return ok && databaseValue != nil && *databaseValue == *reportValue
	default:
		return false
	}
}
