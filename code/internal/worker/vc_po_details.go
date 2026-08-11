package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"lingxing-sync/internal/db"
)

func (w *EndpointWorker) vcPOCandidateWindow(req triggerReq) (string, string, error) {
	if (req.dateFrom == "") != (req.dateTo == "") {
		return "", "", fmt.Errorf("VC PO detail 手动日期范围必须同时提供 date_from/date_to")
	}
	if req.dateFrom != "" {
		return req.dateFrom, req.dateTo, nil
	}
	if w.Endpoint.WindowDays <= 0 {
		return "", "", fmt.Errorf("VC PO detail window_days 必须 > 0")
	}
	now := time.Now()
	return now.AddDate(0, 0, -w.Endpoint.WindowDays).Format("2006-01-02"), now.Format("2006-01-02"), nil
}

func (w *EndpointWorker) fetchVCPODetail(ctx context.Context, limiter *Limiter, candidate db.VCPOCandidate) (map[string]any, int, int, int, error) {
	if candidate.LocalPONumber == "" || candidate.VCStoreID == "" {
		return nil, 0, 0, 0, fmt.Errorf("VC PO detail 候选缺少 local_po_number/vc_store_id")
	}
	params := map[string]any{"local_po_number": candidate.LocalPONumber}
	result, httpStatus, apiCode, durationMs, err := w.fetchPageWithRetry(ctx, limiter, w.Endpoint.Method, w.Endpoint.Path, params)
	if err != nil {
		return nil, httpStatus, apiCode, durationMs, err
	}
	if result == nil {
		return nil, httpStatus, apiCode, durationMs, fmt.Errorf("VC PO detail 返回空结果")
	}
	if len(result.List) != 1 {
		return nil, httpStatus, apiCode, durationMs, fmt.Errorf("VC PO detail 应返回 1 个对象，实际 %d 个", len(result.List))
	}

	identity := map[string]any{
		"vc_store_id":     candidate.VCStoreID,
		"local_po_number": candidate.LocalPONumber,
	}
	if err := shapeRows(result.List, w.Endpoint.FieldPaths, w.Endpoint.InjectParams, w.Endpoint.ForceInjectParams, identity); err != nil {
		return nil, httpStatus, apiCode, durationMs, fmt.Errorf("VC PO detail 行整形: %w", err)
	}
	return result.List[0], httpStatus, apiCode, durationMs, nil
}

func forEachVCPODetailCandidate(candidates []db.VCPOCandidate, syncOne func(int, db.VCPOCandidate) error) (int, error) {
	for i, candidate := range candidates {
		if err := syncOne(i+1, candidate); err != nil {
			return i, err
		}
	}
	return len(candidates), nil
}

func (w *EndpointWorker) syncVCPODetails(ctx context.Context, taskID int64, req triggerReq) (int, int, error) {
	dateFrom, dateTo, err := w.vcPOCandidateWindow(req)
	if err != nil {
		_ = db.InsertTaskLog(w.DB, taskID, 1, 0, 0, 0, 0, err.Error())
		return 0, 0, err
	}
	candidates, err := db.QueryRecentVCPOCandidates(w.DB, w.Account.ID, dateFrom, dateTo)
	if err != nil {
		_ = db.InsertTaskLog(w.DB, taskID, 1, 0, 0, 0, 0, err.Error())
		return 0, 0, err
	}
	limiter := w.Limiters.Get(w.Account.QuotaGroupOrID(), w.Endpoint.Path, w.Endpoint.Rate.Bucket, w.Endpoint.Rate.IntervalMs)
	completed, err := forEachVCPODetailCandidate(candidates, func(page int, candidate db.VCPOCandidate) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		row, httpStatus, apiCode, durationMs, fetchErr := w.fetchVCPODetail(ctx, limiter, candidate)
		if fetchErr != nil {
			_ = db.InsertTaskLog(w.DB, taskID, page, httpStatus, apiCode, 0, durationMs, fetchErr.Error())
			return fmt.Errorf("VC PO detail %s/%s 请求失败: %w", candidate.VCStoreID, candidate.LocalPONumber, fetchErr)
		}
		if upsertErr := db.UpsertRows(w.DB, w.Endpoint.Table, []map[string]any{row}, w.Columns, w.JSONCols, w.Account.ID); upsertErr != nil {
			_ = db.InsertTaskLog(w.DB, taskID, page, httpStatus, apiCode, 0, durationMs, "upsert: "+upsertErr.Error())
			return fmt.Errorf("VC PO detail %s/%s 落库失败: %w", candidate.VCStoreID, candidate.LocalPONumber, upsertErr)
		}
		_ = db.InsertTaskLog(w.DB, taskID, page, httpStatus, apiCode, 1, durationMs, "")
		return nil
	})
	if err != nil {
		log.Printf("[worker:%s] %v", w.Endpoint.Name, err)
		return completed, completed, err
	}
	return completed, completed, nil
}
