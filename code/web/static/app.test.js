const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

const sandbox = {
  window: { addEventListener() {}, dispatchEvent() {}, location: { search: '?tab=schedule&add=1', pathname: '/dataset-fields', hash: '' }, history: { replaceState(_state, _title, url) { this.lastURL = url; const next = new URL(url, 'http://test.local'); sandbox.window.location.pathname = next.pathname; sandbox.window.location.search = next.search; sandbox.window.location.hash = next.hash; } } },
  document: { querySelector() { return null; } },
  CustomEvent: class CustomEvent {},
  URLSearchParams,
  setTimeout,
  clearTimeout,
};
sandbox.window.window = sandbox.window;
vm.runInNewContext(fs.readFileSync(__dirname + '/app.js', 'utf8'), sandbox);

// 同步日志是面向中国用户的操作时间；显示不能跟随浏览器或服务器时区漂移。
{
  const originalTZ = process.env.TZ;
  process.env.TZ = 'UTC';
  try {
    assert.equal(sandbox.window.fmtTime('2026-08-10T04:30:00Z'), '2026-08-10 12:30:00');
  } finally {
    if (originalTZ === undefined) delete process.env.TZ;
    else process.env.TZ = originalTZ;
  }
}

const root = sandbox.window.AppRoot();
root.init();
assert.equal(typeof sandbox.window.syncConfirm, 'function');

const entryManage = sandbox.window.syncManage();
assert.equal(entryManage.tab, 'manual');
assert.equal(entryManage.advancedAdd, false);

// 报表核验矩阵固定展示数据表、下载能力、核验能力和当前启用状态。
{
  const matrix = sandbox.window.syncManage();
  matrix.reportExportConfigs = [
    { type: 'fba_customer_returns', enabled: true },
    { type: 'fba_customer_returns', enabled: false },
  ];
  const rows = matrix.reportCoverageRows();
  const returns = rows.find((row) => row.type === 'fba_customer_returns');
  const rawOnly = rows.find((row) => row.type === 'fba_storage_fee_charges');
  assert.equal(returns.enabledCount, 1);
  assert.equal(matrix.reportCoverageEnabledText(returns), '已启用（1条）');
  assert.equal(matrix.reportCoverageEnabledText(rawOnly), '不适用（raw-only）');
}

// 同步配置标题提示必须支持键盘聚焦，并把“修改时间增量”和“业务日期重拉”说清楚。
{
  const template = fs.readFileSync(__dirname + '/../templates/sync_manage.html', 'utf8');
  assert.match(template, /aria-describedby="sync-modified-time-tip"/);
  assert.match(template, /id="sync-modified-time-tip"[^>]*role="tooltip"/);
  assert.match(template, /订单、Listing、FBA 退货、VC PO/);
  assert.match(template, /销量、Performance、SP、SD、HSA、VC 销量\/库存/);
  assert.match(template, /<span class="block">支持按修改时间补拉：订单、Listing、FBA 退货、VC PO。<\/span>/);
  assert.match(template, /<span class="block">销量、Performance、SP、SD、HSA、VC 销量\/库存按业务日期重拉最近范围。<\/span>/);
  assert.match(template, /<span class="block">raw-only：这类报告目前只能下载留档。它还没有完整映射到下游日维表的“店铺 \+ ASIN \+ 日期”行，无法判断具体差异并更新哪一行，因此不能用来核算或覆盖日维数据。<\/span>/);
  assert.match(template, /fixed left-4 right-4 top-44/);
  assert.match(template, /data-report-coverage-matrix/);
  assert.match(template, /是否有报表下载/);
  assert.match(template, /是否能核验/);
  assert.match(template, /是否已启用核验/);
  assert.match(template, /sm:absolute sm:left-full sm:right-auto sm:top-1\/2/);
  assert.match(template, /sm:-translate-y-\[40px\]/);
  assert.doesNotMatch(template, /absolute left-1\/2 top-full/);
}

// 数据表配置与新增下游项目是两个切卡；表定义不依赖下游项目，且不接受表名或 SQL 输入。
{
  const template = fs.readFileSync(__dirname + '/../templates/dataset_fields.html', 'utf8');
  assert.match(template, /数据表配置/);
  assert.match(template, /新增下游项目/);
  assert.doesNotMatch(template, /新增项目/);
  assert.match(template, /数据表 ID/);
  assert.match(template, /x-text="datasetID"/);
  assert.match(template, /flex flex-wrap items-center justify-between gap-3/);
  assert.match(template, /<nav class="flex items-center gap-1\.5" aria-label="数据表管理">/);
  assert.match(template, /x-data="dataSources\(\)"/);
  assert.match(template, /x-init="loadDatasetCatalog\(\); loadDatasetStores\(\)"/);
  assert.match(template, /CSV 导出范围/);
  assert.match(template, /只筛选导出的记录，不修改数据表或字段/);
  assert.match(template, /grid grid-cols-1 items-stretch gap-3[\s\S]*md:h-48[\s\S]*md:grid-cols-\[minmax\(0,1fr\)_15rem\][\s\S]*md:overflow-hidden/);
  assert.match(template, /flex h-full min-h-0 min-w-0 w-full flex-col overflow-hidden rounded-md border border-slate-300/);
  assert.match(template, /<div class="flex flex-col gap-3">[\s\S]*开始日期[\s\S]*结束日期[\s\S]*导出 CSV/);
  assert.match(template, /x-show="datasetExportRows>0 \|\| datasetExportError"[\s\S]*已导出/);
  assert.match(template, /flex min-w-0 items-center gap-3 whitespace-nowrap[\s\S]*CSV 导出范围[\s\S]*全选[\s\S]*清空[\s\S]*全部已勾选店铺/);
  assert.match(template, /grid min-h-0 flex-1 grid-cols-1 gap-2 overflow-y-auto[\s\S]*sm:grid-cols-2[\s\S]*lg:grid-cols-3/);
  assert.match(template, /<label class="flex cursor-pointer items-center gap-1\.5 rounded-md border/);
  assert.match(template, /可添加字段/);
  assert.match(template, /当前版本锁定/);
  assert.match(template, /已发布字段/);
  assert.match(template, /固定字段/);
  assert.match(template, /固定字段，不能删除/);
  assert.match(template, /fieldGroupSource\(group\.source\)/);
  assert.match(template, /availableTableFieldGroups/);
  assert.match(template, /Token ID/);
  assert.match(template, /店铺范围/);
  assert.match(template, /setDatasetTab\('projects'\)/);
  assert.equal((template.match(/grid grid-cols-1[^\"]*sm:grid-cols-2[^\"]*lg:grid-cols-3/g) || []).length, 2, '数据表和店铺选择区必须在桌面端使用三列紧凑网格');
  assert.equal((template.match(/flex cursor-pointer items-center gap-1\.5 rounded-md border px-2 py-1\.5/g) || []).length, 2, '两类选择卡片必须压缩间距但保留整卡片命中区');
  assert.match(template, /max-h-72[^\"]*gap-1\.5[^\"]*overflow-y-auto[^\"]*p-2[^\"]*sm:grid-cols-2[^\"]*lg:grid-cols-3/);
  assert.match(template, /:title="definition\.name"/);
  assert.match(template, /:title="store\.store_name \|\| '未命名店铺'"/);
  assert.match(template, /w-full min-w-\[1104px\] table-fixed/);
  assert.doesNotMatch(template, /min-w-\[1180px\]/);
  assert.match(template, /datasetStoreScopeSummary\(project\.store_scopes\)/);
  assert.match(template, /id="dataset-projects-table"/);
  assert.match(template, /<span class="block truncate font-mono text-\[11px\] text-slate-600"/);
  assert.doesNotMatch(template, /block break-all/);
  assert.match(template, /Token ID[\s\S]*Bearer Token/);
  assert.match(template, /#dataset-projects-table table tbody td/);
  assert.match(template, /min-height: 24px/);
  const projectRows = template.match(/<template x-for="project in datasetProjects"[\s\S]*?<\/template>/)?.[0];
  assert.ok(projectRows, '下游项目列表必须存在项目循环模板');
  assert.equal((projectRows.match(/<tr\b/g) || []).length, 1, 'Alpine x-for 每个项目必须只渲染一个表格行');
  const projectCells = projectRows.match(/<td\b[^>]*>/g) || [];
  assert.equal(projectCells.length, 5, '每个下游项目必须渲染五个表格单元格');
  assert.equal(projectCells.filter((cell) => /align-middle/.test(cell)).length, 4, '项目名、店铺范围、状态和操作列必须垂直居中');
  assert.equal(projectCells.filter((cell) => /align-top/.test(cell)).length, 0, '项目行不得把任何列顶对齐');
  assert.match(template, /createDatasetProjectToken\(\)/);
  assert.match(template, /addTableField\(field\.name\)/);
  assert.match(template, /removeTableField\(field\.name\)/);
  assert.match(template, /loadDatasetProjectGuide\(project\)/);
  assert.match(template, /editDatasetProject\(project\)/);
  assert.match(template, /project\.token/);
  assert.match(template, /downloadDatasetGuide\(\)/);
  assert.match(template, /接入说明/);
  assert.match(template, /h-\[720px\] overflow-y-auto/);
  assert.match(template, /saveDatasetFieldAllowlist\(\)/);
  assert.match(template, /registerDatasetVersion\(\)/);
  assert.match(template, /配置新版本/);
  assert.doesNotMatch(template, /注册新版本/);
  assert.match(template, /发布 v2/);
  assert.match(template, /暂无可添加字段/);
  assert.doesNotMatch(template, /<input[^>]*x-model[^>]*(?:table|sql)/i);
  assert.doesNotMatch(template, /type=["']password/i);
  assert.doesNotMatch(template, /CREATE\s+TABLE|ALTER\s+TABLE|DROP\s+TABLE|SQL\s*:/i);
  const dataSourcesTemplate = fs.readFileSync(__dirname + '/../templates/datasources.html', 'utf8');
  assert.match(dataSourcesTemplate, /日维数据预览/);
  assert.match(dataSourcesTemplate, /applyDailyPreviewFilters\(\)/);
  assert.match(dataSourcesTemplate, /dailyPreviewValue/);
}

// 数据表页刷新时保留当前切卡，避免新增下游项目后回到数据表配置页。
{
  sandbox.window.location.search = '?tab=projects';
  const tabs = sandbox.window.dataSources();
  assert.equal(tabs.datasetTab, 'projects');
  tabs.setDatasetTab('table');
  assert.equal(tabs.datasetTab, 'table');
  assert.equal(sandbox.window.history.lastURL, '/dataset-fields');
  tabs.setDatasetTab('projects');
  assert.equal(sandbox.window.history.lastURL, '/dataset-fields?tab=projects');
}

// 项目 Token 入口必须把逗号分隔的店铺/字段转换为固定数组，并显示一次性明文结果。
{
  const source = fs.readFileSync(__dirname + '/app.js', 'utf8');
  assert.match(source, /datasetCreateForm/);
  assert.match(source, /const storeScopes = this\.datasetStoreScopes\(\)/);
  assert.match(source, /store_scopes: storeScopes/);
  assert.doesNotMatch(source, /datasetCreateForm\.store_scopes/);
  assert.doesNotMatch(source, /datasetCreateForm\.fields/);
  assert.match(source, /datasetCreateResult = await window\.apiPost/);
}

// 接口清单启用状态必须在下拉和按钮上可见、可禁用。
{
  const m = sandbox.window.syncManage();
  m.catalog = { accounts: [
    { id: 'sc_us_1', name: '自营领星' },
    { id: 'sc_us_2', name: '联营领星' },
  ], templates: [] };
  const enabled = { key: 'vc_stores', enabled_accounts: ['sc_us_1'] };
  const pending = { key: 'vc_margin', enabled_accounts: [] };
  assert.equal(m.catalogEnabledLabel(enabled), '已启用：自营领星');
  m.catalogBatchAccount = 'sc_us_1';
  assert.equal(m.catalogBatchAvailable(enabled), false);
  m.catalogBatchAccount = 'sc_us_2';
  assert.equal(m.catalogBatchAvailable(pending), true);
}

// 清单批量启用：先选一次账号，再勾选多个未启用接口；已启用项不可选。
{
  const m = sandbox.window.syncManage();
  m.catalog = { accounts: [{ id: 'sc_us_1', name: '自营领星' }], templates: [
    { key: 'stores', enabled_accounts: ['sc_us_1'] },
    { key: 'listing', enabled_accounts: [] },
    { key: 'margin', enabled_accounts: [] },
  ] };
  m.catalogBatchAccount = 'sc_us_1';
  m.catalogBatchKeys = [];
  assert.equal(m.catalogBatchAvailable(m.catalog.templates[0]), false);
  assert.equal(m.catalogBatchAvailable(m.catalog.templates[1]), true);
  m.selectAllCatalogPending();
  assert.deepEqual(m.catalogBatchKeys, ['listing', 'margin']);
  m.toggleCatalogBatch('listing');
  assert.deepEqual(m.catalogBatchKeys, ['margin']);
  m.clearCatalogBatch();
  assert.equal(JSON.stringify(m.catalogBatchKeys), '[]');
}

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
  assert.equal(m.dateControlsEnabled, false, '纯快照接口不应允许填写日期');
  m.form.types = ['/range'];
  assert.equal(m.dateControlsEnabled, true, '声明日期合同的接口应允许填写日期');
  m.form.date_from = '2026-08-01';
  m.form.date_to = '2026-08-01';
  m.form.types = ['/snapshot'];
  m.pruneUnavailableTypes();
  assert.equal(m.form.date_from, '', '切换到无日期接口时应清空失效的开始日期');
  assert.equal(m.form.date_to, '', '切换到无日期接口时应清空失效的结束日期');
  m.form.date_from = '2026-08-01';
  m.form.date_to = '2026-08-01';
  m.clearAccounts();
  assert.equal(JSON.stringify(m.form.accounts), '[]');
  assert.equal(JSON.stringify(m.form.types), '[]');
  assert.equal(m.form.date_from, '');
  assert.equal(m.form.date_to, '');
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

// 固定工作台：账号切换不能让类型卡消失或重排；不可用类型留在原位并禁用。
{
  const m = sandbox.window.syncManage();
  m.endpoints = [
    { name: 'sc_stores_sc_us_1', display: 'SC 店铺列表', account_id: 'sc_us_1', path: '/stores', iterate_by_store: false },
    { name: 'sc_stores_sc_us_2', display: 'SC 店铺列表（联营）', account_id: 'sc_us_2', path: '/stores', iterate_by_store: false },
    { name: 'vc_margin_sc_us_2', display: 'VC 毛利日报', account_id: 'sc_us_2', path: '/vc-margin', iterate_by_store: true, store_type: 'VC' },
    { name: 'vc_stores_sc_us_1', display: 'VC 店铺列表', account_id: 'sc_us_1', path: '/vc-stores', iterate_by_store: false },
  ];
  const keysBefore = m.dataTypes.map(t => t.key);
  m.form.accounts = ['sc_us_1'];
  assert.equal(m.dataTypes.some(t => t.key === '/vc-margin'), true, '自营未配置 VC 毛利时仍应保留卡片位置');
  assert.equal(m.isTypeAvailable('/vc-margin'), false, '自营未配置 VC 毛利时应禁用该卡片');
  m.form.types = ['/stores', '/vc-margin'];
  m.pruneUnavailableTypes();
  assert.equal(JSON.stringify(m.form.types), JSON.stringify(['/stores']), '账号切换后只清掉不可用的已选类型');
  m.form.accounts = ['sc_us_2'];
  m.pruneUnavailableTypes();
  assert.equal(JSON.stringify(m.dataTypes.map(t => t.key)), JSON.stringify(keysBefore), '账号切换后类型卡顺序必须保持不变');
  assert.equal(JSON.stringify(m.form.types), JSON.stringify(['/stores']), '共享类型在账号切换后应保留选择');
  assert.equal(m.isTypeAvailable('/vc-margin'), true, '联营已配置 VC 毛利时应启用该卡片');
}

const confirmation = sandbox.window.syncConfirm('删除账号？', '确认');
root.confirmResolve(true);
confirmation.then((accepted) => assert.equal(accepted, true));

// 正式报告保存不再从页面读取店铺；请求只带共同配置，后端按账号全局店铺选择展开。
{
  const report = sandbox.window.syncManage();
  report.reportExportConfigs = [{
    type: 'fba_customer_returns', enabled: true, account: 'sc_us', seller_id: 'SELLER-OLD',
    store_id: 'OLD', region: 'na', marketplace_ids: ['OLD-MKT'], cron: '0 4 * * *', window_days: 3,
  }];
  report.reportBatch = { type: 'fba_customer_returns', account: 'sc_us', region: 'na', cron: '0 5 * * *', window_days: 7, enabled: true };
  let request = null;
  sandbox.window.apiPut = (url, body) => {
    request = { url, body };
    return Promise.resolve({ message: '已保存' });
  };
  report.loadReportExport = async () => {};
  void report.saveReportExportBatch();
  assert.equal(JSON.stringify(request), JSON.stringify({
    url: '/api/report-exports/config',
    body: { report_exports: [{ type: 'fba_customer_returns', enabled: true, account: 'sc_us', region: 'na', cron: '0 5 * * *', window_days: 7 }] },
  }));
}

void (async () => {
  // 数据表选择由 URL 驱动：无参数默认第一张已发布表，有效参数恢复（包括未发布 v2），无效参数回退并清除。
  {
    const definitions = [
      { id: 'address-v1', name: 'Address v1', published: true },
      { id: 'listing-daily-v1', name: 'Listing v1', published: true },
      { id: 'listing-daily-v2', name: 'Listing v2', published: false, parent_dataset_id: 'listing-daily-v1' },
    ];
    const configFor = (id) => ({
      dataset_id: id,
      dataset_name: definitions.find((definition) => definition.id === id).name,
      fixed_fields: ['store'],
      available_fields: ['sales_units'],
      configured_fields: [],
      published: id !== 'listing-daily-v2',
      parent_dataset_id: id === 'listing-daily-v2' ? 'listing-daily-v1' : '',
      projects: [],
    });
    sandbox.window.location.pathname = '/dataset-fields';
    sandbox.window.location.hash = '#fields';
    sandbox.window.location.search = '?tab=projects&keep=yes';
    sandbox.window.apiGet = async (url) => {
      if (url === '/api/datasources/datasets/catalog') return { datasets: definitions, projects: [] };
      const prefix = '/api/datasources/datasets/';
      if (url.startsWith(prefix) && url.endsWith('/fields/config')) {
        const id = decodeURIComponent(url.slice(prefix.length, -'/fields/config'.length));
        return configFor(id);
      }
      throw new Error('unexpected dataset GET ' + url);
    };

    const defaultTable = sandbox.window.dataSources();
    await defaultTable.loadDatasetCatalog();
    assert.equal(defaultTable.datasetID, 'address-v1');
    assert.doesNotMatch(sandbox.window.history.lastURL || '', /dataset=/);

    sandbox.window.location.search = '?tab=projects&keep=yes&dataset=listing-daily-v2';
    const draftTable = sandbox.window.dataSources();
    await draftTable.loadDatasetCatalog();
    assert.equal(draftTable.datasetID, 'listing-daily-v2');
    assert.equal(draftTable.parentDatasetID, 'listing-daily-v1');
    await draftTable.selectDatasetTable('address-v1');
    assert.equal(sandbox.window.history.lastURL, '/dataset-fields?tab=projects&keep=yes&dataset=address-v1#fields');

    sandbox.window.location.search = '?tab=projects&keep=yes&dataset=missing';
    const invalidTable = sandbox.window.dataSources();
    await invalidTable.loadDatasetCatalog();
    assert.equal(invalidTable.datasetID, 'address-v1');
    assert.equal(sandbox.window.history.lastURL, '/dataset-fields?tab=projects&keep=yes#fields');
  }

  // 旧目录请求晚于新请求返回时，不得覆盖新请求的选择或 URL。
  {
    const catalogRequests = [];
    const definitions = [
      { id: 'listing-daily-v1', name: 'Listing v1', published: true },
    ];
    const config = { dataset_id: 'listing-daily-v1', dataset_name: 'Listing v1', fixed_fields: ['store'], available_fields: [], configured_fields: [], published: true, projects: [] };
    sandbox.window.location.search = '?tab=projects&keep=yes&dataset=listing-daily-v1';
    sandbox.window.apiGet = (url) => {
      if (url !== '/api/datasources/datasets/catalog') return Promise.resolve(config);
      return new Promise((resolve) => catalogRequests.push(resolve));
    };
    const racing = sandbox.window.dataSources();
    const oldRequest = racing.loadDatasetCatalog();
    const newRequest = racing.loadDatasetCatalog();
    catalogRequests[1]({ datasets: definitions, projects: [] });
    await newRequest;
    assert.equal(racing.datasetID, 'listing-daily-v1');
    assert.match(sandbox.window.location.search, /dataset=listing-daily-v1/);
    catalogRequests[0]({ datasets: [{ id: 'stale-only', name: 'Stale', published: true }], projects: [] });
    await oldRequest;
    assert.equal(racing.datasetID, 'listing-daily-v1');
    assert.match(sandbox.window.location.search, /dataset=listing-daily-v1/);
  }

  // 未发布 v2 可以退出草稿：无修改直接返回父版本，有修改先确认，拒绝确认则保持原表。
  {
    sandbox.window.location.search = '?tab=projects&keep=yes&dataset=listing-daily-v2';
    const draftTable = sandbox.window.dataSources();
    await draftTable.loadDatasetCatalog();
    assert.equal(draftTable.canExitDatasetDraft, true);
    draftTable.tableFieldsPublished = true;
    assert.equal(draftTable.canExitDatasetDraft, false, 'v2 发布后必须立即隐藏退出草稿入口');
    draftTable.tableFieldsPublished = false;
    let confirmCalls = 0;
    sandbox.window.syncConfirm = async () => { confirmCalls++; return false; };
    draftTable.tableSelectedFields = ['sales_units'];
    draftTable.tableSavedFields = [];
    await draftTable.exitDatasetDraft();
    assert.equal(confirmCalls, 1);
    assert.equal(draftTable.datasetID, 'listing-daily-v2');
    assert.match(sandbox.window.location.search, /dataset=listing-daily-v2/);

    sandbox.window.syncConfirm = async () => { confirmCalls++; return true; };
    await draftTable.exitDatasetDraft();
    assert.equal(draftTable.datasetID, 'listing-daily-v1');
    assert.equal(sandbox.window.history.lastURL, '/dataset-fields?tab=projects&keep=yes&dataset=listing-daily-v1#fields');

    sandbox.window.location.search = '?tab=projects&keep=yes&dataset=listing-daily-v2';
    const cleanDraft = sandbox.window.dataSources();
    await cleanDraft.loadDatasetCatalog();
    confirmCalls = 0;
    sandbox.window.syncConfirm = async () => { confirmCalls++; return false; };
    await cleanDraft.exitDatasetDraft();
    assert.equal(confirmCalls, 0);
    assert.equal(cleanDraft.datasetID, 'listing-daily-v1');

  }

  // 脏草稿从顶部切卡离开时也必须确认；退出草稿复用同一切换路径，不能重复确认。
  {
    sandbox.window.location.search = '?tab=projects&keep=yes&dataset=listing-daily-v2';
    const clickDraft = sandbox.window.dataSources();
    await clickDraft.loadDatasetCatalog();
    clickDraft.tableSelectedFields = ['sales_units'];
    clickDraft.tableSavedFields = [];
    let confirmCalls = 0;
    sandbox.window.syncConfirm = async () => { confirmCalls++; return false; };
    await clickDraft.selectDatasetTable('listing-daily-v1');
    assert.equal(confirmCalls, 1);
    assert.equal(clickDraft.datasetID, 'listing-daily-v2');
    assert.match(sandbox.window.location.search, /dataset=listing-daily-v2/);

    sandbox.window.syncConfirm = async () => { confirmCalls++; return true; };
    await clickDraft.selectDatasetTable('listing-daily-v1');
    assert.equal(clickDraft.datasetID, 'listing-daily-v1');
    assert.equal(confirmCalls, 2);

    sandbox.window.location.search = '?tab=projects&keep=yes&dataset=listing-daily-v2';
    const exitDraft = sandbox.window.dataSources();
    await exitDraft.loadDatasetCatalog();
    exitDraft.tableSelectedFields = ['sales_units'];
    exitDraft.tableSavedFields = [];
    confirmCalls = 0;
    sandbox.window.syncConfirm = async () => { confirmCalls++; return true; };
    await exitDraft.exitDatasetDraft();
    assert.equal(exitDraft.datasetID, 'listing-daily-v1');
    assert.equal(confirmCalls, 1, '退出草稿只能确认一次');

    sandbox.window.location.search = '?tab=projects&keep=yes&dataset=listing-daily-v2';
    const raceDraft = sandbox.window.dataSources();
    await raceDraft.loadDatasetCatalog();
    raceDraft.tableSelectedFields = ['sales_units'];
    raceDraft.tableSavedFields = [];
    let resolveConfirm;
    sandbox.window.syncConfirm = () => new Promise((resolve) => { resolveConfirm = resolve; });
    const pendingSwitch = raceDraft.selectDatasetTable('listing-daily-v1');
    raceDraft.tableFieldsSaving = true;
    resolveConfirm(true);
    await pendingSwitch;
    assert.equal(raceDraft.datasetID, 'listing-daily-v2', '确认等待期间开始发布后不得切表');
  }

  // 发布 PUT 进行中禁止切表、配置新版本和退出草稿，避免旧响应污染新表。
  {
    const saving = sandbox.window.dataSources();
    saving.datasetDefinitions = [
      { id: 'listing-daily-v1', name: 'Listing v1', published: true, next_version_id: 'listing-daily-v2' },
      { id: 'listing-daily-v2', name: 'Listing v2', published: false, parent_dataset_id: 'listing-daily-v1' },
    ];
    saving.datasetID = 'listing-daily-v2';
    saving.loadDatasetCatalog = async () => { throw new Error('切表不应加载新数据表'); };
    saving.tableSelectedFields = ['sales_units'];
    saving.tableSavedFields = [];
    saving.tableFieldsPublished = false;
    saving.parentDatasetID = 'listing-daily-v1';
    let resolveSave;
    sandbox.window.apiPut = () => new Promise((resolve) => { resolveSave = resolve; });
    const savePromise = saving.saveDatasetFieldAllowlist();
    assert.equal(saving.tableFieldsSaving, true);
    await saving.selectDatasetTable('listing-daily-v1');
    assert.equal(saving.datasetID, 'listing-daily-v2');
    sandbox.window.syncConfirm = async () => { throw new Error('保存中不应弹退出确认'); };
    await saving.exitDatasetDraft();
    assert.equal(saving.datasetID, 'listing-daily-v2');
    resolveSave({ fields: ['sales_units'] });
    await savePromise;
    assert.equal(saving.tableFieldsSaving, false);

    const registering = sandbox.window.dataSources();
    registering.datasetDefinitions = saving.datasetDefinitions;
    registering.datasetID = 'listing-daily-v1';
    registering.nextVersionID = 'listing-daily-v2';
    registering.tableFieldsSaving = true;
    registering.loadDatasetCatalog = async () => { throw new Error('发布中不应配置新版本'); };
    registering.registerDatasetVersion();
    assert.equal(registering.datasetID, 'listing-daily-v1');
  }

  // 数据表字段和下游项目权限分开；左侧只显示尚未加入当前表的字段。
  {
    const catalog = sandbox.window.dataSources();
    assert.equal(JSON.stringify(catalog.selectedDatasetScopes), JSON.stringify([]), '新增下游项目不应默认勾选任何数据表');
    sandbox.window.apiGet = async (url) => {
      if (url === '/api/datasources/datasets/catalog') {
        return { datasets: [
          { id: 'listing-daily-v1', name: 'Listing 日维指标表', kind: 'daily', source: 'listing_dimensions + listing_daily_metrics', grain: 'store + channel + asin + sku + business_date' },
          { id: 'return-reason-detail-v1', name: '退货原因明细表', kind: 'detail', source: 'ls_sc_refunds', grain: 'store + license_plate_number' },
        ], need_restart: true, projects: [
          { project_id: 'polabel2', token_id: 'tok_reader', token: 'visible-token', dataset_scopes: ['listing-daily-v1'], store_scopes: ['12534'] },
          { project_id: 'returns-reader', token_id: 'tok_returns', token: 'returns-token', dataset_scopes: ['return-reason-detail-v1'], store_scopes: ['12536'] },
        ] };
      }
      assert.equal(url, '/api/datasources/datasets/listing-daily-v1/fields/config');
      return {
        dataset_id: 'listing-daily-v1',
        dataset_name: 'Listing 日维指标表',
        fixed_fields: ['store', 'channel', 'asin', 'sku', 'business_date', 'updated_at', 'is_provisional', 'verification_status'],
        available_fields: ['sales_units', 'inventory_sellable', 'sessions_total'],
        configured_fields: ['sales_units', 'sessions_total'],
        projects: [{ project_id: 'polabel2', token_id: 'tok_reader', token: 'visible-token', dataset_scopes: ['listing-daily-v1'], store_scopes: ['12534'] }],
      };
    };
    await catalog.loadDatasetCatalog();
    assert.equal(catalog.datasetProjectsNeedRestart, true, '刷新后的项目目录必须恢复重启提示');
    assert.equal(catalog.datasetName, 'Listing 日维指标表');
    assert.equal(catalog.datasetDefinitions.length, 2, '下游项目必须能从静态目录选择多张数据表');
    assert.equal(catalog.fixedFields.length, 8);
    assert.equal(catalog.availableTableFieldCount, 1, '左侧只显示尚未加入当前表的字段');
    assert.equal(JSON.stringify(catalog.availableTableFieldGroups.map((group) => group.fields.map((field) => field.name))), JSON.stringify([['inventory_sellable']]));
    assert.match(catalog.fieldGroupSource('销量'), /ls_sc_sales_report/);
    assert.equal(JSON.stringify(catalog.tableSelectedFields), JSON.stringify(['sales_units', 'sessions_total']));
    assert.equal(catalog.tableFieldDirty, false);
    catalog.addTableField('inventory_sellable');
    assert.equal(JSON.stringify(catalog.tableSelectedFields), JSON.stringify(['sales_units', 'sessions_total']), '已发布版本不能在原表追加字段');
    assert.equal(catalog.availableTableFieldCount, 1);
    assert.equal(JSON.stringify(catalog.publishedTableFields.map((field) => field.name)), JSON.stringify(['sales_units', 'sessions_total']));
    assert.equal(catalog.canRemoveTableField('sessions_total'), false, '同一 v1 已发布字段必须锁定，避免下游删列崩溃');
    assert.equal(catalog.canRemoveTableField('inventory_sellable'), false, '已发布版本不能删除字段');
    catalog.removeTableField('sessions_total');
    assert.equal(JSON.stringify(catalog.publishedTableFields.map((field) => field.name)), JSON.stringify(['sales_units', 'sessions_total']));
    catalog.removeTableField('inventory_sellable');
    assert.equal(JSON.stringify(catalog.publishedTableFields.map((field) => field.name)), JSON.stringify(['sales_units', 'sessions_total']));
    assert.equal(catalog.availableTableFieldCount, 1);
    assert.equal(catalog.tableFieldsLocked, true);
    assert.equal(catalog.datasetProjects[0].token_id, 'tok_reader');
    assert.equal(catalog.datasetProjects[1].token_id, 'tok_returns', '只授权其他数据表的项目刷新后也必须保留');
    assert.equal(catalog.datasetProjects[0].token, 'visible-token');
    assert.equal(JSON.stringify(catalog.datasetProjects[0].store_scopes), JSON.stringify(['12534']));

    const draft = sandbox.window.dataSources();
    draft.datasetDefinitions = [
      { id: 'return-reason-detail-v1', name: '退货原因明细表', published: true, next_version_id: 'return-reason-detail-v2' },
      { id: 'return-reason-detail-v2', name: '退货原因明细表 v2', published: false, parent_dataset_id: 'return-reason-detail-v1' },
    ];
    draft.datasetID = 'return-reason-detail-v1';
    draft.nextVersionID = 'return-reason-detail-v2';
    draft.loadDatasetCatalog = async () => {};
    assert.equal(draft.visibleDatasetDefinitions.length, 1, '未发布 v2 不应出现在普通数据表切卡');
    assert.equal(draft.canRegisterNextVersion, true, 'v1 只能注册代码预登记且未发布的下一个版本');
    draft.registerDatasetVersion();
    assert.equal(draft.datasetID, 'return-reason-detail-v2', '注册新版本只能切换到代码预注册的 v2');
    assert.equal(draft.visibleDatasetDefinitions.length, 2, '进入草稿后应保留父版本和当前 v2 切卡');
    const normalizedDraft = sandbox.normalizeListingDailyCatalog({
      dataset_id: 'return-reason-detail-v2', dataset_name: '退货原因明细表 v2', fixed_fields: ['store'],
      available_fields: ['reason', 'return_date_locale'], configured_fields: ['reason'], published: false,
      parent_dataset_id: 'return-reason-detail-v1', next_version_id: '', projects: [],
    });
    draft.tableSelectedFields = [...normalizedDraft.tableSelectedFields];
    draft.tableSavedFields = [...normalizedDraft.tableSelectedFields];
    draft.fieldGroups = normalizedDraft.fieldGroups;
    draft.tableFieldsPublished = normalizedDraft.published;
    assert.equal(draft.tableFieldsLocked, false, 'v2 继承字段但未发布时不能因 savedFields 非空而锁定');
    assert.equal(draft.availableTableFieldCount, 1, 'v2 左侧必须显示尚未加入的原始候选字段');
    draft.addTableField('return_date_locale');
    sandbox.window.apiPut = async () => ({ fields: ['reason', 'return_date_locale'], need_restart: true });
    await draft.saveDatasetFieldAllowlist();
    assert.equal(draft.tableFieldsLocked, true, 'v2 首次发布后必须立即锁定');
    draft.datasetDefinitions[1].published = true;
    draft.datasetID = 'return-reason-detail-v1';
    assert.equal(draft.canRegisterNextVersion, false, 'v2 已发布后 v1 不应继续显示无效注册入口');

    sandbox.window.location.search = '?tab=projects';
    const refreshedTab = sandbox.window.dataSources();
    assert.equal(refreshedTab.datasetTab, 'projects', '刷新后应保持新增下游项目切卡');

    const storeCalls = [];
    sandbox.window.apiGet = async (url) => {
      storeCalls.push(url);
      if (url === '/api/datasources/datasets/catalog') return {
        datasets: [
          { id: 'listing-daily-v1', name: 'Listing 日维指标表', kind: 'daily', source: 'listing_dimensions + listing_daily_metrics', grain: 'store + channel + asin + sku + business_date' },
          { id: 'return-reason-detail-v1', name: '退货原因明细表', kind: 'detail', source: 'ls_sc_refunds', grain: 'store + license_plate_number' },
        ],
        projects: [
          { project_id: 'polabel2', token_id: 'tok_reader', token: 'visible-token', dataset_scopes: ['listing-daily-v1'], store_scopes: ['12534'] },
          { project_id: 'returns-reader', token_id: 'tok_returns', token: 'returns-token', dataset_scopes: ['return-reason-detail-v1'], store_scopes: ['12536'] },
          { project_id: 'reader', token_id: 'tok_new', token: 'secret', dataset_scopes: ['listing-daily-v1', 'return-reason-detail-v1'], store_scopes: ['12534', '12536'] },
        ],
      };
      if (url === '/api/datasources/datasets/listing-daily-v1/fields/config') return {
        dataset_id: 'listing-daily-v1', dataset_name: 'Listing 日维指标表',
        fixed_fields: ['store', 'channel', 'asin', 'sku', 'business_date', 'updated_at', 'is_provisional', 'verification_status'],
        available_fields: ['sales_units', 'inventory_sellable', 'sessions_total'],
        configured_fields: ['sales_units', 'sessions_total'],
      };
      if (url === '/api/config') return { accounts: [{ id: 'sc_us_1', name: '美国自营' }] };
      if (url === '/api/accounts/sc_us_1/stores') return {
        items: [
          { sid: '12534', store_name: '美国主店', country: 'US', store_type: 'SC', enabled: true },
          { sid: '12536', store_name: '加拿大店', country: 'CA', store_type: 'SC', enabled: true },
          { sid: '12535', store_name: '未授权店铺', country: 'US', store_type: 'SC', enabled: false },
        ],
      };
      throw new Error('unexpected store URL ' + url);
    };
    await catalog.loadDatasetStores();
    assert.equal(JSON.stringify(storeCalls), JSON.stringify(['/api/config', '/api/accounts/sc_us_1/stores']));
    assert.equal(catalog.datasetStores.length, 2, '未勾选的店铺不得进入数据表项目范围');
    assert.equal(catalog.datasetStores[0].store_name, '美国主店');
    catalog.toggleAllDatasetStores(true);
    assert.equal(JSON.stringify(catalog.datasetStoreScopes()), JSON.stringify(['12534', '12536']));
    assert.equal(catalog.formatDatasetStoreScopes(['12534']), '美国主店（12534）');

    catalog.editDatasetProject(catalog.datasetProjects[0]);
    assert.equal(catalog.datasetEditingTokenId, 'tok_reader');
    assert.equal(catalog.datasetCreateForm.project_id, 'polabel2');
    assert.equal(JSON.stringify(catalog.selectedDatasetScopes), JSON.stringify(['listing-daily-v1']), '编辑已有项目才回填已授权数据表');
    assert.equal(JSON.stringify(catalog.datasetStoreScopes()), JSON.stringify(['12534']));
    catalog.cancelDatasetProjectEdit();
    assert.equal(JSON.stringify(catalog.selectedDatasetScopes), JSON.stringify([]), '取消编辑后回到新增表单时不应默认勾选数据表');
    catalog.editDatasetProject(catalog.datasetProjects[0]);
    catalog.toggleDatasetStore(catalog.datasetStores[1]);
    let updateRequest = null;
    sandbox.window.apiPut = async (url, body) => {
      updateRequest = { url, body };
      return { project_id: 'polabel2', token_id: 'tok_reader', token: 'visible-token', dataset_scopes: body.dataset_scopes, store_scopes: body.store_scopes, need_restart: true };
    };
    await catalog.saveDatasetProjectScopes();
    assert.equal(JSON.stringify(updateRequest), JSON.stringify({
      url: '/api/datasources/datasets/projects/tok_reader',
      body: { dataset_scopes: ['listing-daily-v1'], store_scopes: ['12534', '12536'] },
    }));
    assert.equal(catalog.datasetProjects[0].token, 'visible-token');
    assert.equal(catalog.datasetProjectsNeedRestart, true);
    assert.equal(JSON.stringify(catalog.selectedDatasetScopes), JSON.stringify([]), '保存编辑后回到新增表单时不应保留旧项目的数据表选择');

    let createRequest = null;
    sandbox.window.apiPost = async (url, body) => {
      createRequest = { url, body };
      return { project_id: 'reader', token_id: 'tok_new', token: 'secret', need_restart: true };
    };
    catalog.datasetCreateForm = { project_id: 'reader' };
    catalog.toggleDatasetScope('listing-daily-v1');
    catalog.toggleDatasetScope('return-reason-detail-v1');
    catalog.toggleAllDatasetStores(true);
    await catalog.createDatasetProjectToken();
    assert.equal(JSON.stringify(createRequest), JSON.stringify({
      url: '/api/datasources/datasets/projects',
      body: { project_id: 'reader', dataset_scopes: ['listing-daily-v1', 'return-reason-detail-v1'], store_scopes: ['12534', '12536'] },
    }));
    assert.equal(catalog.datasetProjectsNeedRestart, true, '创建下游项目后必须提示重启');
    assert.equal(JSON.stringify(catalog.selectedDatasetScopes), JSON.stringify([]), '创建完成后下一次新增项目不应默认沿用数据表选择');

    let deleteURL = '';
    sandbox.window.syncConfirm = async () => true;
    sandbox.window.apiDelete = async (url) => {
      deleteURL = url;
      return { project_id: 'polabel2', token_id: 'tok_reader', need_restart: true };
    };
    await catalog.deleteDatasetProject(catalog.datasetProjects[0]);
    assert.equal(deleteURL, '/api/datasources/datasets/projects/tok_reader');
    assert.equal(catalog.datasetProjects.some((project) => project.token_id === 'tok_reader'), false);
    assert.equal(catalog.datasetProjectsNeedRestart, true);

    let exportURL = '';
    sandbox.window.apiDownload = async (url) => {
      exportURL = url;
      return { rows: 2 };
    };
    catalog.toggleDatasetExportStore(catalog.datasetStores[0]);
    assert.equal(JSON.stringify(catalog.datasetExport.stores), JSON.stringify(['12534']));
    catalog.toggleAllDatasetExportStores(true);
    assert.equal(JSON.stringify(catalog.datasetExport.stores), JSON.stringify(['12534', '12536']));
    catalog.datasetExport.date_from = '2026-08-14';
    catalog.datasetExport.date_to = '2026-08-15';
    await catalog.exportDatasetCSV();
    assert.equal(exportURL, '/api/datasources/datasets/listing-daily-v1/export?date_from=2026-08-14&date_to=2026-08-15&stores=12534%2C12536');
    assert.equal(catalog.datasetExportRows, 2);
  }

  let request = null;
  sandbox.window.syncConfirm = async () => true;
  sandbox.window.apiPost = async (url, body) => {
    request = { url, body };
    return { message: '已请求取消' };
  };

  // 正式报表切卡按账号+店铺读取固定报告状态。
  {
    const template = fs.readFileSync(__dirname + '/../templates/sync_manage.html', 'utf8');
    assert.match(template, /正式报表校验/);
    assert.match(template, /saveReportExportBatch\(\)/);
    assert.match(template, /reportStatusText\(row\)/);

    const reportCalls = [];
    sandbox.window.apiGet = async (url) => {
      reportCalls.push({ method: 'GET', url });
      if (url === '/api/config') return { accounts: [], endpoints: [] };
      if (url === '/api/catalog') return { templates: [], accounts: [] };
      if (url === '/api/tasks?page=1&page_size=50') return { items: [] };
      if (url === '/api/report-exports/config') return {
        available_types: ['fba_customer_returns', 'fba_customer_shipment_sales'],
        report_exports: [{
          type: 'fba_customer_returns', enabled: true, account: 'sc_us', seller_id: 'SELLER-1',
          store_id: 'STORE-1', region: 'na', marketplace_ids: ['ATVPDKIKX0DER'], cron: '0 4 * * *', window_days: 3,
        }],
      };
      if (url === '/api/report-exports/status?type=fba_customer_returns&account=sc_us&store_id=STORE-1') return {
        configured: true,
        latest_task: { status: 'success', rows: 18, finished_at: '2026-08-13T04:00:00Z' },
        differences: { database_missing: 1, report_missing: 2, value_mismatch: 3 },
      };
      throw new Error('unexpected GET ' + url);
    };
    const report = sandbox.window.syncManage();
    await report.load();
    assert.equal(report.reportExportConfigs.length, 1);
    assert.deepEqual(report.reportAvailableTypes, ['fba_customer_returns', 'fba_customer_shipment_sales']);
    assert.equal(report.reportTypeLabel('fba_customer_shipment_sales'), 'FBA 发货销售');
    assert.equal(report.reportStatusText(report.reportExportConfigs[0]), '已完成');
    assert.equal(report.reportDifferenceFor(report.reportExportConfigs[0], 'database_missing'), 1);
  }

  // 报表检验仅提交共同参数；店铺范围、Seller ID 和 Marketplace ID 全部由后端按账号全局选择解析。
  {
    const report = sandbox.window.syncManage();
    report.reportExportConfigs = [{
      type: 'fba_customer_returns', enabled: true, account: 'sc_us', seller_id: 'SELLER-OLD',
      store_id: 'OLD', region: 'na', marketplace_ids: ['OLD-MKT'], cron: '0 4 * * *', window_days: 3,
    }];
    report.reportBatch = { type: 'fba_customer_returns', account: 'sc_us', region: 'na', cron: '0 5 * * *', window_days: 7, enabled: true };
    let put = null;
    sandbox.window.apiPut = async (url, body) => { put = { url, body }; return { message: '已保存' }; };
    report.loadReportExport = async () => {};
    await report.saveReportExportBatch();
    assert.equal(put.url, '/api/report-exports/config');
    assert.equal(put.body.report_exports.length, 1);
    assert.equal(JSON.stringify(put.body.report_exports[0]), JSON.stringify({
      type: 'fba_customer_returns', enabled: true, account: 'sc_us', region: 'na', cron: '0 5 * * *', window_days: 7,
    }));

    report.reportBatch.type = 'fba_customer_shipment_sales';
    await report.saveReportExportBatch();
    assert.equal(put.body.report_exports[0].type, 'fba_customer_shipment_sales');
    report.reportBatch.type = 'fba_customer_returns';

    report.reportStatuses[report.reportScopeKey(report.reportExportConfigs[0])] = {
      latest_task: { status: 'success' }, differences: { database_missing: 2 },
    };
    assert.equal(report.reportStatusText(report.reportExportConfigs[0]), '已完成');
    assert.equal(report.reportDifferenceFor(report.reportExportConfigs[0], 'database_missing'), 2);

    report.reportExportConfigs = [
      { type: 'fba_customer_returns', account: 'sc_us', store_id: 'OLD' },
      { type: 'fba_customer_returns', account: 'sc_us', store_id: '1001' },
      { type: 'fba_customer_returns', account: 'sc_us', store_id: '1002' },
    ];
    sandbox.window.syncConfirm = async () => true;
    await report.deleteReportExport(report.reportExportConfigs[1]);
    assert.deepEqual(put.body.report_exports.map(row => row.store_id), ['OLD', '1002']);
  }

  // 日维预览固定使用筛选和分页合同，NULL 显示短横线，错误和空结果分别留在组件状态。
  {
    const previewCalls = [];
    sandbox.window.apiGet = async (url) => {
      previewCalls.push(url);
      if (url.startsWith('/api/datasets/listing-daily-v1/preview?')) {
        return {
          items: [{ business_date: '2026-08-12', store: 'US', asin: 'B001', sku: 'SKU-1', sales_units: null }],
          page: 2, page_size: 20, total: 41,
        };
      }
      throw new Error('unexpected GET ' + url);
    };
    const preview = sandbox.window.dataSources();
    preview.dailyPreviewFilters = {
      date_from: '2026-08-01', date_to: '2026-08-12', store: 'US & West', asin: 'B001', sku: 'SKU/1', page: 2, page_size: 20,
    };
    await preview.loadDailyPreview();
    assert.equal(previewCalls[0], '/api/datasets/listing-daily-v1/preview?date_from=2026-08-01&date_to=2026-08-12&store=US+%26+West&asin=B001&sku=SKU%2F1&page=2&page_size=20');
    assert.equal(preview.dailyPreviewValue(preview.dailyPreviewItems[0].sales_units), '—');
    assert.equal(preview.dailyPreviewIdentity({ store: null, store_id: 'US' }, 'store', 'store_id'), 'US');
  assert.equal(preview.dailyPreviewPages, 3);
  assert.equal(preview.dailyPreviewError, '');
    assert.equal(preview.dailyPreviewStatusText({ is_provisional: true, is_verified: false }), '未验证');
    assert.equal(preview.dailyPreviewStatusText({ is_provisional: false, is_verified: true }), '已验证');

    preview.dailyPreviewFilters.page = 1;
    await preview.changeDailyPreviewPage(2);
    assert.equal(preview.dailyPreviewFilters.page, 2);
    assert.equal(previewCalls[1], '/api/datasets/listing-daily-v1/preview?date_from=2026-08-01&date_to=2026-08-12&store=US+%26+West&asin=B001&sku=SKU%2F1&page=2&page_size=20');

    sandbox.window.apiGet = async () => { throw new Error('日维查询失败'); };
    await preview.loadDailyPreview();
    assert.equal(preview.dailyPreviewError, '日维查询失败');
    assert.equal(preview.dailyPreviewItems.length, 0);
  }

  // 批量启用必须按所选顺序复用单接口 API，并在任一项成功后提示重启。
  const batch = sandbox.window.syncManage();
  batch.catalog = { accounts: [{ id: 'sc_us_1', name: '自营领星' }], templates: [
    { key: 'listing', enabled_accounts: [] },
    { key: 'margin', enabled_accounts: [] },
  ] };
  batch.catalogBatchAccount = 'sc_us_1';
  batch.catalogBatchKeys = ['listing', 'margin'];
  batch.load = async () => {};
  const batchRequests = [];
  sandbox.window.apiPost = async (url, body) => {
    batchRequests.push({ url, body });
    return { need_restart: true };
  };
  await batch.enableCatalogBatch();
  assert.equal(JSON.stringify(batchRequests), JSON.stringify([
    { url: '/api/catalog/enable', body: { key: 'listing', account: 'sc_us_1' } },
    { url: '/api/catalog/enable', body: { key: 'margin', account: 'sc_us_1' } },
  ]));
  assert.equal(batch.needRestart, true);
  assert.equal(JSON.stringify(batch.catalogBatchKeys), '[]');

  // 手动同步先确认摘要；取消时不发请求，确认后保留原有按接口 fan-out 和店铺范围合同。
  const manual = sandbox.window.syncManage();
  manual.endpoints = [
    { name: 'stores', display: '店铺列表', account_id: 'sc_us_1', path: '/stores', iterate_by_store: false },
    { name: 'inventory', display: 'FBA 库存', account_id: 'sc_us_1', path: '/inventory', iterate_by_store: true },
  ];
  manual.accountNames = { sc_us_1: '自营领星' };
  manual.form.accounts = ['sc_us_1'];
  manual.form.types = ['/stores', '/inventory'];
  manual.storesByAccount = { sc_us_1: { selected: { '1001': true } } };
  manual.load = async () => {};
  const syncCalls = [];
  sandbox.window.apiPost = async (url, body) => {
    syncCalls.push({ url, body });
    return { task_id: 1 };
  };
  let confirmMessage = '';
  sandbox.window.syncConfirm = async (message) => { confirmMessage = message; return false; };
  await manual.triggerSync();
  assert.equal(syncCalls.length, 0, '取消确认后不得创建同步任务');
  assert.match(confirmMessage, /账号：自营领星/);
  assert.match(confirmMessage, /数据类型：店铺列表、FBA 库存/);
  assert.match(confirmMessage, /任务：2 个/);
  assert.match(confirmMessage, /店铺：已选 1 家店铺/);
  sandbox.window.syncConfirm = async () => true;
  await manual.triggerSync();
  assert.equal(JSON.stringify(syncCalls), JSON.stringify([
    { url: '/api/sync/stores', body: {} },
    { url: '/api/sync/inventory', body: { store_sids: ['1001'] } },
  ]));

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

  // 三张日志卡各自读取真实历史表；核对记录通过报告审计号回到对应下载记录。
  sandbox.window.__PAGE__ = { reportTypes: ['fba_customer_returns'] };
  const historyLogs = sandbox.window.logsPage();
  historyLogs.filters.account = 'sc_us';
  historyLogs.filters.report_type = 'fba_customer_returns';
  historyLogs.filters.date_from = '2026-08-01';
  historyLogs.filters.date_to = '2026-08-25';
  const historyCalls = [];
  sandbox.window.apiGet = async (url) => {
    historyCalls.push(url);
    if (url.startsWith('/api/report-exports/history?')) return { items: [{ report_audit_id: 93 }], total: 1 };
    if (url.startsWith('/api/report-reconciliations?')) return { items: [{ report_audit_id: 93, business_date: '2026-08-24' }], total: 1 };
    throw new Error('unexpected history URL ' + url);
  };
  await historyLogs.switchTab('report');
  assert.equal(historyCalls[0], '/api/report-exports/history?account=sc_us&type=fba_customer_returns&date_from=2026-08-01&date_to=2026-08-25&page=1&page_size=20');
  await historyLogs.switchTab('reconciliation');
  assert.equal(historyCalls[1], '/api/report-reconciliations?account=sc_us&type=fba_customer_returns&date_from=2026-08-01&date_to=2026-08-25&page=1&page_size=20');
  historyLogs.openReportAudit(93);
  assert.equal(historyCalls[2], '/api/report-exports/history?audit_id=93&account=sc_us&type=fba_customer_returns&date_from=2026-08-01&date_to=2026-08-25&page=1&page_size=20');

  {
    const logsTemplate = fs.readFileSync(__dirname + '/../templates/logs.html', 'utf8');
    assert.match(logsTemplate, /接口同步数据/);
    assert.match(logsTemplate, /接口下载报告/);
    assert.match(logsTemplate, /核对数据/);
    assert.match(logsTemplate, /openReportAudit\(row\.report_audit_id\)/);
  }

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

  // 定时调度批量保存复用现有逐行 API；窗口天数只应用到支持日期范围的接口。
  const scheduleBatch = sandbox.window.syncManage();
  scheduleBatch.schedule = [
    scheduleBatch.normalizeRow({
      name: 'sc_sales', display: '销量', account: 'sc_us', cron: '0 1 * * *', enabled: true,
      rate: { bucket: 1, interval_ms: 1000 }, store_sids: [], date_range_capable: true, window_days: 3,
    }),
    scheduleBatch.normalizeRow({
      name: 'sc_inventory', display: '库存', account: 'sc_us', cron: '0 2 * * *', enabled: true,
      rate: { bucket: 1, interval_ms: 1000 }, store_sids: [], date_range_capable: false, window_days: 0,
    }),
  ];
  for (const row of scheduleBatch.schedule) scheduleBatch.scheduleBaseline[row.name] = scheduleBatch.rowSnap(row);
  scheduleBatch.scheduleSelected = ['sc_sales', 'sc_inventory'];
  scheduleBatch.scheduleBatch = { enabled: 'disabled', cron: '0 5 * * *', window_days: 7 };
  const scheduleBatchCalls = [];
  sandbox.window.apiPut = async (url, body) => {
    scheduleBatchCalls.push({ url, body });
    return { message: '已保存', need_restart: false };
  };
  await scheduleBatch.saveScheduleBatch();
  assert.deepEqual(scheduleBatchCalls.map(call => [call.url, call.body.enabled, call.body.cron, call.body.window_days]), [
    ['/api/endpoints/sc_sales', false, '0 5 * * *', 7],
    ['/api/endpoints/sc_inventory', false, '0 5 * * *', 0],
  ]);
  assert.equal(JSON.stringify(scheduleBatch.scheduleSelected), '[]');

  // 数据集字段双栏：项目与 token ID 共同标识字段清单；不接触明文 bearer。
  const fieldCalls = [];
  const fieldLoad = {
    dataset_id: 'listing-daily-v1',
    project_id: 'project-a',
    token_id: 'token-a',
    available_fields: ['store', 'sales', 'impressions'],
    fields: ['store'],
  };
  const projectBLoad = {
    dataset_id: 'listing-daily-v1',
    project_id: 'project-b',
    token_id: 'token-b',
    available_fields: ['store', 'sales', 'impressions'],
    fields: ['impressions'],
  };
  let projectResponse = { projects: [
    { project_id: 'project-a', token_id: 'token-a', fields: ['store'] },
    { project_id: 'project-b', token_id: 'token-b', fields: ['impressions'] },
  ] };
  sandbox.window.apiGet = async (url) => {
    fieldCalls.push({ method: 'GET', url });
    if (url === '/api/datasources/datasets/listing-daily-v1/fields/config') return projectResponse;
    if (url === '/api/datasources/datasets/listing-daily-v1/fields?project_id=project-a&token_id=token-a') return fieldLoad;
    if (url === '/api/datasources/datasets/listing-daily-v1/fields?project_id=project-b&token_id=token-b') return projectBLoad;
    throw new Error('unexpected dataset GET ' + url);
  };
  sandbox.window.apiPut = async (url, body) => {
    fieldCalls.push({ method: 'PUT', url, body });
    return { message: '已保存' };
  };
  const fields = sandbox.window.dataSources();
  await fields.loadDatasetProjects();
  assert.equal(JSON.stringify(fieldCalls.slice(0, 2).map(call => call.url)), JSON.stringify([
    '/api/datasources/datasets/listing-daily-v1/fields/config',
    '/api/datasources/datasets/listing-daily-v1/fields?project_id=project-a&token_id=token-a',
  ]));
  assert.equal(fields.selectedProjectId, 'project-a');
  assert.equal(fields.selectedTokenId, 'token-a');
  assert.ok(fields.fieldStateByProject[JSON.stringify(['project-a', 'token-a'])]);
  assert.equal(fields.fieldStateByProject['project-a'], undefined);
  assert.equal(fields.fieldGroups.length, 1);
  assert.equal(JSON.stringify(fields.selectedFields), JSON.stringify(['store']));
  assert.equal(fields.fieldsDirty, false);
  assert.equal(JSON.stringify(fields.displayFields(fields.fieldGroups[0]).map(f => f.name)), JSON.stringify(['store', 'sales', 'impressions']));
  assert.equal(fields.catalogFieldCount, 3, '左栏必须保留完整字段目录，不因右栏已选字段而减少');
  fields.addField('sales');
  assert.equal(JSON.stringify(fields.selectedFields), JSON.stringify(['store', 'sales']));
  assert.equal(fields.fieldsDirty, true);
  fields.removeField('store');
  assert.equal(JSON.stringify(fields.selectedFields), JSON.stringify(['sales']));
  await fields.saveDatasetFields();
  assert.equal(JSON.stringify(fieldCalls.at(-1)), JSON.stringify({
    method: 'PUT',
    url: '/api/datasources/datasets/listing-daily-v1/fields?project_id=project-a&token_id=token-a',
    body: { project_id: 'project-a', token_id: 'token-a', fields: ['sales'] },
  }));
  assert.equal(fields.fieldsDirty, false);

  await fields.selectProject(JSON.stringify(['project-b', 'token-b']));
  assert.equal(JSON.stringify(fields.selectedFields), JSON.stringify(['impressions']));
  fields.removeField('impressions');
  assert.equal(fields.fieldsDirty, true);
  await fields.selectProject(JSON.stringify(['project-a', 'token-a']));
  assert.equal(JSON.stringify(fields.selectedFields), JSON.stringify(['sales']));
  assert.equal(fields.fieldsDirty, false);

  // 项目详情读取失败时不得继续展示上一项目字段，也不能误保存到新项目。
  const putCount = fieldCalls.filter(call => call.method === 'PUT').length;
  delete fields.fieldStateByProject[JSON.stringify(['project-b', 'token-b'])];
  sandbox.window.apiGet = async (url) => {
    if (url.endsWith('project_id=project-b&token_id=token-b')) throw new Error('项目字段不可用');
    throw new Error('unexpected dataset GET ' + url);
  };
  await fields.selectProject(JSON.stringify(['project-b', 'token-b']));
  assert.equal(fields.fieldsError, '项目字段不可用');
  assert.equal(fields.fieldGroups.length, 0);
  assert.equal(fields.fieldsDirty, false);
  assert.equal(fields.fieldStateByProject[JSON.stringify(['project-b', 'token-b'])], undefined);
  await fields.saveDatasetFields();
  assert.equal(fieldCalls.filter(call => call.method === 'PUT').length, putCount);
  await fields.selectProject(JSON.stringify(['project-a', 'token-a']));

  // 快速切换项目时，迟到的旧请求不能覆盖当前项目的字段状态。
  const concurrent = sandbox.window.dataSources();
  concurrent.datasetProjects = [
    { project_id: 'project-a', token_id: 'token-a', key: JSON.stringify(['project-a', 'token-a']), label: 'project-a / token-a' },
    { project_id: 'project-b', token_id: 'token-b', key: JSON.stringify(['project-b', 'token-b']), label: 'project-b / token-b' },
  ];
  concurrent.selectedProjectKey = JSON.stringify(['project-a', 'token-a']);
  concurrent.selectedProjectId = 'project-a';
  concurrent.selectedTokenId = 'token-a';
  const pending = {};
  sandbox.window.apiGet = (url) => new Promise((resolve) => { pending[url] = resolve; });
  const slowA = concurrent.loadDatasetFields();
  const fastB = concurrent.selectProject(JSON.stringify(['project-b', 'token-b']));
  pending['/api/datasources/datasets/listing-daily-v1/fields?project_id=project-b&token_id=token-b'](projectBLoad);
  await fastB;
  pending['/api/datasources/datasets/listing-daily-v1/fields?project_id=project-a&token_id=token-a'](fieldLoad);
  await slowA;
  assert.equal(concurrent.selectedProjectId, 'project-b');
  assert.equal(JSON.stringify(concurrent.selectedFields), JSON.stringify(['impressions']));

  // 响应身份和分组键必须固定，错误数据不能替换当前清单。
  sandbox.window.apiGet = async () => ({ ...fieldLoad, dataset_id: 'other-dataset' });
  await fields.loadDatasetFields();
  assert.equal(fields.fieldsError, '数据集字段响应格式错误');
  assert.equal(fields.fieldGroups.length, 1);
  sandbox.window.apiGet = async () => ({
    dataset_id: 'listing-daily-v1',
    project_id: 'project-a',
    token_id: 'token-a',
    available_fields: ['sales', 'sales'],
    fields: [],
  });
  await fields.loadDatasetFields();
  assert.equal(fields.fieldsError, '数据集字段重复: sales');
  assert.equal(fields.fieldGroups.length, 1);

  // 加载失败不能把已有选择静默清空；保存失败也必须保留待保存选择。
  fields.selectedFields = ['sales'];
  fields.savedFields = ['sales'];
  fields.fieldsError = '';
  sandbox.window.apiGet = async () => { throw new Error('字段服务不可用'); };
  await fields.loadDatasetFields();
  assert.equal(fields.fieldsError, '字段服务不可用');
  assert.equal(JSON.stringify(fields.selectedFields), JSON.stringify(['sales']));
  assert.equal(fields.fieldGroups.length, 1, '加载失败时应保留旧字段分组以继续显示当前选择');
  fields.addField('impressions');
  fields.fieldsError = '';
  sandbox.window.apiPut = async () => { throw new Error('保存被拒绝'); };
  await fields.saveDatasetFields();
  assert.equal(fields.fieldsSaveError, '保存被拒绝');
  assert.equal(JSON.stringify(fields.selectedFields), JSON.stringify(['sales', 'impressions']));
  assert.equal(fields.fieldsDirty, true);

  // 服务端返回空字段组是合法空态，不应被当成加载错误。
  sandbox.window.apiGet = async () => ({ dataset_id: 'listing-daily-v1', project_id: 'project-a', token_id: 'token-a', available_fields: [], fields: [] });
  fields.selectedProjectKey = JSON.stringify(['project-a', 'token-a']);
  fields.selectedProjectId = 'project-a';
  fields.selectedTokenId = 'token-a';
  await fields.loadDatasetFields();
  assert.equal(fields.fieldsError, '');
  assert.equal(fields.catalogFieldCount, 0);
  assert.equal(fields.selectedFieldCount, 0);

  // 只有单项目详情时，首次 GET 的响应也可直接作为当前项目清单。
  const single = sandbox.window.dataSources();
  const singleCalls = [];
  sandbox.window.apiGet = async (url) => {
    singleCalls.push(url);
    if (url === '/api/datasources/datasets/listing-daily-v1/fields/config') {
      return { projects: [{ project_id: 'project-a', token_id: 'token-a', fields: ['store'] }] };
    }
    return { ...fieldLoad, project_id: 'project-a', token_id: 'token-a' };
  };
  await single.loadDatasetProjects();
  assert.equal(single.selectedProjectId, 'project-a');
  assert.equal(JSON.stringify(singleCalls), JSON.stringify([
    '/api/datasources/datasets/listing-daily-v1/fields/config',
    '/api/datasources/datasets/listing-daily-v1/fields?project_id=project-a&token_id=token-a',
  ]));

  // 尚未登记项目/Token 是合法空态；页面保留双栏骨架，不显示红色加载错误。
  const empty = sandbox.window.dataSources();
  sandbox.window.apiGet = async () => ({ projects: [] });
  await empty.loadDatasetProjects();
  assert.equal(empty.fieldsError, '');
  assert.equal(empty.hasDatasetSelection, false);
  assert.equal(empty.projectOptions.length, 0);
  assert.equal(empty.fieldGroups.length, 0);

  // 日维字段只做固定展示分组，不改变 API 字段名或选择结果。
  const grouped = sandbox.window.dataSources();
  sandbox.window.apiGet = async () => ({
    dataset_id: 'listing-daily-v1',
    project_id: 'project-a',
    token_id: 'token-a',
    available_fields: [
      'sales_units', 'sales_amount', 'returns_qty',
      'inventory_sellable', 'inventory_sellable_source',
      'sessions_desktop', 'review_count', 'rating',
      'sp_spend', 'sp_spend_source', 'sd_sales', 'hsa_orders', 'sb_spend',
      'is_provisional',
    ],
    fields: ['sales_units', 'inventory_sellable', 'sessions_desktop', 'sp_spend', 'sd_sales', 'hsa_orders', 'sb_spend'],
  });
  grouped.selectedProjectKey = JSON.stringify(['project-a', 'token-a']);
  grouped.selectedProjectId = 'project-a';
  grouped.selectedTokenId = 'token-a';
  await grouped.loadDatasetFields();
  assert.equal(JSON.stringify(grouped.fieldGroups.map(group => group.source)), JSON.stringify([
    '销量', '库存', 'Performance', 'SP', 'SD', 'HSA', 'SB', '状态',
  ]));
  assert.equal(JSON.stringify(grouped.fieldGroups.map(group => group.fields.map(field => field.name))), JSON.stringify([
    ['sales_units', 'sales_amount', 'returns_qty'],
    ['inventory_sellable', 'inventory_sellable_source'],
    ['sessions_desktop', 'review_count', 'rating'],
    ['sp_spend', 'sp_spend_source'],
    ['sd_sales'],
    ['hsa_orders'],
    ['sb_spend'],
    ['is_provisional'],
  ]));
})().catch((error) => {
  process.nextTick(() => { throw error; });
});
