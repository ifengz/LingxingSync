package server

import (
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
