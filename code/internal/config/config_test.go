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
