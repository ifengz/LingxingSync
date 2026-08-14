package reportexport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	createPath = "/basicOpen/report/create/reportExportTask"
	queryPath  = "/basicOpen/report/query/reportExportTask"
	renewPath  = "/basicOpen/report/amazonReportExportTask"
	// Keep one request within one calendar month; callers can split larger windows.
	maxDateRange           = 31 * 24 * time.Hour
	defaultDownloadTimeout = 60 * time.Second
	defaultSharedTaskWait  = 10 * time.Second
)

// SignedJSONClient is implemented by api.Client. Keeping this small interface
// lets the lifecycle test use an httptest-backed client without a second signer.
type SignedJSONClient interface {
	DoSignedJSON(context.Context, string, string, map[string]any) ([]byte, int, int, error)
}

type Limiter interface {
	Wait(context.Context) error
}

type Request struct {
	AccountID      string
	SellerID       string
	StoreID        string
	Region         string
	MarketplaceIDs []string
	DateFrom       string
	DateTo         string
}

// CanonicalMarketplaceIDs makes marketplace order irrelevant to active-task
// deduplication while preserving the exact IDs sent in the request body.
func CanonicalMarketplaceIDs(ids []string) string {
	canonical := append([]string(nil), ids...)
	sort.Strings(canonical)
	encoded, _ := json.Marshal(canonical)
	return string(encoded)
}

// ActiveScopeKey is a stable digest for the one active report task allowed for
// an account/seller/store/report/region/marketplace/date scope.
func ActiveScopeKey(request Request) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		request.AccountID, request.SellerID, request.StoreID, CustomerReturnsReportType,
		request.Region, CanonicalMarketplaceIDs(request.MarketplaceIDs), canonicalDate(request.DateFrom), canonicalDate(request.DateTo),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func canonicalDate(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format(time.RFC3339Nano)
}

type Audit struct {
	ID               int64
	ReportTaskID     string
	ReportDocumentID string
	Status           string
	CreateClaimed    bool
}

type Store interface {
	EnsureReport(context.Context, Request) (Audit, error)
	LoadReport(context.Context, int64) (Audit, error)
	MarkReportCreated(context.Context, int64, string) error
	MarkReportProgress(context.Context, int64, string, string, string, string) error
	SaveCustomerReturns(context.Context, int64, []CustomerReturn, string, string) error
	MarkReportError(context.Context, int64, string, error) error
}

type Result struct {
	AuditID          int64
	ReportTaskID     string
	ReportDocumentID string
	Status           string
	DownloadSHA256   string
	DownloadedBytes  int64
	Rows             int
}

type Runner struct {
	Client       SignedJSONClient
	Store        Store
	HTTP         *http.Client
	Limiter      Limiter
	PollInterval time.Duration
	PollTimeout  time.Duration
}

func (r *Runner) Run(ctx context.Context, request Request) (result Result, err error) {
	if r.Client == nil || r.Store == nil {
		return result, fmt.Errorf("report export: client and store are required")
	}
	if err := validateRequest(request); err != nil {
		return result, err
	}
	audit, err := r.Store.EnsureReport(ctx, request)
	if err != nil {
		return result, err
	}
	result.AuditID = audit.ID
	result.ReportTaskID = audit.ReportTaskID
	result.ReportDocumentID = audit.ReportDocumentID
	if strings.EqualFold(audit.Status, "SUCCESS") {
		result.Status = audit.Status
		return result, nil
	}
	ownsAudit := audit.CreateClaimed
	fail := func(cause error) (Result, error) {
		if ownsAudit {
			if auditErr := r.Store.MarkReportError(ctx, audit.ID, "ERROR", cause); auditErr != nil {
				cause = fmt.Errorf("%w; mark audit error: %v", cause, auditErr)
			}
		}
		result.Status = "ERROR"
		return result, cause
	}

	if audit.ReportTaskID == "" && !audit.CreateClaimed {
		audit, err = r.waitForTask(ctx, audit.ID)
		if err != nil {
			return fail(err)
		}
		result.ReportTaskID = audit.ReportTaskID
		result.ReportDocumentID = audit.ReportDocumentID
		if strings.EqualFold(audit.Status, "SUCCESS") {
			result.Status = audit.Status
			return result, nil
		}
	}

	if audit.ReportTaskID == "" {
		raw, callErr := r.call(ctx, createPath, createBody(request))
		if callErr != nil {
			return fail(callErr)
		}
		var data struct {
			TaskID string `json:"task_id"`
		}
		if parseErr := parseEnvelope(raw, &data); parseErr != nil {
			return fail(parseErr)
		}
		if data.TaskID == "" {
			return fail(fmt.Errorf("report export: create response missing data.task_id"))
		}
		audit.ReportTaskID = data.TaskID
		result.ReportTaskID = data.TaskID
		if err := r.Store.MarkReportCreated(ctx, audit.ID, data.TaskID); err != nil {
			return fail(err)
		}
	}

	data, pollErr := r.waitForDone(ctx, request, audit.ID, audit.ReportTaskID)
	if pollErr != nil {
		return fail(pollErr)
	}
	result.ReportDocumentID = data.ReportDocumentID
	if data.ReportDocumentID == "" || data.URL == "" {
		return fail(fmt.Errorf("report export: DONE response missing report_document_id or url"))
	}

	body, hash, contentType, downloadErr := r.download(ctx, request, data)
	if downloadErr != nil {
		return fail(downloadErr)
	}
	rows, parseErr := ParseCustomerReturns(body, data.CompressionAlgorithm, contentType)
	if parseErr != nil {
		return fail(parseErr)
	}
	if err := r.Store.SaveCustomerReturns(ctx, audit.ID, rows, hash, data.ReportDocumentID); err != nil {
		return fail(err)
	}
	result.Status = "SUCCESS"
	result.DownloadSHA256 = hash
	result.DownloadedBytes = int64(len(body))
	result.Rows = len(rows)
	return result, nil
}

func (r *Runner) waitForTask(ctx context.Context, auditID int64) (Audit, error) {
	interval := r.PollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	timeout := defaultSharedTaskWait
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		audit, err := r.Store.LoadReport(ctx, auditID)
		if err != nil {
			return Audit{}, fmt.Errorf("report export: load shared audit: %w", err)
		}
		if audit.ReportTaskID != "" || strings.EqualFold(audit.Status, "SUCCESS") {
			return audit, nil
		}
		if strings.EqualFold(audit.Status, "ERROR") {
			return audit, fmt.Errorf("report export: shared audit entered ERROR before task creation")
		}
		select {
		case <-ctx.Done():
			return Audit{}, ctx.Err()
		case <-deadline.C:
			return Audit{}, fmt.Errorf("report export: waiting for shared task timed out after %s", timeout)
		case <-time.After(interval):
		}
	}
}

type reportStatus struct {
	ReportDocumentID     string `json:"report_document_id"`
	ProgressStatus       string `json:"progress_status"`
	CompressionAlgorithm string `json:"compression_algorithm"`
	URL                  string `json:"url"`
}

func (r *Runner) waitForDone(ctx context.Context, request Request, auditID int64, taskID string) (reportStatus, error) {
	interval := r.PollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	timeout := r.PollTimeout
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var lastStatus reportStatus
	var lastRaw []byte
	for {
		raw, err := r.call(ctx, queryPath, map[string]any{"seller_id": request.SellerID, "task_id": taskID, "region": request.Region})
		if err != nil {
			return reportStatus{}, err
		}
		var data reportStatus
		if err := decodeEnvelope(raw, &data); err != nil {
			return data, err
		}
		if err := r.Store.MarkReportProgress(ctx, auditID, data.ProgressStatus, data.ReportDocumentID, data.URL, data.CompressionAlgorithm); err != nil {
			return data, err
		}
		switch strings.ToUpper(data.ProgressStatus) {
		case "DONE":
			return data, nil
		case "IN_PROGRESS", "IN_QUEUE":
			lastStatus = reportStatus{}
			lastRaw = nil
		case "CANCELLED", "FATAL":
			return data, fmt.Errorf("report export: upstream progress_status=%s; %s", data.ProgressStatus, responseDiagnostics(raw))
		case "UNKNOWN":
			lastStatus = data
			lastRaw = append(lastRaw[:0], raw...)
		default:
			return data, fmt.Errorf("report export: unknown progress_status=%q", data.ProgressStatus)
		}
		select {
		case <-ctx.Done():
			return data, ctx.Err()
		case <-deadline.C:
			if strings.EqualFold(lastStatus.ProgressStatus, "UNKNOWN") {
				return lastStatus, fmt.Errorf("report export: polling timed out after %s; last progress_status=%s; %s", timeout, lastStatus.ProgressStatus, responseDiagnostics(lastRaw))
			}
			return data, fmt.Errorf("report export: polling timed out after %s", timeout)
		case <-time.After(interval):
		}
	}
}

func (r *Runner) download(ctx context.Context, request Request, data reportStatus) ([]byte, string, string, error) {
	hc := r.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: defaultDownloadTimeout}
	}
	get := func(url string) ([]byte, string, error) {
		downloadCtx := ctx
		cancel := func() {}
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			downloadCtx, cancel = context.WithTimeout(ctx, defaultDownloadTimeout)
		}
		defer cancel()
		req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, url, nil)
		if err != nil {
			return nil, "", err
		}
		response, err := hc.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, "", fmt.Errorf("download report: HTTP %d", response.StatusCode)
		}
		body, err := io.ReadAll(response.Body)
		return body, response.Header.Get("Content-Type"), err
	}
	body, contentType, err := get(data.URL)
	if err != nil {
		if data.ReportDocumentID == "" {
			return nil, "", "", err
		}
		raw, renewErr := r.call(ctx, renewPath, map[string]any{"region": request.Region, "seller_id": request.SellerID, "report_document_id": data.ReportDocumentID})
		if renewErr != nil {
			return nil, "", "", fmt.Errorf("download report: %v; renew link: %w", err, renewErr)
		}
		var renewed struct {
			URL string `json:"url"`
		}
		if renewErr := decodeEnvelope(raw, &renewed); renewErr != nil || renewed.URL == "" {
			if renewErr == nil {
				renewErr = fmt.Errorf("renew response missing data.url")
			}
			return nil, "", "", fmt.Errorf("download report: %v; renew link: %w", err, renewErr)
		}
		body, contentType, err = get(renewed.URL)
		if err != nil {
			return nil, "", "", err
		}
	}
	sum := sha256.Sum256(body)
	return body, hex.EncodeToString(sum[:]), contentType, nil
}

func (r *Runner) call(ctx context.Context, path string, body map[string]any) ([]byte, error) {
	if r.Limiter != nil {
		if err := r.Limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("report export: rate limiter: %w", err)
		}
	}
	raw, _, _, err := r.Client.DoSignedJSON(ctx, http.MethodPost, path, body)
	return raw, err
}

func createBody(request Request) map[string]any {
	body := map[string]any{"seller_id": request.SellerID, "report_type": CustomerReturnsReportType, "marketplace_ids": request.MarketplaceIDs, "region": request.Region}
	if request.DateFrom != "" {
		body["data_start_time"] = request.DateFrom
	}
	if request.DateTo != "" {
		body["data_end_time"] = request.DateTo
	}
	return body
}

func validateRequest(request Request) error {
	if !validIdentifier(request.AccountID, 32) {
		return fmt.Errorf("report export: invalid account_id")
	}
	if !validIdentifier(request.SellerID, 64) {
		return fmt.Errorf("report export: invalid seller_id")
	}
	if !validIdentifier(request.StoreID, 64) {
		return fmt.Errorf("report export: store_id is required")
	}
	if request.Region != "na" && request.Region != "eu" && request.Region != "fe" {
		return fmt.Errorf("report export: region must be na, eu or fe")
	}
	if len(request.MarketplaceIDs) == 0 {
		return fmt.Errorf("report export: marketplace_ids are required")
	}
	seenMarketplaces := make(map[string]struct{}, len(request.MarketplaceIDs))
	for _, marketplaceID := range request.MarketplaceIDs {
		if !validIdentifier(marketplaceID, 64) {
			return fmt.Errorf("report export: marketplace_ids contains an invalid item")
		}
		if _, exists := seenMarketplaces[marketplaceID]; exists {
			return fmt.Errorf("report export: marketplace_ids contains a duplicate item")
		}
		seenMarketplaces[marketplaceID] = struct{}{}
	}
	if request.DateFrom == "" || request.DateTo == "" {
		return fmt.Errorf("report export: data_start_time and data_end_time are required")
	}
	from, err := time.Parse(time.RFC3339, request.DateFrom)
	if err != nil {
		return fmt.Errorf("report export: invalid data_start_time %q: %w", request.DateFrom, err)
	}
	to, err := time.Parse(time.RFC3339, request.DateTo)
	if err != nil {
		return fmt.Errorf("report export: invalid data_end_time %q: %w", request.DateTo, err)
	}
	if to.Before(from) {
		return fmt.Errorf("report export: data_end_time is before data_start_time")
	}
	if to.Sub(from) > maxDateRange {
		return fmt.Errorf("report export: date range exceeds %s", maxDateRange)
	}
	return nil
}

func validIdentifier(value string, maxLength int) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && trimmed == value && len(value) <= maxLength && strings.IndexFunc(value, unicode.IsSpace) < 0
}

func decodeEnvelope(raw []byte, target any) error {
	var envelope struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
		Msg     string          `json:"msg"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("report export: decode response: %w", err)
	}
	var code int
	if err := json.Unmarshal(envelope.Code, &code); err != nil {
		var text string
		if stringErr := json.Unmarshal(envelope.Code, &text); stringErr != nil || text != "0" {
			return fmt.Errorf("report export: upstream code is not zero")
		}
	}
	if code != 0 {
		message := envelope.Message
		if message == "" {
			message = envelope.Msg
		}
		return fmt.Errorf("report export: upstream code=%d message=%q", code, message)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("report export: response data is null")
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return fmt.Errorf("report export: decode response data: %w", err)
	}
	return nil
}

func responseDiagnostics(raw []byte) string {
	var envelope struct {
		RequestID    string          `json:"request_id"`
		Message      string          `json:"message"`
		Msg          string          `json:"msg"`
		ErrorDetails json.RawMessage `json:"error_details"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "response diagnostics unavailable"
	}
	message := strings.TrimSpace(envelope.Message)
	if message == "" {
		message = strings.TrimSpace(envelope.Msg)
	}
	parts := make([]string, 0, 3)
	if envelope.RequestID != "" {
		parts = append(parts, "request_id="+envelope.RequestID)
	}
	if message != "" {
		parts = append(parts, "message="+strconv.Quote(message))
	}
	if len(envelope.ErrorDetails) > 0 && string(envelope.ErrorDetails) != "null" && string(envelope.ErrorDetails) != "[]" {
		parts = append(parts, "error_details="+string(envelope.ErrorDetails))
	}
	if len(parts) == 0 {
		return "response diagnostics unavailable"
	}
	return strings.Join(parts, "; ")
}

func parseEnvelope(raw []byte, target any) error { return decodeEnvelope(raw, target) }
