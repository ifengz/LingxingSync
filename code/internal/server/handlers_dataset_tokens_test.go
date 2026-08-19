package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/datasetapi"
)

type datasetExportFixtureReader struct {
	queries []datasetapi.Query
}

func (r *datasetExportFixtureReader) Snapshot(_ context.Context, query datasetapi.Query) (datasetapi.Page, error) {
	r.queries = append(r.queries, query)
	return datasetapi.Page{Rows: []datasetapi.Row{{
		Store: "12534", Channel: "SC", ASIN: "ASIN1", SKU: "SKU1", BusinessDate: "2026-08-14",
		UpdatedAt: time.Date(2026, 8, 16, 3, 4, 5, 0, time.UTC), StableKey: "1|2026-08-14", VerificationStatus: "verified",
		Values: map[string]any{"sales_units": int64(3)},
	}}}, nil
}

func (datasetExportFixtureReader) Changes(context.Context, datasetapi.Query) (datasetapi.Page, error) {
	return datasetapi.Page{}, nil
}

const datasetProjectsPath = "/api/datasources/datasets/listing-daily-v1/projects"
const datasetFieldsCompletePath = "/api/datasources/datasets/listing-daily-v1/fields/complete"
const datasetFieldsConfigPath = "/api/datasources/datasets/listing-daily-v1/fields/config"

func TestExportDatasetCSVUsesRegisteredReaderAndDateRange(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.FieldAllowlist = []string{"sales_units"}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	definition, _ := datasetapi.DefinitionFor(datasetapi.DatasetID)
	reader := &datasetExportFixtureReader{}
	handler, err := datasetapi.New(datasetapi.Config{Definition: definition, FieldAllowlist: []string{"sales_units"}, CursorSecret: []byte("cursor-secret-for-tests")}, reader)
	if err != nil {
		t.Fatalf("new dataset handler: %v", err)
	}
	s.datasetAPIs[datasetapi.DatasetID] = handler
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/datasources/datasets/listing-daily-v1/export?stores=12534,12536&date_from=2026-08-14&date_to=2026-08-14", nil))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") || !strings.Contains(rec.Header().Get("Content-Disposition"), "listing-daily-v1_2026-08-14_2026-08-14.csv") || !strings.Contains(rec.Body.String(), "ASIN1") {
		t.Fatalf("export status=%d headers=%v body=%q", rec.Code, rec.Header(), rec.Body.String())
	}
	if len(reader.queries) != 1 || strings.Join(reader.queries[0].Stores, ",") != "12534,12536" {
		t.Fatalf("export stores=%+v, want [12534 12536]", reader.queries)
	}
}

func TestSaveDatasetFieldAllowlistPersistsTableSelectionForAllProjects(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.FieldAllowlist = nil
	cfg.DatasetAPI.Tokens = []config.DatasetToken{{
		ID: "tok_reader", ProjectID: "reader", TokenHash: strings.Repeat("a", 64),
		DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"12534"},
		Fields: []string{"sales_units", "returns_qty"},
	}}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodPut, datasetFieldsConfigPath, strings.NewReader(`{"fields":["sales_units","returns_qty"]}`))
	rec := httptest.NewRecorder()

	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("save dataset fields status=%d body=%s", rec.Code, rec.Body.String())
	}
	saved := store.Current().DatasetAPI
	if got, want := strings.Join(saved.FieldAllowlist, ","), "sales_units,returns_qty"; got != want {
		t.Fatalf("saved table fields=%q, want %q", got, want)
	}
	if got, want := strings.Join(saved.Tokens[0].Fields, ","), "sales_units,returns_qty"; got != want {
		t.Fatalf("saved project fields=%q, want %q", got, want)
	}
	getRec := httptest.NewRecorder()
	s.Routes().ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, datasetFieldsConfigPath, nil))
	if getRec.Code != http.StatusOK || !strings.Contains(getRec.Body.String(), `"configured_fields":["sales_units","returns_qty"]`) || !strings.Contains(getRec.Body.String(), `"sales_amount"`) {
		t.Fatalf("get dataset field config status=%d body=%s", getRec.Code, getRec.Body.String())
	}
}

func TestSaveDatasetFieldAllowlistRejectsChangingPublishedV1FieldSet(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.FieldAllowlist = []string{"sales_units", "returns_qty"}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodPut, datasetFieldsConfigPath, strings.NewReader(`{"fields":["sales_units","returns_qty","sessions_total"]}`))
	rec := httptest.NewRecorder()

	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "数据表版本已发布，字段不能增删改") {
		t.Fatalf("status=%d body=%s, want immutable-version conflict", rec.Code, rec.Body.String())
	}
	if got := strings.Join(store.Current().DatasetAPI.FieldAllowlist, ","); got != "sales_units,returns_qty" {
		t.Fatalf("field allowlist changed after rejected version change: %q", got)
	}
}

func TestVersionedDatasetFieldAllowlistInheritsDraftThenLocksAfterFirstPublish(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.FieldAllowlist = []string{"sales_units", "returns_qty"}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	path := "/api/datasources/datasets/return-reason-detail-v2/fields/config"
	v1 := httptest.NewRecorder()
	s.Routes().ServeHTTP(v1, httptest.NewRequest(http.MethodGet, "/api/datasources/datasets/return-reason-detail-v1/fields/config", nil))
	if v1.Code != http.StatusOK || !strings.Contains(v1.Body.String(), `"published":true`) || !strings.Contains(v1.Body.String(), `"next_version_id":"return-reason-detail-v2"`) {
		t.Fatalf("v1 metadata status=%d body=%s", v1.Code, v1.Body.String())
	}
	get := httptest.NewRecorder()
	s.Routes().ServeHTTP(get, httptest.NewRequest(http.MethodGet, path, nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"published":false`) || !strings.Contains(get.Body.String(), `"parent_dataset_id":"return-reason-detail-v1"`) {
		t.Fatalf("draft metadata status=%d body=%s", get.Code, get.Body.String())
	}
	if !strings.Contains(get.Body.String(), `"configured_fields":["license_plate_number","order_id"`) || !strings.Contains(get.Body.String(), `"return_date_locale"`) {
		t.Fatalf("draft must inherit v1 fields and expose candidates: %s", get.Body.String())
	}
	missingInherited := httptest.NewRecorder()
	s.Routes().ServeHTTP(missingInherited, httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"fields":["license_plate_number","return_date_locale"]}`)))
	if missingInherited.Code != http.StatusBadRequest || !strings.Contains(missingInherited.Body.String(), "继承字段不能删除") {
		t.Fatalf("missing inherited fields status=%d body=%s", missingInherited.Code, missingInherited.Body.String())
	}
	definition, _ := datasetapi.DefinitionFor("return-reason-detail-v2")
	publishBody, err := json.Marshal(map[string]any{"fields": append(definition.Fields, "return_date_locale")})
	if err != nil {
		t.Fatalf("encode publish body: %v", err)
	}
	first := httptest.NewRecorder()
	s.Routes().ServeHTTP(first, httptest.NewRequest(http.MethodPut, path, strings.NewReader(string(publishBody))))
	if first.Code != http.StatusOK {
		t.Fatalf("first draft publish status=%d body=%s", first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	s.Routes().ServeHTTP(second, httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"fields":["license_plate_number"]}`)))
	if second.Code != http.StatusConflict {
		t.Fatalf("second draft change status=%d body=%s", second.Code, second.Body.String())
	}
}

func TestSaveDatasetFieldAllowlistRejectsRemovingPublishedV1Field(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.FieldAllowlist = []string{"sales_units", "returns_qty"}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodPut, datasetFieldsConfigPath, strings.NewReader(`{"fields":["returns_qty"]}`))
	rec := httptest.NewRecorder()

	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "数据表版本已发布，字段不能增删改") {
		t.Fatalf("status=%d body=%s, want conflict explaining versioned schema", rec.Code, rec.Body.String())
	}
	if got := strings.Join(store.Current().DatasetAPI.FieldAllowlist, ","); got != "sales_units,returns_qty" {
		t.Fatalf("field allowlist changed after rejected removal: %q", got)
	}
}

func TestSaveDatasetFieldAllowlistRejectsUnknownDuplicateOrEmptyFields(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.Tokens = []config.DatasetToken{{
		ID: "tok_reader", ProjectID: "reader", TokenHash: strings.Repeat("a", 64),
		DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"12534"},
		Fields: []string{"sales_units", "returns_qty"},
	}}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "empty", body: `{"fields":[]}`, want: "字段不能为空"},
		{name: "unknown", body: `{"fields":["not_a_field"]}`, want: "字段不可用"},
		{name: "duplicate", body: `{"fields":["sales_units","sales_units"]}`, want: "字段不能重复"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, datasetFieldsConfigPath, strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			s.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("status=%d body=%s, want %d containing %q", rec.Code, rec.Body.String(), http.StatusBadRequest, tc.want)
			}
		})
	}
}

func TestCreateDatasetProjectTokenPersistsPlaintextForInternalProjects(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodPost, datasetProjectsPath, strings.NewReader(`{
		"project_id":"polabel2",
		"dataset_scopes":["listing-daily-v1"],
		"store_scopes":["12534"]
	}`))
	rec := httptest.NewRecorder()

	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create dataset token status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			ProjectID   string `json:"project_id"`
			TokenID     string `json:"token_id"`
			Token       string `json:"token"`
			NeedRestart bool   `json:"need_restart"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if response.Data.ProjectID != "polabel2" || !strings.HasPrefix(response.Data.TokenID, "tok_") || len(response.Data.TokenID) != 36 || response.Data.TokenID == "polabel2" || response.Data.Token == "" || !response.Data.NeedRestart {
		t.Fatalf("create response=%+v", response.Data)
	}
	saved := store.Current().DatasetAPI.Tokens
	if len(saved) != 1 {
		t.Fatalf("saved tokens=%d, want 1", len(saved))
	}
	if saved[0].TokenHash != datasetapi.HashToken(response.Data.Token) {
		t.Fatalf("saved token hash does not match returned plaintext")
	}
	if saved[0].Token != response.Data.Token {
		t.Fatalf("saved plaintext token=%q, want returned token", saved[0].Token)
	}
	if saved[0].ID != response.Data.TokenID || strings.Join(saved[0].Fields, ",") != "sales_units,returns_qty" {
		t.Fatalf("saved token id/fields=%q/%v", saved[0].ID, saved[0].Fields)
	}
}

func TestDatasetCatalogKeepsRestartRequiredUntilNewServerStarts(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	assets := Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}
	s := New(cfg, nil, nil, nil, "", assets, store, nil, nil, "")

	create := httptest.NewRecorder()
	s.Routes().ServeHTTP(create, httptest.NewRequest(http.MethodPost, datasetProjectsPath, strings.NewReader(`{
		"project_id":"polabel2",
		"dataset_scopes":["listing-daily-v1"],
		"store_scopes":["12534"]
	}`)))
	if create.Code != http.StatusOK {
		t.Fatalf("create dataset project status=%d body=%s", create.Code, create.Body.String())
	}

	readRestartState := func(server *Server) bool {
		rec := httptest.NewRecorder()
		server.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/datasources/datasets/catalog", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("catalog status=%d body=%s", rec.Code, rec.Body.String())
		}
		var response struct {
			Data struct {
				NeedRestart bool `json:"need_restart"`
				Projects    []struct {
					ProjectID string `json:"project_id"`
				} `json:"projects"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode catalog: %v", err)
		}
		if len(response.Data.Projects) != 1 || response.Data.Projects[0].ProjectID != "polabel2" {
			t.Fatalf("catalog projects=%+v, want persisted polabel2 project", response.Data.Projects)
		}
		return response.Data.NeedRestart
	}

	if !readRestartState(s) {
		t.Fatal("catalog must retain restart-required state after a page refresh")
	}
	if readRestartState(New(store.Current(), nil, nil, nil, "", assets, store, nil, nil, "")) {
		t.Fatal("catalog must clear restart-required state after the new server starts")
	}
}

func TestUpdateDownstreamProjectScopesKeepsPlaintextToken(t *testing.T) {
	rawToken := "internal-reader-token"
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.Tokens = []config.DatasetToken{{
		ID: "tok_reader", ProjectID: "reader", Token: rawToken, TokenHash: datasetapi.HashToken(rawToken),
		DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"12534"}, Fields: []string{"sales_units", "returns_qty"},
	}}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodPut, "/api/datasources/datasets/projects/tok_reader", strings.NewReader(`{"dataset_scopes":["listing-daily-v1","return-reason-detail-v1"],"store_scopes":["12536"]}`))
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"need_restart":true`) || !strings.Contains(rec.Body.String(), rawToken) {
		t.Fatalf("update status=%d body=%s", rec.Code, rec.Body.String())
	}
	saved := store.Current().DatasetAPI.Tokens[0]
	if saved.Token != rawToken || saved.TokenHash != datasetapi.HashToken(rawToken) || strings.Join(saved.DatasetScopes, ",") != "listing-daily-v1,return-reason-detail-v1" || strings.Join(saved.StoreScopes, ",") != "12536" {
		t.Fatalf("updated token=%+v", saved)
	}
}

func TestDeleteDownstreamProjectRemovesOnlyMatchingToken(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.Tokens = []config.DatasetToken{
		{ID: "tok_delete", ProjectID: "polabel2", TokenHash: strings.Repeat("a", 64), DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"12534"}, Fields: []string{"sales_units"}},
		{ID: "tok_keep", ProjectID: "warehouse", TokenHash: strings.Repeat("b", 64), DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"12536"}, Fields: []string{"sales_units"}},
	}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	rec := httptest.NewRecorder()

	s.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/datasources/datasets/projects/tok_delete", nil))

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"need_restart":true`) || !strings.Contains(rec.Body.String(), `"project_id":"polabel2"`) {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	saved := store.Current().DatasetAPI.Tokens
	if len(saved) != 1 || saved[0].ID != "tok_keep" {
		t.Fatalf("saved tokens=%+v, want only tok_keep", saved)
	}
	missing := httptest.NewRecorder()
	s.Routes().ServeHTTP(missing, httptest.NewRequest(http.MethodDelete, "/api/datasources/datasets/projects/not-found", nil))
	if missing.Code != http.StatusNotFound || len(store.Current().DatasetAPI.Tokens) != 1 {
		t.Fatalf("missing delete status=%d body=%s tokens=%+v", missing.Code, missing.Body.String(), store.Current().DatasetAPI.Tokens)
	}
}

func TestDownstreamProjectGuideContainsAuthorizedSchemaAndExamples(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.Server.Secret = "admin-secret"
	cfg.DatasetAPI.Tokens = []config.DatasetToken{{
		ID: "tok_reader", ProjectID: "reader", Token: "visible-internal-token", TokenHash: datasetapi.HashToken("visible-internal-token"),
		DatasetScopes: []string{datasetapi.DatasetID, "return-reason-detail-v1", "fba-inventory-snapshot-v1"}, StoreScopes: []string{"12534"},
		Fields: []string{"sales_units", "returns_qty"},
	}}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/datasources/datasets/projects/tok_reader/guide", nil)
	req.Header.Set("X-Sync-Secret", "admin-secret")
	s.withMiddleware(s.Routes()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("guide status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"接入说明", "listing-daily-v1", "return-reason-detail-v1", "fba-inventory-snapshot-v1", "历史从本版本部署后每次成功同步开始累计", "CREATE TABLE", "/snapshot", "/changes", "12534", "visible-internal-token"} {
		if !strings.Contains(body, want) {
			t.Fatalf("guide missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "当前源表是库存当前状态") {
		t.Fatalf("guide still describes FBA dataset as current-state only: %s", body)
	}
}

func TestDownstreamProjectGuideRejectsAnonymousAccess(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.Server.Secret = "admin-secret"
	cfg.DatasetAPI.Tokens = []config.DatasetToken{{
		ID: "tok_reader", ProjectID: "reader", Token: "visible-internal-token", TokenHash: datasetapi.HashToken("visible-internal-token"),
		DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"12534"}, Fields: []string{"sales_units"},
	}}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/datasources/datasets/projects/tok_reader/guide", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous guide status=%d, want 401; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "visible-internal-token") {
		t.Fatalf("anonymous guide leaked bearer token: %s", rec.Body.String())
	}
}

func TestDownstreamProjectGuideRejectsWrongAdminSecret(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.Server.Secret = "admin-secret"
	cfg.DatasetAPI.Tokens = []config.DatasetToken{{
		ID: "tok_reader", ProjectID: "reader", Token: "visible-internal-token", TokenHash: datasetapi.HashToken("visible-internal-token"),
		DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"12534"}, Fields: []string{"sales_units"},
	}}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodGet, "/api/datasources/datasets/projects/tok_reader/guide", nil)
	req.Header.Set("X-Sync-Secret", "wrong-secret")
	rec := httptest.NewRecorder()
	s.withMiddleware(s.Routes()).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-secret guide status=%d, want 401; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "visible-internal-token") {
		t.Fatalf("wrong-secret guide leaked bearer token: %s", rec.Body.String())
	}
}

func TestDownstreamProjectGuideFailsClosedWithoutAdminSecret(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.Tokens = []config.DatasetToken{{
		ID: "tok_reader", ProjectID: "reader", Token: "visible-internal-token", TokenHash: datasetapi.HashToken("visible-internal-token"),
		DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"12534"}, Fields: []string{"sales_units"},
	}}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodGet, "/api/datasources/datasets/projects/tok_reader/guide", nil)
	rec := httptest.NewRecorder()
	s.withMiddleware(s.Routes()).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unconfigured secret guide status=%d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "visible-internal-token") {
		t.Fatalf("unconfigured secret guide leaked bearer token: %s", rec.Body.String())
	}
}

func TestCreateDownstreamProjectTokenPersistsSelectedDatasetScopes(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodPost, "/api/datasources/datasets/projects", strings.NewReader(`{
		"project_id":"warehouse_reader",
		"dataset_scopes":["listing-daily-v1","return-reason-detail-v1"],
		"store_scopes":["12534"]
	}`))
	rec := httptest.NewRecorder()

	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create multi-dataset token status=%d body=%s", rec.Code, rec.Body.String())
	}
	saved := store.Current().DatasetAPI.Tokens
	if len(saved) != 1 || strings.Join(saved[0].DatasetScopes, ",") != "listing-daily-v1,return-reason-detail-v1" {
		t.Fatalf("saved dataset scopes=%+v", saved)
	}
}

func TestDatasetCatalogListsProjectsOutsideCurrentDatasetScope(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.Tokens = []config.DatasetToken{
		{ID: "tok_listing", ProjectID: "listing_reader", Token: "listing-token", TokenHash: datasetapi.HashToken("listing-token"), DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"12534"}, Fields: []string{"sales_units"}},
		{ID: "tok_returns", ProjectID: "returns_reader", Token: "returns-token", TokenHash: datasetapi.HashToken("returns-token"), DatasetScopes: []string{"return-reason-detail-v1"}, StoreScopes: []string{"12536"}, Fields: []string{"sales_units"}},
	}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	rec := httptest.NewRecorder()

	s.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/datasources/datasets/catalog", nil))

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"project_id":"listing_reader"`) || !strings.Contains(rec.Body.String(), `"project_id":"returns_reader"`) || !strings.Contains(rec.Body.String(), `"token":"returns-token"`) {
		t.Fatalf("catalog must return every downstream project, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateDatasetProjectTokenRejectsDuplicateAndInvalidScope(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.Tokens = []config.DatasetToken{{ID: "existing", ProjectID: "polabel2", TokenHash: strings.Repeat("a", 64), DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"12534"}, Fields: []string{"sales_units"}}}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	for _, tc := range []struct {
		name string
		body string
		code int
		want string
	}{
		{name: "project exists", body: `{"project_id":"polabel2","dataset_scopes":["listing-daily-v1"],"store_scopes":["12534"]}`, code: http.StatusConflict, want: "下游项目 ID 已存在"},
		{name: "empty datasets", body: `{"project_id":"new_reader","dataset_scopes":[],"store_scopes":["12534"]}`, code: http.StatusBadRequest, want: "数据表范围不能为空"},
		{name: "empty stores", body: `{"project_id":"new_reader","dataset_scopes":["listing-daily-v1"],"store_scopes":[]}`, code: http.StatusBadRequest, want: "店铺范围不能为空"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, datasetProjectsPath, strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			s.Routes().ServeHTTP(rec, req)
			if rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("status=%d body=%s, want status=%d containing %q", rec.Code, rec.Body.String(), tc.code, tc.want)
			}
		})
	}
}

func TestCreateDatasetProjectTokenRejectsUnconfiguredDataset(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.CursorSecret = ""
	cfg.DatasetAPI.FieldAllowlist = nil
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodPost, datasetProjectsPath, strings.NewReader(`{"project_id":"polabel2","dataset_scopes":["listing-daily-v1"],"store_scopes":["12534"]}`))
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "数据表尚未发布字段") {
		t.Fatalf("empty config status=%d body=%s, want conflict", rec.Code, rec.Body.String())
	}
	saved := store.Current().DatasetAPI
	if len(saved.FieldAllowlist) != 0 || saved.CursorSecret != "" || len(saved.Tokens) != 0 {
		t.Fatalf("rejected create changed dataset config=%+v", saved)
	}
}

func TestCompleteDatasetFieldsAddsMissingMetricsWithoutChangingProjectPermission(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.Tokens = []config.DatasetToken{{ID: "polabel2", ProjectID: "polabel2", TokenHash: strings.Repeat("a", 64), DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"12534"}, Fields: []string{"sales_units"}}}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	rec := httptest.NewRecorder()

	s.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, datasetFieldsCompletePath, nil))

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"need_restart":true`) {
		t.Fatalf("complete fields status=%d body=%s", rec.Code, rec.Body.String())
	}
	saved := store.Current().DatasetAPI
	if len(saved.FieldAllowlist) != len(availableDatasetFields) || saved.Tokens[0].Fields[0] != "sales_units" || len(saved.Tokens[0].Fields) != 1 {
		t.Fatalf("completed dataset config=%+v", saved)
	}
}

func validDatasetProjectTestConfig() *config.Config {
	return &config.Config{
		Server:    config.Server{Port: 7799},
		Database:  config.Database{Host: "127.0.0.1", Port: 3306, User: "test", DB: "lingsync", MaxOpen: 20, MaxIdle: 5, ConnTimeoutSec: 10},
		Accounts:  []config.Account{{ID: "sc_us", AppKey: "key", AppSecret: "secret", ConnectionCheck: config.DefaultConnectionCheck()}},
		Retention: config.Retention{TaskLogsDays: 90, TasksDays: 365, CleanupCron: "0 3 * * *"},
		DatasetAPI: config.DatasetAPIConfig{
			CursorSecret:   "cursor-secret-for-test",
			FieldAllowlist: []string{"sales_units", "returns_qty"},
		},
	}
}
