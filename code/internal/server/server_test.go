package server

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/datasetapi"
	"lingxing-sync/internal/db"
	"lingxing-sync/internal/worker"
)

//go:embed testdata/*.html
var renderTestFS embed.FS

func TestRenderPageWritesLayout(t *testing.T) {
	s := &Server{
		cfg:    &config.Config{},
		assets: Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"},
		pages:  map[string]*template.Template{},
	}
	if err := s.parseTemplates(); err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	recorder := httptest.NewRecorder()
	s.renderPage(recorder, "sync_manage", pageData{Active: "sync_manage"})
	if !strings.Contains(recorder.Body.String(), "<html>") {
		t.Fatalf("rendered page missing layout: %q", recorder.Body.String())
	}
}

func TestRenderPageIncludesSyncSecretMeta(t *testing.T) {
	s := &Server{
		cfg:    &config.Config{Server: config.Server{Secret: "admin-secret"}},
		assets: Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"},
		pages:  map[string]*template.Template{},
	}
	if err := s.parseTemplates(); err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	recorder := httptest.NewRecorder()
	s.renderPage(recorder, "sync_manage", s.newPageData("sync_manage"))
	if got := recorder.Body.String(); !strings.Contains(got, `<meta name="sync-secret" content="admin-secret">`) {
		t.Fatalf("rendered page missing sync secret meta: %q", got)
	}
}

func TestNavigationIncludesDedicatedDatasetFieldsPage(t *testing.T) {
	items := sharedFuncs()["listItems"].(func() []navItem)()
	if len(items) != 5 {
		t.Fatalf("navigation items=%d, want 5", len(items))
	}
	want := navItem{Key: "dataset_fields", Href: "/dataset-fields", Label: "数据表字段"}
	if items[4] != want {
		t.Fatalf("fifth navigation item=%+v, want %+v", items[4], want)
	}
}

func TestDatasetFieldsPageRouteRendersDedicatedTemplate(t *testing.T) {
	s := &Server{
		cfg:    &config.Config{},
		assets: Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"},
		pages:  map[string]*template.Template{},
	}
	if err := s.parseTemplates(); err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	recorder := httptest.NewRecorder()
	s.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/dataset-fields", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "<html>") {
		t.Fatalf("dataset fields page status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestShortBuildCommit(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{name: "full sha", in: "0123456789abcdef0123456789abcdef01234567", want: "0123456"},
		{name: "local development", in: "dev", want: ""},
		{name: "trimmed", in: "  abcdefg  ", want: "abcdefg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortBuildCommit(tc.in); got != tc.want {
				t.Fatalf("shortBuildCommit(%q)=%q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSettingsExposeDeployedCommit(t *testing.T) {
	previousCommit := BuildCommit
	BuildCommit = "0123456789abcdef0123456789abcdef01234567"
	t.Cleanup(func() { BuildCommit = previousCommit })

	dbx, err := sqlx.Open("mysql", "invalid:invalid@tcp(127.0.0.1:1)/invalid?timeout=10ms")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer dbx.Close()

	s := &Server{
		cfg:       &config.Config{},
		dbx:       dbx,
		startTime: time.Now(),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rec := httptest.NewRecorder()

	s.apiSettings(rec, req)

	var response struct {
		Data struct {
			DeployCommit string `json:"deploy_commit"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	if response.Data.DeployCommit != BuildCommit {
		t.Fatalf("deploy_commit=%q, want %q; body=%s", response.Data.DeployCommit, BuildCommit, rec.Body.String())
	}
}

func TestManualSyncRejectsDisabledEndpoint(t *testing.T) {
	registry := worker.NewRegistry()
	registry.Register(&worker.EndpointWorker{Endpoint: config.Endpoint{Name: "inventory", Enabled: false}})
	s := &Server{reg: registry}
	req := httptest.NewRequest(http.MethodPost, "/api/sync/inventory", nil)
	req.SetPathValue("name", "inventory")
	rec := httptest.NewRecorder()

	s.apiSyncTrigger(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("disabled endpoint status=%d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "已禁用") {
		t.Fatalf("disabled endpoint response=%q, want disabled explanation", rec.Body.String())
	}
}

func TestDatasetRoutesUseBearerAndReportMissingReader(t *testing.T) {
	rawToken := "dataset-project-token"
	cfg := &config.Config{
		Server: config.Server{Secret: "admin-secret"},
		DatasetAPI: config.DatasetAPIConfig{
			CursorSecret:   "cursor-secret-for-server-test",
			FieldAllowlist: []string{"units"},
			Tokens: []config.DatasetToken{{
				ID: "project-a", TokenHash: datasetapi.HashToken(rawToken), DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"store-a"}, Fields: []string{"units"},
			}},
		},
	}
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, nil, nil, nil, "")
	h := s.withMiddleware(s.Routes())

	req := httptest.NewRequest(http.MethodPost, datasetapi.SnapshotPath, strings.NewReader(`{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01"}`))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("snapshot status=%d, want 503; body=%s", rec.Code, rec.Body.String())
	}

	fieldsReq := httptest.NewRequest(http.MethodGet, datasetapi.FieldsPath, nil)
	fieldsReq.Header.Set("Authorization", "Bearer "+rawToken)
	fieldsRec := httptest.NewRecorder()
	h.ServeHTTP(fieldsRec, fieldsReq)
	if fieldsRec.Code != http.StatusUnauthorized {
		t.Fatalf("fields without admin secret status=%d, want 401; body=%s", fieldsRec.Code, fieldsRec.Body.String())
	}
	fieldsReq = httptest.NewRequest(http.MethodGet, datasetapi.FieldsPath+"?project_id=project-a", nil)
	fieldsReq.Header.Set("X-Sync-Secret", "admin-secret")
	fieldsRec = httptest.NewRecorder()
	h.ServeHTTP(fieldsRec, fieldsReq)
	if fieldsRec.Code != http.StatusOK {
		t.Fatalf("fields with admin secret status=%d, want 200; body=%s", fieldsRec.Code, fieldsRec.Body.String())
	}
}

func TestDatasetRoutesInjectSQLReaderWhenDBProvided(t *testing.T) {
	rawToken := "dataset-project-token"
	cfg := &config.Config{
		Server: config.Server{Secret: "admin-secret"},
		DatasetAPI: config.DatasetAPIConfig{
			CursorSecret:   "cursor-secret-for-server-test",
			FieldAllowlist: []string{"units"},
			Tokens: []config.DatasetToken{{
				ID: "project-a", TokenHash: datasetapi.HashToken(rawToken), DatasetScopes: []string{datasetapi.DatasetID}, StoreScopes: []string{"store-a"}, Fields: []string{"units"},
			}},
		},
	}
	dbx, err := sqlx.Open("mysql", "invalid:invalid@tcp(127.0.0.1:1)/invalid?timeout=1ms")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer dbx.Close()
	s := New(cfg, dbx, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, nil, nil, nil, "")
	h := s.withMiddleware(s.Routes())
	req := httptest.NewRequest(http.MethodPost, datasetapi.SnapshotPath, strings.NewReader(`{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01"}`))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("configured db must inject SQL reader, got 503: %s", rec.Body.String())
	}
}

func TestOperationsLogV2RouteInjectsDedicatedSQLReader(t *testing.T) {
	rawToken := "operations-v2-token"
	cfg := &config.Config{
		Server: config.Server{Secret: "admin-secret"},
		DatasetAPI: config.DatasetAPIConfig{
			CursorSecret: "cursor-secret-for-server-test",
			FieldAllowlists: map[string][]string{
				"operations-log-v2": {"channel_type", "identity_scope", "asin", "listing_sku", "sales_units", "sales_amount", "returns_qty", "inventory_sellable", "inventory_inbound", "inventory_reserved", "inventory_unfulfillable", "sessions_total", "sessions_desktop", "sessions_mobile", "rating", "review_count", "sp_spend", "sp_sales", "sp_orders", "sp_impressions", "sp_clicks", "sd_spend", "sd_sales", "sd_orders", "sd_impressions", "sd_clicks", "hsa_spend", "hsa_sales", "hsa_orders", "hsa_impressions", "hsa_clicks", "sb_spend", "sb_sales", "sb_orders", "sb_impressions", "sb_clicks", "is_provisional", "is_verified", "verified_fields"},
			},
			Tokens: []config.DatasetToken{{
				ID: "project-a", TokenHash: datasetapi.HashToken(rawToken), DatasetScopes: []string{"operations-log-v2"}, StoreScopes: []string{"store-a"},
			}},
		},
	}
	dbx, err := sqlx.Open("mysql", "invalid:invalid@tcp(127.0.0.1:1)/invalid?timeout=1ms")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer dbx.Close()
	s := New(cfg, dbx, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, nil, nil, nil, "")
	req := httptest.NewRequest(http.MethodPost, datasetapi.SnapshotPathFor("operations-log-v2"), strings.NewReader(`{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01"}`))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rec := httptest.NewRecorder()
	s.withMiddleware(s.Routes()).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("operations log v2 route must reach its SQL reader: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRegisteredDetailDatasetRouteUsesBearerAuthentication(t *testing.T) {
	rawToken := "return-reader-token"
	cfg := &config.Config{
		Server: config.Server{Secret: "admin-secret"},
		DatasetAPI: config.DatasetAPIConfig{
			CursorSecret:   "cursor-secret-for-server-test",
			FieldAllowlist: []string{"units"},
			FieldAllowlists: map[string][]string{
				"return-reason-detail-v1": {"reason"},
			},
			Tokens: []config.DatasetToken{{
				ID: "project-a", TokenHash: datasetapi.HashToken(rawToken), DatasetScopes: []string{"return-reason-detail-v1"}, StoreScopes: []string{"store-a"}, Fields: []string{"units"},
			}},
		},
	}
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, nil, nil, nil, "")
	h := s.withMiddleware(s.Routes())
	req := httptest.NewRequest(http.MethodPost, datasetapi.SnapshotPathFor("return-reason-detail-v1"), strings.NewReader(`{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01"}`))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("registered detail route status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// 配置 API 必须完整保留同步合同；否则线上经 GET 后编辑一行定时配置时，
// 广告账号迭代、协议头或主键回填会在无提示的情况下丢失。
func TestEndpointDTORoundTripPreservesAdvancedSyncContract(t *testing.T) {
	want := config.Endpoint{
		Name:               "ad_sp_product",
		Display:            "SP 商品广告报表",
		Account:            "sc_us_1",
		Path:               "/pb/openapi/newad/spProductAdReports",
		Method:             "POST",
		Table:              "ls_ad_sp_product",
		RecordIDFields:     []string{"sid", "profile_id", "report_date", "ad_id"},
		Rate:               config.Rate{Bucket: 1, IntervalMs: 1000, Dimension: "account+path"},
		Cron:               "0 5 * * *",
		Enabled:            true,
		DateField:          "report_date",
		DateOffsetDays:     2,
		ExtraParams:        map[string]any{"show_detail": 0},
		RequestHeaders:     map[string]string{"X-API-VERSION": "2"},
		IterateByAdAccount: true,
		AdAccountType:      "seller",
		ForceInjectParams:  []string{"sid", "profile_id"},
	}

	got := dtoToEndpoint(endpointToDTO(want))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("endpoint DTO round trip lost sync contract:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestEndpointDTORoundTripPreservesVCPOOrderIteration(t *testing.T) {
	want := config.Endpoint{IterateByVCOrders: true}
	got := dtoToEndpoint(endpointToDTO(want))
	if !got.IterateByVCOrders {
		t.Fatal("endpoint DTO round trip lost iterate_by_vc_orders")
	}
}

func TestEndpointDTORoundTripPreservesSalesOrderIteration(t *testing.T) {
	want := config.Endpoint{IterateBySalesOrders: true}
	got := dtoToEndpoint(endpointToDTO(want))
	if !got.IterateBySalesOrders {
		t.Fatal("endpoint DTO round trip lost iterate_by_sales_orders")
	}
}

func TestUpdateEndpointPersistsDailyDateContract(t *testing.T) {
	cfg := &config.Config{
		Database: config.Database{Host: "127.0.0.1", User: "test", DB: "lingsync"},
		Accounts: []config.Account{{ID: "sc_us_1", AppKey: "key", AppSecret: "secret"}},
		Endpoints: []config.Endpoint{{
			Name: "sc_performance", Account: "sc_us_1", Path: "/performance", Method: "POST",
			Table: "ls_sc_performance_daily", RecordIDFields: []string{"sid", "asin", "business_date"},
			Rate: config.Rate{Bucket: 1, IntervalMs: 1000, Dimension: "account+path"},
			Cron: "0 5 * * *", Enabled: true, WindowDays: 7,
		}},
	}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := &Server{cfg: cfg, store: store}
	body := `{
		"name":"sc_performance",
		"display":"SC 产品表现（ASIN）",
		"account":"sc_us_1",
		"path":"/performance",
		"method":"POST",
		"table":"ls_sc_performance_daily",
		"record_id_fields":["sid","asin","business_date"],
		"rate":{"bucket":1,"interval_ms":1000,"multi_interval_ms":10000,"dimension":"account+path"},
		"cron":"0 5 * * *",
		"enabled":true,
		"window_days":7,
		"single_day_window":true,
		"row_date_field":"business_date",
		"window_start_field":"start_date",
		"window_end_field":"end_date",
		"date_field":"",
		"date_offset_days":0,
		"extra_params":{},
		"headers":{},
		"iterate_by_store":true,
		"store_param_name":"sid",
		"store_sids":[],
		"store_type":"SC",
		"field_paths":{"asin":"asins[0].asin"},
		"inject_params":["sid"]
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/endpoints/sc_performance", strings.NewReader(body))
	req.SetPathValue("name", "sc_performance")
	rec := httptest.NewRecorder()

	s.apiUpdateEndpoint(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("update endpoint status=%d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	got := store.Current().Endpoints[0]
	if !got.SingleDayWindow || got.RowDateField != "business_date" ||
		got.WindowStartField != "start_date" || got.WindowEndField != "end_date" ||
		got.DateField != "" || got.DateOffsetDays != 0 {
		t.Fatalf("daily date contract not persisted: %#v", got)
	}
}

func TestUpdateEndpointSyncsRateForSharedLimiterVariants(t *testing.T) {
	base := config.Endpoint{
		Account: "sc_us_1", Path: "/pb/openapi/newad/spProductAdReports", Method: "POST",
		RecordIDFields: []string{"profile_id", "report_date", "ad_id"},
		Rate:           config.Rate{Bucket: 1, IntervalMs: 1000, Dimension: "account+path"},
		Cron:           "0 5 * * *", Enabled: true, ExtraParams: map[string]any{"type": "seller"},
	}
	seller := base
	seller.Name, seller.Table = "ad_sp_product_sc_us_1", "ls_ad_sp_product"
	vendor := base
	vendor.Name, vendor.Table, vendor.ExtraParams = "ad_vc_sp_product_sc_us_1", "ls_ad_vc_sp_product", map[string]any{"type": "vendor"}
	cfg := &config.Config{
		Database:  config.Database{Host: "127.0.0.1", User: "test", DB: "lingsync"},
		Accounts:  []config.Account{{ID: "sc_us_1", AppKey: "key", AppSecret: "secret"}},
		Endpoints: []config.Endpoint{seller, vendor},
	}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := &Server{cfg: cfg, store: store}
	update := endpointToDTO(vendor)
	update.Rate.Bucket = 10
	body, err := json.Marshal(update)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/endpoints/ad_vc_sp_product_sc_us_1", strings.NewReader(string(body)))
	req.SetPathValue("name", "ad_vc_sp_product_sc_us_1")
	rec := httptest.NewRecorder()

	s.apiUpdateEndpoint(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("update endpoint status=%d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	for _, ep := range store.Current().Endpoints {
		if ep.Rate.Bucket != 10 || ep.Rate.IntervalMs != 1000 {
			t.Fatalf("shared limiter variant %s rate = %#v, want bucket 10 interval 1000", ep.Name, ep.Rate)
		}
	}
}

func TestCreateProbeEndpointDoesNotRequireTableOrRecordIDs(t *testing.T) {
	cfg := &config.Config{
		Database: config.Database{Host: "127.0.0.1", User: "test", DB: "test"},
		Accounts: []config.Account{{ID: "sc_us", AppKey: "key", AppSecret: "secret"}},
	}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := &Server{cfg: cfg, store: store}
	body := `{
		"name":"sc_sales_revenue_probe",
		"account":"sc_us",
		"path":"/erp/sc/data/sales_report/asinDailyLists",
		"method":"POST",
		"table":"ls_sc_sales_revenue_probe",
		"record_id_fields":[],
		"rate":{"bucket":5,"interval_ms":200,"multi_interval_ms":1000,"dimension":"account+path"},
		"cron":"0 0 1 1 *",
		"enabled":true,
		"extra_params":{"type":1,"asin_type":1},
		"probe":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/endpoints", strings.NewReader(body))
	rec := httptest.NewRecorder()

	s.apiCreateEndpoint(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("probe endpoint status=%d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestApplyConfigWriteReturnsErrorWhenSchedulerRebuildFails(t *testing.T) {
	oldCfg, newCfg := reportHotReloadConfigs()
	endpoint := config.Endpoint{
		Name: "orders", Account: "sc_us", Path: "/orders", Method: "POST", Table: "ls_orders", RecordIDFields: []string{"order_id"},
		Rate: config.Rate{Bucket: 1, IntervalMs: 1000, Dimension: "account+path"}, Cron: "0 * * * *", Enabled: false,
	}
	oldCfg.Endpoints = []config.Endpoint{endpoint}
	endpoint.Enabled = true
	newCfg.Endpoints = []config.Endpoint{endpoint}
	store := config.NewStore(t.TempDir()+"/config.yaml", oldCfg)
	registry := worker.NewRegistry()
	registered := &worker.EndpointWorker{Endpoint: oldCfg.Endpoints[0]}
	registry.Register(registered)
	scheduler := worker.NewScheduler(oldCfg, registry, nil, func(context.Context, string) error { return nil })
	s := &Server{cfg: oldCfg, store: store, reg: registry, sched: scheduler, limiters: worker.NewLimiterRegistry()}
	rec := httptest.NewRecorder()

	s.applyConfigWrite(rec, oldCfg, newCfg, "updated")

	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "Rebuild") {
		t.Fatalf("rebuild failure response status=%d body=%s", rec.Code, rec.Body.String())
	}
	if registered.Endpoint.Enabled {
		t.Fatal("worker hot state changed even though scheduler Rebuild failed")
	}
}

func TestSettingsReloadReturnsErrorWhenSchedulerRebuildFails(t *testing.T) {
	oldCfg, newCfg := reportHotReloadConfigs()
	path := t.TempDir() + "/config.yaml"
	if err := config.NewStore(path, newCfg).Save(newCfg); err != nil {
		t.Fatal(err)
	}
	store := config.NewStore(path, oldCfg)
	registry := worker.NewRegistry()
	scheduler := worker.NewScheduler(oldCfg, registry, nil, func(context.Context, string) error { return nil })
	s := &Server{cfg: oldCfg, configPath: path, store: store, reg: registry, sched: scheduler, limiters: worker.NewLimiterRegistry()}
	rec := httptest.NewRecorder()

	s.apiSettingsReload(rec, httptest.NewRequest(http.MethodPost, "/api/settings/reload", nil))

	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "Rebuild") {
		t.Fatalf("reload rebuild failure response status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func reportHotReloadConfigs() (*config.Config, *config.Config) {
	oldCfg := &config.Config{
		Server:    config.Server{Port: 7799},
		Database:  config.Database{Host: "127.0.0.1", Port: 3306, User: "test", DB: "lingsync", MaxOpen: 20, MaxIdle: 5, ConnTimeoutSec: 10},
		Accounts:  []config.Account{{ID: "sc_us", AppKey: "key", AppSecret: "secret", ConnectionCheck: config.DefaultConnectionCheck()}},
		Retention: config.Retention{TaskLogsDays: 90, TasksDays: 365, CleanupCron: "0 3 * * *"},
	}
	newCfg := *oldCfg
	newCfg.ReportExports = []config.ReportExport{{
		Type: config.ReportExportCustomerReturns, Enabled: true, Account: "sc_us", SellerID: "SELLER-1", StoreID: "STORE-1",
		Region: "na", MarketplaceIDs: []string{"ATVPDKIKX0DER"}, Cron: "0 4 * * *", WindowDays: 3,
	}}
	return oldCfg, &newCfg
}

func TestValidateSyncDateRange(t *testing.T) {
	if err := validateSyncDateRange("2026-08-01", "2026-08-03"); err != nil {
		t.Fatalf("valid date range rejected: %v", err)
	}
	for _, tc := range []struct{ from, to string }{
		{"2026-08-03", "2026-08-01"},
		{"2026/08/01", "2026-08-03"},
		{"", "2026-08-03"},
	} {
		if err := validateSyncDateRange(tc.from, tc.to); err == nil {
			t.Fatalf("invalid date range accepted: %#v", tc)
		}
	}
}

func TestValidateSingleDaySyncDateRangeLimit(t *testing.T) {
	if err := validateSingleDaySyncDateRange("2024-01-01", "2024-04-01"); err != nil {
		t.Fatalf("92-day range rejected: %v", err)
	}
	if err := validateSingleDaySyncDateRange("2024-01-01", "2024-04-02"); err == nil {
		t.Fatal("93-day range was accepted")
	}
}

func TestManualSyncRejectsSingleDayRangeOver92Days(t *testing.T) {
	registry := worker.NewRegistry()
	registry.Register(&worker.EndpointWorker{Endpoint: config.Endpoint{
		Name: "performance", Enabled: true, SingleDayWindow: true,
	}})
	s := &Server{reg: registry}
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/sync/performance",
		strings.NewReader(`{"date_from":"2024-01-01","date_to":"2024-04-02"}`),
	)
	req.SetPathValue("name", "performance")
	rec := httptest.NewRecorder()

	s.apiSyncTrigger(rec, req)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "92") {
		t.Fatalf("93-day single-day range status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestParseEgressIPRejectsNonIPResponse(t *testing.T) {
	if _, err := parseEgressIP("not an ip"); err == nil {
		t.Fatal("non-IP egress response was accepted")
	}
	got, err := parseEgressIP(" 203.0.113.9 ")
	if err != nil || got != "203.0.113.9" {
		t.Fatalf("parseEgressIP valid response = %q, %v", got, err)
	}
}

func TestAccountStoresRejectsUnknownAccount(t *testing.T) {
	s := &Server{cfg: &config.Config{Accounts: []config.Account{{ID: "sc_us"}}}}
	req := httptest.NewRequest(http.MethodGet, "/api/accounts/missing/stores", nil)
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()

	s.apiAccountStores(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown account stores status=%d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestAccountStoreSyncRejectsAccountWithoutStoreSource(t *testing.T) {
	s := &Server{
		cfg: &config.Config{Accounts: []config.Account{{ID: "sc_us"}}},
		reg: worker.NewRegistry(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/accounts/sc_us/stores/sync", nil)
	req.SetPathValue("id", "sc_us")
	rec := httptest.NewRecorder()

	s.apiAccountStoreSync(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("missing source status=%d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestFindVCStoreForProfileRequiresMatchingVCStore(t *testing.T) {
	stores := []db.StoreSummary{
		{SID: "sc-1", StoreType: "SC"},
		{SID: "vc-1", StoreType: "VC"},
	}

	store, status, err := findVCStoreForProfile(stores, "vc-1")
	if err != nil || status != http.StatusOK || store.SID != "vc-1" {
		t.Fatalf("VC store result = %#v, status=%d, err=%v", store, status, err)
	}
	if _, status, err = findVCStoreForProfile(stores, "sc-1"); err == nil || status != http.StatusBadRequest {
		t.Fatalf("SC store status=%d, err=%v; want 400", status, err)
	}
	if _, status, err = findVCStoreForProfile(stores, "missing"); err == nil || status != http.StatusNotFound {
		t.Fatalf("unknown store status=%d, err=%v; want 404", status, err)
	}
}

func TestSaveVCStoreProfileRejectsMissingProfileField(t *testing.T) {
	s := &Server{cfg: &config.Config{Accounts: []config.Account{{ID: "vc_account"}}}}
	req := httptest.NewRequest(http.MethodPut, "/api/accounts/vc_account/stores/vc-1/vc-profile", strings.NewReader(`{}`))
	req.SetPathValue("id", "vc_account")
	req.SetPathValue("sid", "vc-1")
	rec := httptest.NewRecorder()

	s.apiSaveVCStoreProfile(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing profile_id status=%d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestUnpublishedVersionCannotAuthorizeBearerReads(t *testing.T) {
	cfg := validDatasetProjectTestConfig()
	cfg.DatasetAPI.Tokens = []config.DatasetToken{{
		ID: "draft-reader", ProjectID: "draft-reader", TokenHash: datasetapi.HashToken("draft-token"),
		DatasetScopes: []string{"return-reason-detail-v2"}, StoreScopes: []string{"store-a"},
	}}
	s := New(cfg, nil, nil, nil, "", Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}, nil, nil, nil, "")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/return-reason-detail-v2/snapshot", strings.NewReader(`{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01"}`))
	req.Header.Set("Authorization", "Bearer draft-token")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unpublished draft read status=%d body=%s", rec.Code, rec.Body.String())
	}
}
