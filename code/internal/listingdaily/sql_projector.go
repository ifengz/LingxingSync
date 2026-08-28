package listingdaily

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// CoverageUnknown records a raw row that could not be assigned a stable
// listing identity. It is intentionally not converted into a guessed metric.
type CoverageUnknown struct {
	Source string
	Store  string
	ASIN   string
	Date   time.Time
	Reason string
}

type SQLProjection struct {
	Records        []RawRecord
	Unknown        []CoverageUnknown
	Reconciliation *Reconciliation
}

type ReportEvidence struct {
	AuditID      int64
	ReportTaskID string
	ReportType   string
}

type ReconciliationStatus string

const (
	ReconciliationCorrected ReconciliationStatus = "corrected"
	ReconciliationMatched   ReconciliationStatus = "matched"
	ReconciliationFailed    ReconciliationStatus = "failed"
)

type ReconciliationAudit struct {
	Evidence       ReportEvidence
	BusinessDate   time.Time
	Status         ReconciliationStatus
	Reconciliation Reconciliation
	ErrorMessage   string
}

type ReconciliationStore interface {
	Store
	PersistReportBatch(context.Context, []Metric, []ReconciliationAudit) error
	PersistFailedReconciliations(context.Context, []ReconciliationAudit) error
}

const reportReturnsSQL = "SELECT raw.asin, raw.sku, CAST(SUM(CAST(raw.quantity AS SIGNED)) AS CHAR) AS quantity\n" +
	"FROM ls_fba_fulfillment_customer_returns raw\n" +
	"JOIN ls_report_export_tasks task ON task.report_task_id = raw.report_task_id\n" +
	"WHERE task.id = ? AND task.report_task_id = ? AND raw.account_id = ? AND raw.store_id = ?\n" +
	"  AND task.account_id = raw.account_id AND task.store_id = raw.store_id\n" +
	"  AND task.status = 'SUCCESS'\n" +
	"  AND LEFT(task.date_from, 10) <= ? AND LEFT(task.date_to, 10) >= ?\n" +
	"  AND LEFT(raw.`return-date`, 10) = ?\n" +
	"GROUP BY raw.asin, raw.sku"

const reportShipmentSalesSQL = "SELECT raw.asin, raw.sku, CAST(SUM(CAST(raw.quantity AS SIGNED)) AS CHAR) AS quantity\n" +
	"FROM ls_fba_fulfillment_customer_shipment_sales raw\n" +
	"JOIN ls_report_export_tasks task ON task.report_task_id = raw.report_task_id\n" +
	"WHERE task.id = ? AND task.report_task_id = ? AND raw.account_id = ? AND raw.store_id = ?\n" +
	"  AND task.account_id = raw.account_id AND task.store_id = raw.store_id\n" +
	"  AND task.status = 'SUCCESS'\n" +
	"  AND LEFT(task.date_from, 10) <= ? AND LEFT(task.date_to, 10) >= ?\n" +
	"  AND LEFT(raw.`shipment-date`, 10) = ?\n" +
	"GROUP BY raw.asin, raw.sku"

const reportFBAInventorySQL = "SELECT raw.asin, raw.sku, " +
	"CAST(SUM(CAST(raw.`afn-fulfillable-quantity` AS SIGNED)) AS CHAR) AS sellable, " +
	"CAST(SUM(CAST(raw.`afn-unsellable-quantity` AS SIGNED)) AS CHAR) AS unfulfillable, " +
	"CAST(SUM(CAST(raw.`afn-reserved-quantity` AS SIGNED)) AS CHAR) AS reserved, " +
	"CAST(SUM(CAST(raw.`afn-inbound-working-quantity` AS SIGNED)) AS CHAR) AS inbound_working, " +
	"CAST(SUM(CAST(raw.`afn-inbound-shipped-quantity` AS SIGNED)) AS CHAR) AS inbound_shipped, " +
	"CAST(SUM(CAST(raw.`afn-inbound-receiving-quantity` AS SIGNED)) AS CHAR) AS inbound_receiving\n" +
	"FROM ls_fba_myi_unsuppressed_inventory raw\n" +
	"JOIN ls_report_export_tasks task ON task.report_task_id = raw.report_task_id\n" +
	"WHERE task.id = ? AND task.report_task_id = ? AND task.report_type = ? AND raw.account_id = ? AND raw.store_id = ?\n" +
	"  AND task.account_id = raw.account_id AND task.store_id = raw.store_id\n" +
	"  AND task.status = 'SUCCESS'\n" +
	"GROUP BY raw.asin, raw.sku"

const reportFBAAllInventorySQL = "SELECT raw.asin, raw.sku, " +
	"CAST(SUM(CAST(raw.`afn-fulfillable-quantity` AS SIGNED)) AS CHAR) AS sellable, " +
	"CAST(SUM(CAST(raw.`afn-unsellable-quantity` AS SIGNED)) AS CHAR) AS unfulfillable, " +
	"CAST(SUM(CAST(raw.`afn-reserved-quantity` AS SIGNED)) AS CHAR) AS reserved, " +
	"CAST(SUM(CAST(raw.`afn-inbound-working-quantity` AS SIGNED)) AS CHAR) AS inbound_working, " +
	"CAST(SUM(CAST(raw.`afn-inbound-shipped-quantity` AS SIGNED)) AS CHAR) AS inbound_shipped, " +
	"CAST(SUM(CAST(raw.`afn-inbound-receiving-quantity` AS SIGNED)) AS CHAR) AS inbound_receiving\n" +
	"FROM ls_fba_myi_all_inventory raw\n" +
	"JOIN ls_report_export_tasks task ON task.report_task_id = raw.report_task_id\n" +
	"WHERE task.id = ? AND task.report_task_id = ? AND task.report_type = ? AND raw.account_id = ? AND raw.store_id = ?\n" +
	"  AND task.account_id = raw.account_id AND task.store_id = raw.store_id\n" +
	"  AND task.status = 'SUCCESS'\n" +
	"GROUP BY raw.asin, raw.sku"

const reportReservedInventorySQL = "SELECT raw.asin, raw.sku, " +
	"CAST(SUM(CAST(raw.reserved_qty AS SIGNED)) AS CHAR) AS reserved, " +
	"CAST(SUM(CAST(raw.reserved_customerorders AS SIGNED)) AS CHAR) AS reserved_customer_orders, " +
	"CAST(SUM(CAST(raw.`reserved_fc-transfers` AS SIGNED)) AS CHAR) AS reserved_fc_transfers, " +
	"CAST(SUM(CAST(raw.`reserved_fc-processing` AS SIGNED)) AS CHAR) AS reserved_fc_processing\n" +
	"FROM ls_fba_reserved_inventory raw\n" +
	"JOIN ls_report_export_tasks task ON task.report_task_id = raw.report_task_id\n" +
	"WHERE task.id = ? AND task.report_task_id = ? AND task.report_type = ? AND raw.account_id = ? AND raw.store_id = ?\n" +
	"  AND task.account_id = raw.account_id AND task.store_id = raw.store_id\n" +
	"  AND task.status = 'SUCCESS'\n" +
	"GROUP BY raw.asin, raw.sku"

const reportAFNInventorySQL = "SELECT raw.asin, raw.`seller-sku` AS sku, " +
	"CAST(SUM(CAST(raw.`Quantity Available` AS SIGNED)) AS CHAR) AS sellable\n" +
	"FROM ls_afn_inventory raw\n" +
	"JOIN ls_report_export_tasks task ON task.report_task_id = raw.report_task_id\n" +
	"WHERE task.id = ? AND task.report_task_id = ? AND task.report_type = ? AND raw.account_id = ? AND raw.store_id = ?\n" +
	"  AND task.account_id = raw.account_id AND task.store_id = raw.store_id\n" +
	"  AND task.status = 'SUCCESS'\n" +
	"GROUP BY raw.asin, raw.`seller-sku`"

const vcSalesSQL = "SELECT asin, shippedUnits, shippedRevenueAmount, customerReturns FROM ls_vc_sales_report WHERE account_id = ? AND sid = ? AND `date` = ?"

// SQLSourceReader reads only the already-retained raw evidence tables. The
// channel is explicit because several raw contracts do not carry fulfillment
// channel in each row.
type SQLSourceReader struct{ DB *sqlx.DB }

type SourceReader interface {
	Read(context.Context, string, string, string, time.Time) (SQLProjection, error)
}

type ReportSourceReader interface {
	ReadReportReturns(context.Context, string, string, string, time.Time, ReportEvidence) ([]RawRecord, error)
}

type ReportSalesSourceReader interface {
	ReadReportSales(context.Context, string, string, string, time.Time, ReportEvidence) ([]RawRecord, error)
}

type ReportInventorySourceReader interface {
	ReadReportInventory(context.Context, string, string, string, time.Time, ReportEvidence) ([]RawRecord, error)
}

func (r SQLSourceReader) Read(ctx context.Context, accountID, storeID, channel string, businessDate time.Time) (SQLProjection, error) {
	if r.DB == nil {
		return SQLProjection{}, fmt.Errorf("listing daily: nil source database")
	}
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(storeID) == "" || strings.TrimSpace(channel) == "" {
		return SQLProjection{}, fmt.Errorf("listing daily: account, store, and channel are required")
	}
	date := calendarDate(businessDate)
	projection := SQLProjection{}
	switch strings.ToLower(channel) {
	case "sc_fba":
		sku, err := r.listingSKUs(ctx, accountID, storeID)
		if err != nil {
			return projection, err
		}
		if err := r.readSCSales(ctx, &projection, accountID, storeID, channel, date, sku); err != nil {
			return projection, err
		}
		if err := r.readReturns(ctx, &projection, accountID, storeID, channel, date, sku); err != nil {
			return projection, err
		}
		if err := r.readSCPerformance(ctx, &projection, accountID, storeID, channel, date, sku); err != nil {
			return projection, err
		}
		if err := r.readInventory(ctx, &projection, accountID, storeID, channel, date, sku); err != nil {
			return projection, err
		}
		if err := r.readAds(ctx, &projection, accountID, storeID, channel, date, sku); err != nil {
			return projection, err
		}
	case "vc":
		skus, invalid, err := r.vcListingSKUs(ctx, accountID, storeID)
		if err != nil {
			return projection, err
		}
		if err := r.readVCSales(ctx, &projection, accountID, storeID, channel, date, skus, invalid); err != nil {
			return projection, err
		}
		if err := r.readVCInventory(ctx, &projection, accountID, storeID, channel, date, skus, invalid); err != nil {
			return projection, err
		}
		projection.Unknown = append(projection.Unknown, CoverageUnknown{"ls_vc_traffic", storeID, "", date, "glanceViews has no authorized listing-daily target field"})
	case "hsa":
		if err := r.readHSA(ctx, &projection, accountID, storeID, channel, date); err != nil {
			return projection, err
		}
	case "sb":
		projection.Unknown = append(projection.Unknown, CoverageUnknown{"sb", storeID, "", date, "no verified SB ASIN/SKU raw source"})
	default:
		return projection, fmt.Errorf("listing daily: unsupported channel %q", channel)
	}
	return projection, nil
}

func ProjectAndPublishFromSQL(ctx context.Context, reader SourceReader, store Store, accountID, storeID, channel string, businessDate, today time.Time, reportState ReportState, evidence ...ReportEvidence) (SQLProjection, error) {
	if store == nil {
		return SQLProjection{}, fmt.Errorf("listing daily: nil store")
	}
	projection, rows, err := BuildFromSQL(ctx, reader, accountID, storeID, channel, businessDate, today, reportState, evidence...)
	if err != nil {
		return projection, err
	}
	return projection, store.Persist(ctx, rows)
}

func BuildFromSQL(ctx context.Context, reader SourceReader, accountID, storeID, channel string, businessDate, today time.Time, reportState ReportState, evidence ...ReportEvidence) (SQLProjection, []Metric, error) {
	projection, err := reader.Read(ctx, accountID, storeID, channel, businessDate)
	if err != nil {
		return projection, nil, err
	}
	var reportRaw []RawRecord
	if reportState == ReportReconciled {
		if len(evidence) != 1 || evidence[0].AuditID <= 0 || strings.TrimSpace(evidence[0].ReportTaskID) == "" {
			return projection, nil, fmt.Errorf("listing daily: reconciled report requires exact audit and task evidence")
		}
		reportType := evidence[0].ReportType
		if strings.TrimSpace(reportType) == "" {
			reportType = "GET_FBA_FULFILLMENT_CUSTOMER_RETURNS_DATA"
		}
		fields := []string{"returns_qty"}
		if reportType == "GET_FBA_FULFILLMENT_CUSTOMER_SHIPMENT_SALES_DATA" {
			fields = []string{"sales_units"}
			reportReader, ok := reader.(ReportSalesSourceReader)
			if !ok {
				return projection, nil, fmt.Errorf("listing daily: shipment sales report requires report raw reader")
			}
			reportRaw, err = reportReader.ReadReportSales(ctx, accountID, storeID, channel, businessDate, evidence[0])
		} else if isInventoryReportType(reportType) {
			fields = inventoryReportFields(reportType)
			reportReader, ok := reader.(ReportInventorySourceReader)
			if !ok {
				return projection, nil, fmt.Errorf("listing daily: inventory report requires report raw reader")
			}
			reportRaw, err = reportReader.ReadReportInventory(ctx, accountID, storeID, channel, businessDate, evidence[0])
		} else if reportType == "GET_FBA_FULFILLMENT_CUSTOMER_RETURNS_DATA" {
			reportReader, ok := reader.(ReportSourceReader)
			if !ok {
				return projection, nil, fmt.Errorf("listing daily: reconciled report requires report raw reader")
			}
			reportRaw, err = reportReader.ReadReportReturns(ctx, accountID, storeID, channel, businessDate, evidence[0])
		} else {
			return projection, nil, fmt.Errorf("listing daily: unsupported reconciled report type %q", reportType)
		}
		if err != nil {
			return projection, nil, err
		}
		if isInventoryReportType(reportType) {
			fields = reportFieldsPresent(fields, metricsFromRaw(reportRaw))
		}
		apiMetrics := metricsWithFields(metricsFromRaw(projection.Records), fields)
		reportMetrics := metricsWithFields(metricsFromRaw(reportRaw), fields)
		reconciliation, reconcileErr := ReconcileFields(apiMetrics, reportMetrics, fields)
		if reconcileErr != nil {
			return projection, nil, reconcileErr
		}
		projection.Reconciliation = &reconciliation
		if len(apiMetrics) > 0 && len(reportMetrics) == 0 {
			label := reportMetricLabel(reportType)
			return projection, nil, fmt.Errorf("listing daily: report reconciliation failed: report has no %s rows while API has %d", label, len(apiMetrics))
		}
	}
	if reportState != ReportReconciled && len(evidence) != 0 {
		return projection, nil, fmt.Errorf("listing daily: report evidence requires reconciled state")
	}
	rows, err := Build(projection.Records, reportRaw, reportState, today)
	return projection, rows, err
}

func metricsFromRaw(records []RawRecord) []Metric {
	rows := make([]Metric, 0, len(records))
	for _, record := range records {
		rows = append(rows, Metric{Key: record.Input.Key, Scope: record.Input.Scope, Values: record.Input.Values})
	}
	return rows
}

func metricsWithFields(rows []Metric, fields []string) []Metric {
	result := make([]Metric, 0, len(rows))
	for _, row := range rows {
		values := Values{}
		for _, field := range fields {
			setMetricField(&values, field, metricField(row.Values, field))
		}
		if len(knownFields(values)) == 0 {
			continue
		}
		result = append(result, Metric{Key: row.Key, Scope: row.Scope, Values: values})
	}
	return result
}

func reportFieldsPresent(fields []string, rows []Metric) []string {
	present := make([]string, 0, len(fields))
	for _, field := range fields {
		for _, row := range rows {
			if valueFieldPresent(row.Values, field) {
				present = append(present, field)
				break
			}
		}
	}
	return present
}

func valueFieldPresent(values Values, field string) bool {
	for _, known := range knownFields(values) {
		if known == field {
			return true
		}
	}
	return false
}

func setMetricField(values *Values, field string, value any) {
	switch field {
	case "sales_units":
		values.SalesUnits, _ = value.(*int64)
	case "returns_qty":
		values.ReturnsQty, _ = value.(*int64)
	case "inventory_sellable":
		values.InventorySellable, _ = value.(*int64)
	case "inventory_unfulfillable":
		values.InventoryUnfulfillable, _ = value.(*int64)
	case "inventory_reserved":
		values.InventoryReserved, _ = value.(*int64)
	case "inventory_inbound_working":
		values.InventoryInboundWorking, _ = value.(*int64)
	case "inventory_inbound_shipped":
		values.InventoryInboundShipped, _ = value.(*int64)
	case "inventory_inbound_receiving":
		values.InventoryInboundReceiving, _ = value.(*int64)
	case "inventory_reserved_customer_orders":
		values.InventoryReservedCustomerOrders, _ = value.(*int64)
	case "inventory_reserved_fc_processing":
		values.InventoryReservedFCProcessing, _ = value.(*int64)
	case "inventory_reserved_fc_transfers":
		values.InventoryReservedFCTransfers, _ = value.(*int64)
	}
}

func isInventoryReportType(reportType string) bool {
	switch reportType {
	case "GET_FBA_MYI_UNSUPPRESSED_INVENTORY_DATA", "GET_FBA_MYI_ALL_INVENTORY_DATA", "GET_RESERVED_INVENTORY_DATA", "GET_AFN_INVENTORY_DATA":
		return true
	default:
		return false
	}
}

func inventoryReportFields(reportType string) []string {
	switch reportType {
	case "GET_FBA_MYI_UNSUPPRESSED_INVENTORY_DATA", "GET_FBA_MYI_ALL_INVENTORY_DATA":
		return []string{"inventory_sellable", "inventory_unfulfillable", "inventory_reserved", "inventory_inbound_working", "inventory_inbound_shipped", "inventory_inbound_receiving"}
	case "GET_RESERVED_INVENTORY_DATA":
		return []string{"inventory_reserved", "inventory_reserved_customer_orders", "inventory_reserved_fc_transfers", "inventory_reserved_fc_processing"}
	case "GET_AFN_INVENTORY_DATA":
		return []string{"inventory_sellable"}
	default:
		return nil
	}
}

func reportMetricLabel(reportType string) string {
	switch reportType {
	case "GET_FBA_FULFILLMENT_CUSTOMER_SHIPMENT_SALES_DATA":
		return "sales"
	case "GET_FBA_FULFILLMENT_CUSTOMER_RETURNS_DATA":
		return "returns"
	case "GET_FBA_MYI_UNSUPPRESSED_INVENTORY_DATA", "GET_FBA_MYI_ALL_INVENTORY_DATA", "GET_RESERVED_INVENTORY_DATA", "GET_AFN_INVENTORY_DATA":
		return "inventory"
	default:
		return "report"
	}
}

func (r SQLSourceReader) listingSKUs(ctx context.Context, accountID, storeID string) (map[string]string, error) {
	var rows []struct {
		ASIN string `db:"asin"`
		SKU  string `db:"seller_sku"`
	}
	if err := r.DB.SelectContext(ctx, &rows, `SELECT asin, seller_sku FROM ls_sc_listing WHERE account_id = ? AND sid = ? AND asin IS NOT NULL AND asin <> '' AND seller_sku IS NOT NULL AND seller_sku <> ''`, accountID, storeID); err != nil {
		return nil, fmt.Errorf("listing daily: read ls_sc_listing: %w", err)
	}
	result := make(map[string]string, len(rows))
	for _, row := range rows {
		if old, exists := result[row.ASIN]; exists && old != row.SKU {
			result[row.ASIN] = ""
			continue
		}
		result[row.ASIN] = row.SKU
	}
	return result, nil
}

func (r SQLSourceReader) vcListingSKUs(ctx context.Context, accountID, storeID string) (map[string]string, map[string]string, error) {
	var rows []struct {
		ASIN     string         `db:"asin"`
		MSKU     sql.NullString `db:"msku"`
		LocalSKU sql.NullString `db:"local_sku"`
	}
	if err := r.DB.SelectContext(ctx, &rows, `SELECT asin, msku, local_sku FROM ls_vc_listing WHERE account_id = ? AND vc_store_id = ? AND asin IS NOT NULL AND asin <> ''`, accountID, storeID); err != nil {
		return nil, nil, fmt.Errorf("listing daily: read ls_vc_listing: %w", err)
	}
	skus := make(map[string]string, len(rows))
	invalid := make(map[string]string)
	for _, row := range rows {
		sku, err := uniqueVCListingSKU(row.MSKU, row.LocalSKU)
		if err != nil {
			invalid[row.ASIN] = err.Error()
			continue
		}
		skus[row.ASIN] = sku
	}
	return skus, invalid, nil
}

func uniqueVCListingSKU(msku, localSKU sql.NullString) (string, error) {
	unique := make(map[string]struct{}, 2)
	for _, raw := range []sql.NullString{msku, localSKU} {
		if !raw.Valid || strings.TrimSpace(raw.String) == "" {
			continue
		}
		unique[strings.TrimSpace(raw.String)] = struct{}{}
	}
	if len(unique) == 0 {
		return "", fmt.Errorf("missing unique VC listing msku/local_sku")
	}
	if len(unique) != 1 {
		return "", fmt.Errorf("multiple VC listing msku/local_sku values")
	}
	for sku := range unique {
		return sku, nil
	}
	return "", fmt.Errorf("missing unique VC listing msku/local_sku")
}

func (r SQLSourceReader) readVCSales(ctx context.Context, out *SQLProjection, accountID, storeID, channel string, date time.Time, skus, invalid map[string]string) error {
	var rows []struct {
		ASIN    string          `db:"asin"`
		Units   sql.NullInt64   `db:"shippedUnits"`
		Revenue sql.NullFloat64 `db:"shippedRevenueAmount"`
		Returns sql.NullInt64   `db:"customerReturns"`
	}
	if err := r.DB.SelectContext(ctx, &rows, vcSalesSQL, accountID, storeID, date.Format("2006-01-02")); err != nil {
		return fmt.Errorf("listing daily: read ls_vc_sales_report: %w", err)
	}
	for _, row := range rows {
		sku, ok := skus[row.ASIN]
		if row.ASIN == "" || !ok {
			reason := "missing same-store ls_vc_listing msku/local_sku"
			if detail, exists := invalid[row.ASIN]; exists {
				reason = detail
			}
			out.Unknown = append(out.Unknown, CoverageUnknown{"ls_vc_sales_report", storeID, row.ASIN, date, reason})
			continue
		}
		values := Values{SalesUnits: nullableInt(row.Units), SalesAmount: nullableFloat(row.Revenue), ReturnsQty: nullableInt(row.Returns)}
		if len(knownFields(values)) == 0 {
			out.Unknown = append(out.Unknown, CoverageUnknown{"ls_vc_sales_report", storeID, row.ASIN, date, "no supported VC sales metric values"})
			continue
		}
		out.Records = append(out.Records, RawRecord{Source: SourceAPI, Input: Input{Key: Key{Store: storeID, Channel: channel, ASIN: row.ASIN, SKU: sku, BusinessDate: date}, Scope: ScopeListing, Values: values}})
	}
	return nil
}

func (r SQLSourceReader) readSCSales(ctx context.Context, out *SQLProjection, accountID, storeID, channel string, date time.Time, skus map[string]string) error {
	var units []struct {
		ASIN  string         `db:"asin"`
		Date  string         `db:"r_date"`
		Value sql.NullString `db:"map_value"`
	}
	if err := r.DB.SelectContext(ctx, &units, "SELECT asin, r_date, map_value FROM ls_sc_sales_report WHERE account_id = ? AND sid = ? AND r_date = ?", accountID, storeID, date.Format("2006-01-02")); err != nil {
		return fmt.Errorf("listing daily: read ls_sc_sales_report: %w", err)
	}
	for _, row := range units {
		sku := skus[row.ASIN]
		if sku == "" {
			out.Unknown = append(out.Unknown, CoverageUnknown{"ls_sc_sales_report", storeID, row.ASIN, date, "missing or ambiguous ls_sc_listing seller_sku"})
			continue
		}
		value, err := integer(row.Value)
		if err != nil {
			return fmt.Errorf("listing daily: parse SC sales quantity asin=%s: %w", row.ASIN, err)
		}
		out.Records = append(out.Records, RawRecord{Source: SourceAPI, Input: Input{Key: Key{Store: storeID, Channel: channel, ASIN: row.ASIN, SKU: sku, BusinessDate: date}, Scope: ScopeListing, Values: Values{SalesUnits: &value}}})
	}
	var revenue []struct {
		ASIN  string         `db:"asin"`
		Date  string         `db:"r_date"`
		Value sql.NullString `db:"map_value"`
	}
	if err := r.DB.SelectContext(ctx, &revenue, "SELECT asin, r_date, map_value FROM ls_sc_sales_revenue WHERE account_id = ? AND sid = ? AND r_date = ?", accountID, storeID, date.Format("2006-01-02")); err != nil {
		return fmt.Errorf("listing daily: read ls_sc_sales_revenue: %w", err)
	}
	for _, row := range revenue {
		sku := skus[row.ASIN]
		if sku == "" {
			out.Unknown = append(out.Unknown, CoverageUnknown{"ls_sc_sales_revenue", storeID, row.ASIN, date, "missing or ambiguous ls_sc_listing seller_sku"})
			continue
		}
		value, err := decimal(row.Value)
		if err != nil {
			return fmt.Errorf("listing daily: parse SC sales amount asin=%s: %w", row.ASIN, err)
		}
		out.Records = append(out.Records, RawRecord{Source: SourceAPI, Input: Input{Key: Key{Store: storeID, Channel: channel, ASIN: row.ASIN, SKU: sku, BusinessDate: date}, Scope: ScopeListing, Values: Values{SalesAmount: &value}}})
	}
	return nil
}

func (r SQLSourceReader) readReturns(ctx context.Context, out *SQLProjection, accountID, storeID, channel string, date time.Time, skus map[string]string) error {
	var rows []struct {
		ASIN     string        `db:"asin"`
		SKU      string        `db:"sku"`
		Quantity sql.NullInt64 `db:"quantity"`
	}
	if err := r.DB.SelectContext(ctx, &rows, "SELECT asin, COALESCE(NULLIF(sku, ''), NULLIF(local_sku, '')) AS sku, SUM(quantity) AS quantity FROM ls_sc_refunds WHERE account_id = ? AND sid = ? AND return_date_locale = ? GROUP BY asin, COALESCE(NULLIF(sku, ''), NULLIF(local_sku, ''))", accountID, storeID, date.Format("2006-01-02")); err != nil {
		return fmt.Errorf("listing daily: read ls_sc_refunds: %w", err)
	}
	for _, row := range rows {
		sku := row.SKU
		if sku == "" {
			sku = skus[row.ASIN]
		}
		if row.ASIN == "" || sku == "" {
			out.Unknown = append(out.Unknown, CoverageUnknown{"ls_sc_refunds", storeID, row.ASIN, date, "missing listing identity"})
			continue
		}
		value := row.Quantity.Int64
		out.Records = append(out.Records, RawRecord{Source: SourceAPI, Input: Input{Key: Key{Store: storeID, Channel: channel, ASIN: row.ASIN, SKU: sku, BusinessDate: date}, Scope: ScopeListing, Values: Values{ReturnsQty: &value}}})
	}
	return nil
}

const scPerformanceDailySQL = "SELECT asin, sessions, sessions_mobile, sessions_total, reviews_count, avg_star FROM ls_sc_performance_daily WHERE account_id = ? AND sid = ? AND business_date = ?"

func (r SQLSourceReader) readSCPerformance(ctx context.Context, out *SQLProjection, accountID, storeID, channel string, date time.Time, skus map[string]string) error {
	var rows []struct {
		ASIN           string          `db:"asin"`
		Sessions       sql.NullInt64   `db:"sessions"`
		SessionsMobile sql.NullInt64   `db:"sessions_mobile"`
		SessionsTotal  sql.NullInt64   `db:"sessions_total"`
		ReviewsCount   sql.NullInt64   `db:"reviews_count"`
		AvgStar        sql.NullFloat64 `db:"avg_star"`
	}
	if err := r.DB.SelectContext(ctx, &rows, scPerformanceDailySQL, accountID, storeID, date.Format("2006-01-02")); err != nil {
		return fmt.Errorf("listing daily: read ls_sc_performance_daily: %w", err)
	}
	for _, row := range rows {
		sku := skus[row.ASIN]
		if row.ASIN == "" || sku == "" {
			out.Unknown = append(out.Unknown, CoverageUnknown{"ls_sc_performance_daily", storeID, row.ASIN, date, "missing or ambiguous ls_sc_listing seller_sku"})
			continue
		}
		out.Records = append(out.Records, RawRecord{Source: SourceAPI, Input: Input{Key: Key{Store: storeID, Channel: channel, ASIN: row.ASIN, SKU: sku, BusinessDate: date}, Scope: ScopeListing, Values: scPerformanceValues(row.Sessions, row.SessionsMobile, row.SessionsTotal, row.ReviewsCount, row.AvgStar)}})
	}
	return nil
}

func scPerformanceValues(sessions, sessionsMobile, sessionsTotal, reviewsCount sql.NullInt64, avgStar sql.NullFloat64) Values {
	return Values{SessionsDesktop: nullableInt(sessions), SessionsMobile: nullableInt(sessionsMobile), SessionsTotal: nullableInt(sessionsTotal), ReviewCount: nullableInt(reviewsCount), Rating: nullableFloat(avgStar)}
}

const scInventorySQL = "SELECT asin, sku, fnsku, afn_fulfillable_quantity, afn_inbound_receiving_quantity, afn_inbound_shipped_quantity, afn_inbound_working_quantity, reserved_customerorders, reserved_fc_processing, reserved_fc_transfers, afn_unsellable_quantity FROM ls_fba_inventory WHERE account_id = ? AND sid = ? AND DATE(synced_at) = ?"

const vcInventorySQL = "SELECT asin, sellableOnHandInventoryUnits, unsellableOnHandInventoryUnits, netReceivedInventoryUnits, unhealthyInventoryUnits, aged90PlusDaysSellableInventoryUnits, sellThroughRate, receiveFillRate, vendorConfirmationRate, averageVendorLeadTimeDays, sellableOnHandInventoryCostAmount, unsellableOnHandInventoryCostAmount, aged90PlusDaysSellableInventoryCostAmount, unhealthyInventoryCostAmount, netReceivedInventoryCostAmount, sellableOnHandInventoryCostCurrencyCode, unsellableOnHandInventoryCostCurrencyCode, aged90PlusDaysSellableInventoryCostCurrencyCode, unhealthyInventoryCostCurrencyCode, netReceivedInventoryCostCurrencyCode FROM ls_vc_inventory WHERE account_id = ? AND sid = ? AND `date` = ?"

func (r SQLSourceReader) readInventory(ctx context.Context, out *SQLProjection, accountID, storeID, channel string, date time.Time, skus map[string]string) error {
	if !sameCalendarDate(date, time.Now().UTC()) {
		out.Unknown = append(out.Unknown, CoverageUnknown{"ls_fba_inventory", storeID, "", date, "inventory raw table is current-state; only today's sync snapshot is eligible"})
		return nil
	}
	out.Unknown = append(out.Unknown,
		CoverageUnknown{"ls_fba_inventory.inbound", storeID, "", date, "raw source provides components only; no total inbound field"},
		CoverageUnknown{"ls_fba_inventory.reserved", storeID, "", date, "raw source provides components only; no total reserved field"},
		CoverageUnknown{"ls_fba_inventory.local_warehouse", storeID, "", date, "no local warehouse field in current-state FBA raw source"},
	)
	var rows []scInventoryRow
	if err := r.DB.SelectContext(ctx, &rows, scInventorySQL, accountID, storeID, date); err != nil {
		return fmt.Errorf("listing daily: read ls_fba_inventory: %w", err)
	}
	records, unknown := aggregateSCInventoryRows(rows, storeID, channel, date, skus)
	out.Records = append(out.Records, records...)
	out.Unknown = append(out.Unknown, unknown...)
	return nil
}

type scInventoryRow struct {
	ASIN               string        `db:"asin"`
	SKU                string        `db:"sku"`
	FNSKU              string        `db:"fnsku"`
	Sellable           sql.NullInt64 `db:"afn_fulfillable_quantity"`
	InboundReceiving   sql.NullInt64 `db:"afn_inbound_receiving_quantity"`
	InboundShipped     sql.NullInt64 `db:"afn_inbound_shipped_quantity"`
	InboundWorking     sql.NullInt64 `db:"afn_inbound_working_quantity"`
	ReservedCustomer   sql.NullInt64 `db:"reserved_customerorders"`
	ReservedProcessing sql.NullInt64 `db:"reserved_fc_processing"`
	ReservedTransfers  sql.NullInt64 `db:"reserved_fc_transfers"`
	Unfulfillable      sql.NullInt64 `db:"afn_unsellable_quantity"`
}

func aggregateSCInventoryRows(rows []scInventoryRow, storeID, channel string, date time.Time, skus map[string]string) ([]RawRecord, []CoverageUnknown) {
	records := make([]RawRecord, 0, len(rows))
	unknown := make([]CoverageUnknown, 0)
	indexes := make(map[string]int, len(rows))
	for _, row := range rows {
		sku := row.SKU
		if sku == "" {
			sku = skus[row.ASIN]
		}
		if row.ASIN == "" || sku == "" {
			unknown = append(unknown, CoverageUnknown{"ls_fba_inventory", storeID, row.ASIN, date, "missing listing identity"})
			continue
		}
		key := Key{Store: storeID, Channel: channel, ASIN: row.ASIN, SKU: sku, BusinessDate: date}
		id := keyID(key, ScopeListing)
		index, exists := indexes[id]
		if !exists {
			index = len(records)
			indexes[id] = index
			records = append(records, RawRecord{Source: SourceAPI, Input: Input{Key: key, Scope: ScopeListing}})
		}
		values := &records[index].Input.Values
		values.InventorySellable = sumNullableInt64(values.InventorySellable, row.Sellable)
		values.InventoryInboundReceiving = sumNullableInt64(values.InventoryInboundReceiving, row.InboundReceiving)
		values.InventoryInboundShipped = sumNullableInt64(values.InventoryInboundShipped, row.InboundShipped)
		values.InventoryInboundWorking = sumNullableInt64(values.InventoryInboundWorking, row.InboundWorking)
		values.InventoryReservedCustomerOrders = sumNullableInt64(values.InventoryReservedCustomerOrders, row.ReservedCustomer)
		values.InventoryReservedFCProcessing = sumNullableInt64(values.InventoryReservedFCProcessing, row.ReservedProcessing)
		values.InventoryReservedFCTransfers = sumNullableInt64(values.InventoryReservedFCTransfers, row.ReservedTransfers)
		values.InventoryUnfulfillable = sumNullableInt64(values.InventoryUnfulfillable, row.Unfulfillable)
	}
	return records, unknown
}

func sumNullableInt64(total *int64, value sql.NullInt64) *int64 {
	if !value.Valid {
		return total
	}
	if total == nil {
		total = new(int64)
	}
	*total += value.Int64
	return total
}

func scInventoryValues(sellable, inboundReceiving, inboundShipped, inboundWorking, reservedCustomer, reservedProcessing, reservedTransfers, unfulfillable sql.NullInt64) Values {
	return Values{
		InventorySellable:               nullableInt(sellable),
		InventoryUnfulfillable:          nullableInt(unfulfillable),
		InventoryInboundReceiving:       nullableInt(inboundReceiving),
		InventoryInboundShipped:         nullableInt(inboundShipped),
		InventoryInboundWorking:         nullableInt(inboundWorking),
		InventoryReservedCustomerOrders: nullableInt(reservedCustomer),
		InventoryReservedFCProcessing:   nullableInt(reservedProcessing),
		InventoryReservedFCTransfers:    nullableInt(reservedTransfers),
	}
}

func (r SQLSourceReader) readVCInventory(ctx context.Context, out *SQLProjection, accountID, storeID, channel string, date time.Time, skus, invalid map[string]string) error {
	out.Unknown = append(out.Unknown,
		CoverageUnknown{"ls_vc_inventory.local_warehouse", storeID, "", date, "VC inventory raw source has no local warehouse field"},
		CoverageUnknown{"ls_vc_inventory.reserved", storeID, "", date, "VC inventory raw source has no reserved quantity field"},
	)
	var rows []struct {
		ASIN                  string          `db:"asin"`
		Sellable              sql.NullInt64   `db:"sellableOnHandInventoryUnits"`
		Unfulfillable         sql.NullInt64   `db:"unsellableOnHandInventoryUnits"`
		Inbound               sql.NullInt64   `db:"netReceivedInventoryUnits"`
		Unhealthy             sql.NullInt64   `db:"unhealthyInventoryUnits"`
		Aged90                sql.NullInt64   `db:"aged90PlusDaysSellableInventoryUnits"`
		SellThrough           sql.NullFloat64 `db:"sellThroughRate"`
		ReceiveFill           sql.NullFloat64 `db:"receiveFillRate"`
		VendorConfirm         sql.NullFloat64 `db:"vendorConfirmationRate"`
		LeadTime              sql.NullFloat64 `db:"averageVendorLeadTimeDays"`
		SellableCost          sql.NullFloat64 `db:"sellableOnHandInventoryCostAmount"`
		UnfulfillableCost     sql.NullFloat64 `db:"unsellableOnHandInventoryCostAmount"`
		Aged90Cost            sql.NullFloat64 `db:"aged90PlusDaysSellableInventoryCostAmount"`
		UnhealthyCost         sql.NullFloat64 `db:"unhealthyInventoryCostAmount"`
		InboundCost           sql.NullFloat64 `db:"netReceivedInventoryCostAmount"`
		SellableCurrency      sql.NullString  `db:"sellableOnHandInventoryCostCurrencyCode"`
		UnfulfillableCurrency sql.NullString  `db:"unsellableOnHandInventoryCostCurrencyCode"`
		Aged90Currency        sql.NullString  `db:"aged90PlusDaysSellableInventoryCostCurrencyCode"`
		UnhealthyCurrency     sql.NullString  `db:"unhealthyInventoryCostCurrencyCode"`
		InboundCurrency       sql.NullString  `db:"netReceivedInventoryCostCurrencyCode"`
	}
	if err := r.DB.SelectContext(ctx, &rows, vcInventorySQL, accountID, storeID, date.Format("2006-01-02")); err != nil {
		return fmt.Errorf("listing daily: read ls_vc_inventory: %w", err)
	}
	for _, row := range rows {
		sku, ok := skus[row.ASIN]
		if row.ASIN == "" || !ok {
			reason := "missing same-store ls_vc_listing msku/local_sku"
			if detail, exists := invalid[row.ASIN]; exists {
				reason = detail
			}
			out.Unknown = append(out.Unknown, CoverageUnknown{"ls_vc_inventory", storeID, row.ASIN, date, reason})
			continue
		}
		currency, err := inventoryCurrency(row.SellableCurrency, row.UnfulfillableCurrency, row.Aged90Currency, row.UnhealthyCurrency, row.InboundCurrency)
		if err != nil {
			return fmt.Errorf("listing daily: ls_vc_inventory asin=%s: %w", row.ASIN, err)
		}
		values := Values{
			InventorySellable:               nullableInt(row.Sellable),
			InventoryInbound:                nullableInt(row.Inbound),
			InventoryUnfulfillable:          nullableInt(row.Unfulfillable),
			InventoryUnhealthyUnits:         nullableInt(row.Unhealthy),
			InventoryAged90SellableUnits:    nullableInt(row.Aged90),
			InventorySellThroughRate:        nullableFloat(row.SellThrough),
			InventoryReceiveFillRate:        nullableFloat(row.ReceiveFill),
			InventoryVendorConfirmationRate: nullableFloat(row.VendorConfirm),
			InventoryAvgLeadTimeDays:        nullableFloat(row.LeadTime),
			InventorySellableCost:           nullableFloat(row.SellableCost),
			InventoryUnfulfillableCost:      nullableFloat(row.UnfulfillableCost),
			InventoryAged90Cost:             nullableFloat(row.Aged90Cost),
			InventoryUnhealthyCost:          nullableFloat(row.UnhealthyCost),
			InventoryInboundCost:            nullableFloat(row.InboundCost),
			InventoryCurrency:               currency,
		}
		out.Records = append(out.Records, RawRecord{Source: SourceAPI, Input: Input{Key: Key{Store: storeID, Channel: channel, ASIN: row.ASIN, SKU: sku, BusinessDate: date}, Scope: ScopeListing, Values: values}})
	}
	return nil
}

func nullableInt(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func nullableFloat(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func inventoryCurrency(values ...sql.NullString) (*string, error) {
	var currency string
	for _, value := range values {
		if !value.Valid || strings.TrimSpace(value.String) == "" {
			continue
		}
		candidate := strings.TrimSpace(value.String)
		if currency == "" {
			currency = candidate
			continue
		}
		if currency != candidate {
			return nil, fmt.Errorf("cost currency mismatch: %q vs %q", currency, candidate)
		}
	}
	if currency == "" {
		return nil, nil
	}
	return &currency, nil
}

func (r SQLSourceReader) readAds(ctx context.Context, out *SQLProjection, accountID, storeID, channel string, date time.Time, skus map[string]string) error {
	queries := []struct {
		table                                     string
		spend, sales, orders, impressions, clicks string
	}{{"ls_ad_sp_product", "cost", "sales", "orders", "impressions", "clicks"}, {"ls_ad_sd_product", "cost", "sales", "orders", "impressions", "clicks"}}
	for _, query := range queries {
		var rows []struct {
			ASIN        string          `db:"asin"`
			SKU         string          `db:"sku"`
			Spend       sql.NullFloat64 `db:"spend"`
			Sales       sql.NullFloat64 `db:"sales"`
			Orders      sql.NullInt64   `db:"orders"`
			Impressions sql.NullInt64   `db:"impressions"`
			Clicks      sql.NullInt64   `db:"clicks"`
		}
		sqlText := fmt.Sprintf("SELECT asin, sku, SUM(%s) AS spend, SUM(%s) AS sales, SUM(%s) AS orders, SUM(%s) AS impressions, SUM(%s) AS clicks FROM %s WHERE account_id = ? AND sid = ? AND report_date = ? GROUP BY asin, sku", query.spend, query.sales, query.orders, query.impressions, query.clicks, query.table)
		if err := r.DB.SelectContext(ctx, &rows, sqlText, accountID, storeID, date); err != nil {
			return fmt.Errorf("listing daily: read %s: %w", query.table, err)
		}
		for _, row := range rows {
			sku := row.SKU
			if sku == "" {
				sku = skus[row.ASIN]
			}
			if row.ASIN == "" || sku == "" {
				out.Unknown = append(out.Unknown, CoverageUnknown{query.table, storeID, row.ASIN, date, "missing listing identity"})
				continue
			}
			values := Values{}
			if query.table == "ls_ad_sp_product" {
				if row.Spend.Valid {
					values.SPSpend = &row.Spend.Float64
				}
				if row.Sales.Valid {
					values.SPSales = &row.Sales.Float64
				}
				if row.Orders.Valid {
					values.SPOrders = &row.Orders.Int64
				}
				if row.Impressions.Valid {
					values.SPImpressions = &row.Impressions.Int64
				}
				if row.Clicks.Valid {
					values.SPClicks = &row.Clicks.Int64
				}
			} else {
				if row.Spend.Valid {
					values.SDSpend = &row.Spend.Float64
				}
				if row.Sales.Valid {
					values.SDSales = &row.Sales.Float64
				}
				if row.Orders.Valid {
					values.SDOrders = &row.Orders.Int64
				}
				if row.Impressions.Valid {
					values.SDImpressions = &row.Impressions.Int64
				}
				if row.Clicks.Valid {
					values.SDClicks = &row.Clicks.Int64
				}
			}
			out.Records = append(out.Records, RawRecord{Source: SourceAPI, Input: Input{Key: Key{Store: storeID, Channel: channel, ASIN: row.ASIN, SKU: sku, BusinessDate: date}, Scope: ScopeListing, Values: values}})
		}
	}
	return nil
}

func (r SQLSourceReader) readHSA(ctx context.Context, out *SQLProjection, accountID, storeID, channel string, date time.Time) error {
	if !strings.EqualFold(channel, "hsa") {
		return nil
	}
	var row struct {
		Spend       sql.NullFloat64 `db:"spend"`
		Sales       sql.NullFloat64 `db:"sales"`
		Orders      sql.NullInt64   `db:"orders"`
		Impressions sql.NullInt64   `db:"impressions"`
		Clicks      sql.NullInt64   `db:"clicks"`
	}
	if err := r.DB.GetContext(ctx, &row, "SELECT SUM(cost) AS spend, SUM(sales) AS sales, SUM(orders) AS orders, SUM(impressions) AS impressions, SUM(clicks) AS clicks FROM ls_ad_hsa_campaign WHERE account_id = ? AND sid = ? AND report_date = ?", accountID, storeID, date); err != nil {
		return fmt.Errorf("listing daily: read ls_ad_hsa_campaign: %w", err)
	}
	if !row.Spend.Valid && !row.Sales.Valid && !row.Orders.Valid && !row.Impressions.Valid && !row.Clicks.Valid {
		return nil
	}
	values := Values{}
	if row.Spend.Valid {
		values.HSASpend = &row.Spend.Float64
	}
	if row.Sales.Valid {
		values.HSASales = &row.Sales.Float64
	}
	if row.Orders.Valid {
		values.HSAOrders = &row.Orders.Int64
	}
	if row.Impressions.Valid {
		values.HSAImpressions = &row.Impressions.Int64
	}
	if row.Clicks.Valid {
		values.HSAClicks = &row.Clicks.Int64
	}
	out.Records = append(out.Records, RawRecord{Source: SourceAPI, Input: Input{Key: Key{Store: storeID, Channel: "hsa", BusinessDate: date}, Scope: ScopeStore, Values: values}})
	return nil
}

func sameCalendarDate(left, right time.Time) bool {
	ly, lm, ld := left.Date()
	ry, rm, rd := right.Date()
	return ly == ry && lm == rm && ld == rd
}

// ReadReportReturns aggregates only the formal raw report's return quantity.
// report_task_id and row_number stay audit identifiers in the raw table; they
// never become a listing-daily business key.
func (r SQLSourceReader) ReadReportReturns(ctx context.Context, accountID, storeID, channel string, date time.Time, evidence ReportEvidence) ([]RawRecord, error) {
	if r.DB == nil {
		return nil, fmt.Errorf("listing daily: nil source database")
	}
	if evidence.AuditID <= 0 || strings.TrimSpace(evidence.ReportTaskID) == "" {
		return nil, fmt.Errorf("listing daily: formal customer returns requires exact audit and task evidence")
	}
	var rows []struct {
		ASIN     string         `db:"asin"`
		SKU      string         `db:"sku"`
		Quantity sql.NullString `db:"quantity"`
	}
	dateText := date.Format("2006-01-02")
	if err := r.DB.SelectContext(ctx, &rows, reportReturnsSQL, evidence.AuditID, evidence.ReportTaskID, accountID, storeID, dateText, dateText, dateText); err != nil {
		return nil, fmt.Errorf("listing daily: read formal customer returns: %w", err)
	}
	records := make([]RawRecord, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.ASIN) == "" || strings.TrimSpace(row.SKU) == "" {
			return nil, fmt.Errorf("listing daily: formal customer returns missing ASIN/SKU for date %s", date.Format("2006-01-02"))
		}
		quantity, err := integer(row.Quantity)
		if err != nil {
			return nil, fmt.Errorf("listing daily: parse formal customer returns quantity asin=%s: %w", row.ASIN, err)
		}
		records = append(records, RawRecord{Source: SourceReport, Input: Input{Key: Key{Store: storeID, Channel: channel, ASIN: row.ASIN, SKU: row.SKU, BusinessDate: date}, Scope: ScopeListing, Values: Values{ReturnsQty: &quantity}}})
	}
	return records, nil
}

func (r SQLSourceReader) ReadReportSales(ctx context.Context, accountID, storeID, channel string, date time.Time, evidence ReportEvidence) ([]RawRecord, error) {
	if r.DB == nil {
		return nil, fmt.Errorf("listing daily: nil source database")
	}
	if evidence.AuditID <= 0 || strings.TrimSpace(evidence.ReportTaskID) == "" {
		return nil, fmt.Errorf("listing daily: formal shipment sales requires exact audit and task evidence")
	}
	var rows []struct {
		ASIN     string         `db:"asin"`
		SKU      string         `db:"sku"`
		Quantity sql.NullString `db:"quantity"`
	}
	dateText := date.Format("2006-01-02")
	if err := r.DB.SelectContext(ctx, &rows, reportShipmentSalesSQL, evidence.AuditID, evidence.ReportTaskID, accountID, storeID, dateText, dateText, dateText); err != nil {
		return nil, fmt.Errorf("listing daily: read formal customer shipment sales: %w", err)
	}
	records := make([]RawRecord, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.ASIN) == "" || strings.TrimSpace(row.SKU) == "" {
			return nil, fmt.Errorf("listing daily: formal customer shipment sales missing ASIN/SKU for date %s", dateText)
		}
		quantity, err := integer(row.Quantity)
		if err != nil {
			return nil, fmt.Errorf("listing daily: parse formal customer shipment sales quantity asin=%s: %w", row.ASIN, err)
		}
		records = append(records, RawRecord{Source: SourceReport, Input: Input{Key: Key{Store: storeID, Channel: channel, ASIN: row.ASIN, SKU: row.SKU, BusinessDate: date}, Scope: ScopeListing, Values: Values{SalesUnits: &quantity}}})
	}
	return records, nil
}

func (r SQLSourceReader) ReadReportInventory(ctx context.Context, accountID, storeID, channel string, date time.Time, evidence ReportEvidence) ([]RawRecord, error) {
	if r.DB == nil {
		return nil, fmt.Errorf("listing daily: nil source database")
	}
	if evidence.AuditID <= 0 || strings.TrimSpace(evidence.ReportTaskID) == "" {
		return nil, fmt.Errorf("listing daily: formal inventory report requires exact audit and task evidence")
	}
	if strings.TrimSpace(channel) != "sc_fba" {
		return nil, fmt.Errorf("listing daily: formal inventory report requires sc_fba channel")
	}
	dateText := date.Format("2006-01-02")
	// Inventory reports are current snapshots and do not carry a business date.
	// The projection date is the UTC download day; the exact task scope is already
	// pinned by audit_id/report_task_id and must not be reused as a historical date.
	args := []any{evidence.AuditID, evidence.ReportTaskID, evidence.ReportType, accountID, storeID}
	var rows []struct {
		ASIN                  string         `db:"asin"`
		SKU                   string         `db:"sku"`
		Sellable              sql.NullString `db:"sellable"`
		Unfulfillable         sql.NullString `db:"unfulfillable"`
		Reserved              sql.NullString `db:"reserved"`
		InboundWorking        sql.NullString `db:"inbound_working"`
		InboundShipped        sql.NullString `db:"inbound_shipped"`
		InboundReceiving      sql.NullString `db:"inbound_receiving"`
		ReservedCustomerOrder sql.NullString `db:"reserved_customer_orders"`
		ReservedFCTransfers   sql.NullString `db:"reserved_fc_transfers"`
		ReservedFCProcessing  sql.NullString `db:"reserved_fc_processing"`
	}
	query := reportFBAInventorySQL
	switch evidence.ReportType {
	case "GET_FBA_MYI_UNSUPPRESSED_INVENTORY_DATA":
		if err := r.DB.SelectContext(ctx, &rows, query, args...); err != nil {
			return nil, fmt.Errorf("listing daily: read formal FBA inventory: %w", err)
		}
	case "GET_FBA_MYI_ALL_INVENTORY_DATA":
		query = reportFBAAllInventorySQL
		if err := r.DB.SelectContext(ctx, &rows, query, args...); err != nil {
			return nil, fmt.Errorf("listing daily: read formal archived FBA inventory: %w", err)
		}
	case "GET_RESERVED_INVENTORY_DATA":
		query = reportReservedInventorySQL
		if err := r.DB.SelectContext(ctx, &rows, query, args...); err != nil {
			return nil, fmt.Errorf("listing daily: read formal reserved inventory: %w", err)
		}
	case "GET_AFN_INVENTORY_DATA":
		query = reportAFNInventorySQL
		if err := r.DB.SelectContext(ctx, &rows, query, args...); err != nil {
			return nil, fmt.Errorf("listing daily: read formal AFN inventory: %w", err)
		}
	default:
		return nil, fmt.Errorf("listing daily: unsupported inventory report type %q", evidence.ReportType)
	}
	records := make([]RawRecord, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.ASIN) == "" || strings.TrimSpace(row.SKU) == "" {
			return nil, fmt.Errorf("listing daily: formal inventory report missing ASIN/SKU for date %s", dateText)
		}
		values, err := inventoryReportValues(evidence.ReportType, row)
		if err != nil {
			return nil, fmt.Errorf("listing daily: parse formal inventory report asin=%s: %w", row.ASIN, err)
		}
		records = append(records, RawRecord{Source: SourceReport, Input: Input{Key: Key{Store: storeID, Channel: channel, ASIN: row.ASIN, SKU: row.SKU, BusinessDate: date}, Scope: ScopeListing, Values: values}})
	}
	return records, nil
}

func inventoryReportValues(reportType string, row struct {
	ASIN                  string         `db:"asin"`
	SKU                   string         `db:"sku"`
	Sellable              sql.NullString `db:"sellable"`
	Unfulfillable         sql.NullString `db:"unfulfillable"`
	Reserved              sql.NullString `db:"reserved"`
	InboundWorking        sql.NullString `db:"inbound_working"`
	InboundShipped        sql.NullString `db:"inbound_shipped"`
	InboundReceiving      sql.NullString `db:"inbound_receiving"`
	ReservedCustomerOrder sql.NullString `db:"reserved_customer_orders"`
	ReservedFCTransfers   sql.NullString `db:"reserved_fc_transfers"`
	ReservedFCProcessing  sql.NullString `db:"reserved_fc_processing"`
}) (Values, error) {
	integerValue := func(value sql.NullString) (*int64, error) {
		parsed, err := integer(value)
		if err != nil {
			return nil, err
		}
		return &parsed, nil
	}
	values := Values{}
	var err error
	switch reportType {
	case "GET_FBA_MYI_UNSUPPRESSED_INVENTORY_DATA", "GET_FBA_MYI_ALL_INVENTORY_DATA":
		if values.InventorySellable, err = integerValue(row.Sellable); err != nil {
			return Values{}, fmt.Errorf("sellable: %w", err)
		}
		if values.InventoryUnfulfillable, err = integerValue(row.Unfulfillable); err != nil {
			return Values{}, fmt.Errorf("unfulfillable: %w", err)
		}
		if values.InventoryReserved, err = integerValue(row.Reserved); err != nil {
			return Values{}, fmt.Errorf("reserved: %w", err)
		}
		if values.InventoryInboundWorking, err = integerValue(row.InboundWorking); err != nil {
			return Values{}, fmt.Errorf("inbound working: %w", err)
		}
		if values.InventoryInboundShipped, err = integerValue(row.InboundShipped); err != nil {
			return Values{}, fmt.Errorf("inbound shipped: %w", err)
		}
		if values.InventoryInboundReceiving, err = integerValue(row.InboundReceiving); err != nil {
			return Values{}, fmt.Errorf("inbound receiving: %w", err)
		}
	case "GET_RESERVED_INVENTORY_DATA":
		if values.InventoryReserved, err = integerValue(row.Reserved); err != nil {
			return Values{}, fmt.Errorf("reserved: %w", err)
		}
		if values.InventoryReservedCustomerOrders, err = integerValue(row.ReservedCustomerOrder); err != nil {
			return Values{}, fmt.Errorf("reserved customer orders: %w", err)
		}
		if row.ReservedFCTransfers.Valid {
			if values.InventoryReservedFCTransfers, err = integerValue(row.ReservedFCTransfers); err != nil {
				return Values{}, fmt.Errorf("reserved FC transfers: %w", err)
			}
		}
		if values.InventoryReservedFCProcessing, err = integerValue(row.ReservedFCProcessing); err != nil {
			return Values{}, fmt.Errorf("reserved FC processing: %w", err)
		}
	case "GET_AFN_INVENTORY_DATA":
		if values.InventorySellable, err = integerValue(row.Sellable); err != nil {
			return Values{}, fmt.Errorf("sellable: %w", err)
		}
	default:
		return Values{}, fmt.Errorf("unsupported report type %q", reportType)
	}
	return values, nil
}

func decimal(value sql.NullString) (float64, error) {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return 0, fmt.Errorf("empty numeric value")
	}
	return strconv.ParseFloat(strings.TrimSpace(value.String), 64)
}

func integer(value sql.NullString) (int64, error) {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return 0, fmt.Errorf("empty integer value")
	}
	return strconv.ParseInt(strings.TrimSpace(value.String), 10, 64)
}

func calendarDate(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
