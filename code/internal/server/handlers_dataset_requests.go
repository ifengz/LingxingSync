// handlers_dataset_requests.go — 下游数据集请求日志（dataset_request_logs）的 API。
//
// 宪法对应：doc/04-api.md（/api/* 路由）。读取口径与 /api/report-reconciliations 相同：
// reader 接口 + 分页过滤，单写者是 datasetapi.Handler 的 RequestLogger 钩子。
package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jmoiron/sqlx"

	"lingxing-sync/internal/db"
)

type datasetRequestLogQuery struct {
	Dataset, Endpoint, Project, Status, DateFrom, DateTo string
	Page, PageSize                                       int
}

type datasetRequestLogItem struct {
	ID           int64    `json:"id"`
	DatasetID    string   `json:"dataset_id"`
	Endpoint     string   `json:"endpoint"`
	ProjectID    string   `json:"project_id"`
	TokenID      string   `json:"token_id"`
	Store        string   `json:"store"`
	DateFrom     string   `json:"date_from"`
	DateTo       string   `json:"date_to"`
	StatusCode   int      `json:"status_code"`
	RowsReturned int      `json:"rows_returned"`
	DurationMs   int64    `json:"duration_ms"`
	ErrorMessage *string  `json:"error_message"`
	CreatedAt    *rfc3339 `json:"created_at"`
}

type datasetRequestLogPage struct {
	Items    []datasetRequestLogItem `json:"items"`
	Total    int                     `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
}

type datasetRequestLogReader interface {
	List(ctx context.Context, q datasetRequestLogQuery) (datasetRequestLogPage, error)
}

type sqlDatasetRequestLogReader struct{ db *sqlx.DB }

func (r sqlDatasetRequestLogReader) List(ctx context.Context, q datasetRequestLogQuery) (datasetRequestLogPage, error) {
	if r.db == nil {
		return datasetRequestLogPage{}, fmt.Errorf("下游请求日志数据库未配置")
	}
	logs, total, err := db.ListDatasetRequests(r.db, db.DatasetRequestLogQuery{
		Dataset: q.Dataset, Endpoint: q.Endpoint, Project: q.Project, Status: q.Status, DateFrom: q.DateFrom, DateTo: q.DateTo, Page: q.Page, PageSize: q.PageSize,
	})
	if err != nil {
		return datasetRequestLogPage{}, err
	}
	items := make([]datasetRequestLogItem, 0, len(logs))
	for _, l := range logs {
		var errMsg *string
		if l.ErrorMessage != "" {
			s := l.ErrorMessage
			errMsg = &s
		}
		created := rfc3339(l.CreatedAt)
		items = append(items, datasetRequestLogItem{
			ID: l.ID, DatasetID: l.DatasetID, Endpoint: l.Endpoint, ProjectID: l.ProjectID, TokenID: l.TokenID,
			Store: l.Store, DateFrom: l.DateFrom, DateTo: l.DateTo, StatusCode: l.StatusCode,
			RowsReturned: l.RowsReturned, DurationMs: l.DurationMs, ErrorMessage: errMsg, CreatedAt: &created,
		})
	}
	return datasetRequestLogPage{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func parseDatasetRequestLogQuery(r *http.Request) datasetRequestLogQuery {
	v := r.URL.Query()
	return datasetRequestLogQuery{
		Dataset: v.Get("dataset"), Endpoint: v.Get("endpoint"), Project: v.Get("project"), Status: v.Get("status"),
		DateFrom: v.Get("date_from"), DateTo: v.Get("date_to"),
		Page: positivePage(v.Get("page"), 1), PageSize: positivePage(v.Get("page_size"), 20),
	}
}

func (s *Server) apiDatasetRequests(w http.ResponseWriter, r *http.Request) {
	if s.datasetRequestLog == nil {
		errJSON(w, http.StatusInternalServerError, "下游请求日志查询未配置")
		return
	}
	page, err := s.datasetRequestLog.List(r.Context(), parseDatasetRequestLogQuery(r))
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	okJSON(w, page)
}
