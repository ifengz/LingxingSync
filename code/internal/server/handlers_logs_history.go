package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type reportHistoryQuery struct {
	AuditID                                                int64
	Account, StoreID, ReportType, Status, DateFrom, DateTo string
	Page, PageSize                                         int
}

type reportHistoryItem struct {
	ID               int64    `json:"report_audit_id"`
	AccountID        string   `json:"account_id"`
	SellerID         string   `json:"seller_id"`
	StoreID          string   `json:"store_id"`
	ReportType       string   `json:"report_type"`
	Region           string   `json:"region"`
	ReportTaskID     string   `json:"report_task_id"`
	ReportDocumentID *string  `json:"report_document_id"`
	Status           string   `json:"status"`
	DownloadSHA256   *string  `json:"download_sha256"`
	RowsImported     int      `json:"rows_imported"`
	ErrorMessage     *string  `json:"error_message"`
	CreatedAt        *rfc3339 `json:"created_at"`
	DownloadedAt     *rfc3339 `json:"downloaded_at"`
	UpdatedAt        *rfc3339 `json:"updated_at"`
}

type reportHistoryPage struct {
	Items    []reportHistoryItem `json:"items"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

type reconciliationHistoryQuery struct {
	AuditID                                                              int64
	Account, StoreID, ReportType, Status, BusinessDate, DateFrom, DateTo string
	Page, PageSize                                                       int
}

type reconciliationHistoryItem struct {
	ReportAuditID    int64    `json:"report_audit_id"`
	ReportTaskID     string   `json:"report_task_id"`
	AccountID        string   `json:"account_id"`
	StoreID          string   `json:"store_id"`
	ReportType       string   `json:"report_type"`
	ReportDocumentID *string  `json:"report_document_id"`
	BusinessDate     string   `json:"business_date"`
	Status           string   `json:"status"`
	DatabaseMissing  int      `json:"database_missing"`
	ReportMissing    int      `json:"report_missing"`
	ValueMismatch    int      `json:"value_mismatch"`
	ErrorMessage     *string  `json:"error_message"`
	UpdatedAt        *rfc3339 `json:"updated_at"`
}

type reconciliationHistoryPage struct {
	Items    []reconciliationHistoryItem `json:"items"`
	Total    int                         `json:"total"`
	Page     int                         `json:"page"`
	PageSize int                         `json:"page_size"`
}

type reportHistoryReader interface {
	History(context.Context, reportHistoryQuery) (reportHistoryPage, error)
	Reconciliations(context.Context, reconciliationHistoryQuery) (reconciliationHistoryPage, error)
}

type sqlReportHistoryReader struct{ db *sqlx.DB }

func (r sqlReportHistoryReader) History(ctx context.Context, query reportHistoryQuery) (reportHistoryPage, error) {
	if r.db == nil {
		return reportHistoryPage{}, fmt.Errorf("报告下载历史数据库未配置")
	}
	if apiType := reportExportAPIType(query.ReportType); apiType != "" {
		query.ReportType = apiType
	}
	where, args := reportHistoryWhere(query)
	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM ls_report_export_tasks "+where, args...); err != nil {
		return reportHistoryPage{}, fmt.Errorf("查询报告下载历史总数: %w", err)
	}
	args = append(args, query.PageSize, (query.Page-1)*query.PageSize)
	var rows []struct {
		ID               int64          `db:"id"`
		AccountID        string         `db:"account_id"`
		SellerID         string         `db:"seller_id"`
		StoreID          string         `db:"store_id"`
		ReportType       string         `db:"report_type"`
		Region           string         `db:"region"`
		ReportTaskID     string         `db:"report_task_id"`
		ReportDocumentID sql.NullString `db:"report_document_id"`
		DownloadSHA256   sql.NullString `db:"download_sha256"`
		ErrorMessage     sql.NullString `db:"error_message"`
		Status           string         `db:"status"`
		RowsImported     int            `db:"rows_imported"`
		CreatedAt        time.Time      `db:"created_at"`
		UpdatedAt        time.Time      `db:"updated_at"`
		DownloadedAt     sql.NullTime   `db:"downloaded_at"`
	}
	const fields = `id, account_id, seller_id, store_id, report_type, region, report_task_id,
report_document_id, status, download_sha256, rows_imported, error_message, created_at, downloaded_at, updated_at`
	if err := r.db.SelectContext(ctx, &rows, "SELECT "+fields+" FROM ls_report_export_tasks "+where+" ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?", args...); err != nil {
		return reportHistoryPage{}, fmt.Errorf("查询报告下载历史: %w", err)
	}
	items := make([]reportHistoryItem, 0, len(rows))
	for _, row := range rows {
		item := reportHistoryItem{ID: row.ID, AccountID: row.AccountID, SellerID: row.SellerID, StoreID: row.StoreID, ReportType: reportExportConfigType(row.ReportType), Region: row.Region, ReportTaskID: row.ReportTaskID, Status: row.Status, RowsImported: row.RowsImported, CreatedAt: toRFC3339(&row.CreatedAt), UpdatedAt: toRFC3339(&row.UpdatedAt), ReportDocumentID: nullStringPtr(row.ReportDocumentID), DownloadSHA256: nullStringPtr(row.DownloadSHA256), ErrorMessage: nullStringPtr(row.ErrorMessage)}
		if row.DownloadedAt.Valid {
			item.DownloadedAt = toRFC3339(&row.DownloadedAt.Time)
		}
		items = append(items, item)
	}
	return reportHistoryPage{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}

func (r sqlReportHistoryReader) Reconciliations(ctx context.Context, query reconciliationHistoryQuery) (reconciliationHistoryPage, error) {
	if r.db == nil {
		return reconciliationHistoryPage{}, fmt.Errorf("核对历史数据库未配置")
	}
	if apiType := reportExportAPIType(query.ReportType); apiType != "" {
		query.ReportType = apiType
	}
	where, args := reconciliationHistoryWhere(query)
	const from = ` FROM listing_daily_reconciliations r LEFT JOIN ls_report_export_tasks t ON t.id = r.report_audit_id `
	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*)"+from+where, args...); err != nil {
		return reconciliationHistoryPage{}, fmt.Errorf("查询核对历史总数: %w", err)
	}
	args = append(args, query.PageSize, (query.Page-1)*query.PageSize)
	var rows []struct {
		ReportAuditID    int64          `db:"report_audit_id"`
		ReportTaskID     string         `db:"report_task_id"`
		AccountID        sql.NullString `db:"account_id"`
		StoreID          sql.NullString `db:"store_id"`
		ReportType       sql.NullString `db:"report_type"`
		ReportDocumentID sql.NullString `db:"report_document_id"`
		BusinessDate     time.Time      `db:"business_date"`
		Status           string         `db:"status"`
		DatabaseMissing  int            `db:"database_missing"`
		ReportMissing    int            `db:"report_missing"`
		ValueMismatch    int            `db:"value_mismatch"`
		ErrorMessage     sql.NullString `db:"error_message"`
		UpdatedAt        time.Time      `db:"updated_at"`
	}
	const fields = `r.report_audit_id, r.report_task_id, t.account_id, t.store_id, t.report_type, t.report_document_id,
r.business_date, r.status, JSON_LENGTH(r.missing_in_db) AS database_missing,
JSON_LENGTH(r.missing_in_report) AS report_missing, JSON_LENGTH(r.field_diffs) AS value_mismatch,
r.error_message, r.updated_at`
	if err := r.db.SelectContext(ctx, &rows, "SELECT "+fields+from+where+" ORDER BY r.business_date DESC, r.report_audit_id DESC LIMIT ? OFFSET ?", args...); err != nil {
		return reconciliationHistoryPage{}, fmt.Errorf("查询核对历史: %w", err)
	}
	items := make([]reconciliationHistoryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, reconciliationHistoryItem{ReportAuditID: row.ReportAuditID, ReportTaskID: row.ReportTaskID, AccountID: row.AccountID.String, StoreID: row.StoreID.String, ReportType: reportExportConfigType(row.ReportType.String), ReportDocumentID: nullStringPtr(row.ReportDocumentID), BusinessDate: row.BusinessDate.Format("2006-01-02"), Status: row.Status, DatabaseMissing: row.DatabaseMissing, ReportMissing: row.ReportMissing, ValueMismatch: row.ValueMismatch, ErrorMessage: nullStringPtr(row.ErrorMessage), UpdatedAt: toRFC3339(&row.UpdatedAt)})
	}
	return reconciliationHistoryPage{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func reportExportConfigType(value string) string {
	for _, candidate := range availableReportExportTypes() {
		if reportExportAPIType(candidate) == value {
			return candidate
		}
	}
	return value
}

func reportHistoryWhere(query reportHistoryQuery) (string, []any) {
	clauses, args := []string{}, []any{}
	for _, filter := range []struct{ column, value string }{{"account_id", query.Account}, {"store_id", query.StoreID}, {"report_type", query.ReportType}, {"status", query.Status}} {
		if filter.value != "" {
			clauses = append(clauses, filter.column+" = ?")
			args = append(args, filter.value)
		}
	}
	if query.AuditID > 0 {
		clauses = append(clauses, "id = ?")
		args = append(args, query.AuditID)
	}
	if query.DateFrom != "" {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, query.DateFrom)
	}
	if query.DateTo != "" {
		clauses = append(clauses, "created_at < DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, query.DateTo)
	}
	return whereSQL(clauses), args
}

func reconciliationHistoryWhere(query reconciliationHistoryQuery) (string, []any) {
	clauses, args := []string{}, []any{}
	for _, filter := range []struct{ column, value string }{{"t.account_id", query.Account}, {"t.store_id", query.StoreID}, {"t.report_type", query.ReportType}, {"r.status", query.Status}, {"r.business_date", query.BusinessDate}} {
		if filter.value != "" {
			clauses = append(clauses, filter.column+" = ?")
			args = append(args, filter.value)
		}
	}
	if query.DateFrom != "" {
		clauses = append(clauses, "r.business_date >= ?")
		args = append(args, query.DateFrom)
	}
	if query.DateTo != "" {
		clauses = append(clauses, "r.business_date <= ?")
		args = append(args, query.DateTo)
	}
	if query.AuditID > 0 {
		clauses = append(clauses, "r.report_audit_id = ?")
		args = append(args, query.AuditID)
	}
	return whereSQL(clauses), args
}

func whereSQL(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(clauses, " AND ")
}

func parseReportHistoryQuery(r *http.Request) reportHistoryQuery {
	q := r.URL.Query()
	return reportHistoryQuery{AuditID: int64(atoiOr(q.Get("audit_id"), 0)), Account: q.Get("account"), StoreID: q.Get("store_id"), ReportType: q.Get("type"), Status: q.Get("status"), DateFrom: q.Get("date_from"), DateTo: q.Get("date_to"), Page: positivePage(q.Get("page"), 1), PageSize: positivePage(q.Get("page_size"), 20)}
}
func parseReconciliationHistoryQuery(r *http.Request) reconciliationHistoryQuery {
	q := r.URL.Query()
	return reconciliationHistoryQuery{AuditID: int64(atoiOr(q.Get("audit_id"), 0)), Account: q.Get("account"), StoreID: q.Get("store_id"), ReportType: q.Get("type"), Status: q.Get("status"), BusinessDate: q.Get("business_date"), DateFrom: q.Get("date_from"), DateTo: q.Get("date_to"), Page: positivePage(q.Get("page"), 1), PageSize: positivePage(q.Get("page_size"), 20)}
}
func positivePage(raw string, fallback int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return fallback
	}
	if n > 200 {
		return 200
	}
	return n
}

func (s *Server) apiReportHistory(w http.ResponseWriter, r *http.Request) {
	if s.reportHistory == nil {
		errJSON(w, http.StatusInternalServerError, "报告下载历史查询未配置")
		return
	}
	page, err := s.reportHistory.History(r.Context(), parseReportHistoryQuery(r))
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	okJSON(w, page)
}
func (s *Server) apiReportReconciliations(w http.ResponseWriter, r *http.Request) {
	if s.reportHistory == nil {
		errJSON(w, http.StatusInternalServerError, "核对历史查询未配置")
		return
	}
	page, err := s.reportHistory.Reconciliations(r.Context(), parseReconciliationHistoryQuery(r))
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	okJSON(w, page)
}
