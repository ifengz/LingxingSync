package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"lingxing-sync/internal/reportexport"
)

// apiReportExportRun 异步执行一笔正式报表（创建→终态→下载→解析→raw/日维投影）。
// 用途：历史月份回填（如 1-3 月销售报表）；日常调度仍走 scheduler 的 cron。
// 领星轮询可能持续数分钟，同步等待会撞反向代理超时，因此后台执行、立即返回；
// 进度经 /api/report-reconciliations 与报告历史查。同一时间只允许一笔在飞。
func (s *Server) apiReportExportRun(w http.ResponseWriter, r *http.Request) {
	if s.reportRun == nil {
		errJSON(w, http.StatusServiceUnavailable, "正式报表执行器未注入")
		return
	}
	s.reportRunMu.Lock()
	if s.reportRunBusy {
		s.reportRunMu.Unlock()
		errJSON(w, http.StatusConflict, "已有一笔报表回填在执行中，请稍后再试")
		return
	}
	s.reportRunBusy = true
	s.reportRunMu.Unlock()
	var in struct {
		Type           string `json:"type"`
		Account        string `json:"account"`
		SellerID       string `json:"seller_id"`
		StoreID        string `json:"store_id"`
		Region         string `json:"region"`
		MarketplaceIDs string `json:"marketplace_ids"`
		DateFrom       string `json:"date_from"`
		DateTo         string `json:"date_to"`
	}
	if err := decodeJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "请求体格式错误: "+err.Error())
		return
	}
	in.Type = strings.TrimSpace(in.Type)
	in.Account = strings.TrimSpace(in.Account)
	in.StoreID = strings.TrimSpace(in.StoreID)
	in.Region = strings.TrimSpace(in.Region)
	if in.Type == "" || in.Account == "" || in.StoreID == "" {
		errJSON(w, http.StatusBadRequest, "type / account / store_id 必填")
		return
	}
	from, err := time.Parse("2006-01-02", strings.TrimSpace(in.DateFrom))
	if err != nil {
		errJSON(w, http.StatusBadRequest, "date_from 必须是 YYYY-MM-DD")
		return
	}
	to, err := time.Parse("2006-01-02", strings.TrimSpace(in.DateTo))
	if err != nil {
		errJSON(w, http.StatusBadRequest, "date_to 必须是 YYYY-MM-DD")
		return
	}
	if to.Before(from) {
		errJSON(w, http.StatusBadRequest, "date_to 不能早于 date_from")
		return
	}
	// 保守上限：正式报表单笔最多 92 天（与 single-day 手动同步一致），防止误传全年拖死执行器。
	if int(to.Sub(from).Hours()/24)+1 > 92 {
		errJSON(w, http.StatusBadRequest, "单笔报表日期范围不能超过 92 天，请按月拆分")
		return
	}

	request := reportexport.Request{
		ReportType:     in.Type,
		AccountID:      in.Account,
		SellerID:       in.SellerID,
		StoreID:        in.StoreID,
		Region:         regionOrDefault(in.Region),
		MarketplaceIDs: splitMarketplaceIDs(in.MarketplaceIDs),
		DateFrom:       from.Format(time.RFC3339),
		DateTo:         to.AddDate(0, 0, 1).Add(-time.Second).Format(time.RFC3339),
	}

	// seller_id 缺省时按账号店铺范围兜底（与页面批量配置同一来源）。
	if strings.TrimSpace(request.SellerID) == "" && s.reportStoreScope != nil {
		stores, scopeErr := s.reportStoreScope.Stores(r.Context(), request.AccountID)
		if scopeErr != nil {
			errJSON(w, http.StatusBadRequest, "读取账号店铺范围失败: "+scopeErr.Error())
			return
		}
		for _, store := range stores {
			if store.SID == request.StoreID {
				request.SellerID = store.SellerID
				if len(request.MarketplaceIDs) == 0 && store.MarketplaceID != "" {
					request.MarketplaceIDs = []string{store.MarketplaceID}
				}
				break
			}
		}
	}
	if strings.TrimSpace(request.SellerID) == "" {
		errJSON(w, http.StatusBadRequest, "seller_id 未提供且店铺表中无该店铺记录")
		return
	}

	go func() {
		defer func() {
			s.reportRunMu.Lock()
			s.reportRunBusy = false
			s.reportRunMu.Unlock()
			if rec := recover(); rec != nil {
				log.Printf("[report-run] panic: %v", rec)
			}
		}()
		runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		result, runErr := s.reportRun(runCtx, request)
		if runErr != nil {
			log.Printf("[report-run] 失败 type=%s store=%s %s..%s: %v", request.ReportType, request.StoreID, request.DateFrom[:10], request.DateTo[:10], runErr)
			return
		}
		log.Printf("[report-run] 完成 type=%s store=%s %s..%s audit=%d rows=%d", request.ReportType, request.StoreID, request.DateFrom[:10], request.DateTo[:10], result.AuditID, result.Rows)
	}()
	okJSON(w, map[string]any{
		"status":      "started",
		"report_type": request.ReportType,
		"store_id":    request.StoreID,
		"date_from":   request.DateFrom,
		"date_to":     request.DateTo,
	})
}

func regionOrDefault(region string) string {
	if region == "" {
		return "na"
	}
	return region
}

func splitMarketplaceIDs(raw string) []string {
	out := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
