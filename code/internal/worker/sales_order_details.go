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

func detailCandidateParams(batch []db.SalesOrderCandidate) (map[string]any, map[string]string, error) {
	ids := make([]string, 0, len(batch))
	sids := make(map[string]string, len(batch))
	for _, candidate := range batch {
		if candidate.SID == "" || candidate.AmazonOrderID == "" {
			return nil, nil, fmt.Errorf("订单详情候选缺少 sid/amazon_order_id")
		}
		if existing, ok := sids[candidate.AmazonOrderID]; ok && existing != candidate.SID {
			return nil, nil, fmt.Errorf("订单详情候选订单 %s 同时属于 sid=%s/%s", candidate.AmazonOrderID, existing, candidate.SID)
		}
		ids = append(ids, candidate.AmazonOrderID)
		sids[candidate.AmazonOrderID] = candidate.SID
	}
	return map[string]any{"order_id": strings.Join(ids, ",")}, sids, nil
}

func shapeSalesOrderDetailRows(rows []map[string]any, expectedSIDs map[string]string) ([]map[string]any, error) {
	seen := make(map[string][]byte, len(rows))
	shaped := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			return nil, fmt.Errorf("订单详情响应包含空行")
		}
		orderID := strings.TrimSpace(fmt.Sprint(row["amazon_order_id"]))
		sid, ok := expectedSIDs[orderID]
		if !ok {
			return nil, fmt.Errorf("订单详情响应返回未请求的 amazon_order_id=%q", orderID)
		}
		if strings.TrimSpace(fmt.Sprint(row["sid"])) != sid {
			return nil, fmt.Errorf("订单详情响应 sid=%q 与候选 sid=%q 不一致 (amazon_order_id=%q)", row["sid"], sid, orderID)
		}
		canonical, err := json.Marshal(row)
		if err != nil {
			return nil, fmt.Errorf("订单详情响应无法规范化 amazon_order_id=%q: %w", orderID, err)
		}
		if previous, duplicate := seen[orderID]; duplicate {
			if !bytes.Equal(previous, canonical) {
				return nil, fmt.Errorf("订单详情响应同一 amazon_order_id 存在非等价重复: %q", orderID)
			}
			continue
		}
		seen[orderID] = canonical
		shaped = append(shaped, row)
	}
	if len(seen) != len(expectedSIDs) {
		missing := make([]string, 0, len(expectedSIDs)-len(seen))
		for orderID := range expectedSIDs {
			if _, ok := seen[orderID]; !ok {
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
