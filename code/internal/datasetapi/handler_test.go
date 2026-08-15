package datasetapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fixtureReader struct {
	snapshotCalls int
	changesCalls  int
	page          Page
	lastQuery     Query
}

func (r *fixtureReader) Snapshot(_ context.Context, query Query) (Page, error) {
	r.snapshotCalls++
	r.lastQuery = query
	return r.page, nil
}

func (r *fixtureReader) Changes(_ context.Context, query Query) (Page, error) {
	r.changesCalls++
	r.lastQuery = query
	return r.page, nil
}

func newFixtureHandler(t *testing.T, reader Reader) (*Handler, string) {
	t.Helper()
	rawToken := "project-token-for-tests"
	h, err := New(Config{
		FieldAllowlist: []string{"units"},
		CursorSecret:   []byte("cursor-secret-for-tests"),
		Tokens: []Token{
			{ID: "project-a", Hash: HashToken(rawToken), DatasetScopes: []string{DatasetID}, StoreScopes: []string{"store-a"}, Fields: []string{"units"}},
		}}, reader)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h, rawToken
}

func requestJSON(t *testing.T, handler http.Handler, method, path, token string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestRoutesRejectNonPostAndUnknownDataset(t *testing.T) {
	h, token := newFixtureHandler(t, &fixtureReader{})
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, SnapshotPath},
		{http.MethodPost, "/api/v1/datasets/other/snapshot"},
		{http.MethodPost, "/api/v1/datasets/listing-daily-v1/other"},
	} {
		rec := requestJSON(t, h, tc.method, tc.path, token, `{}`)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s status=%d, want 404/405", tc.method, tc.path, rec.Code)
		}
	}
}

func TestSnapshotRejectsMissingScopeAndInvalidBounds(t *testing.T) {
	reader := &fixtureReader{}
	h, token := newFixtureHandler(t, reader)

	if rec := requestJSON(t, h, http.MethodPost, SnapshotPath, "", `{}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing bearer status=%d, want 401", rec.Code)
	}

	body := `{"store":"store-b","date_from":"2026-08-01","date_to":"2026-08-01"}`
	if rec := requestJSON(t, h, http.MethodPost, SnapshotPath, token, body); rec.Code != http.StatusForbidden {
		t.Fatalf("store scope status=%d, want 403", rec.Code)
	}

	body = `{"store":"store-a","date_from":"2026-08-02","date_to":"2026-08-01"}`
	if rec := requestJSON(t, h, http.MethodPost, SnapshotPath, token, body); rec.Code != http.StatusBadRequest {
		t.Fatalf("reverse date status=%d, want 400", rec.Code)
	}

	body = `{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-31","page_size":9999}`
	if rec := requestJSON(t, h, http.MethodPost, SnapshotPath, token, body); rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized request status=%d, want 400", rec.Code)
	}
	if reader.snapshotCalls != 0 {
		t.Fatalf("invalid requests reached reader %d times", reader.snapshotCalls)
	}
}

func TestSnapshotReturnsAllowlistedFieldsAndMetadata(t *testing.T) {
	updated := time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC)
	reader := &fixtureReader{page: Page{Rows: []Row{{
		AccountID: "account-a", Store: "store-a", Channel: "SC", ASIN: "ASIN1", SKU: "SKU1", BusinessDate: "2026-08-01",
		UpdatedAt: updated, StableKey: "account-a|store-a|SC|ASIN1|SKU1|2026-08-01", IsProvisional: true,
		VerificationStatus: "api_unverified", Values: map[string]any{"units": 4, "internal_secret": "must-not-leak"},
	}}}}
	h, token := newFixtureHandler(t, reader)
	body := `{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01","fields":["units"]}`
	rec := requestJSON(t, h, http.MethodPost, SnapshotPath, token, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "internal_secret") {
		t.Fatal("response leaked a non-allowlisted field")
	}
	var out Response
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Data.SchemaVersion == "" || len(out.Data.Rows) != 1 {
		t.Fatalf("response metadata/rows missing: %+v", out.Data)
	}
	if out.Data.NextCursor != "" || out.Data.ChangesCursor == "" {
		t.Fatalf("single-page snapshot cursors=%+v", out.Data)
	}
	row := out.Data.Rows[0]
	if row["units"] != float64(4) || row["is_provisional"] != true || row["verification_status"] != "api_unverified" {
		t.Fatalf("response row=%v", row)
	}
}

func TestSnapshotDefaultsToDatasetFieldsInsteadOfProjectFieldSubset(t *testing.T) {
	reader := &fixtureReader{}
	rawToken := "project-token-for-all-fields"
	h, err := New(Config{
		FieldAllowlist: []string{"sales", "returns"},
		CursorSecret:   []byte("cursor-secret-for-tests"),
		Tokens:         []Token{{ID: "tok_reader", ProjectID: "reader", Hash: HashToken(rawToken), DatasetScopes: []string{DatasetID}, StoreScopes: []string{"store-a"}, Fields: []string{"sales"}}},
	}, reader)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := requestJSON(t, h, http.MethodPost, SnapshotPath, rawToken, `{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01"}`)
	if rec.Code != http.StatusOK || strings.Join(reader.lastQuery.Fields, ",") != "returns,sales" {
		t.Fatalf("dataset default fields status=%d fields=%v body=%s", rec.Code, reader.lastQuery.Fields, rec.Body.String())
	}
	rec = requestJSON(t, h, http.MethodPost, SnapshotPath, rawToken, `{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01","fields":["returns"]}`)
	if rec.Code != http.StatusOK || strings.Join(reader.lastQuery.Fields, ",") != "returns" {
		t.Fatalf("dataset requested field status=%d fields=%v body=%s", rec.Code, reader.lastQuery.Fields, rec.Body.String())
	}
}

func TestChangesRejectsMissingAndSnapshotCursor(t *testing.T) {
	updated := time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC)
	reader := &fixtureReader{page: Page{Rows: []Row{{
		AccountID: "account-a", Store: "store-a", Channel: "SC", ASIN: "ASIN1", SKU: "SKU1", BusinessDate: "2026-08-01",
		UpdatedAt: updated, StableKey: "stable-1", DeletedAt: &updated,
	}}, HasMore: true, Next: &CursorKey{UpdatedAt: updated, StableKey: "stable-1"}}}
	h, token := newFixtureHandler(t, reader)
	first := requestJSON(t, h, http.MethodPost, ChangesPath, token, `{"store":"store-a","cursor":""}`)
	if first.Code != http.StatusBadRequest {
		t.Fatalf("missing changes cursor status=%d, want 400", first.Code)
	}

	first = requestJSON(t, h, http.MethodPost, SnapshotPath, token, `{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01"}`)
	if first.Code != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", first.Code, first.Body.String())
	}
	var snapshot Response
	if err := json.Unmarshal(first.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snapshot.Data.NextCursor == "" {
		t.Fatal("expected an opaque snapshot cursor")
	}
	changes := requestJSON(t, h, http.MethodPost, ChangesPath, token, `{"store":"store-a","cursor":"`+snapshot.Data.NextCursor+`"}`)
	if changes.Code != http.StatusBadRequest {
		t.Fatalf("snapshot cursor accepted by changes status=%d, want 400", changes.Code)
	}
}

func TestSnapshotFinalPageReturnsIndependentChangesCursor(t *testing.T) {
	updated := time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC)
	reader := &fixtureReader{page: Page{
		Rows: []Row{{
			AccountID: "account-a", Store: "store-a", Channel: "SC", ASIN: "ASIN1", SKU: "SKU1", BusinessDate: "2026-08-01",
			UpdatedAt: updated, StableKey: "stable-1",
		}},
		HasMore: true,
		Next:    &CursorKey{UpdatedAt: updated, StableKey: "stable-1"},
	}}
	h, token := newFixtureHandler(t, reader)
	body := `{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01"}`
	first := requestJSON(t, h, http.MethodPost, SnapshotPath, token, body)
	if first.Code != http.StatusOK {
		t.Fatalf("first snapshot status=%d body=%s", first.Code, first.Body.String())
	}
	var firstPage struct {
		Data struct {
			NextCursor    string `json:"next_cursor"`
			ChangesCursor string `json:"changes_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstPage); err != nil {
		t.Fatalf("decode first snapshot: %v", err)
	}
	if firstPage.Data.NextCursor == "" || firstPage.Data.ChangesCursor != "" {
		t.Fatalf("non-final snapshot cursors=%+v", firstPage.Data)
	}
	firstCursor, err := h.decodeCursor(firstPage.Data.NextCursor, "snapshot", "project-a", "store-a", "2026-08-01", "2026-08-01")
	if err != nil || !validCursorKey(firstCursor.Watermark) || firstCursor.Watermark.StableKey == firstCursor.Key.StableKey {
		t.Fatalf("snapshot pagination watermark=%+v err=%v", firstCursor, err)
	}

	reader.page = Page{Rows: []Row{{
		AccountID: "account-a", Store: "store-a", Channel: "SC", ASIN: "ASIN2", SKU: "SKU2", BusinessDate: "2026-08-01",
		UpdatedAt: updated.Add(time.Second), StableKey: "stable-2",
	}}}
	second := requestJSON(t, h, http.MethodPost, SnapshotPath, token, `{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01","cursor":"`+firstPage.Data.NextCursor+`"}`)
	if second.Code != http.StatusOK {
		t.Fatalf("final snapshot status=%d body=%s", second.Code, second.Body.String())
	}
	var finalPage struct {
		Data struct {
			NextCursor    string `json:"next_cursor"`
			ChangesCursor string `json:"changes_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &finalPage); err != nil {
		t.Fatalf("decode final snapshot: %v", err)
	}
	if finalPage.Data.NextCursor != "" || finalPage.Data.ChangesCursor == "" || finalPage.Data.ChangesCursor == firstPage.Data.NextCursor {
		t.Fatalf("final snapshot cursors=%+v", finalPage.Data)
	}
	changesCursor, err := h.decodeCursor(finalPage.Data.ChangesCursor, "changes", "project-a", "store-a", "", "")
	if err != nil || changesCursor.Key != *firstCursor.Watermark {
		t.Fatalf("final changes cursor=%+v watermark=%+v err=%v", changesCursor.Key, firstCursor.Watermark, err)
	}

	changes := requestJSON(t, h, http.MethodPost, ChangesPath, token, `{"store":"store-a","cursor":"`+finalPage.Data.ChangesCursor+`"}`)
	if changes.Code != http.StatusOK {
		t.Fatalf("changes cursor from final snapshot status=%d body=%s", changes.Code, changes.Body.String())
	}
	if reader.changesCalls != 1 || reader.lastQuery.Cursor == nil || reader.lastQuery.Cursor.UpdatedAt.IsZero() {
		t.Fatalf("final snapshot cursor did not enter changes: calls=%d query=%+v", reader.changesCalls, reader.lastQuery)
	}
}

func TestSnapshotSinglePageChangesCursorCoversRowsReturnedThisSecond(t *testing.T) {
	updated := time.Date(2026, 8, 1, 1, 2, 3, 900_000_000, time.UTC)
	reader := &fixtureReader{page: Page{Rows: []Row{{
		AccountID: "account-a", Store: "store-a", Channel: "SC", ASIN: "ASIN1", SKU: "SKU1", BusinessDate: "2026-08-01",
		UpdatedAt: updated, StableKey: "stable-1",
	}}}}
	h, token := newFixtureHandler(t, reader)
	rec := requestJSON(t, h, http.MethodPost, SnapshotPath, token, `{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response Response
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	cursor, err := h.decodeCursor(response.Data.ChangesCursor, "changes", "project-a", "store-a", "", "")
	if err != nil || !cursor.Key.UpdatedAt.After(updated) {
		t.Fatalf("single-page changes cursor=%+v must be after returned row err=%v", cursor.Key, err)
	}
}

func TestHashTokenDoesNotReturnRawToken(t *testing.T) {
	raw := "do-not-persist-this"
	if HashToken(raw) == raw || HashToken(raw) == "" {
		t.Fatal("token hash is empty or contains the raw token")
	}
}

func TestFieldsListReturnsEmptyProjectsArray(t *testing.T) {
	h, err := New(Config{}, &fixtureReader{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := requestJSON(t, h, http.MethodGet, FieldsPath, "", ``)
	if rec.Code != http.StatusOK {
		t.Fatalf("fields status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			Projects json.RawMessage `json:"projects"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode fields response: %v", err)
	}
	if string(response.Data.Projects) != "[]" {
		t.Fatalf("empty projects = %s, want []", response.Data.Projects)
	}
}

func TestFieldsAreScopedPerProjectToken(t *testing.T) {
	reader := &fixtureReader{}
	firstRaw := "project-a-token"
	secondRaw := "project-b-token"
	h, err := New(Config{
		FieldAllowlist: []string{"units", "returns"},
		CursorSecret:   []byte("cursor-secret-for-tests"),
		Tokens: []Token{
			{ID: "project-a", Hash: HashToken(firstRaw), DatasetScopes: []string{DatasetID}, StoreScopes: []string{"store-a"}, Fields: []string{"units"}},
			{ID: "project-b", Hash: HashToken(secondRaw), DatasetScopes: []string{DatasetID}, StoreScopes: []string{"store-a"}, Fields: []string{"returns"}},
		},
	}, reader)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h.SetFieldPersistence(func(string, []string) error { return nil })
	projects := requestJSON(t, h, http.MethodGet, FieldsPath, "", ``)
	if projects.Code != http.StatusOK || !strings.Contains(projects.Body.String(), `"projects"`) || !strings.Contains(projects.Body.String(), `"token_id":"project-a"`) || strings.Contains(projects.Body.String(), `"token_hash"`) {
		t.Fatalf("project list status=%d body=%s", projects.Code, projects.Body.String())
	}
	put := requestJSON(t, h, http.MethodPut, FieldsPath+"?project_id=project-a&token_id=project-a", firstRaw, `{"project_id":"project-a","token_id":"project-a","fields":["returns"]}`)
	if put.Code != http.StatusOK {
		t.Fatalf("project A field PUT status=%d body=%s", put.Code, put.Body.String())
	}
	get := requestJSON(t, h, http.MethodGet, FieldsPath+"?project_id=project-b&token_id=project-b", secondRaw, ``)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"fields":["returns"]`) {
		t.Fatalf("project B fields changed unexpectedly: status=%d body=%s", get.Code, get.Body.String())
	}
	if strings.Contains(get.Body.String(), `"token_hash"`) {
		t.Fatalf("fields response leaked token hash: %s", get.Body.String())
	}
	wrongToken := requestJSON(t, h, http.MethodGet, FieldsPath+"?project_id=project-b&token_id=project-a", "", ``)
	if wrongToken.Code != http.StatusForbidden {
		t.Fatalf("mismatched project/token status=%d, want 403; body=%s", wrongToken.Code, wrongToken.Body.String())
	}
	missingProject := requestJSON(t, h, http.MethodPut, FieldsPath, "", `{"fields":["units"]}`)
	if missingProject.Code != http.StatusBadRequest {
		t.Fatalf("PUT without project status=%d, want 400; body=%s", missingProject.Code, missingProject.Body.String())
	}
}

func TestFieldsAreScopedAcrossMultipleTokensForOneProject(t *testing.T) {
	h, err := New(Config{
		FieldAllowlist: []string{"units", "returns", "sales"},
		CursorSecret:   []byte("cursor-secret-for-tests"),
		Tokens: []Token{
			{ID: "token-a", ProjectID: "project-a", Hash: HashToken("raw-a"), DatasetScopes: []string{DatasetID}, StoreScopes: []string{"store-a"}, Fields: []string{"units"}},
			{ID: "token-b", ProjectID: "project-a", Hash: HashToken("raw-b"), DatasetScopes: []string{DatasetID}, StoreScopes: []string{"store-a"}, Fields: []string{"returns"}},
		},
	}, &fixtureReader{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var persistedToken string
	var persistedFields []string
	h.SetFieldPersistence(func(tokenID string, fields []string) error {
		persistedToken = tokenID
		persistedFields = append([]string(nil), fields...)
		return nil
	})
	projects := requestJSON(t, h, http.MethodGet, FieldsPath, "", ``)
	if projects.Code != http.StatusOK || !strings.Contains(projects.Body.String(), `"token_id":"token-a"`) || !strings.Contains(projects.Body.String(), `"token_id":"token-b"`) {
		t.Fatalf("same-project token list status=%d body=%s", projects.Code, projects.Body.String())
	}
	first := requestJSON(t, h, http.MethodGet, FieldsPath+"?project_id=project-a&token_id=token-a", "", ``)
	second := requestJSON(t, h, http.MethodGet, FieldsPath+"?project_id=project-a&token_id=token-b", "", ``)
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"fields":["units"]`) {
		t.Fatalf("token-a fields status=%d body=%s", first.Code, first.Body.String())
	}
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"fields":["returns"]`) {
		t.Fatalf("token-b fields status=%d body=%s", second.Code, second.Body.String())
	}
	put := requestJSON(t, h, http.MethodPut, FieldsPath+"?project_id=project-a&token_id=token-a", "", `{"project_id":"project-a","token_id":"token-a","fields":["sales"]}`)
	if put.Code != http.StatusOK {
		t.Fatalf("token-a field PUT status=%d body=%s", put.Code, put.Body.String())
	}
	if persistedToken != "token-a" || len(persistedFields) != 1 || persistedFields[0] != "sales" {
		t.Fatalf("persisted token/fields = %q/%v", persistedToken, persistedFields)
	}
	first = requestJSON(t, h, http.MethodGet, FieldsPath+"?project_id=project-a&token_id=token-a", "", ``)
	second = requestJSON(t, h, http.MethodGet, FieldsPath+"?project_id=project-a&token_id=token-b", "", ``)
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"fields":["sales"]`) {
		t.Fatalf("token-a fields were not updated: status=%d body=%s", first.Code, first.Body.String())
	}
	var secondFields FieldsResponse
	if err := json.Unmarshal(second.Body.Bytes(), &secondFields); err != nil {
		t.Fatalf("decode token-b fields: %v", err)
	}
	if second.Code != http.StatusOK || len(secondFields.Data.Fields) != 1 || secondFields.Data.Fields[0] != "returns" {
		t.Fatalf("token-a update changed token-b: status=%d body=%s", second.Code, second.Body.String())
	}
}

func TestChangesPassesUpdatedAtAndStableKeyCursorToReader(t *testing.T) {
	reader := &fixtureReader{page: Page{Rows: []Row{{
		AccountID: "account-a", Store: "store-a", Channel: "SC", ASIN: "ASIN1", SKU: "SKU1", BusinessDate: "2026-08-01",
		UpdatedAt: time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC), StableKey: "next-key",
	}}}}
	h, token := newFixtureHandler(t, reader)
	cursor, err := h.encodeCursor(cursorEnvelope{Version: 1, Dataset: DatasetID, Kind: "changes", TokenID: "project-a", Store: "store-a", Key: CursorKey{UpdatedAt: time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC), StableKey: "after-key"}})
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	rec := requestJSON(t, h, http.MethodPost, ChangesPath, token, `{"store":"store-a","cursor":"`+cursor+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("changes status=%d body=%s", rec.Code, rec.Body.String())
	}
	if reader.lastQuery.Cursor == nil || reader.lastQuery.Cursor.StableKey != "after-key" || reader.lastQuery.Cursor.UpdatedAt.IsZero() {
		t.Fatalf("reader did not receive keyset cursor: %+v", reader.lastQuery)
	}
	var response Response
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode changes response: %v", err)
	}
	if response.Data.NextCursor == "" || response.Data.NextCursor == cursor {
		t.Fatalf("final changes page did not advance cursor: %+v", response.Data)
	}
	advanced, err := h.decodeCursor(response.Data.NextCursor, "changes", "project-a", "store-a", "", "")
	if err != nil || advanced.Key.StableKey != "next-key" {
		t.Fatalf("advanced changes cursor=%+v err=%v", advanced, err)
	}
}

func TestChangesEmptyPageKeepsInputCursor(t *testing.T) {
	h, token := newFixtureHandler(t, &fixtureReader{})
	cursor, err := h.encodeCursor(cursorEnvelope{Version: 1, Dataset: DatasetID, Kind: "changes", TokenID: "project-a", Store: "store-a", Key: CursorKey{UpdatedAt: time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC), StableKey: "after-key"}})
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	rec := requestJSON(t, h, http.MethodPost, ChangesPath, token, `{"store":"store-a","cursor":"`+cursor+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("empty changes status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response Response
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode empty changes response: %v", err)
	}
	if response.Data.NextCursor != cursor {
		t.Fatalf("empty changes cursor=%q, want original cursor", response.Data.NextCursor)
	}
}

func TestCursorCannotCrossProjectToken(t *testing.T) {
	reader := &fixtureReader{}
	firstRaw := "project-a-token"
	secondRaw := "project-b-token"
	h, err := New(Config{
		FieldAllowlist: []string{"units"},
		CursorSecret:   []byte("cursor-secret-for-tests"),
		Tokens: []Token{
			{ID: "project-a", Hash: HashToken(firstRaw), DatasetScopes: []string{DatasetID}, StoreScopes: []string{"store-a"}, Fields: []string{"units"}},
			{ID: "project-b", Hash: HashToken(secondRaw), DatasetScopes: []string{DatasetID}, StoreScopes: []string{"store-a"}, Fields: []string{"units"}},
		},
	}, reader)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cursor, err := h.encodeCursor(cursorEnvelope{Version: 1, Dataset: DatasetID, Kind: "changes", TokenID: "project-a", Store: "store-a", Key: CursorKey{UpdatedAt: time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC), StableKey: "after-key"}})
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	rec := requestJSON(t, h, http.MethodPost, ChangesPath, secondRaw, `{"store":"store-a","cursor":"`+cursor+`"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-project cursor status=%d, want 403", rec.Code)
	}
}

func TestFieldsRouteReturnsAndPersistsOnlyAllowlistedFields(t *testing.T) {
	var persisted []string
	h, token := newFixtureHandler(t, &fixtureReader{})
	h.SetFieldPersistence(func(_ string, fields []string) error {
		persisted = append([]string(nil), fields...)
		return nil
	})

	get := requestJSON(t, h, http.MethodGet, FieldsPath, "", ``)
	var catalog FieldsResponse
	if err := json.Unmarshal(get.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("decode fields catalog: %v", err)
	}
	if get.Code != http.StatusOK || catalog.Data.DatasetName != DatasetName || strings.Join(catalog.Data.FixedFields, ",") != strings.Join(FixedFields, ",") || !strings.Contains(get.Body.String(), "units") {
		t.Fatalf("fields GET status=%d body=%s", get.Code, get.Body.String())
	}
	put := requestJSON(t, h, http.MethodPut, FieldsPath, token, `{"project_id":"project-a","fields":["internal_secret"]}`)
	if put.Code != http.StatusBadRequest {
		t.Fatalf("unknown field PUT status=%d, want 400", put.Code)
	}
	put = requestJSON(t, h, http.MethodPut, FieldsPath, token, `{"project_id":"project-a","fields":["units"]}`)
	if put.Code != http.StatusOK || len(persisted) != 1 || persisted[0] != "units" {
		t.Fatalf("valid field PUT status=%d body=%s persisted=%v", put.Code, put.Body.String(), persisted)
	}
}
