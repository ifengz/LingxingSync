package config

import (
	"os"
	"path/filepath"
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
