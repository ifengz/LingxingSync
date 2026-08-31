// Package datasetapi exposes the one fixed internal listing dataset contract.
// The reader is injected so this package does not depend on the 02 projection
// package or guess its physical SQL schema.
package datasetapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DatasetID       = "listing-daily-v1"
	DatasetName     = "Listing 日维指标表"
	SnapshotPath    = "/api/v1/datasets/listing-daily-v1/snapshot"
	ChangesPath     = "/api/v1/datasets/listing-daily-v1/changes"
	FieldsPath      = "/api/datasources/datasets/listing-daily-v1/fields"
	SchemaVersion   = "listing-daily-v1"
	DefaultPageSize = 100
	DefaultDateSpan = 31
	DefaultMaxPage  = 1000
)

var FixedFields = []string{
	"store", "channel", "asin", "sku", "business_date", "updated_at", "is_provisional", "verification_status",
}

type Config struct {
	Definition      Definition
	Tokens          []Token
	FieldAllowlist  []string
	CatalogFields   []string
	MaxDateSpanDays int
	MaxPageSize     int
	CursorSecret    []byte
	PersistFields   func(string, []string) error
	RequestLogger   func(RequestLog)
}

type Token struct {
	ID            string
	ProjectID     string
	Hash          string
	DatasetScopes []string
	StoreScopes   []string
	Fields        []string
	ExpiresAt     time.Time
	Revoked       bool
}

// RequestLog 是一次下游请求的落定事实，由 handler 出口单点回调。
// 留存端（server 层注入 db 写入）负责落库；未注入时只留 stdout。
type RequestLog struct {
	DatasetID    string
	Endpoint     string // snapshot | changes | fields
	ProjectID    string // 认证失败时为空
	TokenID      string // 认证失败时为空
	Store        string
	DateFrom     string
	DateTo       string
	StatusCode   int
	RowsReturned int
	DurationMs   int64
	ErrorMessage string
}

type CursorKey struct {
	UpdatedAt time.Time `json:"updated_at"`
	StableKey string    `json:"stable_key"`
}

type Row struct {
	AccountID          string
	Store              string
	Channel            string
	ASIN               string
	SKU                string
	BusinessDate       string
	UpdatedAt          time.Time
	StableKey          string
	DeletedAt          *time.Time
	IsProvisional      bool
	VerificationStatus string
	FixedValues        map[string]any
	Values             map[string]any
}

type Query struct {
	Store    string
	Stores   []string
	DateFrom string
	DateTo   string
	Fields   []string
	PageSize int
	Cursor   *CursorKey
}

type Page struct {
	Rows    []Row
	HasMore bool
	Next    *CursorKey
}

type Reader interface {
	Snapshot(context.Context, Query) (Page, error)
	Changes(context.Context, Query) (Page, error)
}

type Handler struct {
	reader     Reader
	cfg        Config
	definition Definition
	mu         sync.RWMutex
	available  map[string]struct{}
	catalog    map[string]struct{}
	tokens     map[string]Token
}

type request struct {
	Store    string   `json:"store"`
	DateFrom string   `json:"date_from"`
	DateTo   string   `json:"date_to"`
	Fields   []string `json:"fields"`
	PageSize *int     `json:"page_size"`
	Cursor   string   `json:"cursor"`
}

type cursorEnvelope struct {
	Version   int        `json:"version"`
	Dataset   string     `json:"dataset"`
	Kind      string     `json:"kind"`
	TokenID   string     `json:"token_id"`
	Store     string     `json:"store"`
	DateFrom  string     `json:"date_from,omitempty"`
	DateTo    string     `json:"date_to,omitempty"`
	Key       CursorKey  `json:"key"`
	Watermark *CursorKey `json:"watermark,omitempty"`
}

type Response struct {
	OK    bool         `json:"ok"`
	Data  ResponseData `json:"data,omitempty"`
	Error string       `json:"error,omitempty"`
}

type ResponseData struct {
	SchemaVersion string           `json:"schema_version"`
	Rows          []map[string]any `json:"rows"`
	NextCursor    string           `json:"next_cursor,omitempty"`
	ChangesCursor string           `json:"changes_cursor,omitempty"`
	HasMore       bool             `json:"has_more"`
}

type FieldsResponse struct {
	OK    bool               `json:"ok"`
	Data  FieldsResponseData `json:"data,omitempty"`
	Error string             `json:"error,omitempty"`
}

type FieldsResponseData struct {
	DatasetID        string          `json:"dataset_id"`
	DatasetName      string          `json:"dataset_name"`
	FixedFields      []string        `json:"fixed_fields"`
	ProjectID        string          `json:"project_id,omitempty"`
	TokenID          string          `json:"token_id,omitempty"`
	AvailableFields  []string        `json:"available_fields"`
	CatalogFields    []string        `json:"catalog_fields,omitempty"`
	ConfiguredFields []string        `json:"configured_fields,omitempty"`
	Fields           []string        `json:"fields,omitempty"`
	Projects         []ProjectFields `json:"projects"`
}

type ProjectFields struct {
	ProjectID     string   `json:"project_id"`
	TokenID       string   `json:"token_id"`
	Token         string   `json:"token,omitempty"`
	DatasetScopes []string `json:"dataset_scopes"`
	StoreScopes   []string `json:"store_scopes"`
	Fields        []string `json:"fields"`
}

type fieldsRequest struct {
	ProjectID string   `json:"project_id"`
	TokenID   string   `json:"token_id"`
	Fields    []string `json:"fields"`
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func New(cfg Config, reader Reader) (*Handler, error) {
	definition := cfg.Definition
	if definition.ID == "" {
		definition, _ = DefinitionFor(DatasetID)
	}
	if definition.ID == "" || definition.Name == "" || len(definition.FixedFields) == 0 {
		return nil, errors.New("dataset definition is invalid")
	}
	if len(cfg.FieldAllowlist) == 0 && len(cfg.Tokens) > 0 {
		return nil, errors.New("dataset field allowlist is empty")
	}
	if cfg.MaxDateSpanDays == 0 {
		cfg.MaxDateSpanDays = DefaultDateSpan
	}
	if cfg.MaxPageSize == 0 {
		cfg.MaxPageSize = DefaultMaxPage
	}
	if cfg.MaxDateSpanDays < 1 || cfg.MaxPageSize < 1 {
		return nil, errors.New("dataset limits must be positive")
	}
	available := make(map[string]struct{}, len(cfg.FieldAllowlist))
	for _, field := range cfg.FieldAllowlist {
		field = strings.TrimSpace(field)
		if field == "" || strings.ContainsAny(field, " .(),=;'") {
			return nil, errors.New("dataset field allowlist contains invalid field")
		}
		if _, exists := available[field]; exists {
			return nil, errors.New("dataset field allowlist contains duplicate field")
		}
		available[field] = struct{}{}
	}
	catalog := make(map[string]struct{}, len(cfg.CatalogFields))
	for _, field := range cfg.CatalogFields {
		field = strings.TrimSpace(field)
		if field == "" || strings.ContainsAny(field, " .(),=;'") {
			return nil, errors.New("dataset field catalog contains invalid field")
		}
		if _, exists := catalog[field]; exists {
			return nil, errors.New("dataset field catalog contains duplicate field")
		}
		catalog[field] = struct{}{}
	}
	if len(catalog) == 0 {
		for field := range available {
			catalog[field] = struct{}{}
		}
	}
	for field := range available {
		catalog[field] = struct{}{}
	}
	tokens := make(map[string]Token, len(cfg.Tokens))
	for _, token := range cfg.Tokens {
		if token.ID == "" || len(token.Hash) != sha256.Size*2 {
			return nil, errors.New("dataset token id/hash is invalid")
		}
		if _, err := hex.DecodeString(token.Hash); err != nil {
			return nil, errors.New("dataset token hash is invalid")
		}
		if _, exists := tokens[token.ID]; exists {
			return nil, errors.New("dataset token id is duplicated")
		}
		if token.ProjectID == "" {
			token.ProjectID = token.ID
		}
		if len(token.Fields) == 0 {
			return nil, errors.New("dataset token fields are empty")
		}
		seenFields := make(map[string]struct{}, len(token.Fields))
		for _, field := range token.Fields {
			if _, ok := available[field]; !ok {
				return nil, errors.New("dataset token field is not allowlisted")
			}
			if _, exists := seenFields[field]; exists {
				return nil, errors.New("dataset token field is duplicated")
			}
			seenFields[field] = struct{}{}
		}
		tokens[token.ID] = token
	}
	return &Handler{reader: reader, cfg: cfg, definition: definition, available: available, catalog: catalog, tokens: tokens}, nil
}

// requestLogAccum 累积一次 snapshot/changes 请求的落定事实，出口经
// logDatasetRequest 回调注入的 RequestLogger（未注入则不留痕）。
type requestLogAccum struct {
	datasetID string
	endpoint  string
	projectID string
	tokenID   string
	store     string
	dateFrom  string
	dateTo    string
	status    int
	rows      int
	errMsg    string
	// fieldsResp：fields 路由的错误响应用 FieldsResponse 包裹（保持原有 wire 形状），
	// snapshot/changes 用 Response。
	fieldsResp bool
}

func (a *requestLogAccum) fail(w http.ResponseWriter, status int, msg string) {
	a.status = status
	a.errMsg = msg
	if a.fieldsResp {
		writeJSON(w, status, FieldsResponse{OK: false, Error: msg})
		return
	}
	writeError(w, status, msg)
}

func (h *Handler) logDatasetRequest(a *requestLogAccum, start time.Time) {
	status := a.status
	if status == 0 { // panic 未及写状态码时按 500 落账
		status = http.StatusInternalServerError
	}
	if h.cfg.RequestLogger == nil {
		return
	}
	h.cfg.RequestLogger(RequestLog{
		DatasetID: a.datasetID, Endpoint: a.endpoint, ProjectID: a.projectID, TokenID: a.tokenID,
		Store: a.store, DateFrom: a.dateFrom, DateTo: a.dateTo,
		StatusCode: status, RowsReturned: a.rows,
		DurationMs: time.Since(start).Milliseconds(), ErrorMessage: a.errMsg,
	})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == h.fieldsPath() {
		h.serveFields(w, r)
		return
	}
	if r.Method != http.MethodPost || (r.URL.Path != h.snapshotPath() && r.URL.Path != h.changesPath()) {
		http.NotFound(w, r)
		return
	}
	endpoint := "changes"
	if r.URL.Path == h.snapshotPath() {
		endpoint = "snapshot"
	}
	start := time.Now()
	acc := &requestLogAccum{datasetID: h.definition.ID, endpoint: endpoint}
	defer func() { h.logDatasetRequest(acc, start) }()
	h.serveDataset(w, r, acc)
}

func (h *Handler) serveDataset(w http.ResponseWriter, r *http.Request, acc *requestLogAccum) {
	token, status, err := h.authenticate(r)
	if err != nil {
		acc.fail(w, status, err.Error())
		return
	}
	acc.projectID, acc.tokenID = token.ProjectID, token.ID
	var in request
	if err := decodeJSON(r, &in); err != nil {
		acc.fail(w, http.StatusBadRequest, err.Error())
		return
	}
	acc.store, acc.dateFrom, acc.dateTo = in.Store, in.DateFrom, in.DateTo
	fields, pageSize, err := h.validateRequest(&in, r.URL.Path == h.snapshotPath(), token)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "scope") {
			status = http.StatusForbidden
		}
		acc.fail(w, status, err.Error())
		return
	}
	h.mu.RLock()
	reader := h.reader
	h.mu.RUnlock()
	if reader == nil {
		acc.fail(w, http.StatusServiceUnavailable, "listing daily reader is not configured")
		return
	}
	var page Page
	var query Query
	var snapshotWatermark *CursorKey
	if r.URL.Path == h.snapshotPath() {
		query = Query{Store: in.Store, DateFrom: in.DateFrom, DateTo: in.DateTo, Fields: fields, PageSize: pageSize}
		if in.Cursor != "" {
			cursor, cursorErr := h.decodeCursor(in.Cursor, "snapshot", token.ID, in.Store, in.DateFrom, in.DateTo)
			if cursorErr != nil {
				acc.fail(w, cursorErrorStatus(cursorErr), cursorErr.Error())
				return
			}
			query.Cursor = &cursor.Key
			snapshotWatermark = cursor.Watermark
		} else {
			initialCursor := h.definition.InitialCursor
			if initialCursor == "" {
				initialCursor = "0|1000-01-01"
			}
			snapshotWatermark = &CursorKey{UpdatedAt: time.Now().UTC(), StableKey: initialCursor}
		}
		page, err = reader.Snapshot(r.Context(), query)
	} else {
		cursor, cursorErr := h.decodeCursor(in.Cursor, "changes", token.ID, in.Store, "", "")
		if cursorErr != nil {
			acc.fail(w, cursorErrorStatus(cursorErr), cursorErr.Error())
			return
		}
		query = Query{Store: in.Store, Fields: fields, PageSize: pageSize, Cursor: &cursor.Key}
		page, err = reader.Changes(r.Context(), query)
	}
	if err != nil {
		acc.fail(w, http.StatusInternalServerError, "listing daily query failed")
		return
	}
	response, err := h.response(r.URL.Path == h.snapshotPath(), in, page, fields, token.ID, snapshotWatermark, query.Cursor)
	if err != nil {
		acc.fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	acc.status, acc.rows = http.StatusOK, len(page.Rows)
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) authenticate(r *http.Request) (Token, int, error) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return Token{}, http.StatusUnauthorized, errors.New("missing or invalid bearer token")
	}
	hash := HashToken(parts[1])
	h.mu.RLock()
	tokens := make([]Token, 0, len(h.tokens))
	for _, token := range h.tokens {
		tokens = append(tokens, token)
	}
	h.mu.RUnlock()
	for _, token := range tokens {
		if subtle.ConstantTimeCompare([]byte(token.Hash), []byte(hash)) != 1 {
			continue
		}
		if token.Revoked || (!token.ExpiresAt.IsZero() && !time.Now().Before(token.ExpiresAt)) {
			return Token{}, http.StatusUnauthorized, errors.New("token is revoked or expired")
		}
		return token, 0, nil
	}
	return Token{}, http.StatusUnauthorized, errors.New("unknown bearer token")
}

func (h *Handler) SetFieldPersistence(persist func(string, []string) error) {
	h.cfg.PersistFields = persist
}

func (h *Handler) SetRequestLogger(logger func(RequestLog)) {
	h.cfg.RequestLogger = logger
}

func (h *Handler) SetReader(reader Reader) {
	h.mu.Lock()
	h.reader = reader
	h.mu.Unlock()
}

// ExportCSV writes one registered dataset in the requested business-date
// range. It reuses the dataset reader and its keyset pagination, so export
// never accepts user-provided SQL or loads the entire result into memory.
func (h *Handler) ExportCSV(ctx context.Context, in Query, out io.Writer) (int, error) {
	from, err := parseDate(in.DateFrom)
	if err != nil {
		return 0, errors.New("date_from must be YYYY-MM-DD")
	}
	to, err := parseDate(in.DateTo)
	if err != nil {
		return 0, errors.New("date_to must be YYYY-MM-DD")
	}
	if to.Before(from) || int(to.Sub(from).Hours()/24)+1 > h.cfg.MaxDateSpanDays {
		return 0, errors.New("date range exceeds the allowed span")
	}
	h.mu.RLock()
	reader := h.reader
	available := make(map[string]struct{}, len(h.available))
	for field := range h.available {
		available[field] = struct{}{}
	}
	h.mu.RUnlock()
	if reader == nil {
		return 0, errors.New("dataset reader is not configured")
	}
	fields := in.Fields
	if len(fields) == 0 {
		fields = sortedKeys(available)
	}
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if _, ok := available[field]; !ok {
			return 0, errors.New("requested field is not allowlisted")
		}
		if _, duplicate := seen[field]; duplicate {
			return 0, errors.New("requested field is duplicated")
		}
		seen[field] = struct{}{}
	}
	csvWriter := csv.NewWriter(out)
	header := append([]string(nil), h.definition.FixedFields...)
	header = append(header, fields...)
	if err := csvWriter.Write(header); err != nil {
		return 0, err
	}
	count := 0
	var cursor *CursorKey
	for {
		page, err := reader.Snapshot(ctx, Query{Store: in.Store, Stores: in.Stores, DateFrom: in.DateFrom, DateTo: in.DateTo, Fields: fields, PageSize: h.cfg.MaxPageSize, Cursor: cursor})
		if err != nil {
			return count, err
		}
		for _, row := range page.Rows {
			if row.DeletedAt != nil {
				continue
			}
			record := make([]string, 0, len(header))
			for _, field := range h.definition.FixedFields {
				record = append(record, csvValue(exportFixedValue(row, field)))
			}
			for _, field := range fields {
				record = append(record, csvValue(row.Values[field]))
			}
			if err := csvWriter.Write(record); err != nil {
				return count, err
			}
			count++
		}
		if !page.HasMore {
			break
		}
		if page.Next == nil || page.Next.UpdatedAt.IsZero() || page.Next.StableKey == "" {
			return count, errors.New("reader returned an invalid export cursor")
		}
		cursor = page.Next
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return count, err
	}
	return count, nil
}

func exportFixedValue(row Row, field string) any {
	if row.FixedValues != nil {
		return row.FixedValues[field]
	}
	switch field {
	case "store":
		return row.Store
	case "channel":
		return row.Channel
	case "asin":
		return row.ASIN
	case "sku":
		return row.SKU
	case "business_date":
		return row.BusinessDate
	case "updated_at":
		return row.UpdatedAt.UTC().Format(time.RFC3339Nano)
	case "is_provisional":
		return row.IsProvisional
	case "verification_status":
		return row.VerificationStatus
	default:
		return nil
	}
}

func csvValue(value any) string {
	if value == nil {
		return ""
	}
	if timestamp, ok := value.(time.Time); ok {
		return timestamp.UTC().Format(time.RFC3339Nano)
	}
	return fmt.Sprint(value)
}

func IsBearerPath(path string) bool {
	for _, definition := range Definitions() {
		if path == SnapshotPathFor(definition.ID) || path == ChangesPathFor(definition.ID) {
			return true
		}
	}
	return false
}

func SnapshotPathFor(datasetID string) string { return "/api/v1/datasets/" + datasetID + "/snapshot" }
func ChangesPathFor(datasetID string) string  { return "/api/v1/datasets/" + datasetID + "/changes" }
func FieldsPathFor(datasetID string) string {
	return "/api/datasources/datasets/" + datasetID + "/fields"
}

func (h *Handler) snapshotPath() string { return SnapshotPathFor(h.definition.ID) }
func (h *Handler) changesPath() string  { return ChangesPathFor(h.definition.ID) }
func (h *Handler) fieldsPath() string   { return FieldsPathFor(h.definition.ID) }

func (h *Handler) serveFields(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	acc := &requestLogAccum{datasetID: h.definition.ID, endpoint: "fields", fieldsResp: true}
	defer func() { h.logDatasetRequest(acc, start) }()
	h.serveFieldsInner(w, r, acc)
}

func (h *Handler) serveFieldsInner(w http.ResponseWriter, r *http.Request, acc *requestLogAccum) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		acc.fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var in fieldsRequest
	requestedProjectID := r.URL.Query().Get("project_id")
	requestedTokenID := r.URL.Query().Get("token_id")
	if r.Method == http.MethodPut {
		dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			acc.fail(w, http.StatusBadRequest, "fields JSON is invalid")
			return
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			acc.fail(w, http.StatusBadRequest, "fields JSON must contain one object")
			return
		}
		if requestedProjectID == "" {
			requestedProjectID = in.ProjectID
		}
		if requestedTokenID == "" {
			requestedTokenID = in.TokenID
		}
	}
	if r.Method == http.MethodGet && requestedProjectID == "" && requestedTokenID == "" {
		h.mu.RLock()
		available := sortedKeys(h.available)
		catalog := sortedKeys(h.catalog)
		projects := make([]ProjectFields, 0, len(h.tokens))
		for _, token := range h.tokens {
			projects = append(projects, ProjectFields{ProjectID: token.ProjectID, TokenID: token.ID, DatasetScopes: append([]string(nil), token.DatasetScopes...), StoreScopes: append([]string(nil), token.StoreScopes...), Fields: append([]string(nil), token.Fields...)})
		}
		h.mu.RUnlock()
		sort.Slice(projects, func(i, j int) bool {
			if projects[i].ProjectID == projects[j].ProjectID {
				return projects[i].TokenID < projects[j].TokenID
			}
			return projects[i].ProjectID < projects[j].ProjectID
		})
		acc.status = http.StatusOK
		writeJSON(w, http.StatusOK, FieldsResponse{OK: true, Data: FieldsResponseData{DatasetID: h.definition.ID, DatasetName: h.definition.Name, FixedFields: append([]string(nil), h.definition.FixedFields...), AvailableFields: available, CatalogFields: catalog, ConfiguredFields: append([]string(nil), available...), Projects: projects}})
		return
	}
	token, err := h.resolveToken(requestedProjectID, requestedTokenID)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not allowed") {
			status = http.StatusForbidden
		}
		acc.fail(w, status, err.Error())
		return
	}
	acc.projectID, acc.tokenID = token.ProjectID, token.ID
	current := make(map[string]struct{}, len(token.Fields))
	for _, field := range token.Fields {
		current[field] = struct{}{}
	}
	if r.Method == http.MethodGet {
		h.mu.RLock()
		available := sortedKeys(h.available)
		h.mu.RUnlock()
		acc.status = http.StatusOK
		writeJSON(w, http.StatusOK, FieldsResponse{OK: true, Data: FieldsResponseData{DatasetID: h.definition.ID, DatasetName: h.definition.Name, FixedFields: append([]string(nil), h.definition.FixedFields...), ProjectID: token.ProjectID, TokenID: token.ID, AvailableFields: available, Fields: sortedKeys(current)}})
		return
	}
	if requestedProjectID == "" {
		acc.fail(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if in.ProjectID != "" && in.ProjectID != token.ProjectID {
		acc.fail(w, http.StatusForbidden, "project_id is not allowed")
		return
	}
	if in.TokenID != "" && in.TokenID != token.ID {
		acc.fail(w, http.StatusForbidden, "token_id is not allowed")
		return
	}
	if len(in.Fields) == 0 {
		acc.fail(w, http.StatusBadRequest, "fields cannot be empty")
		return
	}
	selected := make(map[string]struct{}, len(in.Fields))
	for _, field := range in.Fields {
		if _, duplicate := selected[field]; duplicate {
			acc.fail(w, http.StatusBadRequest, "fields contains a duplicate")
			return
		}
		h.mu.RLock()
		_, available := h.available[field]
		h.mu.RUnlock()
		if !available {
			acc.fail(w, http.StatusBadRequest, "field is not available")
			return
		}
		selected[field] = struct{}{}
	}
	fields := sortedKeys(selected)
	if h.cfg.PersistFields != nil {
		if err := h.cfg.PersistFields(token.ID, fields); err != nil {
			acc.fail(w, http.StatusInternalServerError, "persisting fields failed")
			return
		}
	}
	h.mu.Lock()
	updatedToken := h.tokens[token.ID]
	updatedToken.Fields = append([]string(nil), fields...)
	h.tokens[token.ID] = updatedToken
	h.mu.Unlock()
	// A PUT updates the persisted project configuration. The current request's
	// token remains authoritative until the config is reloaded, so one project
	// cannot mutate another project's in-memory field scope.
	acc.status = http.StatusOK
	writeJSON(w, http.StatusOK, FieldsResponse{OK: true, Data: FieldsResponseData{DatasetID: h.definition.ID, DatasetName: h.definition.Name, FixedFields: append([]string(nil), h.definition.FixedFields...), ProjectID: token.ProjectID, TokenID: token.ID, AvailableFields: sortedKeys(h.available), Fields: fields}})
}

func (h *Handler) resolveToken(projectID, tokenID string) (Token, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if tokenID != "" {
		token, ok := h.tokens[tokenID]
		if !ok || (projectID != "" && token.ProjectID != projectID) {
			return Token{}, errors.New("project_id or token_id is not allowed")
		}
		return token, nil
	}
	if projectID == "" {
		return Token{}, errors.New("project_id is required")
	}
	var match Token
	for _, token := range h.tokens {
		if token.ProjectID != projectID {
			continue
		}
		if match.ID != "" {
			return Token{}, errors.New("project_id maps to multiple tokens")
		}
		match = token
	}
	if match.ID != "" {
		return match, nil
	}
	return Token{}, errors.New("project_id or token_id is not allowed")
}

func (h *Handler) validateRequest(in *request, snapshot bool, token Token) ([]string, int, error) {
	if in.Store == "" {
		return nil, 0, errors.New("store is required")
	}
	if !contains(token.DatasetScopes, h.definition.ID) {
		return nil, 0, errors.New("dataset scope is not allowed")
	}
	if !contains(token.StoreScopes, in.Store) {
		return nil, 0, errors.New("store scope is not allowed")
	}
	if !snapshot && (in.DateFrom != "" || in.DateTo != "") {
		return nil, 0, errors.New("changes does not accept date scope")
	}
	if !snapshot && in.Cursor == "" {
		return nil, 0, errors.New("changes cursor is required")
	}
	if snapshot {
		from, err := parseDate(in.DateFrom)
		if err != nil {
			return nil, 0, errors.New("date_from must be YYYY-MM-DD")
		}
		to, err := parseDate(in.DateTo)
		if err != nil {
			return nil, 0, errors.New("date_to must be YYYY-MM-DD")
		}
		if to.Before(from) || int(to.Sub(from).Hours()/24)+1 > h.cfg.MaxDateSpanDays {
			return nil, 0, errors.New("date range exceeds the allowed span")
		}
	}
	h.mu.RLock()
	allow := make(map[string]struct{}, len(h.available))
	for field := range h.available {
		allow[field] = struct{}{}
	}
	h.mu.RUnlock()
	fields := in.Fields
	if len(fields) == 0 {
		fields = make([]string, 0, len(allow))
		for field := range allow {
			fields = append(fields, field)
		}
		sort.Strings(fields)
	}
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if _, ok := allow[field]; !ok {
			return nil, 0, errors.New("requested field is not allowlisted")
		}
		if _, ok := seen[field]; ok {
			return nil, 0, errors.New("requested field is duplicated")
		}
		seen[field] = struct{}{}
	}
	pageSize := DefaultPageSize
	if in.PageSize != nil {
		pageSize = *in.PageSize
	}
	if pageSize < 1 || pageSize > h.cfg.MaxPageSize {
		return nil, 0, errors.New("page_size is outside the allowed range")
	}
	return fields, pageSize, nil
}

func (h *Handler) response(snapshot bool, in request, page Page, fields []string, tokenID string, snapshotWatermark, changesCursor *CursorKey) (Response, error) {
	rows := make([]map[string]any, 0, len(page.Rows))
	for _, row := range page.Rows {
		if snapshot && row.DeletedAt != nil {
			continue
		}
		if row.UpdatedAt.IsZero() || row.StableKey == "" {
			return Response{}, errors.New("listing row has no stable keyset identity")
		}
		out := map[string]any{
			"account_id":          row.AccountID,
			"store":               row.Store,
			"channel":             row.Channel,
			"asin":                row.ASIN,
			"sku":                 row.SKU,
			"business_date":       row.BusinessDate,
			"updated_at":          row.UpdatedAt.UTC().Format(time.RFC3339Nano),
			"is_provisional":      row.IsProvisional,
			"verification_status": row.VerificationStatus,
		}
		if row.FixedValues != nil {
			out = make(map[string]any, len(row.FixedValues)+len(fields))
			for key, value := range row.FixedValues {
				out[key] = value
			}
		}
		if row.FixedValues == nil && row.DeletedAt != nil {
			out["deleted_at"] = row.DeletedAt.UTC().Format(time.RFC3339Nano)
		} else if row.FixedValues == nil {
			out["deleted_at"] = nil
		}
		for _, field := range fields {
			if value, ok := row.Values[field]; ok {
				out[field] = value
			}
		}
		rows = append(rows, out)
	}
	data := ResponseData{SchemaVersion: h.definition.ID, Rows: rows, HasMore: page.HasMore}
	if !snapshot {
		if !validCursorKey(changesCursor) {
			return Response{}, errors.New("changes cursor is invalid")
		}
		key := *changesCursor
		if len(page.Rows) > 0 {
			last := page.Rows[len(page.Rows)-1]
			key = CursorKey{UpdatedAt: last.UpdatedAt, StableKey: last.StableKey}
		}
		cursor, err := h.encodeCursor(cursorEnvelope{Version: 1, Dataset: h.definition.ID, Kind: "changes", TokenID: tokenID, Store: in.Store, Key: key})
		if err != nil {
			return Response{}, err
		}
		data.NextCursor = cursor
	}
	if page.HasMore {
		if page.Next == nil || page.Next.UpdatedAt.IsZero() || page.Next.StableKey == "" {
			return Response{}, errors.New("reader returned an invalid next cursor")
		}
		kind := "changes"
		if snapshot {
			kind = "snapshot"
		}
		cursor, err := h.encodeCursor(cursorEnvelope{Version: 1, Dataset: h.definition.ID, Kind: kind, TokenID: tokenID, Store: in.Store, DateFrom: in.DateFrom, DateTo: in.DateTo, Key: *page.Next, Watermark: snapshotWatermark})
		if err != nil {
			return Response{}, err
		}
		data.NextCursor = cursor
	}
	if snapshot && !page.HasMore {
		if !validCursorKey(snapshotWatermark) {
			return Response{}, errors.New("snapshot changes watermark is invalid")
		}
		cursor, err := h.encodeCursor(cursorEnvelope{Version: 1, Dataset: h.definition.ID, Kind: "changes", TokenID: tokenID, Store: in.Store, Key: *snapshotWatermark})
		if err != nil {
			return Response{}, err
		}
		data.ChangesCursor = cursor
	}
	return Response{OK: true, Data: data}, nil
}

func (h *Handler) encodeCursor(cursor cursorEnvelope) (string, error) {
	if len(h.cfg.CursorSecret) < 16 {
		return "", errors.New("dataset cursor secret is not configured")
	}
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha512.New512_256, h.cfg.CursorSecret)
	_, _ = mac.Write(payload)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + signature, nil
}

func (h *Handler) decodeCursor(raw, kind, tokenID, store, dateFrom, dateTo string) (cursorEnvelope, error) {
	if len(h.cfg.CursorSecret) < 16 {
		return cursorEnvelope{}, errors.New("dataset cursor secret is not configured")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return cursorEnvelope{}, errors.New("cursor is invalid")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return cursorEnvelope{}, errors.New("cursor is invalid")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return cursorEnvelope{}, errors.New("cursor is invalid")
	}
	mac := hmac.New(sha512.New512_256, h.cfg.CursorSecret)
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return cursorEnvelope{}, errors.New("cursor is invalid")
	}
	var cursor cursorEnvelope
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Version != 1 || cursor.Dataset != h.definition.ID || cursor.Kind != kind || cursor.DateFrom != dateFrom || cursor.DateTo != dateTo || !validCursorKey(&cursor.Key) {
		return cursorEnvelope{}, errors.New("cursor is invalid")
	}
	if kind == "snapshot" && !validCursorKey(cursor.Watermark) {
		return cursorEnvelope{}, errors.New("cursor is invalid")
	}
	if cursor.TokenID != tokenID || cursor.Store != store {
		return cursorEnvelope{}, errors.New("cursor scope is not allowed")
	}
	return cursor, nil
}

func validCursorKey(key *CursorKey) bool {
	return key != nil && !key.UpdatedAt.IsZero() && key.StableKey != ""
}

func cursorErrorStatus(err error) int {
	if strings.Contains(err.Error(), "scope") {
		return http.StatusForbidden
	}
	return http.StatusBadRequest
}

func decodeJSON(r *http.Request, out *request) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return errors.New("request JSON is invalid")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("request JSON must contain one object")
	}
	return nil
}

func parseDate(raw string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil || parsed.Format("2006-01-02") != raw {
		return time.Time{}, errors.New("invalid date")
	}
	return parsed, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, Response{OK: false, Error: message})
}

var _ = subtle.ConstantTimeCompare
