package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAddsDefaultConnectionCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := `database:
  host: 127.0.0.1
  user: test
  db: lingsync
accounts:
  - id: sc_us
    name: US
    app_key: key
    app_secret: secret
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	check := cfg.Accounts[0].ConnectionCheck
	if check.Cron != "*/20 * * * *" || !check.Enabled {
		t.Fatalf("default connection check = %+v, want enabled every 20 minutes", check)
	}
}

func TestClassifyChangeTreatsConnectionCheckAsHot(t *testing.T) {
	oldCfg := &Config{Accounts: []Account{{
		ID: "sc_us", Name: "US", AppKey: "key", AppSecret: "secret",
		ConnectionCheck: ConnectionCheck{Cron: "*/20 * * * *", Enabled: true},
	}}}
	newCfg := deepCopy(oldCfg)
	newCfg.Accounts[0].ConnectionCheck.Enabled = false

	if got := ClassifyChange(oldCfg, newCfg); got != ChangeHot {
		t.Fatalf("connection check change = %v, want ChangeHot", got)
	}
}

func TestLoadValidatesEnabledReportExport(t *testing.T) {
	valid := `database:
  host: 127.0.0.1
  user: test
  db: lingsync
accounts:
  - id: sc_us
    app_key: key
    app_secret: secret
report_exports:
  - type: fba_customer_returns
    enabled: true
    account: sc_us
    seller_id: SELLER-1
    store_id: STORE-1
    region: na
    marketplace_ids: [ATVPDKIKX0DER]
    cron: "0 4 * * *"
    window_days: 3
`
	tests := map[string]string{
		"valid":                 valid,
		"missing seller":        strings.Replace(valid, "    seller_id: SELLER-1\n", "", 1),
		"blank seller":          strings.Replace(valid, "    seller_id: SELLER-1", `    seller_id: ""`, 1),
		"spaced seller":         strings.Replace(valid, "    seller_id: SELLER-1", `    seller_id: " SELLER-1"`, 1),
		"long seller":           strings.Replace(valid, "    seller_id: SELLER-1", "    seller_id: "+strings.Repeat("S", 65), 1),
		"missing store":         strings.Replace(valid, "    store_id: STORE-1\n", "", 1),
		"blank store":           strings.Replace(valid, "    store_id: STORE-1", `    store_id: ""`, 1),
		"spaced store":          strings.Replace(valid, "    store_id: STORE-1", `    store_id: " STORE-1"`, 1),
		"unknown account":       strings.Replace(valid, "    account: sc_us", "    account: missing", 1),
		"bad region":            strings.Replace(valid, "    region: na", "    region: us", 1),
		"missing marketplace":   strings.Replace(valid, "    marketplace_ids: [ATVPDKIKX0DER]", "    marketplace_ids: []", 1),
		"blank marketplace":     strings.Replace(valid, "    marketplace_ids: [ATVPDKIKX0DER]", `    marketplace_ids: [""]`, 1),
		"spaced marketplace":    strings.Replace(valid, "    marketplace_ids: [ATVPDKIKX0DER]", `    marketplace_ids: [" ATVPDKIKX0DER"]`, 1),
		"long marketplace":      strings.Replace(valid, "    marketplace_ids: [ATVPDKIKX0DER]", "    marketplace_ids: ["+strings.Repeat("M", 65)+"]", 1),
		"duplicate marketplace": strings.Replace(valid, "    marketplace_ids: [ATVPDKIKX0DER]", "    marketplace_ids: [ATVPDKIKX0DER, ATVPDKIKX0DER]", 1),
		"bad cron":              strings.Replace(valid, `    cron: "0 4 * * *"`, `    cron: "bad"`, 1),
		"missing window":        strings.Replace(valid, "    window_days: 3\n", "", 1),
		"too many days":         strings.Replace(valid, "    window_days: 3", "    window_days: 32", 1),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if name == "valid" && err != nil {
				t.Fatalf("valid report export rejected: %v", err)
			}
			if name != "valid" && err == nil {
				t.Fatal("invalid enabled report export accepted")
			}
		})
	}
}

func TestLoadValidatesEnabledCustomerShipmentSalesReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := `database:
  host: 127.0.0.1
  user: test
  db: lingsync
accounts:
  - id: sc_us
    app_key: key
    app_secret: secret
report_exports:
  - type: fba_customer_shipment_sales
    enabled: true
    account: sc_us
    seller_id: SELLER-1
    store_id: STORE-1
    region: na
    marketplace_ids: [ATVPDKIKX0DER]
    cron: "0 4 * * *"
    window_days: 1
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("customer shipment sales report rejected: %v", err)
	}
}

func TestLoadRequiresEstimatedFeesDailyScheduleAndSeventyTwoHourWindow(t *testing.T) {
	base := `database:
  host: 127.0.0.1
  user: test
  db: lingsync
accounts:
  - id: sc_us
    app_key: key
    app_secret: secret
report_exports:
  - type: fba_estimated_fees
    enabled: true
    account: sc_us
    seller_id: SELLER-1
    store_id: STORE-1
    region: na
    marketplace_ids: [ATVPDKIKX0DER]
    cron: "0 4 * * *"
    window_days: 3
`
	tests := map[string]struct {
		raw  string
		want bool
	}{
		"daily 72 hours": {raw: base, want: true},
		"short window":   {raw: strings.Replace(base, "window_days: 3", "window_days: 2", 1)},
		"repeats daily":  {raw: strings.Replace(base, `cron: "0 4 * * *"`, `cron: "*/30 * * * *"`, 1)},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(test.raw), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if test.want && err != nil {
				t.Fatalf("valid estimated fees config rejected: %v", err)
			}
			if !test.want && err == nil {
				t.Fatal("invalid estimated fees config accepted")
			}
		})
	}
}

func TestLoadAllowsDisabledEmptyReportExport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := `database:
  host: 127.0.0.1
  user: test
  db: lingsync
accounts:
  - id: sc_us
    app_key: key
    app_secret: secret
report_exports:
  - type: fba_customer_returns
    enabled: false
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("disabled report export should not require runtime fields: %v", err)
	}
}

func TestLoadRejectsDuplicateReportExportScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := `
server: {port: 7799, secret: test}
database: {host: 127.0.0.1, port: 3306, user: test, password: test, db: test}
accounts:
  - {id: sc_us, app_key: key, app_secret: secret}
report_exports:
  - {type: fba_customer_returns, enabled: false, account: sc_us, store_id: store-1}
  - {type: fba_customer_returns, enabled: false, account: sc_us, store_id: store-1}
endpoints: []
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("duplicate report scope err=%v", err)
	}
}

func TestLoadRejectsUnknownDisabledReportExportType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := `database:
  host: 127.0.0.1
  user: test
  db: lingsync
accounts:
  - id: sc_us
    app_key: key
    app_secret: secret
report_exports:
  - type: unknown_report
    enabled: false
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("unknown disabled report type was accepted")
	}
}

func TestClassifyChangeTreatsReportExportsAsHot(t *testing.T) {
	oldCfg := &Config{}
	newCfg := &Config{ReportExports: []ReportExport{{Type: ReportExportCustomerReturns, Enabled: true}}}
	if got := ClassifyChange(oldCfg, newCfg); got != ChangeHot {
		t.Fatalf("report export change = %v, want ChangeHot", got)
	}
}

func TestClassifyChangeKeepsRestartPriorityOverReportExportChange(t *testing.T) {
	oldCfg := &Config{Database: Database{Host: "old"}}
	newCfg := &Config{
		Database:      Database{Host: "new"},
		ReportExports: []ReportExport{{Type: ReportExportCustomerReturns, Enabled: true}},
	}
	if got := ClassifyChange(oldCfg, newCfg); got != ChangeRestart {
		t.Fatalf("database plus report export change = %v, want ChangeRestart", got)
	}
}

func TestClassifyChangeDoesNotHideEndpointRestartBehindReportExportChange(t *testing.T) {
	oldCfg := &Config{Accounts: []Account{{ID: "sc_us"}}, Endpoints: []Endpoint{{Name: "sales", Path: "/old"}}}
	newCfg := &Config{
		Accounts:      []Account{{ID: "sc_us"}},
		Endpoints:     []Endpoint{{Name: "sales", Path: "/new"}},
		ReportExports: []ReportExport{{Type: ReportExportCustomerReturns, Enabled: true}},
	}
	if got := ClassifyChange(oldCfg, newCfg); got != ChangeRestart {
		t.Fatalf("endpoint path plus report export change = %v, want ChangeRestart", got)
	}
}

func TestClassifyChangeRequiresRestartForDatasetAPI(t *testing.T) {
	oldCfg := &Config{DatasetAPI: DatasetAPIConfig{
		CursorSecret:   "old-cursor-secret",
		FieldAllowlist: []string{"sales_units"},
	}}
	newCfg := deepCopy(oldCfg)
	newCfg.DatasetAPI.CursorSecret = "new-cursor-secret"

	if got := ClassifyChange(oldCfg, newCfg); got != ChangeRestart {
		t.Fatalf("dataset API change = %v, want ChangeRestart", got)
	}
}

func TestLoadRejectsDuplicateLimiterKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := `database:
  host: 127.0.0.1
  user: test
  db: lingsync
accounts:
  - id: sc_us
    name: US
    app_key: key
    app_secret: secret
endpoints:
  - name: orders_a
    account: sc_us
    path: /orders
    method: POST
    table: ls_orders_a
    record_id_fields: [order_id]
    rate: { bucket: 1, interval_ms: 1000, dimension: account+path }
    cron: "0 * * * *"
  - name: orders_b
    account: sc_us
    path: /orders
    method: POST
    table: ls_orders_b
    record_id_fields: [order_id]
    rate: { bucket: 1, interval_ms: 1000, dimension: account+path }
    cron: "0 * * * *"
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("duplicate quota_group+path was accepted")
	}
}

func TestLoadRejectsInvalidEndpointCronBeforeSchedulerRebuild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := `database:
  host: 127.0.0.1
  user: test
  db: lingsync
accounts:
  - id: sc_us
    app_key: key
    app_secret: secret
endpoints:
  - name: orders
    account: sc_us
    path: /orders
    method: POST
    table: ls_orders
    record_id_fields: [order_id]
    rate: { bucket: 1, interval_ms: 1000, dimension: account+path }
    cron: "not-a-cron"
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "cron") {
		t.Fatalf("invalid endpoint cron error = %v", err)
	}
}

func TestLoadRejectsInvalidConnectionCheckCronBeforeSchedulerRebuild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := `database:
  host: 127.0.0.1
  user: test
  db: lingsync
accounts:
  - id: sc_us
    app_key: key
    app_secret: secret
    connection_check:
      enabled: true
      cron: "not-a-cron"
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "cron") {
		t.Fatalf("invalid connection check cron error = %v", err)
	}
}

func TestValidateAllowsSeparatedSalesReportTypeVariants(t *testing.T) {
	rate := Rate{Bucket: 5, IntervalMs: 200, MultiIntervalMs: 1000, Dimension: "account+path"}
	base := Endpoint{
		Name:           "sc_sales_quantity",
		Account:        "sc_us",
		Path:           "/erp/sc/data/sales_report/asinDailyLists",
		Method:         "POST",
		Table:          "ls_sc_sales_report",
		RecordIDFields: []string{"sid", "r_date", "asin"},
		Rate:           rate,
		Cron:           "*/30 * * * *",
		ExtraParams:    map[string]any{"type": 2, "asin_type": 1},
		DateField:      "event_date",
		DateOffsetDays: 2,
		IterateByStore: true,
		StoreParamName: "sid",
		StoreType:      "SC",
	}
	revenue := base
	revenue.Name = "sc_sales_revenue"
	revenue.Table = "ls_sc_sales_revenue"
	revenue.ExtraParams = map[string]any{"type": 1, "asin_type": 1}

	cfg := &Config{
		Database:  Database{Host: "h", User: "u", DB: "d"},
		Accounts:  []Account{{ID: "sc_us", AppKey: "key", AppSecret: "secret"}},
		Endpoints: []Endpoint{base, revenue},
	}

	if err := cfg.validate(); err != nil {
		t.Fatalf("separated type=1/type=2 raw lanes should share their upstream limiter: %v", err)
	}

	t.Run("same raw table still conflicts", func(t *testing.T) {
		duplicateTarget := revenue
		duplicateTarget.Table = base.Table
		cfg.Endpoints = []Endpoint{base, duplicateTarget}
		if err := cfg.validate(); err == nil {
			t.Fatal("same-path variants writing one raw table would overwrite metric meaning")
		}
	})

	t.Run("same fixed params still conflict", func(t *testing.T) {
		duplicateRequest := revenue
		duplicateRequest.ExtraParams = map[string]any{"type": 2, "asin_type": 1}
		cfg.Endpoints = []Endpoint{base, duplicateRequest}
		if err := cfg.validate(); err == nil {
			t.Fatal("duplicate fixed request params would create a redundant raw lane")
		}
	})

	t.Run("multiple fixed param differences still conflict", func(t *testing.T) {
		driftedRequest := revenue
		driftedRequest.ExtraParams = map[string]any{"type": 1, "asin_type": 2}
		cfg.Endpoints = []Endpoint{base, driftedRequest}
		if err := cfg.validate(); err == nil {
			t.Fatal("multiple request-shape changes must not be hidden as one metric variant")
		}
	})
}

func TestValidateRejectsCaseInsensitiveDuplicateAccountID(t *testing.T) {
	// sc_us 与 Sc_us 归一化后同为 sc_us，视为撞名，必须 fail-loud。
	cfg := &Config{
		Database: Database{Host: "h", User: "u", DB: "d"},
		Accounts: []Account{
			{ID: "sc_us", Name: "self", AppKey: "k1", AppSecret: "s1"},
			{ID: "Sc_us", Name: "aff", AppKey: "k2", AppSecret: "s2"},
		},
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("仅大小写不同的两个账号 ID 应被判撞名并报错")
	}
}

func TestValidateRejectsIllegalSlugAccountID(t *testing.T) {
	cases := []string{"has space", "_leading", "trailing_", "-lead", "中文", "toolong_aaaaaaaaaaaaaaaaaaaaaaaaaaaa", "a@b"}
	for _, id := range cases {
		cfg := &Config{
			Database: Database{Host: "h", User: "u", DB: "d"},
			Accounts: []Account{{ID: id, Name: "n", AppKey: "k", AppSecret: "s"}},
		}
		if err := cfg.validate(); err == nil {
			t.Fatalf("非法账号 ID %q 应被 slug 校验拒绝", id)
		}
	}
}

func TestValidateAcceptsValidSlugAccountID(t *testing.T) {
	for _, id := range []string{"sc_us_1", "sc_us_2", "vc-de", "a", "A9", "self_us"} {
		cfg := &Config{
			Database: Database{Host: "h", User: "u", DB: "d"},
			Accounts: []Account{{ID: id, Name: "n", AppKey: "k", AppSecret: "s"}},
		}
		if err := cfg.validate(); err != nil {
			t.Fatalf("合法账号 ID %q 不应被拒: %v", id, err)
		}
	}
}

func TestValidateDatasetAPIAcceptsMultipleTokensForOneProject(t *testing.T) {
	cfg := DatasetAPIConfig{
		CursorSecret:   "cursor-secret-for-tests",
		FieldAllowlist: []string{"sales_units"},
		Tokens: []DatasetToken{
			{ID: "token-a", ProjectID: "project-a", TokenHash: strings.Repeat("a", 64), Fields: []string{"sales_units"}},
			{ID: "token-b", ProjectID: "project-a", TokenHash: strings.Repeat("b", 64), Fields: []string{"sales_units"}},
		},
	}
	if err := validateDatasetAPI(cfg); err != nil {
		t.Fatalf("same project with distinct token ids should be valid: %v", err)
	}
}

func TestValidateDatasetAPIRejectsUppercaseTokenHash(t *testing.T) {
	cfg := DatasetAPIConfig{
		CursorSecret:   "cursor-secret-for-tests",
		FieldAllowlist: []string{"sales_units"},
		Tokens: []DatasetToken{{
			ID: "token-a", TokenHash: strings.Repeat("A", 64), Fields: []string{"sales_units"},
		}},
	}
	if err := validateDatasetAPI(cfg); err == nil {
		t.Fatal("uppercase token hash was accepted but runtime authentication compares lowercase SHA-256")
	}
}

func TestValidateStoreType(t *testing.T) {
	mk := func(storeType string) *Config {
		return &Config{
			Database: Database{Host: "h", User: "u", DB: "d"},
			Accounts: []Account{{ID: "sc_us_1", Name: "n", AppKey: "k", AppSecret: "s"}},
			Endpoints: []Endpoint{{
				Name: "inv", Account: "sc_us_1", Path: "/inv", Method: "GET", Table: "ls_fba_inventory",
				RecordIDFields: []string{"sid"}, Cron: "0 * * * *",
				Rate:      Rate{Bucket: 1, IntervalMs: 1000, Dimension: "account+path"},
				StoreType: storeType,
			}},
		}
	}
	for _, ok := range []string{"", "SC", "VC"} {
		if err := mk(ok).validate(); err != nil {
			t.Fatalf("store_type=%q 应合法: %v", ok, err)
		}
	}
	for _, bad := range []string{"sc", "vc", "ALL", "x"} {
		if err := mk(bad).validate(); err == nil {
			t.Fatalf("store_type=%q 应被拒", bad)
		}
	}
}

func TestValidateResponseShape(t *testing.T) {
	mk := func(shape string) *Config {
		return &Config{
			Database: Database{Host: "h", User: "u", DB: "d"},
			Accounts: []Account{{ID: "sc_us_1", Name: "n", AppKey: "k", AppSecret: "s"}},
			Endpoints: []Endpoint{{
				Name: "product_info", Account: "sc_us_1", Path: "/productInfo", Method: "POST", Table: "ls_product_info",
				RecordIDFields: []string{"sku"}, Cron: "0 * * * *",
				Rate: Rate{Bucket: 1, IntervalMs: 1000, Dimension: "account+path"}, ResponseShape: shape,
			}},
		}
	}
	for _, shape := range []string{"", "list", "object"} {
		if err := mk(shape).validate(); err != nil {
			t.Fatalf("response_shape=%q 应合法: %v", shape, err)
		}
	}
	if err := mk("single").validate(); err == nil {
		t.Fatal("response_shape=single 应被拒绝")
	}
}

func TestValidateForceInjectParams(t *testing.T) {
	cfg := &Config{
		Database: Database{Host: "h", User: "u", DB: "d"},
		Accounts: []Account{{ID: "sc_us_1", Name: "n", AppKey: "k", AppSecret: "s"}},
		Endpoints: []Endpoint{{
			Name: "vc_margin", Account: "sc_us_1", Path: "/margin", Method: "POST", Table: "ls_margin",
			RecordIDFields: []string{"sid", "asin", "date"}, Cron: "0 * * * *",
			Rate:              Rate{Bucket: 1, IntervalMs: 1000, Dimension: "account+path"},
			ForceInjectParams: []string{"sid"},
		}},
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("valid force_inject_params rejected: %v", err)
	}
	cfg.Endpoints[0].ForceInjectParams = []string{""}
	if err := cfg.validate(); err == nil {
		t.Fatal("empty force_inject_params name accepted")
	}
}

func TestValidateRequestHeaders(t *testing.T) {
	mk := func(headers map[string]string) *Config {
		return &Config{
			Database: Database{Host: "h", User: "u", DB: "d"},
			Accounts: []Account{{ID: "sc_us_1", Name: "n", AppKey: "k", AppSecret: "s"}},
			Endpoints: []Endpoint{{
				Name: "sp_report", Account: "sc_us_1", Path: "/sp", Method: "POST", Table: "ls_sp",
				RecordIDFields: []string{"id"}, Cron: "0 * * * *",
				Rate: Rate{Bucket: 1, IntervalMs: 1000, Dimension: "account+path"}, RequestHeaders: headers,
			}},
		}
	}
	if err := mk(map[string]string{"X-API-VERSION": "2"}).validate(); err != nil {
		t.Fatalf("valid request header rejected: %v", err)
	}
	for _, headers := range []map[string]string{{"": "2"}, {"X-Test": ""}, {"Authorization": "secret"}, {"Content-Type": "text/plain"}} {
		if err := mk(headers).validate(); err == nil {
			t.Fatalf("unsafe headers accepted: %#v", headers)
		}
	}
}

func TestValidateAdAccountIteration(t *testing.T) {
	mk := func(iterateByStore, iterateByAdAccount bool, accountType string) *Config {
		return &Config{
			Database: Database{Host: "h", User: "u", DB: "d"},
			Accounts: []Account{{ID: "sc_us_1", Name: "n", AppKey: "k", AppSecret: "s"}},
			Endpoints: []Endpoint{{
				Name: "sp_report", Account: "sc_us_1", Path: "/sp", Method: "POST", Table: "ls_sp",
				RecordIDFields: []string{"id"}, Cron: "0 * * * *",
				Rate:           Rate{Bucket: 1, IntervalMs: 1000, Dimension: "account+path"},
				IterateByStore: iterateByStore, IterateByAdAccount: iterateByAdAccount, AdAccountType: accountType,
			}},
		}
	}
	if err := mk(false, true, "seller").validate(); err != nil {
		t.Fatalf("seller ad iteration rejected: %v", err)
	}
	if err := mk(false, true, "").validate(); err == nil {
		t.Fatal("ad iteration without type accepted")
	}
	if err := mk(false, true, "vendor").validate(); err == nil {
		t.Fatal("未验证的 vendor 广告账号迭代被接受")
	}
	if err := mk(true, true, "seller").validate(); err == nil {
		t.Fatal("store and ad-account iteration together accepted")
	}
}

func TestValidateVCPOOrderIterationContract(t *testing.T) {
	endpoint := Endpoint{
		Name: "vc_po_details", Account: "sc_us_1", Path: "/basicOpen/platformOrder/vcOrderPo/detail",
		Method: "POST", Table: "ls_vc_po_details", RecordIDFields: []string{"vc_store_id", "local_po_number"},
		ResponseShape: "object", Cron: "*/30 * * * *", WindowDays: 7,
		Rate:              Rate{Bucket: 1, IntervalMs: 1000, Dimension: "account+path"},
		IterateByVCOrders: true, ForceInjectParams: []string{"vc_store_id", "local_po_number"},
	}
	mk := func(ep Endpoint) *Config {
		return &Config{
			Database:  Database{Host: "h", User: "u", DB: "d"},
			Accounts:  []Account{{ID: "sc_us_1", AppKey: "k", AppSecret: "s"}},
			Endpoints: []Endpoint{ep},
		}
	}
	if err := mk(endpoint).validate(); err != nil {
		t.Fatalf("valid VC PO detail iteration rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Endpoint){
		"GET":                 func(ep *Endpoint) { ep.Method = "GET" },
		"list response":       func(ep *Endpoint) { ep.ResponseShape = "list" },
		"missing store force": func(ep *Endpoint) { ep.ForceInjectParams = []string{"local_po_number"} },
		"extra body params":   func(ep *Endpoint) { ep.ExtraParams = map[string]any{"sid": "x"} },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := endpoint
			mutate(&candidate)
			if err := mk(candidate).validate(); err == nil {
				t.Fatal("invalid VC PO detail iteration contract was accepted")
			}
		})
	}
}

func TestClassifyChangeTreatsSingleDayFieldsAsHot(t *testing.T) {
	oldCfg := &Config{Endpoints: []Endpoint{{Name: "performance", WindowDays: 2}}}
	newCfg := &Config{Endpoints: []Endpoint{{Name: "performance", WindowDays: 2, SingleDayWindow: true, RowDateField: "business_date"}}}
	if got := ClassifyChange(oldCfg, newCfg); got != ChangeHot {
		t.Fatalf("single-day fields change = %v, want hot reload", got)
	}
}

func TestValidateSingleDayRowDateFieldConflicts(t *testing.T) {
	endpoint := Endpoint{
		Name: "performance", Account: "sc_us_1", Path: "/performance", Method: "POST", Table: "ls_performance",
		RecordIDFields: []string{"sid", "asin", "business_date"}, Cron: "0 5 * * *", WindowDays: 7,
		Rate:            Rate{Bucket: 1, IntervalMs: 1000, Dimension: "account+path"},
		SingleDayWindow: true, RowDateField: "business_date",
	}
	validate := func(ep Endpoint) error {
		return (&Config{
			Database:  Database{Host: "h", User: "u", DB: "d"},
			Accounts:  []Account{{ID: "sc_us_1", AppKey: "k", AppSecret: "s"}},
			Endpoints: []Endpoint{ep},
		}).validate()
	}
	if err := validate(endpoint); err != nil {
		t.Fatalf("valid single-day row_date_field rejected: %v", err)
	}

	tests := map[string]func(*Endpoint){
		"extra param": func(ep *Endpoint) { ep.ExtraParams = map[string]any{"business_date": "2026-08-01"} },
		"start field": func(ep *Endpoint) {
			ep.WindowStartField = "from"
			ep.RowDateField = "from"
		},
		"end field": func(ep *Endpoint) {
			ep.WindowEndField = "to"
			ep.RowDateField = "to"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := endpoint
			mutate(&candidate)
			if err := validate(candidate); err == nil {
				t.Fatal("row_date_field conflict was accepted")
			}
		})
	}
}

func TestResponseShapeOrDefault(t *testing.T) {
	if got := (Endpoint{}).ResponseShapeOrDefault(); got != "list" {
		t.Fatalf("空 response_shape 默认值 = %q, want list", got)
	}
	if got := (Endpoint{ResponseShape: " object "}).ResponseShapeOrDefault(); got != "object" {
		t.Fatalf("response_shape 归一化值 = %q, want object", got)
	}
}

func TestFindAccountCaseInsensitive(t *testing.T) {
	cfg := &Config{Accounts: []Account{{ID: "sc_us_1", Name: "self", AppKey: "k", AppSecret: "s"}}}
	for _, q := range []string{"sc_us_1", "SC_US_1", "Sc_Us_1", " sc_us_1 "} {
		if a := cfg.FindAccount(q); a == nil || a.ID != "sc_us_1" {
			t.Fatalf("FindAccount(%q) 应命中 sc_us_1，got %v", q, a)
		}
	}
	if cfg.FindAccount("sc_us_2") != nil {
		t.Fatal("FindAccount 不存在的 ID 应返回 nil")
	}
}

func TestNormIDAndValidAccountID(t *testing.T) {
	if NormID(" Sc_US ") != "sc_us" {
		t.Fatalf("NormID 应去空白并转小写，got %q", NormID(" Sc_US "))
	}
	if !ValidAccountID("sc_us_2") || ValidAccountID("_bad") || ValidAccountID("bad-") {
		t.Fatal("ValidAccountID slug 判定不符预期")
	}
}

func TestConflictingLimiterKey(t *testing.T) {
	// 两个账号共享同一 quota_group，故同 path 会落到同一个限流桶。
	cfg := &Config{
		Accounts: []Account{
			{ID: "key1", QuotaGroup: "sc_us", AppKey: "k1", AppSecret: "s1"},
			{ID: "key2", QuotaGroup: "sc_us", AppKey: "k2", AppSecret: "s2"},
		},
		Endpoints: []Endpoint{
			{Name: "orders", Account: "key1", Path: "/orders"},
		},
	}

	// 同分组同 path（即便账号不同）→ 冲突，命中已有 orders。
	if owner, dup := cfg.ConflictingLimiterKey(Endpoint{Name: "orders_dup", Account: "key2", Path: "/orders"}); !dup || owner != "orders" {
		t.Fatalf("shared quota_group + same path: got (%q,%v), want (\"orders\",true)", owner, dup)
	}
	// 同账号不同 path → 不冲突。
	if _, dup := cfg.ConflictingLimiterKey(Endpoint{Name: "inv", Account: "key1", Path: "/inventory"}); dup {
		t.Fatal("different path should not conflict")
	}
	// 同名（更新自身）→ 豁免，不把自己判成冲突。
	if _, dup := cfg.ConflictingLimiterKey(Endpoint{Name: "orders", Account: "key1", Path: "/orders"}); dup {
		t.Fatal("same-name endpoint (update) must be exempt from self-conflict")
	}
}

func TestConflictingLimiterKeyAllowsSeparatedFixedParamVariant(t *testing.T) {
	rate := Rate{Bucket: 5, IntervalMs: 200, MultiIntervalMs: 1000, Dimension: "account+path"}
	quantity := Endpoint{
		Name:        "sc_sales_quantity",
		Account:     "sc_us",
		Path:        "/erp/sc/data/sales_report/asinDailyLists",
		Method:      "POST",
		Table:       "ls_sc_sales_report",
		Rate:        rate,
		ExtraParams: map[string]any{"type": 2, "asin_type": 1},
	}
	cfg := &Config{
		Accounts:  []Account{{ID: "sc_us", AppKey: "key", AppSecret: "secret"}},
		Endpoints: []Endpoint{quantity},
	}
	revenue := quantity
	revenue.Name = "sc_sales_revenue"
	revenue.Table = "ls_sc_sales_revenue"
	revenue.ExtraParams = map[string]any{"type": 1, "asin_type": 1}

	if owner, conflict := cfg.ConflictingLimiterKey(revenue); conflict {
		t.Fatalf("separated type variant reported conflict with %q", owner)
	}
}
