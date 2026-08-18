package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestConfigStoreSaveProtectsPlaintextDatasetToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("old backup"), 0644); err != nil {
		t.Fatal(err)
	}
	rawToken := "internal-reader-token"
	sum := sha256.Sum256([]byte(rawToken))
	cfg := &Config{
		Database: Database{Host: "h", User: "u", DB: "d"},
		Accounts: []Account{{ID: "test", AppKey: "key", AppSecret: "secret"}},
		DatasetAPI: DatasetAPIConfig{
			CursorSecret:   "1234567890abcdef",
			FieldAllowlist: []string{"sales_units"},
			Tokens: []DatasetToken{{
				ID: "tok_test", ProjectID: "project", Token: rawToken, TokenHash: hex.EncodeToString(sum[:]),
				DatasetScopes: []string{"listing-daily-v1"}, StoreScopes: []string{"12534"}, Fields: []string{"sales_units"},
			}},
		},
	}
	store := NewStore(path, cfg)
	if err := store.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	for _, filename := range []string{path, path + ".bak"} {
		info, err := os.Stat(filename)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Fatalf("%s permission=%#o, want 0600", filename, got)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "token: internal-reader-token") {
		t.Fatal("saved config did not persist plaintext dataset token")
	}
}

func TestConfigStoreMaskHidesPlaintextDatasetToken(t *testing.T) {
	store := NewStore("", &Config{DatasetAPI: DatasetAPIConfig{Tokens: []DatasetToken{{Token: "internal-reader-token"}}}})
	masked := store.Mask()
	if masked.DatasetAPI.Tokens[0].Token == "internal-reader-token" {
		t.Fatal("masked config exposed plaintext dataset token")
	}
	if store.Current().DatasetAPI.Tokens[0].Token != "internal-reader-token" {
		t.Fatal("mask changed stored plaintext token")
	}
}

func TestConfigStoreSerializesConcurrentSaves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	base := &Config{
		Server:    Server{Port: 7799},
		Database:  Database{Host: "h", Port: 3306, User: "u", DB: "d", MaxOpen: 20, MaxIdle: 5, ConnTimeoutSec: 10},
		Accounts:  []Account{{ID: "test", AppKey: "key", AppSecret: "secret", ConnectionCheck: DefaultConnectionCheck()}},
		Retention: Retention{TaskLogsDays: 90, TasksDays: 365, CleanupCron: "0 3 * * *"},
	}
	store := NewStore(path, base)
	if err := store.Save(base); err != nil {
		t.Fatalf("save initial config: %v", err)
	}

	const writers = 24
	start := make(chan struct{})
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		cfg := store.Current()
		cfg.Accounts[0].Name = fmt.Sprintf("writer-%d", i)
		wg.Add(1)
		go func(cfg *Config) {
			defer wg.Done()
			<-start
			errs <- store.Save(cfg)
		}(cfg)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent save failed: %v", err)
		}
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("saved config is unreadable: %v", err)
	}
}

func TestConfigStoreRejectsStaleSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	base := &Config{
		Server:    Server{Port: 7799},
		Database:  Database{Host: "h", Port: 3306, User: "u", DB: "d", MaxOpen: 20, MaxIdle: 5, ConnTimeoutSec: 10},
		Accounts:  []Account{{ID: "test", AppKey: "key", AppSecret: "secret", ConnectionCheck: DefaultConnectionCheck()}},
		Retention: Retention{TaskLogsDays: 90, TasksDays: 365, CleanupCron: "0 3 * * *"},
	}
	store := NewStore(path, base)
	if err := store.Save(base); err != nil {
		t.Fatalf("save initial config: %v", err)
	}

	expected := store.Current()
	winner := store.Current()
	winner.Accounts[0].Name = "winner"
	if err := store.SaveIfCurrent(expected, winner); err != nil {
		t.Fatalf("save current snapshot: %v", err)
	}

	stale := deepCopy(expected)
	stale.Accounts[0].Name = "stale"
	err := store.SaveIfCurrent(expected, stale)
	if !errors.Is(err, ErrConfigChanged) {
		t.Fatalf("stale snapshot error=%v, want ErrConfigChanged", err)
	}
	if got := store.Current().Accounts[0].Name; got != "winner" {
		t.Fatalf("stale snapshot overwrote latest config: got %q", got)
	}
}
