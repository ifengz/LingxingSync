package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// SalesOrderCandidate is the exact identity required by the SC order-detail
// endpoint. Order IDs remain strings so Amazon's dashed identifier never
// passes through a numeric decoder.
type SalesOrderCandidate struct {
	SID           string `db:"sid"`
	AmazonOrderID string `db:"amazon_order_id"`
}

// QueryRecentSalesOrderCandidates reads only orders discovered through the
// verified date_type=2 order-list contract. dateTo is inclusive.
func QueryRecentSalesOrderCandidates(dbx *sqlx.DB, accountID, dateFrom, dateTo string) ([]SalesOrderCandidate, error) {
	start, err := time.Parse("2006-01-02", dateFrom)
	if err != nil {
		return nil, fmt.Errorf("db.QueryRecentSalesOrderCandidates: date_from=%q 非法: %w", dateFrom, err)
	}
	end, err := time.Parse("2006-01-02", dateTo)
	if err != nil {
		return nil, fmt.Errorf("db.QueryRecentSalesOrderCandidates: date_to=%q 非法: %w", dateTo, err)
	}
	if end.Before(start) {
		return nil, fmt.Errorf("db.QueryRecentSalesOrderCandidates: date_to=%s 早于 date_from=%s", dateTo, dateFrom)
	}
	if dbx == nil {
		return nil, fmt.Errorf("db.QueryRecentSalesOrderCandidates: db 不能为空")
	}

	const query = `
SELECT sid, amazon_order_id
FROM ls_sales_orders
WHERE account_id = ?
  AND last_update_date >= ?
  AND last_update_date < ?
ORDER BY sid ASC, amazon_order_id ASC`
	var candidates []SalesOrderCandidate
	if err := dbx.Select(&candidates, query, accountID, dateFrom, end.AddDate(0, 0, 1).Format("2006-01-02")); err != nil {
		return nil, fmt.Errorf("db.QueryRecentSalesOrderCandidates: 查近期订单候选 (account=%s, %s..%s) 失败: %w", accountID, dateFrom, dateTo, err)
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.SID) == "" || strings.TrimSpace(candidate.AmazonOrderID) == "" {
			return nil, fmt.Errorf("db.QueryRecentSalesOrderCandidates: 近期订单候选缺少 sid/amazon_order_id (account=%s, order=%q)", accountID, candidate.AmazonOrderID)
		}
	}
	return candidates, nil
}
