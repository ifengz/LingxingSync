package server

import (
	"os"
	"strings"
	"testing"
)

func TestManualSyncTemplateKeepsScopeAndActionSlotsStable(t *testing.T) {
	raw, err := os.ReadFile("../../web/templates/sync_manage.html")
	if err != nil {
		t.Fatalf("读 sync_manage.html: %v", err)
	}
	source := string(raw)
	for _, required := range []string{`data-manual-workbench class="flex flex-col`, "data-sync-date-slot", "data-sync-store-slot", "data-sync-action-bar", "fixed bottom-3", "lg:sticky lg:bottom-0", `:disabled="manualSubmitDisabled"`} {
		if !strings.Contains(source, required) {
			t.Fatalf("手动同步固定工作台缺少 %q", required)
		}
	}
	for _, unstable := range []string{`x-show="selectedCount > 0" x-cloak class="border-b`, `x-show="showStoreGrid" x-cloak class="border-b`} {
		if strings.Contains(source, unstable) {
			t.Fatalf("日期或店铺根区域不应随选择消失：%q", unstable)
		}
	}
	if strings.Contains(source, `x-model="storesByAccount[acc].query"`) {
		t.Fatal("店铺状态尚未初始化时，搜索框不得直接读取 query")
	}
	if !strings.Contains(source, `:value="storesByAccount[acc] ? storesByAccount[acc].query : ''"`) {
		t.Fatal("店铺搜索框必须使用可空状态绑定")
	}
}

func TestSyncTemplateUsesFourPeerTabsAndKeepsReportsOutOfSchedule(t *testing.T) {
	raw, err := os.ReadFile("../../web/templates/sync_manage.html")
	if err != nil {
		t.Fatalf("读 sync_manage.html: %v", err)
	}
	source := string(raw)
	for _, required := range []string{`switchTab('manual')`, `switchTab('schedule')`, `switchTab('reports')`, `switchTab('add')`, `x-show="tab==='reports'"`, "报表检验"} {
		if !strings.Contains(source, required) {
			t.Fatalf("同步页缺少四切卡合同 %q", required)
		}
	}
	reports := strings.Index(source, `x-show="tab==='reports'"`)
	schedule := strings.Index(source, `x-show="tab==='schedule'"`)
	add := strings.Index(source, `x-show="tab==='add'"`)
	if reports < 0 || schedule < reports || add < schedule {
		t.Fatalf("tab section order schedule=%d reports=%d add=%d", schedule, reports, add)
	}
}

func TestSyncTemplateExposesScheduleBatchAndReportRowActions(t *testing.T) {
	raw, err := os.ReadFile("../../web/templates/sync_manage.html")
	if err != nil {
		t.Fatalf("读 sync_manage.html: %v", err)
	}
	source := string(raw)
	for _, required := range []string{
		`saveScheduleBatch()`,
		`toggleAllVisibleSchedule()`,
		`x-model="scheduleBatch.cron"`,
		`x-model.number="scheduleBatch.window_days"`,
		`deleteReportExport(row)`,
		`reportDifferenceFor(row, 'database_missing')`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("同步页缺少批量或逐行操作 %q", required)
		}
	}
}
