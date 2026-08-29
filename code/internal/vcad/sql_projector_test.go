package vcad

import (
	"os"
	"strings"
	"testing"
)

func TestVCAwardMigrationDefinesRawAndCanonicalTables(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/073_add_vc_ad_tables.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, table := range []string{"ls_ad_vendor_accounts", "ls_ad_vc_sp_product", "ls_ad_vc_sd_product", "ls_ad_vc_hsa_product", "vc_ad_daily"} {
		if !strings.Contains(sql, "create table if not exists "+table) {
			t.Fatalf("migration missing table %s", table)
		}
	}
	for _, column := range []string{"profile_id", "report_date", "impressions", "clicks", "cost", "orders", "sales"} {
		if !strings.Contains(sql, column) {
			t.Fatalf("migration missing VC ad field %s", column)
		}
	}
}
