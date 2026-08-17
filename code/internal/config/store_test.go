package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
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
