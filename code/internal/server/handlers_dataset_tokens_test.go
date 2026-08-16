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
	cfg.DatasetAPI.FieldAllowlist = []string{"sales_units", "returns_qty"}
	cfg.DatasetAPI.Tokens = []config.DatasetToken{{
		ID: "tok_reader", ProjectID: "reader", TokenHash: strings.Repeat("a", 64),
		DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"12534"},
		Fields: []string{"sales_units", "returns_qty"},
	}}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodPut, datasetFieldsConfigPath, strings.NewReader(`{"fields":["returns_qty","sessions_total"]}`))
	rec := httptest.NewRecorder()

	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("save dataset fields status=%d body=%s", rec.Code, rec.Body.String())
	}
	saved := store.Current().DatasetAPI
	if got, want := strings.Join(saved.FieldAllowlist, ","), "returns_qty,sessions_total"; got != want {
		t.Fatalf("saved table fields=%q, want %q", got, want)
	}
	if got, want := strings.Join(saved.Tokens[0].Fields, ","), "returns_qty,sessions_total"; got != want {
		t.Fatalf("saved project fields=%q, want %q", got, want)
	}
	getRec := httptest.NewRecorder()
	s.Routes().ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, datasetFieldsConfigPath, nil))
	if getRec.Code != http.StatusOK || !strings.Contains(getRec.Body.String(), `"configured_fields":["returns_qty","sessions_total"]`) || !strings.Contains(getRec.Body.String(), `"sales_amount"`) {
		t.Fatalf("get dataset field config status=%d body=%s", getRec.Code, getRec.Body.String())
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

func TestCreateDatasetProjectTokenReturnsPlaintextOnceAndPersistsOnlyHash(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodPost, datasetProjectsPath, strings.NewReader(`{
		"project_id":"polabel2",
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
	if response.Data.ProjectID != "polabel2" || !strings.HasPrefix(response.Data.TokenID, "tok_") || response.Data.TokenID == "polabel2" || response.Data.Token == "" || !response.Data.NeedRestart {
		t.Fatalf("create response=%+v", response.Data)
	}
	saved := store.Current().DatasetAPI.Tokens
	if len(saved) != 1 {
		t.Fatalf("saved tokens=%d, want 1", len(saved))
	}
	if saved[0].TokenHash != datasetapi.HashToken(response.Data.Token) {
		t.Fatalf("saved token hash does not match returned plaintext")
	}
	if strings.Contains(saved[0].TokenHash, response.Data.Token) || saved[0].TokenHash == response.Data.Token {
		t.Fatal("plaintext token must not be persisted")
	}
	if saved[0].ID != response.Data.TokenID || len(saved[0].Fields) != len(availableDatasetFields) {
		t.Fatalf("saved token id/fields=%q/%d", saved[0].ID, len(saved[0].Fields))
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
		{name: "project exists", body: `{"project_id":"polabel2","store_scopes":["12534"]}`, code: http.StatusConflict, want: "下游项目 ID 已存在"},
		{name: "empty stores", body: `{"project_id":"new_reader","store_scopes":[]}`, code: http.StatusBadRequest, want: "店铺范围不能为空"},
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

func TestCreateDatasetProjectTokenInitializesEmptyDatasetConfig(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.CursorSecret = ""
	cfg.DatasetAPI.FieldAllowlist = nil
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodPost, datasetProjectsPath, strings.NewReader(`{"project_id":"polabel2","store_scopes":["12534"]}`))
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("empty config status=%d body=%s", rec.Code, rec.Body.String())
	}
	saved := store.Current().DatasetAPI
	if len(saved.FieldAllowlist) != len(availableDatasetFields) || len(saved.CursorSecret) < 16 || len(saved.Tokens) != 1 {
		t.Fatalf("initialized dataset config=%+v", saved)
	}
	if got := saved.Tokens[0].Fields; len(got) != len(availableDatasetFields) || got[0] != "sales_units" || got[len(got)-1] != "sb_orders" {
		t.Fatalf("initialized token fields=%v", got)
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
