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

// 同步配置标题提示必须支持键盘聚焦，并把“修改时间增量”和“业务日期重拉”说清楚。
{
  const template = fs.readFileSync(__dirname + '/../templates/sync_manage.html', 'utf8');
  assert.match(template, /aria-describedby="sync-modified-time-tip"/);
  assert.match(template, /id="sync-modified-time-tip"[^>]*role="tooltip"/);
  assert.match(template, /订单、Listing、FBA 退货、VC PO/);
  assert.match(template, /销量、Performance、SP、SD、HSA、VC 销量\/库存/);
  assert.match(template, /<span class="block">支持按修改时间补拉：订单、Listing、FBA 退货、VC PO。<\/span>/);
  assert.match(template, /<span class="block">销量、Performance、SP、SD、HSA、VC 销量\/库存按业务日期重拉最近范围。<\/span>/);
  assert.match(template, /fixed left-4 right-4 top-44/);
  assert.match(template, /sm:absolute sm:left-full sm:right-auto/);
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
  assert.match(template, /可添加字段/);
  assert.match(template, /已发布字段/);
  assert.match(template, /固定字段/);
  assert.match(template, /固定字段，不能删除/);
  assert.match(template, /fieldGroupSource\(group\.source\)/);
  assert.match(template, /availableTableFieldGroups/);
  assert.match(template, /Token ID/);
  assert.match(template, /店铺范围/);
  assert.match(template, /createDatasetProjectToken\(\)/);
  assert.match(template, /addTableField\(field\.name\)/);
  assert.match(template, /removeTableField\(field\.name\)/);
  assert.match(template, /h-\[720px\] overflow-y-auto/);
  assert.match(template, /saveDatasetFieldAllowlist\(\)/);
  assert.match(template, /暂无可添加字段/);
  assert.doesNotMatch(template, /<input[^>]*x-model[^>]*(?:table|sql)/i);
  assert.doesNotMatch(template, /type=["']password/i);
  assert.doesNotMatch(template, /CREATE\s+TABLE|ALTER\s+TABLE|DROP\s+TABLE|SQL\s*:/i);
  const dataSourcesTemplate = fs.readFileSync(__dirname + '/../templates/datasources.html', 'utf8');
  assert.match(dataSourcesTemplate, /日维数据预览/);
  assert.match(dataSourcesTemplate, /applyDailyPreviewFilters\(\)/);
  assert.match(dataSourcesTemplate, /dailyPreviewValue/);
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

void (async () => {
  // 数据表目录独立于下游项目：完整业务字段和固定字段先可见，下游项目只带读取范围。
  {
    const catalog = sandbox.window.dataSources();
    sandbox.window.apiGet = async (url) => {
      if (url === '/api/datasources/datasets/catalog') {
        return { datasets: [
          { id: 'listing-daily-v1', name: 'Listing 日维指标表', kind: 'daily', source: 'listing_dimensions + listing_daily_metrics', grain: 'store + channel + asin + sku + business_date' },
          { id: 'return-reason-detail-v1', name: '退货原因明细表', kind: 'detail', source: 'ls_sc_refunds', grain: 'store + license_plate_number' },
        ] };
      }
      assert.equal(url, '/api/datasources/datasets/listing-daily-v1/fields/config');
      return {
        dataset_id: 'listing-daily-v1',
        dataset_name: 'Listing 日维指标表',
        fixed_fields: ['store', 'channel', 'asin', 'sku', 'business_date', 'updated_at', 'is_provisional', 'verification_status'],
        available_fields: ['sales_units', 'inventory_sellable', 'sessions_total'],
        configured_fields: ['sales_units', 'sessions_total'],
        projects: [{ project_id: 'polabel2', token_id: 'tok_reader', store_scopes: ['12534'] }],
      };
    };
    await catalog.loadDatasetCatalog();
    assert.equal(catalog.datasetName, 'Listing 日维指标表');
    assert.equal(catalog.datasetDefinitions.length, 2, '下游项目必须能从静态目录选择多张数据表');
    assert.equal(catalog.fixedFields.length, 8);
    assert.equal(catalog.availableTableFieldCount, 1);
    assert.equal(JSON.stringify(catalog.availableTableFieldGroups.map((group) => group.fields.map((field) => field.name))), JSON.stringify([['inventory_sellable']]));
    assert.match(catalog.fieldGroupSource('销量'), /ls_sc_sales_report/);
    assert.equal(JSON.stringify(catalog.tableSelectedFields), JSON.stringify(['sales_units', 'sessions_total']));
    assert.equal(catalog.tableFieldDirty, false);
    catalog.addTableField('inventory_sellable');
    assert.equal(JSON.stringify(catalog.tableSelectedFields), JSON.stringify(['sales_units', 'sessions_total', 'inventory_sellable']));
    assert.equal(catalog.availableTableFieldCount, 0);
    assert.equal(JSON.stringify(catalog.publishedTableFields.map((field) => field.name)), JSON.stringify(['sales_units', 'sessions_total', 'inventory_sellable']));
    catalog.removeTableField('sessions_total');
    assert.equal(JSON.stringify(catalog.publishedTableFields.map((field) => field.name)), JSON.stringify(['sales_units', 'inventory_sellable']));
    assert.equal(catalog.availableTableFieldCount, 1);
    catalog.addTableField('sessions_total');
    assert.equal(catalog.tableFieldDirty, true);
    let tableFieldRequest = null;
    sandbox.window.apiPut = async (url, body) => {
      tableFieldRequest = { url, body };
      return { fields: body.fields, need_restart: true };
    };
    await catalog.saveDatasetFieldAllowlist();
    assert.equal(JSON.stringify(tableFieldRequest), JSON.stringify({
      url: '/api/datasources/datasets/listing-daily-v1/fields/config',
      body: { fields: ['sales_units', 'inventory_sellable', 'sessions_total'] },
    }));
    assert.equal(catalog.tableFieldDirty, false);
    assert.equal(catalog.tableFieldsNeedRestart, true);
    assert.equal(catalog.datasetProjects[0].token_id, 'tok_reader');
    assert.equal(JSON.stringify(catalog.datasetProjects[0].store_scopes), JSON.stringify(['12534']));

    const storeCalls = [];
    sandbox.window.apiGet = async (url) => {
      storeCalls.push(url);
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

    let createRequest = null;
    sandbox.window.apiPost = async (url, body) => {
      createRequest = { url, body };
      return { project_id: 'reader', token_id: 'tok_new', token: 'secret' };
    };
    catalog.datasetCreateForm = { project_id: 'reader' };
    catalog.toggleDatasetScope('return-reason-detail-v1');
    await catalog.createDatasetProjectToken();
    assert.equal(JSON.stringify(createRequest), JSON.stringify({
      url: '/api/datasources/datasets/projects',
      body: { project_id: 'reader', dataset_scopes: ['listing-daily-v1', 'return-reason-detail-v1'], store_scopes: ['12534', '12536'] },
    }));

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

  // 报表检验批量配置：选一次账号和共同参数，勾多个店铺后生成多条固定报告配置。
  {
    const report = sandbox.window.syncManage();
    report.reportExportConfigs = [{
      type: 'fba_customer_returns', enabled: true, account: 'sc_us', seller_id: 'SELLER-OLD',
      store_id: 'OLD', region: 'na', marketplace_ids: ['OLD-MKT'], cron: '0 4 * * *', window_days: 3,
    }];
    report.reportBatch = { account: 'sc_us', store_sids: [], region: 'na', cron: '0 5 * * *', window_days: 7, enabled: true };
    report.storesByAccount = { sc_us: { loaded: true, selected: {}, query: '', items: [
      { sid: '1001', store_type: 'SC', seller_id: 'SELLER-1', marketplace_id: 'MKT-1', store_name: '美国店' },
      { sid: '1002', store_type: 'SC', seller_id: 'SELLER-2', marketplace_id: 'MKT-2', store_name: '加拿大店' },
      { sid: '2001', store_type: 'VC', seller_id: 'SELLER-VC', marketplace_id: 'MKT-VC', store_name: 'VC 店' },
    ] } };
    report.selectAllReportStores();
    assert.deepEqual(report.reportBatch.store_sids, ['1001', '1002'], 'FBA 退货报表只能选择 SC 店铺');
    let put = null;
    sandbox.window.apiPut = async (url, body) => { put = { url, body }; return { message: '已保存' }; };
    report.loadReportExport = async () => {};
    await report.saveReportExportBatch();
    assert.equal(put.url, '/api/report-exports/config');
    assert.equal(put.body.report_exports.length, 3);
    assert.deepEqual(put.body.report_exports.slice(1).map(row => [row.store_id, row.seller_id, row.marketplace_ids[0], row.cron, row.window_days]), [
      ['1001', 'SELLER-1', 'MKT-1', '0 5 * * *', 7],
      ['1002', 'SELLER-2', 'MKT-2', '0 5 * * *', 7],
    ]);

    report.reportBatch.type = 'fba_customer_shipment_sales';
    report.reportBatch.store_sids = ['1001'];
    await report.saveReportExportBatch();
    assert.equal(put.body.report_exports.find(row => row.store_id === '1001').type, 'fba_customer_shipment_sales');
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
