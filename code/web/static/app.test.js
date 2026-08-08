const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

const sandbox = {
  window: { addEventListener() {}, dispatchEvent() {}, location: { search: '?tab=schedule&add=1' } },
  document: { querySelector() { return null; } },
  CustomEvent: class CustomEvent {},
  URLSearchParams,
  setTimeout,
  clearTimeout,
};
sandbox.window.window = sandbox.window;
vm.runInNewContext(fs.readFileSync(__dirname + '/app.js', 'utf8'), sandbox);

const root = sandbox.window.AppRoot();
root.init();
assert.equal(typeof sandbox.window.syncConfirm, 'function');

const entryManage = sandbox.window.syncManage();
assert.equal(entryManage.tab, 'manual');
assert.equal(entryManage.advancedAdd, false);

// 手动日期只允许已声明日期合同的接口，并且单日接口只能选同一天。
{
  const m = sandbox.window.syncManage();
  m.accounts = ['sc_us'];
  m.endpoints = [
    { name: 'range', display: '范围报表', account_id: 'sc_us', path: '/range', date_range_capable: true },
    { name: 'single', display: '单日报表', account_id: 'sc_us', path: '/single', date_field: 'event_date' },
    { name: 'snapshot', display: '快照', account_id: 'sc_us', path: '/snapshot' },
  ];
  m.form.accounts = ['sc_us'];
  m.form.types = ['/range', '/single'];
  m.form.date_from = '2026-08-01';
  m.form.date_to = '2026-08-03';
  assert.match(m.dateRangeIssue, /单日报表/);
  m.form.date_to = '2026-08-01';
  assert.equal(m.dateRangeIssue, '');
  m.form.types = ['/snapshot'];
  assert.match(m.dateRangeIssue, /快照/);
}

// 触发同步矩阵模型：两账号同 path 接口在 UI 合并成一份数据类型；
// 勾账号 × 勾类型 → resolvedEndpoints 笛卡尔积解析回真实 name。
{
  const m = sandbox.window.syncManage();
  m.accounts = ['sc_us_1', 'sc_us_2'];
  m.endpoints = [
    { name: 'sc_stores', display: 'SC 店铺列表', account_id: 'sc_us_1', path: '/stores', iterate_by_store: false },
    { name: 'sc_stores_aff', display: 'SC 店铺列表（联营）', account_id: 'sc_us_2', path: '/stores', iterate_by_store: false },
    { name: 'sc_inventory', display: 'SC FBA 库存', account_id: 'sc_us_1', path: '/inv', iterate_by_store: true },
    { name: 'sc_inventory_aff', display: 'SC FBA 库存（联营）', account_id: 'sc_us_2', path: '/inv', iterate_by_store: true },
  ];
  // 4 个接口按 path 去重成 2 个数据类型，label 去掉「（联营）」后缀后一致
  assert.equal(m.dataTypes.length, 2, 'dataTypes 应按 path 去重成 2 个');
  const storeType = m.dataTypes.find(t => t.key === '/stores');
  assert.equal(storeType.label, 'SC 店铺列表', 'label 应去掉尾部（联营）括注');
  assert.equal(m.dataTypes.find(t => t.key === '/inv').iterate_by_store, true, '任一账号该类型按店铺则合并为 true');

  // 勾 2 账号 × 勾 1 类型(/stores) → 2 个真实接口
  m.form.accounts = ['sc_us_1', 'sc_us_2'];
  m.form.types = ['/stores'];
  assert.equal(JSON.stringify(m.resolvedEndpoints.map(e => e.name).sort()), JSON.stringify(['sc_stores', 'sc_stores_aff']));
  assert.equal(m.selectedCount, 2);
  assert.equal(m.showStoreGrid, false, '/stores 非按店铺，不显示网格');

  // 只勾 1 账号 × 勾 2 类型 → 只该账号的 2 个接口；含按店铺类型 → 显示网格
  m.form.accounts = ['sc_us_2'];
  m.form.types = ['/stores', '/inv'];
  assert.equal(JSON.stringify(m.resolvedEndpoints.map(e => e.name).sort()), JSON.stringify(['sc_inventory_aff', 'sc_stores_aff']));
  assert.equal(m.showStoreGrid, true, '选中 /inv（按店铺）应显示网格');
  assert.equal(JSON.stringify(m.storeAccounts), JSON.stringify(['sc_us_2']), 'storeAccounts 只含选中且按店铺接口的账号');
}

// 店铺网格按 store_type 过滤：SC 迭代接口不显示 VC 店铺（修 VC 店铺混入 SC 接口的 bug）。
{
  const m = sandbox.window.syncManage();
  m.accounts = ['sc_us_2'];
  m.endpoints = [
    { name: 'sc_inventory_aff', display: 'SC FBA 库存（联营）', account_id: 'sc_us_2', path: '/inv', iterate_by_store: true, store_type: 'SC' },
  ];
  m.storesByAccount = { sc_us_2: { loaded: true, query: '', selected: {}, items: [
    { sid: '7860', store_name: '结福-CA', store_type: 'SC' },
    { sid: '134710', store_name: '日本-VC-New-JP', store_type: 'VC' },
  ] } };
  m.form.accounts = ['sc_us_2'];
  m.form.types = ['/inv'];
  // neededStoreTypes 只含 SC → VC 店铺被过滤掉
  assert.equal(JSON.stringify([...m.neededStoreTypes('sc_us_2')]), JSON.stringify(['SC']));
  const vis = m.visibleStores('sc_us_2');
  assert.equal(vis.length, 1, 'SC 迭代接口只应显示 SC 店铺');
  assert.equal(vis[0].sid, '7860');
}

const confirmation = sandbox.window.syncConfirm('删除账号？', '确认');
root.confirmResolve(true);
confirmation.then((accepted) => assert.equal(accepted, true));

void (async () => {
  let request = null;
  sandbox.window.syncConfirm = async () => true;
  sandbox.window.apiPost = async (url, body) => {
    request = { url, body };
    return { message: '已请求取消' };
  };
  // 取消任务的入口只剩同步日志页（/logs）行内按钮：同步管理页已不再渲染任务历史表，
  // 那张表与 /logs 读同一个 GET /api/tasks，属重复展示。契约不变：POST /api/sync/{endpoint}/cancel。
  const cancelLogs = sandbox.window.logsPage();
  cancelLogs.load = async () => {};
  await cancelLogs.cancelRow({ endpoint: 'orders', id: 42 });

  assert.equal(request.url, '/api/sync/orders/cancel');
  assert.equal(request.body.task_id, 42);

  // 日志日期范围：快捷范围必须作为一个入口更新起止日期、回到第一页并重拉主表。
  const logs = sandbox.window.logsPage();
  let dateRangeLoads = 0;
  logs.filters.page = 3;
  logs.load = async () => { dateRangeLoads++; };
  await logs.applyDatePreset('last_7_days');
  assert.match(logs.filters.date_from, /^\d{4}-\d{2}-\d{2}$/);
  assert.match(logs.filters.date_to, /^\d{4}-\d{2}-\d{2}$/);
  assert.equal(logs.filters.page, 1);
  assert.equal(dateRangeLoads, 1);

  sandbox.window.__PAGE__ = { accountOptions: [{ id: 'sc_us_1', name: '美国自营' }] };
  const namedLogs = sandbox.window.logsPage();
  assert.equal(namedLogs.accountLabel('sc_us_1'), '美国自营');
  assert.equal(namedLogs.accountLabel('unknown'), 'unknown');

  // 防回归：同步管理页不得再出现任务历史表、也不得出现运行态摘要条。
  // 任务状态明细只有一个去处 —— 同步日志页（/logs）。
  const noTableManage = sandbox.window.syncManage();
  assert.equal(typeof noTableManage.cancel, 'undefined');
  assert.equal(typeof noTableManage.taskStatusText, 'undefined');
  assert.equal(typeof noTableManage.runningCount, 'undefined');
  assert.equal(typeof noTableManage.recentErrorCount, 'undefined');
  assert.equal(typeof noTableManage.lastTaskLabel, 'undefined');
  assert.equal(typeof noTableManage.loadRunning, 'undefined');
  assert.equal(typeof noTableManage.startPolling, 'undefined');
  assert.equal(typeof noTableManage.stopPolling, 'undefined');
  assert.equal('runningList' in noTableManage, false);
  assert.equal('polling' in noTableManage, false);

  // recentTasks 现在唯一的消费者：定时调度 Tab 的「上次运行」列。
  const lastRun = sandbox.window.syncManage();
  lastRun.recentTasks = [
    { id: 2, endpoint: 'sc_stores', status: 'success', started_at: new Date(Date.now() - 30000).toISOString() }
  ];
  assert.ok(lastRun.lastRunOf('sc_stores').endsWith('秒前'), 'lastRunOf 应给相对时间：' + lastRun.lastRunOf('sc_stores'));
  assert.equal(lastRun.lastRunOf('sc_orders'), '—');

  const calls = [];
  sandbox.window.apiGet = async (url) => {
    calls.push({ method: 'GET', url });
    if (url === '/api/settings') {
      return {
        version: '0.1.0', uptime_sec: 10, db_connected: true,
        accounts: [{ id: 'sc_us', name: '美国领星', token_known: false, token_valid: false }]
      };
    }
    if (url === '/api/config') {
      return {
        accounts: [{
          id: 'sc_us', name: '美国领星', quota_group: 'sc_us', app_key: 'ak_old', app_secret: '****',
          connection_check: { cron: '*/20 * * * *', enabled: true }
        }],
        endpoints: [{
          name: 'sc_stores', display: '店铺目录', account: 'sc_us', path: '/stores', method: 'POST', table: 'ls_stores',
          record_id_fields: ['sid'], rate: { bucket: 2, interval_ms: 1000, multi_interval_ms: 0, dimension: 'account+path' },
          cron: '*/20 * * * *', enabled: true
        }]
      };
    }
    if (url === '/api/accounts/sc_us/stores') return { total: 0, last_synced_at: null, items: [] };
    throw new Error('unexpected GET ' + url);
  };
  sandbox.window.apiPut = async (url, body) => {
    calls.push({ method: 'PUT', url, body });
    return { message: '已保存', need_restart: false };
  };
  const settings = sandbox.window.settingsApi();
  await settings.load();
  assert.equal(settings.selectedAccountId, 'sc_us');
  assert.equal(settings.accountForm.app_secret, '');
  assert.equal('schedules' in settings, false);
  assert.equal(typeof settings.saveSchedule, 'undefined');
  assert.equal(settings.connectionCheck.cron, '*/20 * * * *');

  // 店铺状态映射：领星 int 状态在页面显示为中文；未知值原样、空值显示 '-'。
  assert.equal(settings.storeStatusText('1'), '正常');
  assert.equal(settings.storeStatusText('2'), '授权异常');
  assert.equal(settings.storeStatusText('0'), '停止同步');
  assert.equal(settings.storeStatusText('3'), '欠费停服');
  assert.equal(settings.storeStatusText(1), '正常', '数字入参也应映射');
  assert.equal(settings.storeStatusText(''), '-');
  assert.equal(settings.storeStatusText(null), '-');
  assert.equal(settings.storeStatusText('9'), '9', '未知状态原样显示');

  // 表头复选框只批量修改当前账号已加载的店铺，并复用既有 dirty/save 流程。
  settings.storeSummary.items = [{ sid: 'store-1' }, { sid: 'store-2' }];
  settings.storeSel = { 'store-1': true, 'store-2': false };
  settings.storeSelBaseline = { 'store-1': true, 'store-2': false };
  assert.equal(settings.storeAllSelected, false);
  settings.toggleAllStores(true);
  assert.equal(settings.storeAllSelected, true);
  assert.equal(settings.storeSelectedCount, 2);
  assert.equal(settings.storeDirty, true);
  settings.toggleAllStores(false);
  assert.equal(settings.storeAllSelected, false);
  assert.equal(settings.storeSelectedCount, 0);

  settings.accountForm.name = '美国主账号';
  await settings.saveAccount();
  const accountSave = calls.find(c => c.method === 'PUT' && c.url === '/api/accounts/sc_us');
  assert.equal(JSON.stringify(accountSave.body), JSON.stringify({
    id: 'sc_us', name: '美国主账号', quota_group: 'sc_us', app_key: 'ak_old', app_secret: ''
  }));

  settings.connectionCheck.enabled = false;
  await settings.saveConnectionCheck();
  const connectionCheckSave = calls.find(c => c.method === 'PUT' && c.url === '/api/accounts/sc_us/connection-check');
  assert.equal(JSON.stringify(connectionCheckSave.body), JSON.stringify({ cron: '*/20 * * * *', enabled: false }));

  const manage = sandbox.window.syncManage();
  const schedule = {
    name: 'sc_stores', display: '店铺目录', account: 'sc_us', path: '/stores', method: 'POST', table: 'ls_stores',
    record_id_fields: ['sid'], rate: { bucket: 2, interval_ms: 1000, multi_interval_ms: 0, dimension: 'account+path' },
    cron: '*/30 * * * *', enabled: true, store_sids: [], store_sids_text: '',
    window_days: 7, date_offset_days: 0
  };
  manage.scheduleBaseline = { sc_stores: manage.rowSnap(schedule) };
  schedule.cron = '*/40 * * * *';
  await manage.saveRow(schedule);
  const cronSave = calls.find(c => c.method === 'PUT' && c.url === '/api/endpoints/sc_stores');
  assert.equal(cronSave.body.cron, '*/40 * * * *');
  assert.equal(cronSave.body.rate.bucket, 2);
  assert.equal(cronSave.body.window_days, 7);
  assert.equal(cronSave.body.date_offset_days, 0);
})().catch((error) => {
  process.nextTick(() => { throw error; });
});
