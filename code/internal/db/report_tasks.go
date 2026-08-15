package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"lingxing-sync/internal/reportexport"
)

// EnsureReport starts one independently auditable formal report run. The
// active-scope unique key makes concurrent callers reuse PENDING/CREATING/
// IN_QUEUE/IN_PROGRESS/UNKNOWN/DONE work, while terminal rows clear that key so a
// later correction can create a fresh report.
func (d *DBReportStore) EnsureReport(ctx context.Context, req reportexport.Request) (reportexport.Audit, error) {
	if d == nil || d.db == nil {
		return reportexport.Audit{}, fmt.Errorf("db report: nil database")
	}
	if strings.TrimSpace(req.AccountID) == "" {
		return reportexport.Audit{}, fmt.Errorf("db report: account_id is required")
	}
	activeKey := reportexport.ActiveScopeKey(req)
	if existing, err := d.findActiveReport(ctx, req, activeKey); err != nil {
		return reportexport.Audit{}, fmt.Errorf("db report: find active audit: %w", err)
	} else if existing.ID != 0 {
		claimed, claimErr := d.claimReportCreation(ctx, existing.ID)
		if claimErr != nil {
			return reportexport.Audit{}, claimErr
		}
		if claimed {
			existing.Status = "CREATING"
			existing.CreateClaimed = true
		}
		return existing, nil
	}
	const q = `INSERT INTO ls_report_export_tasks
(account_id, seller_id, store_id, report_type, region, marketplace_ids, date_from, date_to, status, active_scope_key)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'PENDING', ?)`
	result, err := d.db.ExecContext(ctx, q, req.AccountID, req.SellerID, req.StoreID, reportType(req), req.Region, reportexport.CanonicalMarketplaceIDs(req.MarketplaceIDs), req.DateFrom, req.DateTo, activeKey)
	if err != nil {
		if !isDuplicateKeyError(err) {
			return reportexport.Audit{}, fmt.Errorf("db report: create audit row: %w", err)
		}
		existing, findErr := d.findActiveReport(ctx, req, activeKey)
		if findErr != nil {
			return reportexport.Audit{}, fmt.Errorf("db report: find concurrent audit: %w", findErr)
		}
		if existing.ID == 0 {
			return reportexport.Audit{}, fmt.Errorf("db report: duplicate active scope has no audit row: %w", err)
		}
		if claimed, claimErr := d.claimReportCreation(ctx, existing.ID); claimErr != nil {
			return reportexport.Audit{}, claimErr
		} else if claimed {
			existing.Status = "CREATING"
			existing.CreateClaimed = true
		}
		return existing, nil
	}
	id, err := result.LastInsertId()
	if err != nil {
		return reportexport.Audit{}, fmt.Errorf("db report: audit id: %w", err)
	}
	claimed, err := d.claimReportCreation(ctx, id)
	if err != nil {
		return reportexport.Audit{}, err
	}
	if !claimed {
		return reportexport.Audit{}, fmt.Errorf("db report: new audit id=%d was not claimable", id)
	}
	return reportexport.Audit{ID: id, Status: "CREATING", CreateClaimed: true}, nil
}

func isDuplicateKeyError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func (d *DBReportStore) claimReportCreation(ctx context.Context, id int64) (bool, error) {
	result, err := d.db.ExecContext(ctx, `UPDATE ls_report_export_tasks SET status = 'CREATING' WHERE id = ? AND status = 'PENDING' AND report_task_id = ''`, id)
	if err != nil {
		return false, fmt.Errorf("db report: claim audit id=%d: %w", id, err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("db report: claim audit id=%d rows affected: %w", id, err)
	}
	return updated == 1, nil
}

func (d *DBReportStore) LoadReport(ctx context.Context, id int64) (reportexport.Audit, error) {
	if err := d.ensure(); err != nil {
		return reportexport.Audit{}, err
	}
	var row struct {
		ID               int64          `db:"id"`
		ReportTaskID     string         `db:"report_task_id"`
		ReportDocumentID sql.NullString `db:"report_document_id"`
		Status           string         `db:"status"`
	}
	if err := d.db.GetContext(ctx, &row, `SELECT id, report_task_id, report_document_id, status FROM ls_report_export_tasks WHERE id = ?`, id); err != nil {
		return reportexport.Audit{}, fmt.Errorf("db report: load audit id=%d: %w", id, err)
	}
	audit := reportexport.Audit{ID: row.ID, ReportTaskID: row.ReportTaskID, Status: row.Status}
	if row.ReportDocumentID.Valid {
		audit.ReportDocumentID = row.ReportDocumentID.String
	}
	return audit, nil
}

func (d *DBReportStore) findActiveReport(ctx context.Context, req reportexport.Request, activeKey string) (reportexport.Audit, error) {
	var row struct {
		ID               int64          `db:"id"`
		ReportTaskID     string         `db:"report_task_id"`
		ReportDocumentID sql.NullString `db:"report_document_id"`
		Status           string         `db:"status"`
	}
	const q = `SELECT id, report_task_id, report_document_id, status
FROM ls_report_export_tasks
WHERE status IN ('PENDING', 'CREATING', 'IN_QUEUE', 'IN_PROGRESS', 'UNKNOWN', 'DONE')
  AND (active_scope_key = ? OR (
    active_scope_key IS NULL AND account_id = ? AND seller_id = ? AND store_id = ?
    AND report_type = ? AND region = ? AND date_from = ? AND date_to = ?
    AND JSON_CONTAINS(marketplace_ids, ?) AND JSON_CONTAINS(?, marketplace_ids)
  ))
ORDER BY id DESC LIMIT 1`
	marketplaces := reportexport.CanonicalMarketplaceIDs(req.MarketplaceIDs)
	if err := d.db.GetContext(ctx, &row, q, activeKey, req.AccountID, req.SellerID, req.StoreID, reportType(req), req.Region, req.DateFrom, req.DateTo, marketplaces, marketplaces); err != nil {
		if err == sql.ErrNoRows {
			return reportexport.Audit{}, nil
		}
		return reportexport.Audit{}, err
	}
	audit := reportexport.Audit{ID: row.ID, ReportTaskID: row.ReportTaskID, Status: row.Status}
	if row.ReportDocumentID.Valid {
		audit.ReportDocumentID = row.ReportDocumentID.String
	}
	return audit, nil
}

// DBReportStore is the narrow SQL adapter used by reportexport.Runner.
type DBReportStore struct{ db *sqlx.DB }

func NewReportStore(dbx *sqlx.DB) *DBReportStore { return &DBReportStore{db: dbx} }

func (d *DBReportStore) MarkReportCreated(ctx context.Context, id int64, taskID string) error {
	if err := d.ensure(); err != nil {
		return err
	}
	if strings.TrimSpace(taskID) == "" {
		return fmt.Errorf("db report: report task id is required")
	}
	return d.update(ctx, id, "CREATING", "report_task_id = ?", taskID)
}

func (d *DBReportStore) MarkReportProgress(ctx context.Context, id int64, status, documentID, url, compression string) error {
	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case "IN_QUEUE", "IN_PROGRESS", "DONE", "CANCELLED", "FATAL", "UNKNOWN":
	default:
		return fmt.Errorf("db report: invalid progress status %q", status)
	}
	if err := d.ensure(); err != nil {
		return err
	}
	const q = `UPDATE ls_report_export_tasks
SET status = ?,
    active_scope_key = CASE WHEN ? IN ('CANCELLED', 'FATAL') THEN NULL ELSE active_scope_key END,
    report_document_id = NULLIF(?, ''), download_url = NULLIF(?, ''), compression_algorithm = NULLIF(?, '')
WHERE id = ?`
	if _, err := d.db.ExecContext(ctx, q, status, status, documentID, url, compression, id); err != nil {
		return fmt.Errorf("db report: update progress id=%d: %w", id, err)
	}
	return nil
}

func (d *DBReportStore) MarkReportError(ctx context.Context, id int64, status string, cause error) error {
	if err := d.ensure(); err != nil {
		return err
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "ERROR" {
		return fmt.Errorf("db report: invalid terminal error status %q", status)
	}
	const q = `UPDATE ls_report_export_tasks SET status = ?, active_scope_key = NULL, error_message = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	if _, err := d.db.ExecContext(ctx, q, status, message, id); err != nil {
		return fmt.Errorf("db report: mark error id=%d: %w", id, err)
	}
	return nil
}

func (d *DBReportStore) SaveCustomerReturns(ctx context.Context, id int64, rows []reportexport.CustomerReturn, downloadSHA string, documentID string) error {
	if err := d.ensure(); err != nil {
		return err
	}
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db report: begin raw transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var meta struct {
		AccountID    string `db:"account_id"`
		SellerID     string `db:"seller_id"`
		StoreID      string `db:"store_id"`
		ReportTaskID string `db:"report_task_id"`
	}
	if err := tx.GetContext(ctx, &meta, `SELECT account_id, seller_id, store_id, report_task_id FROM ls_report_export_tasks WHERE id = ?`, id); err != nil {
		return fmt.Errorf("db report: load audit id=%d: %w", id, err)
	}
	if strings.TrimSpace(meta.ReportTaskID) == "" {
		return fmt.Errorf("db report: audit id=%d has no report task id", id)
	}
	const insert = "INSERT INTO ls_fba_fulfillment_customer_returns\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256,\n" +
		" `return-date`, `order-id`, sku, asin, fnsku, `product-name`, quantity,\n" +
		" `fulfillment-center-id`, `detailed-disposition`, reason, status, `license-plate-number`, `customer-comments`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256 = VALUES(row_sha256), `return-date` = VALUES(`return-date`), `order-id` = VALUES(`order-id`), sku = VALUES(sku), asin = VALUES(asin), fnsku = VALUES(fnsku), `product-name` = VALUES(`product-name`), quantity = VALUES(quantity), `fulfillment-center-id` = VALUES(`fulfillment-center-id`), `detailed-disposition` = VALUES(`detailed-disposition`), reason = VALUES(reason), status = VALUES(status), `license-plate-number` = VALUES(`license-plate-number`), `customer-comments` = VALUES(`customer-comments`)"
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("db report: prepare raw insert: %w", err)
	}
	defer stmt.Close()
	for i, row := range rows {
		quantity := row.QuantityRaw
		if quantity == "" {
			quantity = strconv.Itoa(row.Quantity)
		}
		rawKey := strings.Join([]string{row.ReturnDate, row.OrderID, row.SKU, row.ASIN, row.FNSKU, row.ProductName, quantity, row.FulfillmentCenterID, row.DetailedDisposition, row.Reason, row.Status, row.LicensePlateNumber, row.CustomerComments}, "\x00")
		sum := sha256.Sum256([]byte(rawKey))
		if _, err := stmt.ExecContext(ctx, meta.AccountID, meta.SellerID, meta.StoreID, meta.ReportTaskID, i+1, hex.EncodeToString(sum[:]), row.ReturnDate, row.OrderID, row.SKU, row.ASIN, row.FNSKU, row.ProductName, quantity, row.FulfillmentCenterID, row.DetailedDisposition, row.Reason, row.Status, row.LicensePlateNumber, row.CustomerComments); err != nil {
			return fmt.Errorf("db report: insert raw row %d: %w", i+1, err)
		}
	}
	const update = `UPDATE ls_report_export_tasks SET status = 'SUCCESS', active_scope_key = NULL, report_document_id = NULLIF(?, ''), download_sha256 = ?, downloaded_at = CURRENT_TIMESTAMP, rows_imported = ?, error_message = NULL WHERE id = ?`
	if _, err := tx.ExecContext(ctx, update, documentID, downloadSHA, len(rows), id); err != nil {
		return fmt.Errorf("db report: finalize audit id=%d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db report: commit raw transaction: %w", err)
	}
	return nil
}

func (d *DBReportStore) SaveCustomerShipmentSales(ctx context.Context, id int64, rows []reportexport.CustomerShipmentSale, downloadSHA string, documentID string) error {
	if err := d.ensure(); err != nil {
		return err
	}
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db report: begin shipment sales transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var meta struct {
		AccountID    string `db:"account_id"`
		SellerID     string `db:"seller_id"`
		StoreID      string `db:"store_id"`
		ReportTaskID string `db:"report_task_id"`
	}
	if err := tx.GetContext(ctx, &meta, `SELECT account_id, seller_id, store_id, report_task_id FROM ls_report_export_tasks WHERE id = ?`, id); err != nil {
		return fmt.Errorf("db report: load audit id=%d: %w", id, err)
	}
	if strings.TrimSpace(meta.ReportTaskID) == "" {
		return fmt.Errorf("db report: audit id=%d has no report task id", id)
	}
	const insert = "INSERT INTO ls_fba_fulfillment_customer_shipment_sales\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256,\n" +
		" `shipment-date`, sku, fnsku, asin, `fulfillment-center-id`, quantity, `amazon-order-id`, currency,\n" +
		" `item-price-per-unit`, `shipping-price`, `gift-wrap-price`, `ship-city`, `ship-state`, `ship-postal-code`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256 = VALUES(row_sha256), `shipment-date` = VALUES(`shipment-date`), sku = VALUES(sku), fnsku = VALUES(fnsku), asin = VALUES(asin), `fulfillment-center-id` = VALUES(`fulfillment-center-id`), quantity = VALUES(quantity), `amazon-order-id` = VALUES(`amazon-order-id`), currency = VALUES(currency), `item-price-per-unit` = VALUES(`item-price-per-unit`), `shipping-price` = VALUES(`shipping-price`), `gift-wrap-price` = VALUES(`gift-wrap-price`), `ship-city` = VALUES(`ship-city`), `ship-state` = VALUES(`ship-state`), `ship-postal-code` = VALUES(`ship-postal-code`)"
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("db report: prepare shipment sales insert: %w", err)
	}
	defer stmt.Close()
	for i, row := range rows {
		quantity := row.QuantityRaw
		if quantity == "" {
			quantity = strconv.Itoa(row.Quantity)
		}
		price := row.ItemPricePerUnitRaw
		if price == "" {
			price = strconv.FormatFloat(row.ItemPricePerUnit, 'f', -1, 64)
		}
		rawKey := strings.Join([]string{row.ShipmentDate, row.SKU, row.FNSKU, row.ASIN, row.FulfillmentCenterID, quantity, row.AmazonOrderID, row.Currency, price, row.ShippingPrice, row.GiftWrapPrice, row.ShipCity, row.ShipState, row.ShipPostalCode}, "\x00")
		sum := sha256.Sum256([]byte(rawKey))
		if _, err := stmt.ExecContext(ctx, meta.AccountID, meta.SellerID, meta.StoreID, meta.ReportTaskID, i+1, hex.EncodeToString(sum[:]), row.ShipmentDate, row.SKU, row.FNSKU, row.ASIN, row.FulfillmentCenterID, quantity, row.AmazonOrderID, row.Currency, price, row.ShippingPrice, row.GiftWrapPrice, row.ShipCity, row.ShipState, row.ShipPostalCode); err != nil {
			return fmt.Errorf("db report: insert shipment sales row %d: %w", i+1, err)
		}
	}
	const update = `UPDATE ls_report_export_tasks SET status = 'SUCCESS', active_scope_key = NULL, report_document_id = NULLIF(?, ''), download_sha256 = ?, downloaded_at = CURRENT_TIMESTAMP, rows_imported = ?, error_message = NULL WHERE id = ?`
	if _, err := tx.ExecContext(ctx, update, documentID, downloadSHA, len(rows), id); err != nil {
		return fmt.Errorf("db report: finalize shipment sales audit id=%d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db report: commit shipment sales transaction: %w", err)
	}
	return nil
}

func (d *DBReportStore) SaveFBAInventory(ctx context.Context, id int64, rows []reportexport.FBAInventory, downloadSHA string, documentID string) error {
	if err := d.ensure(); err != nil {
		return err
	}
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db report: begin FBA inventory transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	meta, err := loadReportMeta(ctx, tx, id)
	if err != nil {
		return err
	}
	const insert = "INSERT INTO ls_fba_myi_unsuppressed_inventory\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, sku, fnsku, asin, `product-name`, `condition`, `your-price`, `mfn-listing-exists`, `mfn-fulfillable-quantity`, `afn-listing-exists`, `afn-warehouse-quantity`, `afn-fulfillable-quantity`, `afn-unsellable-quantity`, `afn-reserved-quantity`, `afn-total-quantity`, `per-unit-volume`, `afn-inbound-working-quantity`, `afn-inbound-shipped-quantity`, `afn-inbound-receiving-quantity`, `afn-researching-quantity`, `afn-reserved-future-supply`, `afn-future-supply-buyable`, `afn-fulfillable-quantity-local`, `afn-fulfillable-quantity-remote`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256 = VALUES(row_sha256), sku = VALUES(sku), fnsku = VALUES(fnsku), asin = VALUES(asin), `product-name` = VALUES(`product-name`), `condition` = VALUES(`condition`), `your-price` = VALUES(`your-price`), `mfn-listing-exists` = VALUES(`mfn-listing-exists`), `mfn-fulfillable-quantity` = VALUES(`mfn-fulfillable-quantity`), `afn-listing-exists` = VALUES(`afn-listing-exists`), `afn-warehouse-quantity` = VALUES(`afn-warehouse-quantity`), `afn-fulfillable-quantity` = VALUES(`afn-fulfillable-quantity`), `afn-unsellable-quantity` = VALUES(`afn-unsellable-quantity`), `afn-reserved-quantity` = VALUES(`afn-reserved-quantity`), `afn-total-quantity` = VALUES(`afn-total-quantity`), `per-unit-volume` = VALUES(`per-unit-volume`), `afn-inbound-working-quantity` = VALUES(`afn-inbound-working-quantity`), `afn-inbound-shipped-quantity` = VALUES(`afn-inbound-shipped-quantity`), `afn-inbound-receiving-quantity` = VALUES(`afn-inbound-receiving-quantity`), `afn-researching-quantity` = VALUES(`afn-researching-quantity`), `afn-reserved-future-supply` = VALUES(`afn-reserved-future-supply`), `afn-future-supply-buyable` = VALUES(`afn-future-supply-buyable`), `afn-fulfillable-quantity-local` = VALUES(`afn-fulfillable-quantity-local`), `afn-fulfillable-quantity-remote` = VALUES(`afn-fulfillable-quantity-remote`)"
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("db report: prepare FBA inventory insert: %w", err)
	}
	defer stmt.Close()
	for i, row := range rows {
		values := []string{row.SKU, row.FNSKU, row.ASIN, row.ProductName, row.Condition, row.YourPrice, row.MFNListingExists, row.MFNFulfillableQuantityRaw, row.AFNListingExists, row.AFNWarehouseQuantityRaw, row.AFNFulfillableQuantityRaw, row.AFNUnsellableQuantityRaw, row.AFNReservedQuantityRaw, row.AFNTotalQuantityRaw, row.PerUnitVolume, row.AFNInboundWorkingRaw, row.AFNInboundShippedRaw, row.AFNInboundReceivingRaw, row.AFNResearchingQuantityRaw, row.AFNReservedFutureSupplyRaw, row.AFNFutureSupplyBuyable, row.AFNFulfillableQuantityLocal, row.AFNFulfillableQuantityRemote}
		if err := execInventoryRow(ctx, stmt, meta, i+1, values); err != nil {
			return fmt.Errorf("db report: insert FBA inventory row %d: %w", i+1, err)
		}
	}
	if err := finalizeReportTx(ctx, tx, id, documentID, downloadSHA, len(rows)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db report: commit FBA inventory transaction: %w", err)
	}
	return nil
}

func (d *DBReportStore) SaveFBAAllInventory(ctx context.Context, id int64, rows []reportexport.FBAAllInventory, downloadSHA string, documentID string) error {
	if err := d.ensure(); err != nil {
		return err
	}
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db report: begin FBA all inventory transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	meta, err := loadReportMeta(ctx, tx, id)
	if err != nil {
		return err
	}
	const insert = "INSERT INTO ls_fba_myi_all_inventory\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, sku, fnsku, asin, `product-name`, `condition`, `your-price`, `mfn-listing-exists`, `mfn-fulfillable-quantity`, `afn-listing-exists`, `afn-warehouse-quantity`, `afn-fulfillable-quantity`, `afn-unsellable-quantity`, `afn-reserved-quantity`, `afn-total-quantity`, `per-unit-volume`, `afn-inbound-working-quantity`, `afn-inbound-shipped-quantity`, `afn-inbound-receiving-quantity`, `afn-researching-quantity`, `afn-reserved-future-supply`, `afn-future-supply-buyable`, `afn-fulfillable-quantity-local`, `afn-fulfillable-quantity-remote`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256 = VALUES(row_sha256), sku = VALUES(sku), fnsku = VALUES(fnsku), asin = VALUES(asin), `product-name` = VALUES(`product-name`), `condition` = VALUES(`condition`), `your-price` = VALUES(`your-price`), `mfn-listing-exists` = VALUES(`mfn-listing-exists`), `mfn-fulfillable-quantity` = VALUES(`mfn-fulfillable-quantity`), `afn-listing-exists` = VALUES(`afn-listing-exists`), `afn-warehouse-quantity` = VALUES(`afn-warehouse-quantity`), `afn-fulfillable-quantity` = VALUES(`afn-fulfillable-quantity`), `afn-unsellable-quantity` = VALUES(`afn-unsellable-quantity`), `afn-reserved-quantity` = VALUES(`afn-reserved-quantity`), `afn-total-quantity` = VALUES(`afn-total-quantity`), `per-unit-volume` = VALUES(`per-unit-volume`), `afn-inbound-working-quantity` = VALUES(`afn-inbound-working-quantity`), `afn-inbound-shipped-quantity` = VALUES(`afn-inbound-shipped-quantity`), `afn-inbound-receiving-quantity` = VALUES(`afn-inbound-receiving-quantity`), `afn-researching-quantity` = VALUES(`afn-researching-quantity`), `afn-reserved-future-supply` = VALUES(`afn-reserved-future-supply`), `afn-future-supply-buyable` = VALUES(`afn-future-supply-buyable`), `afn-fulfillable-quantity-local` = VALUES(`afn-fulfillable-quantity-local`), `afn-fulfillable-quantity-remote` = VALUES(`afn-fulfillable-quantity-remote`)"
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("db report: prepare FBA all inventory insert: %w", err)
	}
	defer stmt.Close()
	for i, row := range rows {
		values := []string{row.SKU, row.FNSKU, row.ASIN, row.ProductName, row.Condition, row.YourPrice, row.MFNListingExists, row.MFNFulfillableQuantityRaw, row.AFNListingExists, row.AFNWarehouseQuantityRaw, row.AFNFulfillableQuantityRaw, row.AFNUnsellableQuantityRaw, row.AFNReservedQuantityRaw, row.AFNTotalQuantityRaw, row.PerUnitVolume, row.AFNInboundWorkingRaw, row.AFNInboundShippedRaw, row.AFNInboundReceivingRaw, row.AFNResearchingQuantityRaw, row.AFNReservedFutureSupplyRaw, row.AFNFutureSupplyBuyable, row.AFNFulfillableQuantityLocal, row.AFNFulfillableQuantityRemote}
		if err := execInventoryRow(ctx, stmt, meta, i+1, values); err != nil {
			return fmt.Errorf("db report: insert FBA all inventory row %d: %w", i+1, err)
		}
	}
	if err := finalizeReportTx(ctx, tx, id, documentID, downloadSHA, len(rows)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db report: commit FBA all inventory transaction: %w", err)
	}
	return nil
}

func (d *DBReportStore) SaveReservedInventory(ctx context.Context, id int64, rows []reportexport.ReservedInventory, downloadSHA string, documentID string) error {
	if err := d.ensure(); err != nil {
		return err
	}
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db report: begin reserved inventory transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	meta, err := loadReportMeta(ctx, tx, id)
	if err != nil {
		return err
	}
	const insert = "INSERT INTO ls_fba_reserved_inventory\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, sku, fnsku, asin, `product-name`, reserved_qty, reserved_customerorders, `reserved_fc-transfers`, `reserved_fc-processing`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256 = VALUES(row_sha256), sku = VALUES(sku), fnsku = VALUES(fnsku), asin = VALUES(asin), `product-name` = VALUES(`product-name`), reserved_qty = VALUES(reserved_qty), reserved_customerorders = VALUES(reserved_customerorders), `reserved_fc-transfers` = VALUES(`reserved_fc-transfers`), `reserved_fc-processing` = VALUES(`reserved_fc-processing`)"
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("db report: prepare reserved inventory insert: %w", err)
	}
	defer stmt.Close()
	for i, row := range rows {
		values := []string{row.SKU, row.FNSKU, row.ASIN, row.ProductName, row.ReservedQtyRaw, row.ReservedCustomerOrdersRaw, row.ReservedFCTransfersRaw, row.ReservedFCProcessingRaw}
		if err := execInventoryRow(ctx, stmt, meta, i+1, values); err != nil {
			return fmt.Errorf("db report: insert reserved inventory row %d: %w", i+1, err)
		}
	}
	if err := finalizeReportTx(ctx, tx, id, documentID, downloadSHA, len(rows)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db report: commit reserved inventory transaction: %w", err)
	}
	return nil
}

func (d *DBReportStore) SaveAFNInventory(ctx context.Context, id int64, rows []reportexport.AFNInventory, downloadSHA string, documentID string) error {
	if err := d.ensure(); err != nil {
		return err
	}
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db report: begin AFN inventory transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	meta, err := loadReportMeta(ctx, tx, id)
	if err != nil {
		return err
	}
	const insert = "INSERT INTO ls_afn_inventory\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, `seller-sku`, `fulfillment-channel-sku`, asin, `condition-type`, `Warehouse-Condition-code`, `Quantity Available`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256 = VALUES(row_sha256), `seller-sku` = VALUES(`seller-sku`), `fulfillment-channel-sku` = VALUES(`fulfillment-channel-sku`), asin = VALUES(asin), `condition-type` = VALUES(`condition-type`), `Warehouse-Condition-code` = VALUES(`Warehouse-Condition-code`), `Quantity Available` = VALUES(`Quantity Available`)"
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("db report: prepare AFN inventory insert: %w", err)
	}
	defer stmt.Close()
	for i, row := range rows {
		values := []string{row.SellerSKU, row.FulfillmentChannelSKU, row.ASIN, row.ConditionType, row.WarehouseConditionCode, row.QuantityAvailableRaw}
		if err := execInventoryRow(ctx, stmt, meta, i+1, values); err != nil {
			return fmt.Errorf("db report: insert AFN inventory row %d: %w", i+1, err)
		}
	}
	if err := finalizeReportTx(ctx, tx, id, documentID, downloadSHA, len(rows)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db report: commit AFN inventory transaction: %w", err)
	}
	return nil
}

func (d *DBReportStore) SaveAFNInventoryByCountry(ctx context.Context, id int64, rows []reportexport.AFNInventoryByCountry, downloadSHA string, documentID string) error {
	if err := d.ensure(); err != nil {
		return err
	}
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db report: begin AFN inventory by country transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	meta, err := loadReportMeta(ctx, tx, id)
	if err != nil {
		return err
	}
	const insert = "INSERT INTO ls_afn_inventory_by_country\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, `seller-sku`, `fulfillment-channel-sku`, asin, `condition-type`, country, `quantity-for-local-fulfillment`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256 = VALUES(row_sha256), `seller-sku` = VALUES(`seller-sku`), `fulfillment-channel-sku` = VALUES(`fulfillment-channel-sku`), asin = VALUES(asin), `condition-type` = VALUES(`condition-type`), country = VALUES(country), `quantity-for-local-fulfillment` = VALUES(`quantity-for-local-fulfillment`)"
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("db report: prepare AFN inventory by country insert: %w", err)
	}
	defer stmt.Close()
	for i, row := range rows {
		values := []string{row.SellerSKU, row.FulfillmentChannelSKU, row.ASIN, row.ConditionType, row.Country, row.QuantityForLocalFulfillmentRaw}
		if err := execInventoryRow(ctx, stmt, meta, i+1, values); err != nil {
			return fmt.Errorf("db report: insert AFN inventory by country row %d: %w", i+1, err)
		}
	}
	if err := finalizeReportTx(ctx, tx, id, documentID, downloadSHA, len(rows)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db report: commit AFN inventory by country transaction: %w", err)
	}
	return nil
}

func (d *DBReportStore) SaveFBAStorageFeeCharges(ctx context.Context, id int64, rows []reportexport.FBAStorageFeeCharges, downloadSHA string, documentID string) error {
	const insert = "INSERT INTO ls_fba_storage_fee_charges\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, asin, fnsku, product_name, fulfillment_center, country_code, longest_side, median_side, shortest_side, measurement_units, weight, weight_units, item_volume, volume_units, product_size_tier, average_quantity_on_hand, average_quantity_pending_removal, estimated_total_item_volume, month_of_charge, storage_rate, currency, estimated_monthly_storage_fee, dangerous_goods_storage_type, eligible_for_inventory_discount, qualifies_for_inventory_discount, total_incentive_fee_amount, breakdown_incentive_fee_amount, average_quantity_customer_orders)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256=VALUES(row_sha256), asin=VALUES(asin), fnsku=VALUES(fnsku), product_name=VALUES(product_name), fulfillment_center=VALUES(fulfillment_center), country_code=VALUES(country_code), longest_side=VALUES(longest_side), median_side=VALUES(median_side), shortest_side=VALUES(shortest_side), measurement_units=VALUES(measurement_units), weight=VALUES(weight), weight_units=VALUES(weight_units), item_volume=VALUES(item_volume), volume_units=VALUES(volume_units), product_size_tier=VALUES(product_size_tier), average_quantity_on_hand=VALUES(average_quantity_on_hand), average_quantity_pending_removal=VALUES(average_quantity_pending_removal), estimated_total_item_volume=VALUES(estimated_total_item_volume), month_of_charge=VALUES(month_of_charge), storage_rate=VALUES(storage_rate), currency=VALUES(currency), estimated_monthly_storage_fee=VALUES(estimated_monthly_storage_fee), dangerous_goods_storage_type=VALUES(dangerous_goods_storage_type), eligible_for_inventory_discount=VALUES(eligible_for_inventory_discount), qualifies_for_inventory_discount=VALUES(qualifies_for_inventory_discount), total_incentive_fee_amount=VALUES(total_incentive_fee_amount), breakdown_incentive_fee_amount=VALUES(breakdown_incentive_fee_amount), average_quantity_customer_orders=VALUES(average_quantity_customer_orders)"
	values := make([][]string, len(rows))
	for i, row := range rows {
		values[i] = row.Values
	}
	return d.saveFixedReportRows(ctx, id, "FBA storage fee charges", insert, values, downloadSHA, documentID)
}

func (d *DBReportStore) SaveFBAOverageFeeCharges(ctx context.Context, id int64, rows []reportexport.FBAOverageFeeCharges, downloadSHA string, documentID string) error {
	const insert = "INSERT INTO ls_fba_overage_fee_charges\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, charged_date, country_code, storage_type, charge_rate, storage_usage_volume, storage_limit_volume, overage_volume, volume_unit, charged_fee_amount, currency_code)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256=VALUES(row_sha256), charged_date=VALUES(charged_date), country_code=VALUES(country_code), storage_type=VALUES(storage_type), charge_rate=VALUES(charge_rate), storage_usage_volume=VALUES(storage_usage_volume), storage_limit_volume=VALUES(storage_limit_volume), overage_volume=VALUES(overage_volume), volume_unit=VALUES(volume_unit), charged_fee_amount=VALUES(charged_fee_amount), currency_code=VALUES(currency_code)"
	values := make([][]string, len(rows))
	for i, row := range rows {
		values[i] = row.Values
	}
	return d.saveFixedReportRows(ctx, id, "FBA overage fee charges", insert, values, downloadSHA, documentID)
}

func (d *DBReportStore) SaveFBALongtermStorageFeeCharges(ctx context.Context, id int64, rows []reportexport.FBALongtermStorageFeeCharges, downloadSHA string, documentID string) error {
	const insert = "INSERT INTO ls_fba_longterm_storage_fee_charges\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, `snapshot-date`, sku, fnsku, asin, `product-name`, `condition`, `per-unit-volume`, currency, `volume-unit`, country, `qty-charged`, `amount-charged`, `surcharge-age-tier`, `rate-surcharge`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256=VALUES(row_sha256), `snapshot-date`=VALUES(`snapshot-date`), sku=VALUES(sku), fnsku=VALUES(fnsku), asin=VALUES(asin), `product-name`=VALUES(`product-name`), `condition`=VALUES(`condition`), `per-unit-volume`=VALUES(`per-unit-volume`), currency=VALUES(currency), `volume-unit`=VALUES(`volume-unit`), country=VALUES(country), `qty-charged`=VALUES(`qty-charged`), `amount-charged`=VALUES(`amount-charged`), `surcharge-age-tier`=VALUES(`surcharge-age-tier`), `rate-surcharge`=VALUES(`rate-surcharge`)"
	values := make([][]string, len(rows))
	for i, row := range rows {
		values[i] = row.Values
	}
	return d.saveFixedReportRows(ctx, id, "FBA longterm storage fee charges", insert, values, downloadSHA, documentID)
}

func (d *DBReportStore) saveFixedReportRows(ctx context.Context, id int64, label, insert string, rows [][]string, downloadSHA, documentID string) error {
	if err := d.ensure(); err != nil {
		return err
	}
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db report: begin %s transaction: %w", label, err)
	}
	defer func() { _ = tx.Rollback() }()
	meta, err := loadReportMeta(ctx, tx, id)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("db report: prepare %s insert: %w", label, err)
	}
	defer stmt.Close()
	for i, values := range rows {
		if err := execInventoryRow(ctx, stmt, meta, i+1, values); err != nil {
			return fmt.Errorf("db report: insert %s row %d: %w", label, i+1, err)
		}
	}
	if err := finalizeReportTx(ctx, tx, id, documentID, downloadSHA, len(rows)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db report: commit %s transaction: %w", label, err)
	}
	return nil
}

func (d *DBReportStore) SaveCustomerShipmentReplacements(ctx context.Context, id int64, rows []reportexport.CustomerShipmentReplacement, downloadSHA string, documentID string) error {
	if err := d.ensure(); err != nil {
		return err
	}
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db report: begin replacements transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	meta, err := loadReportMeta(ctx, tx, id)
	if err != nil {
		return err
	}
	const insert = "INSERT INTO ls_fba_fulfillment_customer_shipment_replacements\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, `shipment-date`, sku, asin, `fulfillment-center-id`, `original-fulfillment-center-id`, quantity, `replacement-reason-code`, `replacement-amazon-order-id`, `original-amazon-order-id`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256 = VALUES(row_sha256), `shipment-date` = VALUES(`shipment-date`), sku = VALUES(sku), asin = VALUES(asin), `fulfillment-center-id` = VALUES(`fulfillment-center-id`), `original-fulfillment-center-id` = VALUES(`original-fulfillment-center-id`), quantity = VALUES(quantity), `replacement-reason-code` = VALUES(`replacement-reason-code`), `replacement-amazon-order-id` = VALUES(`replacement-amazon-order-id`), `original-amazon-order-id` = VALUES(`original-amazon-order-id`)"
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("db report: prepare replacements insert: %w", err)
	}
	defer stmt.Close()
	for i, row := range rows {
		quantity := row.QuantityRaw
		if quantity == "" {
			quantity = strconv.FormatInt(row.Quantity, 10)
		}
		values := []string{row.ShipmentDate, row.SKU, row.ASIN, row.FulfillmentCenterID, row.OriginalFulfillmentCenterID, quantity, row.ReplacementReasonCode, row.ReplacementAmazonOrderID, row.OriginalAmazonOrderID}
		if err := execInventoryRow(ctx, stmt, meta, i+1, values); err != nil {
			return fmt.Errorf("db report: insert replacement row %d: %w", i+1, err)
		}
	}
	if err := finalizeReportTx(ctx, tx, id, documentID, downloadSHA, len(rows)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db report: commit replacements transaction: %w", err)
	}
	return nil
}

func (d *DBReportStore) SaveFBAReimbursements(ctx context.Context, id int64, rows []reportexport.FBAReimbursement, downloadSHA string, documentID string) error {
	if err := d.ensure(); err != nil {
		return err
	}
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db report: begin reimbursements transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	meta, err := loadReportMeta(ctx, tx, id)
	if err != nil {
		return err
	}
	const insert = "INSERT INTO ls_fba_reimbursements\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, `approval-date`, `reimbursement-id`, `case-id`, `amazon-order-id`, reason, sku, fnsku, asin, `product-name`, `condition`, `currency-unit`, `amount-per-unit`, `amount-total`, `quantity-reimbursed-cash`, `quantity-reimbursed-inventory`, `quantity-reimbursed-total`, `original-reimbursement-id`, `original-reimbursement-type`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256 = VALUES(row_sha256), `approval-date` = VALUES(`approval-date`), `reimbursement-id` = VALUES(`reimbursement-id`), `case-id` = VALUES(`case-id`), `amazon-order-id` = VALUES(`amazon-order-id`), reason = VALUES(reason), sku = VALUES(sku), fnsku = VALUES(fnsku), asin = VALUES(asin), `product-name` = VALUES(`product-name`), `condition` = VALUES(`condition`), `currency-unit` = VALUES(`currency-unit`), `amount-per-unit` = VALUES(`amount-per-unit`), `amount-total` = VALUES(`amount-total`), `quantity-reimbursed-cash` = VALUES(`quantity-reimbursed-cash`), `quantity-reimbursed-inventory` = VALUES(`quantity-reimbursed-inventory`), `quantity-reimbursed-total` = VALUES(`quantity-reimbursed-total`), `original-reimbursement-id` = VALUES(`original-reimbursement-id`), `original-reimbursement-type` = VALUES(`original-reimbursement-type`)"
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("db report: prepare reimbursements insert: %w", err)
	}
	defer stmt.Close()
	for i, row := range rows {
		values := []string{row.ApprovalDate, row.ReimbursementID, row.CaseID, row.AmazonOrderID, row.Reason, row.SKU, row.FNSKU, row.ASIN, row.ProductName, row.Condition, row.CurrencyUnit, row.AmountPerUnit, row.AmountTotal, row.QuantityReimbursedCash, row.QuantityReimbursedInventory, row.QuantityReimbursedTotal, row.OriginalReimbursementID, row.OriginalReimbursementType}
		if err := execInventoryRow(ctx, stmt, meta, i+1, values); err != nil {
			return fmt.Errorf("db report: insert reimbursement row %d: %w", i+1, err)
		}
	}
	if err := finalizeReportTx(ctx, tx, id, documentID, downloadSHA, len(rows)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db report: commit reimbursements transaction: %w", err)
	}
	return nil
}

type reportMeta struct {
	AccountID    string `db:"account_id"`
	SellerID     string `db:"seller_id"`
	StoreID      string `db:"store_id"`
	ReportTaskID string `db:"report_task_id"`
}

func loadReportMeta(ctx context.Context, tx *sqlx.Tx, id int64) (reportMeta, error) {
	var meta reportMeta
	if err := tx.GetContext(ctx, &meta, `SELECT account_id, seller_id, store_id, report_task_id FROM ls_report_export_tasks WHERE id = ?`, id); err != nil {
		return meta, fmt.Errorf("db report: load audit id=%d: %w", id, err)
	}
	if strings.TrimSpace(meta.ReportTaskID) == "" {
		return meta, fmt.Errorf("db report: audit id=%d has no report task id", id)
	}
	return meta, nil
}

func execInventoryRow(ctx context.Context, stmt *sql.Stmt, meta reportMeta, rowNumber int, values []string) error {
	rawKey := strings.Join(values, "\x00")
	sum := sha256.Sum256([]byte(rawKey))
	args := make([]any, 0, len(values)+6)
	args = append(args, meta.AccountID, meta.SellerID, meta.StoreID, meta.ReportTaskID, rowNumber, hex.EncodeToString(sum[:]))
	for _, value := range values {
		args = append(args, value)
	}
	_, err := stmt.ExecContext(ctx, args...)
	return err
}

func finalizeReportTx(ctx context.Context, tx *sqlx.Tx, id int64, documentID, downloadSHA string, rows int) error {
	const update = `UPDATE ls_report_export_tasks SET status = 'SUCCESS', active_scope_key = NULL, report_document_id = NULLIF(?, ''), download_sha256 = ?, downloaded_at = CURRENT_TIMESTAMP, rows_imported = ?, error_message = NULL WHERE id = ?`
	if _, err := tx.ExecContext(ctx, update, documentID, downloadSHA, rows, id); err != nil {
		return fmt.Errorf("db report: finalize audit id=%d: %w", id, err)
	}
	return nil
}

func reportType(req reportexport.Request) string {
	if strings.TrimSpace(req.ReportType) == "" {
		return reportexport.CustomerReturnsReportType
	}
	return strings.TrimSpace(req.ReportType)
}

func (d *DBReportStore) update(ctx context.Context, id int64, status, assignment, value string) error {
	q := "UPDATE ls_report_export_tasks SET status = ?, " + assignment + " WHERE id = ?"
	if _, err := d.db.ExecContext(ctx, q, status, value, id); err != nil {
		return fmt.Errorf("db report: update id=%d: %w", id, err)
	}
	return nil
}

func (d *DBReportStore) ensure() error {
	if d == nil || d.db == nil {
		return fmt.Errorf("db report: nil database")
	}
	return nil
}
