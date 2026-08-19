package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"lingxing-sync/internal/db"
)

const salesOrderDetailBatchSize = 200

func (w *EndpointWorker) salesOrderCandidateWindow(req triggerReq) (string, string, error) {
	if (req.dateFrom == "") != (req.dateTo == "") {
		return "", "", fmt.Errorf("订单详情手动日期范围必须同时提供 date_from/date_to")
	}
	if req.dateFrom != "" {
		return req.dateFrom, req.dateTo, nil
	}
	if w.Endpoint.WindowDays <= 0 {
		return "", "", fmt.Errorf("订单详情 window_days 必须 > 0")
	}
	now := time.Now()
	return now.AddDate(0, 0, -w.Endpoint.WindowDays).Format("2006-01-02"), now.Format("2006-01-02"), nil
}

func salesOrderDetailBatches(candidates []db.SalesOrderCandidate) [][]db.SalesOrderCandidate {
	batches := make([][]db.SalesOrderCandidate, 0, (len(candidates)+salesOrderDetailBatchSize-1)/salesOrderDetailBatchSize)
	for start := 0; start < len(candidates); {
		end := start + 1
		for end < len(candidates) && candidates[end].SID == candidates[start].SID && end-start < salesOrderDetailBatchSize {
			end++
		}
		batches = append(batches, candidates[start:end])
		start = end
	}
	return batches
}

func detailCandidateParams(batch []db.SalesOrderCandidate) (map[string]any, map[string]struct{}, error) {
	ids := make([]string, 0, len(batch))
	expectedIDs := make(map[string]struct{}, len(batch))
	for _, candidate := range batch {
		if candidate.SID == "" || candidate.AmazonOrderID == "" {
			return nil, nil, fmt.Errorf("订单详情候选缺少 sid/amazon_order_id")
		}
		if _, duplicate := expectedIDs[candidate.AmazonOrderID]; duplicate {
			return nil, nil, fmt.Errorf("订单详情候选重复 amazon_order_id=%s", candidate.AmazonOrderID)
		}
		ids = append(ids, candidate.AmazonOrderID)
		expectedIDs[candidate.AmazonOrderID] = struct{}{}
	}
	return map[string]any{"order_id": strings.Join(ids, ",")}, expectedIDs, nil
}

func shapeSalesOrderDetailRows(rows []map[string]any, expectedIDs map[string]struct{}) ([]map[string]any, error) {
	seen := make(map[string][]byte, len(rows))
	returnedIDs := make(map[string]struct{}, len(rows))
	shaped := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			return nil, fmt.Errorf("订单详情响应包含空行")
		}
		orderID := strings.TrimSpace(fmt.Sprint(row["amazon_order_id"]))
		if _, ok := expectedIDs[orderID]; !ok {
			return nil, fmt.Errorf("订单详情响应返回未请求的 amazon_order_id=%q", orderID)
		}
		sid := strings.TrimSpace(fmt.Sprint(row["sid"]))
		if sid == "" || sid == "<nil>" {
			return nil, fmt.Errorf("订单详情响应缺少 sid (amazon_order_id=%q)", orderID)
		}
		identity := sid + "|" + orderID
		canonical, err := json.Marshal(row)
		if err != nil {
			return nil, fmt.Errorf("订单详情响应无法规范化 amazon_order_id=%q: %w", orderID, err)
		}
		if previous, duplicate := seen[identity]; duplicate {
			if !bytes.Equal(previous, canonical) {
				return nil, fmt.Errorf("订单详情响应同一 sid/amazon_order_id 存在非等价重复: %q", identity)
			}
			continue
		}
		seen[identity] = canonical
		returnedIDs[orderID] = struct{}{}
		shaped = append(shaped, row)
	}
	if len(returnedIDs) != len(expectedIDs) {
		missing := make([]string, 0, len(expectedIDs)-len(returnedIDs))
		for orderID := range expectedIDs {
			if _, ok := returnedIDs[orderID]; !ok {
				missing = append(missing, orderID)
			}
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("订单详情响应缺少 %d 个请求订单: %s", len(missing), strings.Join(missing, ","))
	}
	return shaped, nil
}

func (w *EndpointWorker) syncSalesOrderDetails(ctx context.Context, taskID int64, req triggerReq) (int, int, error) {
	dateFrom, dateTo, err := w.salesOrderCandidateWindow(req)
	if err != nil {
		_ = db.InsertTaskLog(w.DB, taskID, 1, 0, 0, 0, 0, err.Error())
		return 0, 0, err
	}
	candidates, err := db.QueryRecentSalesOrderCandidates(w.DB, w.Account.ID, dateFrom, dateTo)
	if err != nil {
		_ = db.InsertTaskLog(w.DB, taskID, 1, 0, 0, 0, 0, err.Error())
		return 0, 0, err
	}
	limiter := w.Limiters.Get(w.Account.QuotaGroupOrID(), w.Endpoint.Path, w.Endpoint.Rate.Bucket, w.Endpoint.Rate.IntervalMs)
	records := 0
	pages := 0
	for _, batch := range salesOrderDetailBatches(candidates) {
		if err := ctx.Err(); err != nil {
			return records, pages, err
		}
		params, expectedSIDs, err := detailCandidateParams(batch)
		if err != nil {
			_ = db.InsertTaskLog(w.DB, taskID, pages+1, 0, 0, 0, 0, err.Error())
			return records, pages, err
		}
		result, httpStatus, apiCode, durationMs, err := w.fetchPageWithRetry(ctx, limiter, w.Endpoint.Method, w.Endpoint.Path, params)
		if err != nil {
			_ = db.InsertTaskLog(w.DB, taskID, pages+1, httpStatus, apiCode, 0, durationMs, err.Error())
			return records, pages, fmt.Errorf("订单详情请求失败: %w", err)
		}
		rows, err := shapeSalesOrderDetailRows(result.List, expectedSIDs)
		if err != nil {
			_ = db.InsertTaskLog(w.DB, taskID, pages+1, httpStatus, apiCode, len(result.List), durationMs, "shape: "+err.Error())
			return records, pages, err
		}
		if err := db.UpsertRows(w.DB, w.Endpoint.Table, rows, w.Columns, w.JSONCols, w.Account.ID); err != nil {
			_ = db.InsertTaskLog(w.DB, taskID, pages+1, httpStatus, apiCode, len(rows), durationMs, "upsert: "+err.Error())
			return records, pages, fmt.Errorf("订单详情落库失败: %w", err)
		}
		pages++
		records += len(rows)
		_ = db.InsertTaskLog(w.DB, taskID, pages, httpStatus, apiCode, len(rows), durationMs, "")
	}
	log.Printf("[worker:%s] 订单详情完成候选=%d records=%d", w.Endpoint.Name, len(candidates), records)
	return records, pages, nil
}
