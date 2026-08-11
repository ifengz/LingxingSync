package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

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
