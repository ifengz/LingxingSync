package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// ValidateVCOrdersStoreScope verifies the exact table contract required before
// the VC PO list worker may write. Migration 031 deliberately leaves unsafe or
// drifted schemas untouched so one bad endpoint never prevents the service from
// starting; this assertion turns only that endpoint into a visible error.
func ValidateVCOrdersStoreScope(dbx *sqlx.DB, table string) error {
	if dbx == nil {
		return fmt.Errorf("db.ValidateVCOrdersStoreScope: db 不能为空")
	}

	const primaryQuery = `
SELECT COALESCE(GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ','), '')
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = 'PRIMARY'`
	var primaryKey string
	if err := dbx.Get(&primaryKey, primaryQuery, table); err != nil {
		return fmt.Errorf("db.ValidateVCOrdersStoreScope: 查 %s 主键失败: %w", table, err)
	}
	if primaryKey != "account_id,vc_store_id,local_po_number" {
		return fmt.Errorf("表 %s 主键=%q，必须是 account_id,vc_store_id,local_po_number", table, primaryKey)
	}

	const columnQuery = `
SELECT DATA_TYPE, CHARACTER_MAXIMUM_LENGTH, IS_NULLABLE
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'vc_store_id'`
	var column struct {
		DataType               string `db:"DATA_TYPE"`
		CharacterMaximumLength int64  `db:"CHARACTER_MAXIMUM_LENGTH"`
		IsNullable             string `db:"IS_NULLABLE"`
	}
	if err := dbx.Get(&column, columnQuery, table); err != nil {
		return fmt.Errorf("db.ValidateVCOrdersStoreScope: 查 %s.vc_store_id 失败: %w", table, err)
	}
	if column.DataType != "varchar" || column.CharacterMaximumLength != 32 || column.IsNullable != "NO" {
		return fmt.Errorf("表 %s.vc_store_id 结构=%s(%d) nullable=%s，必须是 varchar(32) NOT NULL",
			table, column.DataType, column.CharacterMaximumLength, column.IsNullable)
	}

	quotedTable := "`" + strings.ReplaceAll(table, "`", "``") + "`"
	var emptyStoreCount int64
	if err := dbx.Get(&emptyStoreCount, "SELECT COUNT(*) FROM "+quotedTable+" WHERE TRIM(vc_store_id) = ''"); err != nil {
		return fmt.Errorf("db.ValidateVCOrdersStoreScope: 查 %s 空店铺失败: %w", table, err)
	}
	if emptyStoreCount > 0 {
		return fmt.Errorf("表 %s 存在 %d 条空 vc_store_id，禁止同步", table, emptyStoreCount)
	}
	return nil
}

// VCPOCandidate carries the exact source identity needed by the PO detail call.
// Both values remain strings so 18-digit Lingxing IDs never pass through float64.
type VCPOCandidate struct {
	VCStoreID     string `db:"vc_store_id"`
	LocalPONumber string `db:"local_po_number"`
}

// QueryRecentVCPOCandidates returns PO-order candidates from the same account.
// dateTo is inclusive; gmt_modified is the field selected by vc_orders'
// search_field_time=3 contract, so old POs changed inside the window are included.
func QueryRecentVCPOCandidates(db *sqlx.DB, accountID, dateFrom, dateTo string) ([]VCPOCandidate, error) {
	start, err := time.Parse("2006-01-02", dateFrom)
	if err != nil {
		return nil, fmt.Errorf("db.QueryRecentVCPOCandidates: date_from=%q 非法: %w", dateFrom, err)
	}
	end, err := time.Parse("2006-01-02", dateTo)
	if err != nil {
		return nil, fmt.Errorf("db.QueryRecentVCPOCandidates: date_to=%q 非法: %w", dateTo, err)
	}
	if end.Before(start) {
		return nil, fmt.Errorf("db.QueryRecentVCPOCandidates: date_to=%s 早于 date_from=%s", dateTo, dateFrom)
	}
	if db == nil {
		return nil, fmt.Errorf("db.QueryRecentVCPOCandidates: db 不能为空")
	}

	const query = `
SELECT DISTINCT COALESCE(vc_store_id, '') AS vc_store_id, local_po_number
FROM ls_vc_orders
WHERE account_id = ?
  AND purchase_order_type = 1
  AND gmt_modified >= ?
  AND gmt_modified < ?
ORDER BY vc_store_id ASC, local_po_number ASC`
	var candidates []VCPOCandidate
	if err := db.Select(&candidates, query, accountID, dateFrom, end.AddDate(0, 0, 1).Format("2006-01-02")); err != nil {
		return nil, fmt.Errorf("db.QueryRecentVCPOCandidates: 查近期 VC PO 候选 (account=%s, %s..%s) 失败: %w", accountID, dateFrom, dateTo, err)
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.VCStoreID) == "" || strings.TrimSpace(candidate.LocalPONumber) == "" {
			return nil, fmt.Errorf("db.QueryRecentVCPOCandidates: 近期 VC PO 候选缺少 vc_store_id/local_po_number (account=%s, local_po_number=%q)", accountID, candidate.LocalPONumber)
		}
	}
	return candidates, nil
}
