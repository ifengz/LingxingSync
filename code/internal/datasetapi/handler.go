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
	"encoding/hex"
	"encoding/json"
	"errors"
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
	Tokens          []Token
	FieldAllowlist  []string
	MaxDateSpanDays int
	MaxPageSize     int
	CursorSecret    []byte
	PersistFields   func(string, []string) error
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
	Values             map[string]any
}

type Query struct {
	Store    string
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
	reader    Reader
	cfg       Config
	mu        sync.RWMutex
	available map[string]struct{}
	tokens    map[string]Token
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
	DatasetID       string          `json:"dataset_id"`
	DatasetName     string          `json:"dataset_name"`
	FixedFields     []string        `json:"fixed_fields"`
	ProjectID       string          `json:"project_id,omitempty"`
	TokenID         string          `json:"token_id,omitempty"`
	AvailableFields []string        `json:"available_fields"`
	Fields          []string        `json:"fields,omitempty"`
	Projects        []ProjectFields `json:"projects"`
}

type ProjectFields struct {
	ProjectID   string   `json:"project_id"`
	TokenID     string   `json:"token_id"`
	StoreScopes []string `json:"store_scopes"`
	Fields      []string `json:"fields"`
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
	return &Handler{reader: reader, cfg: cfg, available: available, tokens: tokens}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == FieldsPath {
		h.serveFields(w, r)
		return
	}
	if r.Method != http.MethodPost || (r.URL.Path != SnapshotPath && r.URL.Path != ChangesPath) {
		http.NotFound(w, r)
		return
	}
	token, status, err := h.authenticate(r)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	var in request
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	fields, pageSize, err := h.validateRequest(&in, r.URL.Path == SnapshotPath, token)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "scope") {
			status = http.StatusForbidden
		}
		writeError(w, status, err.Error())
		return
	}
	h.mu.RLock()
	reader := h.reader
	h.mu.RUnlock()
	if reader == nil {
		writeError(w, http.StatusServiceUnavailable, "listing daily reader is not configured")
		return
	}
	var page Page
	var query Query
	var snapshotWatermark *CursorKey
	if r.URL.Path == SnapshotPath {
		query = Query{Store: in.Store, DateFrom: in.DateFrom, DateTo: in.DateTo, Fields: fields, PageSize: pageSize}
		if in.Cursor != "" {
			cursor, err := h.decodeCursor(in.Cursor, "snapshot", token.ID, in.Store, in.DateFrom, in.DateTo)
			if err != nil {
				writeError(w, cursorErrorStatus(err), err.Error())
				return
			}
			query.Cursor = &cursor.Key
			snapshotWatermark = cursor.Watermark
		} else {
			snapshotWatermark = &CursorKey{UpdatedAt: time.Now().UTC(), StableKey: "0|1000-01-01"}
		}
		page, err = reader.Snapshot(r.Context(), query)
	} else {
		cursor, err := h.decodeCursor(in.Cursor, "changes", token.ID, in.Store, "", "")
		if err != nil {
			writeError(w, cursorErrorStatus(err), err.Error())
			return
		}
		query = Query{Store: in.Store, Fields: fields, PageSize: pageSize, Cursor: &cursor.Key}
		page, err = reader.Changes(r.Context(), query)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing daily query failed")
		return
	}
	response, err := h.response(r.URL.Path == SnapshotPath, in, page, fields, token.ID, snapshotWatermark, query.Cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
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

func (h *Handler) SetReader(reader Reader) {
	h.mu.Lock()
	h.reader = reader
	h.mu.Unlock()
}

func IsBearerPath(path string) bool {
	return path == SnapshotPath || path == ChangesPath
}

func (h *Handler) serveFields(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, FieldsResponse{OK: false, Error: "method not allowed"})
		return
	}
	var in fieldsRequest
	requestedProjectID := r.URL.Query().Get("project_id")
	requestedTokenID := r.URL.Query().Get("token_id")
	if r.Method == http.MethodPut {
		dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, FieldsResponse{OK: false, Error: "fields JSON is invalid"})
			return
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			writeJSON(w, http.StatusBadRequest, FieldsResponse{OK: false, Error: "fields JSON must contain one object"})
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
		projects := make([]ProjectFields, 0, len(h.tokens))
		for _, token := range h.tokens {
			projects = append(projects, ProjectFields{ProjectID: token.ProjectID, TokenID: token.ID, StoreScopes: append([]string(nil), token.StoreScopes...), Fields: append([]string(nil), token.Fields...)})
		}
		h.mu.RUnlock()
		sort.Slice(projects, func(i, j int) bool {
			if projects[i].ProjectID == projects[j].ProjectID {
				return projects[i].TokenID < projects[j].TokenID
			}
			return projects[i].ProjectID < projects[j].ProjectID
		})
		writeJSON(w, http.StatusOK, FieldsResponse{OK: true, Data: FieldsResponseData{DatasetID: DatasetID, DatasetName: DatasetName, FixedFields: append([]string(nil), FixedFields...), AvailableFields: available, Projects: projects}})
		return
	}
	token, err := h.resolveToken(requestedProjectID, requestedTokenID)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not allowed") {
			status = http.StatusForbidden
		}
		writeFieldsError(w, status, err.Error())
		return
	}
	current := make(map[string]struct{}, len(token.Fields))
	for _, field := range token.Fields {
		current[field] = struct{}{}
	}
	if r.Method == http.MethodGet {
		h.mu.RLock()
		available := sortedKeys(h.available)
		h.mu.RUnlock()
		writeJSON(w, http.StatusOK, FieldsResponse{OK: true, Data: FieldsResponseData{DatasetID: DatasetID, DatasetName: DatasetName, FixedFields: append([]string(nil), FixedFields...), ProjectID: token.ProjectID, TokenID: token.ID, AvailableFields: available, Fields: sortedKeys(current)}})
		return
	}
	if requestedProjectID == "" {
		writeFieldsError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if in.ProjectID != "" && in.ProjectID != token.ProjectID {
		writeFieldsError(w, http.StatusForbidden, "project_id is not allowed")
		return
	}
	if in.TokenID != "" && in.TokenID != token.ID {
		writeFieldsError(w, http.StatusForbidden, "token_id is not allowed")
		return
	}
	if len(in.Fields) == 0 {
		writeJSON(w, http.StatusBadRequest, FieldsResponse{OK: false, Error: "fields cannot be empty"})
		return
	}
	selected := make(map[string]struct{}, len(in.Fields))
	for _, field := range in.Fields {
		if _, duplicate := selected[field]; duplicate {
			writeJSON(w, http.StatusBadRequest, FieldsResponse{OK: false, Error: "fields contains a duplicate"})
			return
		}
		h.mu.RLock()
		_, available := h.available[field]
		h.mu.RUnlock()
		if !available {
			writeJSON(w, http.StatusBadRequest, FieldsResponse{OK: false, Error: "field is not available"})
			return
		}
		selected[field] = struct{}{}
	}
	fields := sortedKeys(selected)
	if h.cfg.PersistFields != nil {
		if err := h.cfg.PersistFields(token.ID, fields); err != nil {
			writeJSON(w, http.StatusInternalServerError, FieldsResponse{OK: false, Error: "persisting fields failed"})
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
	writeJSON(w, http.StatusOK, FieldsResponse{OK: true, Data: FieldsResponseData{DatasetID: DatasetID, DatasetName: DatasetName, FixedFields: append([]string(nil), FixedFields...), ProjectID: token.ProjectID, TokenID: token.ID, AvailableFields: sortedKeys(h.available), Fields: fields}})
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

func writeFieldsError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, FieldsResponse{OK: false, Error: message})
}

func (h *Handler) validateRequest(in *request, snapshot bool, token Token) ([]string, int, error) {
	if in.Store == "" {
		return nil, 0, errors.New("store is required")
	}
	if !contains(token.DatasetScopes, DatasetID) {
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
		if row.DeletedAt != nil {
			out["deleted_at"] = row.DeletedAt.UTC().Format(time.RFC3339Nano)
		} else {
			out["deleted_at"] = nil
		}
		for _, field := range fields {
			if value, ok := row.Values[field]; ok {
				out[field] = value
			}
		}
		rows = append(rows, out)
	}
	data := ResponseData{SchemaVersion: SchemaVersion, Rows: rows, HasMore: page.HasMore}
	if !snapshot {
		if !validCursorKey(changesCursor) {
			return Response{}, errors.New("changes cursor is invalid")
		}
		key := *changesCursor
		if len(page.Rows) > 0 {
			last := page.Rows[len(page.Rows)-1]
			key = CursorKey{UpdatedAt: last.UpdatedAt, StableKey: last.StableKey}
		}
		cursor, err := h.encodeCursor(cursorEnvelope{Version: 1, Dataset: DatasetID, Kind: "changes", TokenID: tokenID, Store: in.Store, Key: key})
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
		cursor, err := h.encodeCursor(cursorEnvelope{Version: 1, Dataset: DatasetID, Kind: kind, TokenID: tokenID, Store: in.Store, DateFrom: in.DateFrom, DateTo: in.DateTo, Key: *page.Next, Watermark: snapshotWatermark})
		if err != nil {
			return Response{}, err
		}
		data.NextCursor = cursor
	}
	if snapshot && !page.HasMore {
		if !validCursorKey(snapshotWatermark) {
			return Response{}, errors.New("snapshot changes watermark is invalid")
		}
		cursor, err := h.encodeCursor(cursorEnvelope{Version: 1, Dataset: DatasetID, Kind: "changes", TokenID: tokenID, Store: in.Store, Key: *snapshotWatermark})
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
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Version != 1 || cursor.Dataset != DatasetID || cursor.Kind != kind || cursor.DateFrom != dateFrom || cursor.DateTo != dateTo || !validCursorKey(&cursor.Key) {
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
