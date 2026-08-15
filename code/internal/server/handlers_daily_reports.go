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

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/reportexport"
)

const (
	dailyPreviewMaxPage     = 10000
	dailyPreviewMaxPageSize = 100
	dailyPreviewFromSQL     = " FROM listing_daily_metrics m JOIN listing_dimensions d ON d.id = m.listing_dimension_id "
)

type dailyPreviewQuery struct {
	DateFrom string
	DateTo   string
	Store    string
	ASIN     string
	SKU      string
	Page     int
	PageSize int
}

type dailyPreviewItem struct {
	BusinessDate    string    `json:"business_date"`
	Store           string    `json:"store"`
	Channel         string    `json:"channel"`
	IdentityScope   string    `json:"identity_scope"`
	ASIN            *string   `json:"asin"`
	SKU             *string   `json:"sku"`
	SalesUnits      any       `json:"sales_units"`
	SalesAmount     any       `json:"sales_amount"`
	ReturnsQty      any       `json:"returns_qty"`
	Inventory       any       `json:"inventory_sellable"`
	SessionsDesktop any       `json:"sessions_desktop"`
	SessionsMobile  any       `json:"sessions_mobile"`
	ReviewCount     any       `json:"review_count"`
	Rating          any       `json:"rating"`
	SPSpend         any       `json:"sp_spend"`
	SDSpend         any       `json:"sd_spend"`
	UpdatedAt       time.Time `json:"updated_at"`
	IsProvisional   bool      `json:"is_provisional"`
	IsVerified      bool      `json:"is_verified"`
}

type dailyPreviewPage struct {
	Items []dailyPreviewItem `json:"items"`
	Total int                `json:"total"`
}

type dailyPreviewReader interface {
	Preview(context.Context, dailyPreviewQuery) (dailyPreviewPage, error)
}

type sqlDailyPreviewReader struct{ db *sqlx.DB }

func (r sqlDailyPreviewReader) Preview(ctx context.Context, query dailyPreviewQuery) (dailyPreviewPage, error) {
	if r.db == nil {
		return dailyPreviewPage{}, fmt.Errorf("日维预览数据库未配置")
	}
	where, args := dailyPreviewWhere(query)
	var total int
	countSQL := "SELECT COUNT(*)" + dailyPreviewFromSQL + where
	if err := r.db.GetContext(ctx, &total, countSQL, args...); err != nil {
		return dailyPreviewPage{}, fmt.Errorf("查询日维总数: %w", err)
	}
	const fields = `m.business_date, d.store_id, d.channel, d.identity_scope, d.asin, d.sku,
m.sales_units, m.sales_amount, m.returns_qty, m.inventory_sellable,
m.sessions_desktop, m.sessions_mobile, m.review_count, m.rating, m.sp_spend, m.sd_spend,
m.updated_at, m.is_provisional, m.is_verified`
	listSQL := "SELECT " + fields + dailyPreviewFromSQL + where + " ORDER BY m.business_date DESC, m.listing_dimension_id ASC LIMIT ? OFFSET ?"
	listArgs := append(append([]any(nil), args...), query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := r.db.QueryxContext(ctx, listSQL, listArgs...)
	if err != nil {
		return dailyPreviewPage{}, fmt.Errorf("查询日维数据: %w", err)
	}
	defer rows.Close()
	items := make([]dailyPreviewItem, 0, query.PageSize)
	for rows.Next() {
		var businessDate time.Time
		var store, channel, scope string
		var asin, sku sql.NullString
		var salesUnits, salesAmount, returnsQty, inventory any
		var sessionsDesktop, sessionsMobile, reviewCount, rating, spSpend, sdSpend any
		var updatedAt time.Time
		var provisional, verified bool
		if err := rows.Scan(&businessDate, &store, &channel, &scope, &asin, &sku,
			&salesUnits, &salesAmount, &returnsQty, &inventory,
			&sessionsDesktop, &sessionsMobile, &reviewCount, &rating, &spSpend, &sdSpend, &updatedAt,
			&provisional, &verified); err != nil {
			return dailyPreviewPage{}, fmt.Errorf("读取日维数据: %w", err)
		}
		items = append(items, dailyPreviewItem{
			BusinessDate: businessDate.Format("2006-01-02"), Store: store, Channel: channel, IdentityScope: scope,
			ASIN: nullableString(asin), SKU: nullableString(sku),
			SalesUnits: normalizePreviewValue(salesUnits), SalesAmount: normalizePreviewValue(salesAmount), ReturnsQty: normalizePreviewValue(returnsQty), Inventory: normalizePreviewValue(inventory),
			SessionsDesktop: normalizePreviewValue(sessionsDesktop), SessionsMobile: normalizePreviewValue(sessionsMobile), ReviewCount: normalizePreviewValue(reviewCount), Rating: normalizePreviewValue(rating),
			SPSpend: normalizePreviewValue(spSpend), SDSpend: normalizePreviewValue(sdSpend), UpdatedAt: updatedAt, IsProvisional: provisional, IsVerified: verified,
		})
	}
	if err := rows.Err(); err != nil {
		return dailyPreviewPage{}, fmt.Errorf("读取日维数据: %w", err)
	}
	return dailyPreviewPage{Items: items, Total: total}, nil
}

func dailyPreviewWhere(query dailyPreviewQuery) (string, []any) {
	clauses := []string{"WHERE m.business_date BETWEEN ? AND ?"}
	args := []any{query.DateFrom, query.DateTo}
	for _, filter := range []struct {
		column string
		value  string
	}{{"d.store_id", query.Store}, {"d.asin", query.ASIN}, {"d.sku", query.SKU}} {
		if filter.value == "" {
			continue
		}
		clauses = append(clauses, "AND "+filter.column+" = ?")
		args = append(args, filter.value)
	}
	return strings.Join(clauses, " "), args
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func normalizePreviewValue(value any) any {
	if raw, ok := value.([]byte); ok {
		return string(raw)
	}
	return value
}

func (s *Server) apiDailyPreview(w http.ResponseWriter, r *http.Request) {
	query, err := parseDailyPreviewQuery(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.dailyPreview == nil {
		errJSON(w, http.StatusInternalServerError, "日维预览查询未配置")
		return
	}
	page, err := s.dailyPreview.Preview(r.Context(), query)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	okJSON(w, map[string]any{"items": page.Items, "total": page.Total, "page": query.Page, "page_size": query.PageSize})
}

func parseDailyPreviewQuery(r *http.Request) (dailyPreviewQuery, error) {
	values := r.URL.Query()
	query := dailyPreviewQuery{
		DateFrom: strings.TrimSpace(values.Get("date_from")), DateTo: strings.TrimSpace(values.Get("date_to")),
		Store: strings.TrimSpace(values.Get("store")), ASIN: strings.TrimSpace(values.Get("asin")), SKU: strings.TrimSpace(values.Get("sku")),
	}
	if err := validateSyncDateRange(query.DateFrom, query.DateTo); err != nil {
		return dailyPreviewQuery{}, err
	}
	var err error
	query.Page, err = strictPositiveInt(values.Get("page"), 1, dailyPreviewMaxPage)
	if err != nil {
		return dailyPreviewQuery{}, fmt.Errorf("page 必须为 1..%d", dailyPreviewMaxPage)
	}
	query.PageSize, err = strictPositiveInt(values.Get("page_size"), 20, dailyPreviewMaxPageSize)
	if err != nil {
		return dailyPreviewQuery{}, fmt.Errorf("page_size 必须为 1..%d", dailyPreviewMaxPageSize)
	}
	return query, nil
}

func strictPositiveInt(raw string, defaultValue, maxValue int) (int, error) {
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || (maxValue > 0 && value > maxValue) {
		return 0, fmt.Errorf("invalid positive integer")
	}
	return value, nil
}

type reportExportConfigDTO struct {
	Type           string   `json:"type"`
	Enabled        bool     `json:"enabled"`
	Account        string   `json:"account"`
	SellerID       string   `json:"seller_id"`
	StoreID        string   `json:"store_id"`
	Region         string   `json:"region"`
	MarketplaceIDs []string `json:"marketplace_ids"`
	Cron           string   `json:"cron"`
	WindowDays     int      `json:"window_days"`
}

type reportExportsPutIn struct {
	ReportExports []reportExportConfigDTO `json:"report_exports"`
}

func defaultReportExportDTO() reportExportConfigDTO {
	return reportExportConfigDTO{Type: config.ReportExportCustomerReturns, Region: "na", Cron: "0 4 * * *", WindowDays: 3, MarketplaceIDs: []string{}}
}

func availableReportExportTypes() []string {
	return []string{config.ReportExportCustomerReturns, config.ReportExportCustomerShipmentSales}
}

func supportedReportExportType(value string) bool {
	switch value {
	case config.ReportExportCustomerReturns, config.ReportExportCustomerShipmentSales:
		return true
	default:
		return false
	}
}

func reportExportAPIType(value string) string {
	switch value {
	case config.ReportExportCustomerReturns:
		return reportexport.CustomerReturnsReportType
	case config.ReportExportCustomerShipmentSales:
		return reportexport.CustomerShipmentSalesReportType
	default:
		return ""
	}
}

func reportExportToDTO(report config.ReportExport) reportExportConfigDTO {
	return reportExportConfigDTO{Type: report.Type, Enabled: report.Enabled, Account: report.Account, SellerID: report.SellerID, StoreID: report.StoreID, Region: report.Region, MarketplaceIDs: append([]string(nil), report.MarketplaceIDs...), Cron: report.Cron, WindowDays: report.WindowDays}
}

func reportExportFromDTO(report reportExportConfigDTO) config.ReportExport {
	return config.ReportExport{Type: report.Type, Enabled: report.Enabled, Account: report.Account, SellerID: report.SellerID, StoreID: report.StoreID, Region: report.Region, MarketplaceIDs: append([]string(nil), report.MarketplaceIDs...), Cron: report.Cron, WindowDays: report.WindowDays}
}

func (s *Server) apiGetReportExportConfig(w http.ResponseWriter, _ *http.Request) {
	cfg := s.cfg
	if s.store != nil {
		cfg = s.store.Current()
	}
	if cfg == nil || len(cfg.ReportExports) == 0 {
		okJSON(w, map[string]any{"report_exports": []reportExportConfigDTO{}, "available_types": availableReportExportTypes(), "default": defaultReportExportDTO()})
		return
	}
	result := make([]reportExportConfigDTO, 0, len(cfg.ReportExports))
	for _, report := range cfg.ReportExports {
		result = append(result, reportExportToDTO(report))
	}
	okJSON(w, map[string]any{"report_exports": result, "available_types": availableReportExportTypes(), "default": defaultReportExportDTO()})
}

func (s *Server) apiPutReportExportConfig(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		errJSON(w, http.StatusInternalServerError, "配置存储未初始化")
		return
	}
	var input reportExportsPutIn
	if err := decodeJSON(r, &input); err != nil {
		errJSON(w, http.StatusBadRequest, "请求体格式错误: "+err.Error())
		return
	}
	if input.ReportExports == nil {
		errJSON(w, http.StatusBadRequest, "缺少 report_exports")
		return
	}
	old := s.store.Current()
	snap := s.store.Snapshot()
	snap.ReportExports = make([]config.ReportExport, 0, len(input.ReportExports))
	for _, report := range input.ReportExports {
		if !supportedReportExportType(report.Type) {
			errJSON(w, http.StatusBadRequest, "不支持的正式报表类型")
			return
		}
		snap.ReportExports = append(snap.ReportExports, reportExportFromDTO(report))
	}
	s.applyConfigWrite(w, old, snap, "正式报表配置已保存")
}

type reportExportTaskOut struct {
	ID               int64      `json:"id"`
	ReportTaskID     string     `json:"report_task_id"`
	ReportDocumentID string     `json:"report_document_id"`
	DownloadSHA256   string     `json:"download_sha256"`
	Status           string     `json:"status"`
	SourceStatus     string     `json:"source_status"`
	Rows             int        `json:"rows"`
	Error            *string    `json:"error"`
	CreatedAt        time.Time  `json:"created_at"`
	FinishedAt       *time.Time `json:"finished_at"`
}

type reportExportDifferenceOut struct {
	ReconciliationCount int     `json:"reconciliation_count"`
	DatabaseMissing     int     `json:"database_missing"`
	ReportMissing       int     `json:"report_missing"`
	ValueMismatch       int     `json:"value_mismatch"`
	Error               *string `json:"error"`
}

type reportExportStatusOut struct {
	Configured  bool                      `json:"configured"`
	LatestTask  *reportExportTaskOut      `json:"latest_task"`
	Differences reportExportDifferenceOut `json:"differences"`
}

type reportStatusReader interface {
	Latest(context.Context, string, string, string) (reportExportStatusOut, error)
}

type sqlReportStatusReader struct{ db *sqlx.DB }

const latestReportTaskSQL = `SELECT id, report_task_id, report_document_id, download_sha256, status, rows_imported, error_message, created_at, downloaded_at, updated_at
FROM ls_report_export_tasks WHERE report_type = ? AND account_id = ? AND store_id = ? ORDER BY id DESC LIMIT 1`

const reportDifferencesSQL = `SELECT
COUNT(*) AS reconciliation_count,
COALESCE(SUM(JSON_LENGTH(missing_in_db)), 0) AS database_missing,
COALESCE(SUM(JSON_LENGTH(missing_in_report)), 0) AS report_missing,
COALESCE(SUM(JSON_LENGTH(field_diffs)), 0) AS value_mismatch,
MAX(error_message) AS reconciliation_error
FROM listing_daily_reconciliations WHERE report_audit_id = ?`

func (r sqlReportStatusReader) Latest(ctx context.Context, accountID, storeID, reportType string) (reportExportStatusOut, error) {
	if r.db == nil {
		return reportExportStatusOut{}, fmt.Errorf("正式报表状态数据库未配置")
	}
	var row struct {
		ID               int64          `db:"id"`
		ReportTaskID     sql.NullString `db:"report_task_id"`
		ReportDocumentID sql.NullString `db:"report_document_id"`
		DownloadSHA256   sql.NullString `db:"download_sha256"`
		Status           string         `db:"status"`
		Rows             int            `db:"rows_imported"`
		Error            sql.NullString `db:"error_message"`
		CreatedAt        time.Time      `db:"created_at"`
		DownloadedAt     sql.NullTime   `db:"downloaded_at"`
		UpdatedAt        time.Time      `db:"updated_at"`
	}
	noTask := false
	apiReportType := reportExportAPIType(reportType)
	if apiReportType == "" {
		return reportExportStatusOut{}, fmt.Errorf("不支持的正式报表类型 %q", reportType)
	}
	if err := r.db.GetContext(ctx, &row, latestReportTaskSQL, apiReportType, accountID, storeID); err != nil {
		noTask = err == sql.ErrNoRows
		if !noTask {
			return reportExportStatusOut{}, fmt.Errorf("查询正式报表任务: %w", err)
		}
	}
	var task *reportExportTaskOut
	if !noTask {
		task = &reportExportTaskOut{ID: row.ID, ReportTaskID: row.ReportTaskID.String, ReportDocumentID: row.ReportDocumentID.String, DownloadSHA256: row.DownloadSHA256.String, Status: reportTaskUIStatus(row.Status), SourceStatus: row.Status, Rows: row.Rows, CreatedAt: row.CreatedAt}
		if row.Error.Valid {
			task.Error = &row.Error.String
		}
		if row.DownloadedAt.Valid {
			task.FinishedAt = &row.DownloadedAt.Time
		} else if task.Status == "success" || task.Status == "error" {
			task.FinishedAt = &row.UpdatedAt
		}
	}
	var counts struct {
		ReconciliationCount int            `db:"reconciliation_count"`
		DatabaseMissing     int            `db:"database_missing"`
		ReportMissing       int            `db:"report_missing"`
		ValueMismatch       int            `db:"value_mismatch"`
		Error               sql.NullString `db:"reconciliation_error"`
	}
	if err := r.db.GetContext(ctx, &counts, reportDifferencesSQL, row.ID); err != nil {
		return reportExportStatusOut{}, fmt.Errorf("查询日维对账差异: %w", err)
	}
	differences := reportExportDifferenceOut{ReconciliationCount: counts.ReconciliationCount, DatabaseMissing: counts.DatabaseMissing, ReportMissing: counts.ReportMissing, ValueMismatch: counts.ValueMismatch}
	if counts.Error.Valid {
		differences.Error = &counts.Error.String
		if task != nil && task.Error == nil {
			task.Error = differences.Error
		}
	}
	return reportExportStatusOut{LatestTask: task, Differences: differences}, nil
}

func reportTaskUIStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS":
		return "success"
	case "ERROR", "FATAL", "CANCELLED":
		return "error"
	case "IN_PROGRESS", "DONE", "UNKNOWN":
		return "running"
	default:
		return "pending"
	}
}

func (s *Server) apiReportExportStatus(w http.ResponseWriter, r *http.Request) {
	if s.reportStatus == nil {
		errJSON(w, http.StatusInternalServerError, "正式报表状态查询未配置")
		return
	}
	accountID := strings.TrimSpace(r.URL.Query().Get("account"))
	storeID := strings.TrimSpace(r.URL.Query().Get("store_id"))
	reportType := strings.TrimSpace(r.URL.Query().Get("type"))
	if reportType == "" {
		reportType = config.ReportExportCustomerReturns
	}
	if !supportedReportExportType(reportType) {
		errJSON(w, http.StatusBadRequest, "不支持的正式报表类型")
		return
	}
	if accountID == "" || storeID == "" {
		errJSON(w, http.StatusBadRequest, "account 和 store_id 必填")
		return
	}
	status, err := s.reportStatus.Latest(r.Context(), accountID, storeID, reportType)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg := s.cfg
	if s.store != nil {
		cfg = s.store.Current()
	}
	status.Configured = reportScopeConfigured(cfg, accountID, storeID, reportType)
	okJSON(w, status)
}

func reportScopeConfigured(cfg *config.Config, accountID, storeID, reportType string) bool {
	if cfg == nil {
		return false
	}
	for _, report := range cfg.ReportExports {
		if report.Type == reportType && config.NormID(report.Account) == config.NormID(accountID) && report.StoreID == storeID {
			return true
		}
	}
	return false
}

func (s *Server) registerDailyReportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/datasets/listing-daily-v1/preview", s.apiDailyPreview)
	mux.HandleFunc("GET /api/report-exports/config", s.apiGetReportExportConfig)
	mux.HandleFunc("PUT /api/report-exports/config", s.apiPutReportExportConfig)
	mux.HandleFunc("GET /api/report-exports/status", s.apiReportExportStatus)
}
