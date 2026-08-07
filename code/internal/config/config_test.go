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
