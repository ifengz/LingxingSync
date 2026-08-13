package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/datasetapi"
)

const datasetProjectsPath = "/api/datasources/datasets/listing-daily-v1/projects"

func TestCreateDatasetProjectTokenReturnsPlaintextOnceAndPersistsOnlyHash(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodPost, datasetProjectsPath, strings.NewReader(`{
		"project_id":"polabel2",
		"token_id":"polabel2_reader_1",
		"store_scopes":["12534"],
		"fields":["sales_units","returns_qty"]
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
	if response.Data.ProjectID != "polabel2" || response.Data.TokenID != "polabel2_reader_1" || response.Data.Token == "" || !response.Data.NeedRestart {
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
		{name: "duplicate token", body: `{"project_id":"polabel2","token_id":"existing","store_scopes":["12534"],"fields":["sales_units"]}`, code: http.StatusConflict, want: "Token ID 已存在"},
		{name: "empty fields", body: `{"project_id":"polabel2","token_id":"new_reader","store_scopes":["12534"],"fields":[]}`, code: http.StatusBadRequest, want: "字段不能为空"},
		{name: "empty stores", body: `{"project_id":"polabel2","token_id":"new_reader","store_scopes":[],"fields":["sales_units"]}`, code: http.StatusBadRequest, want: "店铺范围不能为空"},
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

func TestCreateDatasetProjectTokenBootstrapsEmptyDatasetConfig(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.CursorSecret = ""
	cfg.DatasetAPI.FieldAllowlist = nil
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, store, nil, nil, "")
	req := httptest.NewRequest(http.MethodPost, datasetProjectsPath, strings.NewReader(`{"project_id":"polabel2","token_id":"reader","store_scopes":["12534"],"fields":["sales_units"]}`))
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%s", rec.Code, rec.Body.String())
	}
	saved := store.Current().DatasetAPI
	if saved.CursorSecret == "" || len(saved.FieldAllowlist) != 1 || saved.FieldAllowlist[0] != "sales_units" {
		t.Fatalf("bootstrap dataset config=%+v", saved)
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
