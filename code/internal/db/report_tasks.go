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
	const q = `UPDATE ls_report_export_tasks SET status = ?, active_scope_key = NULL, error_message = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status <> 'SUCCESS'`
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
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, sku, fnsku, asin, `product-name`, `condition`, `your-price`, `mfn-listing-exists`, `mfn-fulfillable-quantity`, `afn-listing-exists`, `afn-warehouse-quantity`, `afn-fulfillable-quantity`, `afn-unsellable-quantity`, `afn-reserved-quantity`, `afn-total-quantity`, `per-unit-volume`, `afn-inbound-working-quantity`, `afn-inbound-shipped-quantity`, `afn-inbound-receiving-quantity`, `afn-researching-quantity`, `afn-reserved-future-supply`, `afn-future-supply-buyable`, `afn-fc-transfer-quantity`, `afn-onhand-buyable-quantity`, `store`, `afn-fulfillable-quantity-local`, `afn-fulfillable-quantity-remote`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256 = VALUES(row_sha256), sku = VALUES(sku), fnsku = VALUES(fnsku), asin = VALUES(asin), `product-name` = VALUES(`product-name`), `condition` = VALUES(`condition`), `your-price` = VALUES(`your-price`), `mfn-listing-exists` = VALUES(`mfn-listing-exists`), `mfn-fulfillable-quantity` = VALUES(`mfn-fulfillable-quantity`), `afn-listing-exists` = VALUES(`afn-listing-exists`), `afn-warehouse-quantity` = VALUES(`afn-warehouse-quantity`), `afn-fulfillable-quantity` = VALUES(`afn-fulfillable-quantity`), `afn-unsellable-quantity` = VALUES(`afn-unsellable-quantity`), `afn-reserved-quantity` = VALUES(`afn-reserved-quantity`), `afn-total-quantity` = VALUES(`afn-total-quantity`), `per-unit-volume` = VALUES(`per-unit-volume`), `afn-inbound-working-quantity` = VALUES(`afn-inbound-working-quantity`), `afn-inbound-shipped-quantity` = VALUES(`afn-inbound-shipped-quantity`), `afn-inbound-receiving-quantity` = VALUES(`afn-inbound-receiving-quantity`), `afn-researching-quantity` = VALUES(`afn-researching-quantity`), `afn-reserved-future-supply` = VALUES(`afn-reserved-future-supply`), `afn-future-supply-buyable` = VALUES(`afn-future-supply-buyable`), `afn-fc-transfer-quantity` = VALUES(`afn-fc-transfer-quantity`), `afn-onhand-buyable-quantity` = VALUES(`afn-onhand-buyable-quantity`), `store` = VALUES(`store`), `afn-fulfillable-quantity-local` = VALUES(`afn-fulfillable-quantity-local`), `afn-fulfillable-quantity-remote` = VALUES(`afn-fulfillable-quantity-remote`)"
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("db report: prepare FBA inventory insert: %w", err)
	}
	defer stmt.Close()
	for i, row := range rows {
		values := []string{row.SKU, row.FNSKU, row.ASIN, row.ProductName, row.Condition, row.YourPrice, row.MFNListingExists, row.MFNFulfillableQuantityRaw, row.AFNListingExists, row.AFNWarehouseQuantityRaw, row.AFNFulfillableQuantityRaw, row.AFNUnsellableQuantityRaw, row.AFNReservedQuantityRaw, row.AFNTotalQuantityRaw, row.PerUnitVolume, row.AFNInboundWorkingRaw, row.AFNInboundShippedRaw, row.AFNInboundReceivingRaw, row.AFNResearchingQuantityRaw, row.AFNReservedFutureSupplyRaw, row.AFNFutureSupplyBuyable, row.AFNFCTransferQuantity, row.AFNOnhandBuyableQuantity, row.Store, row.AFNFulfillableQuantityLocal, row.AFNFulfillableQuantityRemote}
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
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, sku, fnsku, asin, `product-name`, `condition`, `your-price`, `mfn-listing-exists`, `mfn-fulfillable-quantity`, `afn-listing-exists`, `afn-warehouse-quantity`, `afn-fulfillable-quantity`, `afn-unsellable-quantity`, `afn-reserved-quantity`, `afn-total-quantity`, `per-unit-volume`, `afn-inbound-working-quantity`, `afn-inbound-shipped-quantity`, `afn-inbound-receiving-quantity`, `afn-researching-quantity`, `afn-reserved-future-supply`, `afn-future-supply-buyable`, `afn-fc-transfer-quantity`, `afn-onhand-buyable-quantity`, `store`, `afn-fulfillable-quantity-local`, `afn-fulfillable-quantity-remote`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256 = VALUES(row_sha256), sku = VALUES(sku), fnsku = VALUES(fnsku), asin = VALUES(asin), `product-name` = VALUES(`product-name`), `condition` = VALUES(`condition`), `your-price` = VALUES(`your-price`), `mfn-listing-exists` = VALUES(`mfn-listing-exists`), `mfn-fulfillable-quantity` = VALUES(`mfn-fulfillable-quantity`), `afn-listing-exists` = VALUES(`afn-listing-exists`), `afn-warehouse-quantity` = VALUES(`afn-warehouse-quantity`), `afn-fulfillable-quantity` = VALUES(`afn-fulfillable-quantity`), `afn-unsellable-quantity` = VALUES(`afn-unsellable-quantity`), `afn-reserved-quantity` = VALUES(`afn-reserved-quantity`), `afn-total-quantity` = VALUES(`afn-total-quantity`), `per-unit-volume` = VALUES(`per-unit-volume`), `afn-inbound-working-quantity` = VALUES(`afn-inbound-working-quantity`), `afn-inbound-shipped-quantity` = VALUES(`afn-inbound-shipped-quantity`), `afn-inbound-receiving-quantity` = VALUES(`afn-inbound-receiving-quantity`), `afn-researching-quantity` = VALUES(`afn-researching-quantity`), `afn-reserved-future-supply` = VALUES(`afn-reserved-future-supply`), `afn-future-supply-buyable` = VALUES(`afn-future-supply-buyable`), `afn-fc-transfer-quantity` = VALUES(`afn-fc-transfer-quantity`), `afn-onhand-buyable-quantity` = VALUES(`afn-onhand-buyable-quantity`), `store` = VALUES(`store`), `afn-fulfillable-quantity-local` = VALUES(`afn-fulfillable-quantity-local`), `afn-fulfillable-quantity-remote` = VALUES(`afn-fulfillable-quantity-remote`)"
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("db report: prepare FBA all inventory insert: %w", err)
	}
	defer stmt.Close()
	for i, row := range rows {
		values := []string{row.SKU, row.FNSKU, row.ASIN, row.ProductName, row.Condition, row.YourPrice, row.MFNListingExists, row.MFNFulfillableQuantityRaw, row.AFNListingExists, row.AFNWarehouseQuantityRaw, row.AFNFulfillableQuantityRaw, row.AFNUnsellableQuantityRaw, row.AFNReservedQuantityRaw, row.AFNTotalQuantityRaw, row.PerUnitVolume, row.AFNInboundWorkingRaw, row.AFNInboundShippedRaw, row.AFNInboundReceivingRaw, row.AFNResearchingQuantityRaw, row.AFNReservedFutureSupplyRaw, row.AFNFutureSupplyBuyable, row.AFNFCTransferQuantity, row.AFNOnhandBuyableQuantity, row.Store, row.AFNFulfillableQuantityLocal, row.AFNFulfillableQuantityRemote}
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
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, sku, fnsku, asin, `product-name`, reserved_qty, reserved_customerorders, `reserved_fc-transfers`, `reserved_fc-processing`, reserved_staging, program)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256 = VALUES(row_sha256), sku = VALUES(sku), fnsku = VALUES(fnsku), asin = VALUES(asin), `product-name` = VALUES(`product-name`), reserved_qty = VALUES(reserved_qty), reserved_customerorders = VALUES(reserved_customerorders), `reserved_fc-transfers` = VALUES(`reserved_fc-transfers`), `reserved_fc-processing` = VALUES(`reserved_fc-processing`), reserved_staging = VALUES(reserved_staging), program = VALUES(program)"
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("db report: prepare reserved inventory insert: %w", err)
	}
	defer stmt.Close()
	for i, row := range rows {
		values := []string{row.SKU, row.FNSKU, row.ASIN, row.ProductName, row.ReservedQtyRaw, row.ReservedCustomerOrdersRaw, row.ReservedFCTransfersRaw, row.ReservedFCProcessingRaw, row.ReservedStagingRaw, row.Program}
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
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, asin, fnsku, product_name, fulfillment_center, country_code, longest_side, median_side, shortest_side, measurement_units, weight, weight_units, item_volume, volume_units, product_size_tier, average_quantity_on_hand, average_quantity_pending_removal, estimated_total_item_volume, month_of_charge, storage_rate, currency, estimated_monthly_storage_fee, dangerous_goods_storage_type, eligible_for_inventory_discount, qualifies_for_inventory_discount, total_incentive_fee_amount, breakdown_incentive_fee_amount, average_quantity_customer_orders, sku, storage_utilization_ratio, storage_utilization_ratio_units, base_rate, utilization_surcharge_rate, avg_qty_for_sus, est_vol_for_sus, est_base_msf, est_sus)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256=VALUES(row_sha256), asin=VALUES(asin), fnsku=VALUES(fnsku), product_name=VALUES(product_name), fulfillment_center=VALUES(fulfillment_center), country_code=VALUES(country_code), longest_side=VALUES(longest_side), median_side=VALUES(median_side), shortest_side=VALUES(shortest_side), measurement_units=VALUES(measurement_units), weight=VALUES(weight), weight_units=VALUES(weight_units), item_volume=VALUES(item_volume), volume_units=VALUES(volume_units), product_size_tier=VALUES(product_size_tier), average_quantity_on_hand=VALUES(average_quantity_on_hand), average_quantity_pending_removal=VALUES(average_quantity_pending_removal), estimated_total_item_volume=VALUES(estimated_total_item_volume), month_of_charge=VALUES(month_of_charge), storage_rate=VALUES(storage_rate), currency=VALUES(currency), estimated_monthly_storage_fee=VALUES(estimated_monthly_storage_fee), dangerous_goods_storage_type=VALUES(dangerous_goods_storage_type), eligible_for_inventory_discount=VALUES(eligible_for_inventory_discount), qualifies_for_inventory_discount=VALUES(qualifies_for_inventory_discount), total_incentive_fee_amount=VALUES(total_incentive_fee_amount), breakdown_incentive_fee_amount=VALUES(breakdown_incentive_fee_amount), average_quantity_customer_orders=VALUES(average_quantity_customer_orders), sku=VALUES(sku), storage_utilization_ratio=VALUES(storage_utilization_ratio), storage_utilization_ratio_units=VALUES(storage_utilization_ratio_units), base_rate=VALUES(base_rate), utilization_surcharge_rate=VALUES(utilization_surcharge_rate), avg_qty_for_sus=VALUES(avg_qty_for_sus), est_vol_for_sus=VALUES(est_vol_for_sus), est_base_msf=VALUES(est_base_msf), est_sus=VALUES(est_sus)"
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
	if len(rows) > 0 {
		want := 6 + len(rows[0])
		if got := strings.Count(insert, "?"); got != want {
			return fmt.Errorf("db report: %s insert placeholders=%d, want %d", label, got, want)
		}
	}
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

func (d *DBReportStore) SaveFBAStrandedInventory(ctx context.Context, id int64, rows []reportexport.FBAStrandedInventory, downloadSHA, documentID string) error {
	const insert = "INSERT INTO ls_fba_stranded_inventory\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, `primary-action`, `date-stranded`, `Date-to-take-auto-removal`, `status-primary`, `status-secondary`, `error-message`, `stranded-reason`, asin, sku, fnsku, `product-name`, `condition`, `fulfilled-by`, `fulfillable-qty`, `your-price`, `unfulfillable-qty`, `reserved-quantity`, `inbound-shipped-qty`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256=VALUES(row_sha256), `primary-action`=VALUES(`primary-action`), `date-stranded`=VALUES(`date-stranded`), `Date-to-take-auto-removal`=VALUES(`Date-to-take-auto-removal`), `status-primary`=VALUES(`status-primary`), `status-secondary`=VALUES(`status-secondary`), `error-message`=VALUES(`error-message`), `stranded-reason`=VALUES(`stranded-reason`), asin=VALUES(asin), sku=VALUES(sku), fnsku=VALUES(fnsku), `product-name`=VALUES(`product-name`), `condition`=VALUES(`condition`), `fulfilled-by`=VALUES(`fulfilled-by`), `fulfillable-qty`=VALUES(`fulfillable-qty`), `your-price`=VALUES(`your-price`), `unfulfillable-qty`=VALUES(`unfulfillable-qty`), `reserved-quantity`=VALUES(`reserved-quantity`), `inbound-shipped-qty`=VALUES(`inbound-shipped-qty`)"
	values := make([][]string, len(rows))
	for i, row := range rows {
		values[i] = row.Values
	}
	return d.saveFixedReportRows(ctx, id, "FBA stranded inventory", insert, values, downloadSHA, documentID)
}

func (d *DBReportStore) SaveFBAEstimatedFees(ctx context.Context, id int64, rows []reportexport.FBAEstimatedFees, downloadSHA, documentID string) error {
	const insert = "INSERT INTO ls_fba_estimated_fees\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, sku, fnsku, asin, `product-name`, `product-group`, brand, `fulfilled-by`, `has-local-inventory`, `your-price`, `sales-price`, `longest-side`, `median-side`, `shortest-side`, `length-and-girth`, `unit-of-dimension`, `item-package-weight`, `unit-of-weight`, `product-size-weight-band`, currency, `estimated-fee-total`, `estimated-referral-fee-per-unit`, `estimated-variable-closing-fee`, `expected-domestic-fulfilment-fee-per-unit`, `expected-efn-fulfilment-fee-per-unit-uk`, `expected-efn-fulfilment-fee-per-unit-de`, `expected-efn-fulfilment-fee-per-unit-fr`, `expected-efn-fulfilment-fee-per-unit-it`, `expected-efn-fulfilment-fee-per-unit-es`, `expected-efn-fulfilment-fee-per-unit-se`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256=VALUES(row_sha256), sku=VALUES(sku), fnsku=VALUES(fnsku), asin=VALUES(asin), `product-name`=VALUES(`product-name`), `product-group`=VALUES(`product-group`), brand=VALUES(brand), `fulfilled-by`=VALUES(`fulfilled-by`), `has-local-inventory`=VALUES(`has-local-inventory`), `your-price`=VALUES(`your-price`), `sales-price`=VALUES(`sales-price`), `longest-side`=VALUES(`longest-side`), `median-side`=VALUES(`median-side`), `shortest-side`=VALUES(`shortest-side`), `length-and-girth`=VALUES(`length-and-girth`), `unit-of-dimension`=VALUES(`unit-of-dimension`), `item-package-weight`=VALUES(`item-package-weight`), `unit-of-weight`=VALUES(`unit-of-weight`), `product-size-weight-band`=VALUES(`product-size-weight-band`), currency=VALUES(currency), `estimated-fee-total`=VALUES(`estimated-fee-total`), `estimated-referral-fee-per-unit`=VALUES(`estimated-referral-fee-per-unit`), `estimated-variable-closing-fee`=VALUES(`estimated-variable-closing-fee`), `expected-domestic-fulfilment-fee-per-unit`=VALUES(`expected-domestic-fulfilment-fee-per-unit`), `expected-efn-fulfilment-fee-per-unit-uk`=VALUES(`expected-efn-fulfilment-fee-per-unit-uk`), `expected-efn-fulfilment-fee-per-unit-de`=VALUES(`expected-efn-fulfilment-fee-per-unit-de`), `expected-efn-fulfilment-fee-per-unit-fr`=VALUES(`expected-efn-fulfilment-fee-per-unit-fr`), `expected-efn-fulfilment-fee-per-unit-it`=VALUES(`expected-efn-fulfilment-fee-per-unit-it`), `expected-efn-fulfilment-fee-per-unit-es`=VALUES(`expected-efn-fulfilment-fee-per-unit-es`), `expected-efn-fulfilment-fee-per-unit-se`=VALUES(`expected-efn-fulfilment-fee-per-unit-se`)"
	values := make([][]string, len(rows))
	for i, row := range rows {
		values[i] = row.Values
	}
	return d.saveFixedReportRows(ctx, id, "FBA estimated fees", insert, values, downloadSHA, documentID)
}

func (d *DBReportStore) SaveFBAInboundNoncompliance(ctx context.Context, id int64, rows []reportexport.FBAInboundNoncompliance, downloadSHA, documentID string) error {
	const insert = "INSERT INTO ls_fba_inbound_noncompliance\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, `issue-reported-date`, `shipment-creation-date`, `fba-shipment-id`, `fba-carton-id`, `fulfillment-center-id`, sku, fnsku, asin, `product-name`, `problem-type`, `problem-quantity`, `expected-quantity`, `received-quantity`, `performance-measurement-unit`, `coaching-level`, `fee-type`, currency, `fee-total`, `problem-level`, `alert-status`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256=VALUES(row_sha256), `issue-reported-date`=VALUES(`issue-reported-date`), `shipment-creation-date`=VALUES(`shipment-creation-date`), `fba-shipment-id`=VALUES(`fba-shipment-id`), `fba-carton-id`=VALUES(`fba-carton-id`), `fulfillment-center-id`=VALUES(`fulfillment-center-id`), sku=VALUES(sku), fnsku=VALUES(fnsku), asin=VALUES(asin), `product-name`=VALUES(`product-name`), `problem-type`=VALUES(`problem-type`), `problem-quantity`=VALUES(`problem-quantity`), `expected-quantity`=VALUES(`expected-quantity`), `received-quantity`=VALUES(`received-quantity`), `performance-measurement-unit`=VALUES(`performance-measurement-unit`), `coaching-level`=VALUES(`coaching-level`), `fee-type`=VALUES(`fee-type`), currency=VALUES(currency), `fee-total`=VALUES(`fee-total`), `problem-level`=VALUES(`problem-level`), `alert-status`=VALUES(`alert-status`)"
	values := make([][]string, len(rows))
	for i, row := range rows {
		values[i] = row.Values
	}
	return d.saveFixedReportRows(ctx, id, "FBA inbound noncompliance", insert, values, downloadSHA, documentID)
}

func (d *DBReportStore) SaveFBARecommendedRemoval(ctx context.Context, id int64, rows []reportexport.FBARecommendedRemoval, downloadSHA, documentID string) error {
	const insert = "INSERT INTO ls_fba_recommended_removals\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, `snapshot-date`, sku, fnsku, asin, `product-name`, `condition`, `sellable-quantity`, `sellable-271-365-days`, `sellable-365+-days`, `sellable-removal-quantity`, `unsellable-quantity`, `unsellable-0-7-days`, `unsellable-8-60-days`, `unsellable-61-90-days`, `sellable-121-180-days`, `sellable-181-270-days`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256=VALUES(row_sha256), `snapshot-date`=VALUES(`snapshot-date`), sku=VALUES(sku), fnsku=VALUES(fnsku), asin=VALUES(asin), `product-name`=VALUES(`product-name`), `condition`=VALUES(`condition`), `sellable-quantity`=VALUES(`sellable-quantity`), `sellable-271-365-days`=VALUES(`sellable-271-365-days`), `sellable-365+-days`=VALUES(`sellable-365+-days`), `sellable-removal-quantity`=VALUES(`sellable-removal-quantity`), `unsellable-quantity`=VALUES(`unsellable-quantity`), `unsellable-0-7-days`=VALUES(`unsellable-0-7-days`), `unsellable-8-60-days`=VALUES(`unsellable-8-60-days`), `unsellable-61-90-days`=VALUES(`unsellable-61-90-days`), `sellable-121-180-days`=VALUES(`sellable-121-180-days`), `sellable-181-270-days`=VALUES(`sellable-181-270-days`)"
	values := make([][]string, len(rows))
	for i, row := range rows {
		values[i] = row.Values
	}
	return d.saveFixedReportRows(ctx, id, "FBA recommended removal", insert, values, downloadSHA, documentID)
}

func (d *DBReportStore) SaveFBARemovalOrder(ctx context.Context, id int64, rows []reportexport.FBARemovalOrder, downloadSHA, documentID string) error {
	const insert = "INSERT INTO ls_fba_removal_order_details\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, `request-date`, `order-id`, `order-type`, `service-speed`, `order-status`, `last-updated-date`, sku, fnsku, disposition, `requested-quantity`, `cancelled-quantity`, `disposed-quantity`, `shipped-quantity`, `in-process-quantity`, `removal-fee`, currency)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256=VALUES(row_sha256), `request-date`=VALUES(`request-date`), `order-id`=VALUES(`order-id`), `order-type`=VALUES(`order-type`), `service-speed`=VALUES(`service-speed`), `order-status`=VALUES(`order-status`), `last-updated-date`=VALUES(`last-updated-date`), sku=VALUES(sku), fnsku=VALUES(fnsku), disposition=VALUES(disposition), `requested-quantity`=VALUES(`requested-quantity`), `cancelled-quantity`=VALUES(`cancelled-quantity`), `disposed-quantity`=VALUES(`disposed-quantity`), `shipped-quantity`=VALUES(`shipped-quantity`), `in-process-quantity`=VALUES(`in-process-quantity`), `removal-fee`=VALUES(`removal-fee`), currency=VALUES(currency)"
	values := make([][]string, len(rows))
	for i, row := range rows {
		values[i] = row.Values
	}
	return d.saveFixedReportRows(ctx, id, "FBA removal order", insert, values, downloadSHA, documentID)
}

func (d *DBReportStore) SaveFBARemovalShipment(ctx context.Context, id int64, rows []reportexport.FBARemovalShipment, downloadSHA, documentID string) error {
	const insert = "INSERT INTO ls_fba_removal_shipment_details\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, `request-date`, `order-id`, `shipment-date`, sku, fnsku, disposition, `shipped-quantity`, carrier, `tracking-number`, `removal-order-type`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256=VALUES(row_sha256), `request-date`=VALUES(`request-date`), `order-id`=VALUES(`order-id`), `shipment-date`=VALUES(`shipment-date`), sku=VALUES(sku), fnsku=VALUES(fnsku), disposition=VALUES(disposition), `shipped-quantity`=VALUES(`shipped-quantity`), carrier=VALUES(carrier), `tracking-number`=VALUES(`tracking-number`), `removal-order-type`=VALUES(`removal-order-type`)"
	values := make([][]string, len(rows))
	for i, row := range rows {
		values[i] = row.Values
	}
	return d.saveFixedReportRows(ctx, id, "FBA removal shipment", insert, values, downloadSHA, documentID)
}

func (d *DBReportStore) SaveAllOrders(ctx context.Context, id int64, rows []reportexport.AllOrder, downloadSHA, documentID string) error {
	const insert = "INSERT INTO ls_amazon_all_orders_by_order_date\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, `amazon-order-id`, `merchant-order-id`, `purchase-date`, `last-updated-date`, `order-status`, `fulfillment-channel`, `sales-channel`, `order-channel`, `ship-service-level`, `product-name`, sku, asin, `item-status`, quantity, currency, `item-price`, `item-tax`, `shipping-price`, `shipping-tax`, `gift-wrap-price`, `gift-wrap-tax`, `item-promotion-discount`, `ship-promotion-discount`, `ship-city`, `ship-state`, `ship-postal-code`, `ship-country`, `promotion-ids`, cpf, `is-business-order`, `purchase-order-number`, `price-designation`, `signature-confirmation-recommended`, `order-item-id`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256=VALUES(row_sha256), `amazon-order-id`=VALUES(`amazon-order-id`), `merchant-order-id`=VALUES(`merchant-order-id`), `purchase-date`=VALUES(`purchase-date`), `last-updated-date`=VALUES(`last-updated-date`), `order-status`=VALUES(`order-status`), `fulfillment-channel`=VALUES(`fulfillment-channel`), `sales-channel`=VALUES(`sales-channel`), `order-channel`=VALUES(`order-channel`), `ship-service-level`=VALUES(`ship-service-level`), `product-name`=VALUES(`product-name`), sku=VALUES(sku), asin=VALUES(asin), `item-status`=VALUES(`item-status`), quantity=VALUES(quantity), currency=VALUES(currency), `item-price`=VALUES(`item-price`), `item-tax`=VALUES(`item-tax`), `shipping-price`=VALUES(`shipping-price`), `shipping-tax`=VALUES(`shipping-tax`), `gift-wrap-price`=VALUES(`gift-wrap-price`), `gift-wrap-tax`=VALUES(`gift-wrap-tax`), `item-promotion-discount`=VALUES(`item-promotion-discount`), `ship-promotion-discount`=VALUES(`ship-promotion-discount`), `ship-city`=VALUES(`ship-city`), `ship-state`=VALUES(`ship-state`), `ship-postal-code`=VALUES(`ship-postal-code`), `ship-country`=VALUES(`ship-country`), `promotion-ids`=VALUES(`promotion-ids`), cpf=VALUES(cpf), `is-business-order`=VALUES(`is-business-order`), `purchase-order-number`=VALUES(`purchase-order-number`), `price-designation`=VALUES(`price-designation`), `signature-confirmation-recommended`=VALUES(`signature-confirmation-recommended`), `order-item-id`=VALUES(`order-item-id`)"
	values := make([][]string, len(rows))
	for i, row := range rows {
		values[i] = row.Values
	}
	return d.saveFixedReportRows(ctx, id, "Amazon all orders", insert, values, downloadSHA, documentID)
}

func (d *DBReportStore) SaveFulfilledShipments(ctx context.Context, id int64, rows []reportexport.FulfilledShipment, downloadSHA, documentID string) error {
	const insert = "INSERT INTO ls_amazon_fulfilled_shipments\n" +
		"(account_id, seller_id, store_id, report_task_id, `row_number`, row_sha256, `amazon-order-id`, `merchant-order-id`, `shipment-id`, `shipment-item-id`, `amazon-order-item-id`, `merchant-order-item-id`, `purchase-date`, `payments-date`, `shipment-date`, `reporting-date`, `buyer-email`, `buyer-name`, `buyer-phone-number`, sku, `product-name`, `quantity-shipped`, currency, `item-price`, `item-tax`, `shipping-price`, `shipping-tax`, `gift-wrap-price`, `gift-wrap-tax`, `ship-service-level`, `recipient-name`, `ship-address-1`, `ship-address-2`, `ship-address-3`, `ship-city`, `ship-state`, `ship-postal-code`, `ship-country`, `ship-phone-number`, `bill-address-1`, `bill-address-2`, `bill-address-3`, `bill-city`, `bill-state`, `bill-postal-code`, `bill-country`, `item-promotion-discount`, `ship-promotion-discount`, carrier, `tracking-number`, `estimated-arrival-date`, `fulfillment-center-id`, `fulfillment-channel`, `sales-channel`)\n" +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)\n" +
		"ON DUPLICATE KEY UPDATE row_sha256=VALUES(row_sha256), `amazon-order-id`=VALUES(`amazon-order-id`), `merchant-order-id`=VALUES(`merchant-order-id`), `shipment-id`=VALUES(`shipment-id`), `shipment-item-id`=VALUES(`shipment-item-id`), `amazon-order-item-id`=VALUES(`amazon-order-item-id`), `merchant-order-item-id`=VALUES(`merchant-order-item-id`), `purchase-date`=VALUES(`purchase-date`), `payments-date`=VALUES(`payments-date`), `shipment-date`=VALUES(`shipment-date`), `reporting-date`=VALUES(`reporting-date`), `buyer-email`=VALUES(`buyer-email`), `buyer-name`=VALUES(`buyer-name`), `buyer-phone-number`=VALUES(`buyer-phone-number`), sku=VALUES(sku), `product-name`=VALUES(`product-name`), `quantity-shipped`=VALUES(`quantity-shipped`), currency=VALUES(currency), `item-price`=VALUES(`item-price`), `item-tax`=VALUES(`item-tax`), `shipping-price`=VALUES(`shipping-price`), `shipping-tax`=VALUES(`shipping-tax`), `gift-wrap-price`=VALUES(`gift-wrap-price`), `gift-wrap-tax`=VALUES(`gift-wrap-tax`), `ship-service-level`=VALUES(`ship-service-level`), `recipient-name`=VALUES(`recipient-name`), `ship-address-1`=VALUES(`ship-address-1`), `ship-address-2`=VALUES(`ship-address-2`), `ship-address-3`=VALUES(`ship-address-3`), `ship-city`=VALUES(`ship-city`), `ship-state`=VALUES(`ship-state`), `ship-postal-code`=VALUES(`ship-postal-code`), `ship-country`=VALUES(`ship-country`), `ship-phone-number`=VALUES(`ship-phone-number`), `bill-address-1`=VALUES(`bill-address-1`), `bill-address-2`=VALUES(`bill-address-2`), `bill-address-3`=VALUES(`bill-address-3`), `bill-city`=VALUES(`bill-city`), `bill-state`=VALUES(`bill-state`), `bill-postal-code`=VALUES(`bill-postal-code`), `bill-country`=VALUES(`bill-country`), `item-promotion-discount`=VALUES(`item-promotion-discount`), `ship-promotion-discount`=VALUES(`ship-promotion-discount`), carrier=VALUES(carrier), `tracking-number`=VALUES(`tracking-number`), `estimated-arrival-date`=VALUES(`estimated-arrival-date`), `fulfillment-center-id`=VALUES(`fulfillment-center-id`), `fulfillment-channel`=VALUES(`fulfillment-channel`), `sales-channel`=VALUES(`sales-channel`)"
	values := make([][]string, len(rows))
	for i, row := range rows {
		values[i] = row.Values
	}
	return d.saveFixedReportRows(ctx, id, "Amazon fulfilled shipments", insert, values, downloadSHA, documentID)
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
