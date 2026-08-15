/* web/static/app.js — 公共 Alpine.js 逻辑 + fetch 封装 + 各页组件工厂。
 *
 * 宪法对应：doc/04-api.md（fetch 路径）、doc/05-pages.md（每页 Alpine 组件）。
 *
 * 组织：
 *   1. 全局工具：apiGet / apiPost / 时间格式化 / 脱敏
 *   2. AppRoot：挂在 body 上的全局根组件，维护 toasts / confirmState，供所有页面共享
 *   3. 每页一个独立组件工厂函数（syncManage / logsPage / taskDetail /
 *      dataSources / settingsApi），逻辑互不依赖，改一页不影响其他页
 *
 * 约定：
 *   - 后端返回 {ok, data} / {ok:false, error}
 *   - 时间字段为 RFC3339 字符串或 null
 */

/* ------------------------------------------------------------------ *
 * 1. 全局工具
 * ------------------------------------------------------------------ */

// 同步读取页面上 meta 标记的 secret（如有），用于给 /api 请求加 X-Sync-Secret。
// 由 layout 模板里没有专门写 meta，这里从 body data-secret 取（页面 handler 可扩展注入）。
function syncSecret() {
  const el = document.querySelector('meta[name="sync-secret"]');
  return el ? el.getAttribute('content') : '';
}

// 统一的 fetch 封装：解析 {ok,data,error}。
async function apiRequest(method, url, body) {
  const opt = { method, headers: {} };
  if (body !== undefined) {
    opt.headers['Content-Type'] = 'application/json';
    opt.body = JSON.stringify(body);
  }
  const sec = syncSecret();
  if (sec) opt.headers['X-Sync-Secret'] = sec;
  let resp;
  try {
    resp = await fetch(url, opt);
  } catch (e) {
    throw new Error('网络错误：' + e.message);
  }
  let payload = null;
  try {
    payload = await resp.json();
  } catch (_) {
    throw new Error('响应非 JSON（HTTP ' + resp.status + '）');
  }
  if (!resp.ok || payload.ok === false) {
    const msg = (payload && payload.error) || ('HTTP ' + resp.status);
    throw new Error(msg);
  }
  return payload.data;
}

// apiGet / apiPost / apiPut / apiDelete 直接挂在 window 上方便调用。
// 四者共用 apiRequest（已解析 {ok,data}、失败抛错、自动带 X-Sync-Secret）。
window.apiGet = (url) => apiRequest('GET', url);
window.apiPost = (url, body) => apiRequest('POST', url, body);
window.apiPut = (url, body) => apiRequest('PUT', url, body);
window.apiDelete = (url) => apiRequest('DELETE', url);

// 把任意错误冒泡到全局 toast。页面里 await apiGet(...).catch(toastError) 即可。
window.toastError = function (err) {
  window.dispatchEvent(new CustomEvent('sync-toast', {
    detail: { type: 'error', msg: (err && err.message) ? err.message : String(err) }
  }));
  return null;
};

// toast(type, msg) 是 dispatch 'sync-toast' 的简写；type ∈ success|error|warn|info。
window.toast = function (type, msg) {
  window.dispatchEvent(new CustomEvent('sync-toast', { detail: { type: type || 'info', msg: msg || '' } }));
};

const shanghaiDateTimeFormatter = new Intl.DateTimeFormat('zh-CN', {
  timeZone: 'Asia/Shanghai',
  year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', second: '2-digit',
  hourCycle: 'h23'
});

// 时间格式化：RFC3339 → "3分钟前" / "刚刚" / "HH:mm:ss"；空值返回 '-'。
window.fmtRel = function (iso) {
  if (!iso) return '-';
  const t = new Date(iso);
  if (isNaN(t.getTime())) return '-';
  const diff = (Date.now() - t.getTime()) / 1000;
  if (diff < 5) return '刚刚';
  if (diff < 60) return Math.floor(diff) + ' 秒前';
  if (diff < 3600) return Math.floor(diff / 60) + ' 分钟前';
  if (diff < 86400) return Math.floor(diff / 3600) + ' 小时前';
  if (diff < 86400 * 7) return Math.floor(diff / 86400) + ' 天前';
  return t.toLocaleDateString('zh-CN', { timeZone: 'Asia/Shanghai' });
};

// 绝对时间：RFC3339 → "2026-08-06 12:34:56"。
window.fmtTime = function (iso) {
  if (!iso) return '-';
  const t = new Date(iso);
  if (isNaN(t.getTime())) return '-';
  const parts = Object.fromEntries(
    shanghaiDateTimeFormatter.formatToParts(t)
      .filter(part => part.type !== 'literal')
      .map(part => [part.type, part.value])
  );
  return parts.year + '-' + parts.month + '-' + parts.day +
    ' ' + parts.hour + ':' + parts.minute + ':' + parts.second;
};

// 耗时秒 → "1m 23s" / "12s"。
window.fmtDur = function (sec) {
  if (sec == null) return '-';
  if (sec < 60) return sec + 's';
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return m + 'm ' + s + 's';
};

// 运行时长秒 → "3d 2h" / "5m"。
window.fmtUptime = function (sec) {
  if (!sec && sec !== 0) return '-';
  if (sec < 60) return sec + 's';
  if (sec < 3600) return Math.floor(sec / 60) + 'm';
  if (sec < 86400) return Math.floor(sec / 3600) + 'h';
  return Math.floor(sec / 86400) + 'd ' + Math.floor((sec % 86400) / 3600) + 'h';
};

// Token 到期秒数 → "5 小时后" / "已过期"。
window.fmtExpire = function (sec) {
  if (sec == null) return '-';
  if (sec <= 0) return '已过期';
  if (sec < 3600) return Math.floor(sec / 60) + ' 分钟后';
  if (sec < 86400) return Math.floor(sec / 3600) + ' 小时后';
  return Math.floor(sec / 86400) + ' 天后';
};

// 简单脱敏：取前 4 + **** + 后 2。
window.maskKey = function (s) {
  if (!s) return '';
  if (s.length <= 6) return '****';
  return s.slice(0, 4) + '****' + s.slice(-2);
};

/* ------------------------------------------------------------------ *
 * 2. AppRoot — 全局根组件（toasts / confirm）
 * ------------------------------------------------------------------ */
window.AppRoot = function () {
  return {
    toasts: [],
    confirmState: { open: false, title: '确认', message: '', resolve: null },

    init() {
	  window.syncConfirm = (message, title) => this.confirm(message, title);
      // 任何地方 dispatchEvent sync-toast 都会落成一条 toast
      window.addEventListener('sync-toast', (e) => {
        const t = {
          id: Date.now() + Math.random(),
          type: e.detail.type || 'info',
          msg: e.detail.msg || '',
          show: true
        };
        this.toasts.push(t);
        // 3.5s 后自动隐藏并清理
        setTimeout(() => { t.show = false; }, 3500);
        setTimeout(() => {
          this.toasts = this.toasts.filter(x => x.id !== t.id);
        }, 4200);
      });
    },

    // confirm 返回 Promise<boolean>，模板里点击确定/取消调 confirmResolve(true/false)
    confirm(message, title) {
      return new Promise((resolve) => {
        this.confirmState = {
          open: true,
          title: title || '确认操作',
          message: message || '',
          resolve
        };
      });
    },
    confirmResolve(v) {
      if (this.confirmState.resolve) this.confirmState.resolve(v);
      this.confirmState.open = false;
      this.confirmState.resolve = null;
    }
  };
};

/* ------------------------------------------------------------------ *
 * 3b. syncManage — 同步管理（/sync）
 * ------------------------------------------------------------------ */
window.syncManage = function () {
  return {
    tab: 'manual',
    endpoints: [],        // 手动同步勾选用：[{name,display,account_id,iterate_by_store,window_days}]
    schedule: [],         // 定时调度表：完整 endpoint 配置对象数组（来自 /api/config）
    accounts: [],         // 账号 id 列表（去重保序）
    accountNames: {},     // 账号 id→名称映射（勾选项/店铺网格显示名称而非 ID）
    // 矩阵选择模型：账号（form.accounts）× 数据类型（form.types，键=path）。
    // 两账号同类型接口（name 不同但 path 相同）在 UI 只列一份「数据类型」，避免整段重复；
    // 触发时按「选中账号 × 选中类型」笛卡尔积解析回真实接口 name（见 resolvedEndpoints）。
    // storeSids 不在 form 上集中存：按账号分区，选择态落在 storesByAccount[acc].selected。
    form: { accounts: [], types: [], date_from: '', date_to: '' },
    // ---- T2 店铺网格（按账号分区）----
    // storesByAccount[accountId] = { items:[StoreSummary], loading:false, loaded:true,
    //                                 selected:{sid:true}, query:'' }
    storesByAccount: {},
    // recentTasks 不渲染成任何表格或摘要：任务态一律去 /logs 页看，本页不重复展示。
    // 唯一用途：定时调度 Tab 的 lastRunOf() —— 给每行接口填「上次运行」。
    recentTasks: [],
    needRestart: false,   // 结构性变更后显示重启横幅
    // 定时调度：内联直接编辑（无「编辑」按钮）。scheduleBaseline[name] 存该行加载/保存后的
    // 基线快照，用于「整行 dirty 判定 + 取消回滚」（复用店铺选择的 baseline 模式，保持一致）。
    scheduleBaseline: {},
    scheduleSelected: [],
    scheduleFilter: { account: '', query: '' },
    scheduleBatch: { enabled: 'unchanged', cron: '', window_days: '' },
    scheduleBatchSaving: false,
    // 高级/开发者「手动填合同」折叠区开关（清单没有的接口才用）。默认收起。
    advancedAdd: false,
    // 接口清单（从后端 /api/catalog 拉）：templates=模板列表，accounts=可选账号。
    catalog: { templates: [], accounts: [] },
    catalogBatchAccount: '',
    catalogBatchKeys: [],
    reportExportConfigs: [],
    reportAvailableTypes: ['fba_customer_returns'],
    reportBatch: { type: 'fba_customer_returns', account: '', store_sids: [], region: 'na', cron: '0 4 * * *', window_days: 3, enabled: true },
    reportStatuses: {},
    reportExportLoading: false,
    reportExportSaving: false,
    reportExportError: '',
    // 保守限流默认：桶 1 / 间隔 1000ms（对齐 otherlingxinggithub.md §3「业务 API ≥0.6s」留足余量）。
    // extra_params_text 是 JSON 文本输入，保存时解析成对象；解析失败拦截不发请求。
    addForm: { name: '', display: '', account: '', path: '', method: 'GET', table: '', record_id_fields: '', cron: '', bucket: 1, interval_ms: 1000, multi_interval_ms: 0, window_days: 0, iterate_by_store: false, store_param_name: '', extra_params_text: '' },

    // Alpine 自动调一次（早于模板里的 x-init="load()"）。
    // 这里只挂 $watch：选中接口变化时，懒加载涉及账号的店铺（T2）。
    init() {
      if (this.$watch) {
        // 账号或类型选择变化 → 懒加载涉及账号的店铺（T2）。
        const reload = () => { for (const acc of this.storeAccounts) this.ensureStores(acc); };
        this.$watch('form.accounts', reload);
        this.$watch('form.types', reload);
      }
      // 深链：/sync?tab=add（或旧链 &add=1）→ 直接落到「添加接口」Tab。
      // 添加接口是独立 Tab（不再挂在定时调度表下面），此处兼容旧的 add=1 写法。
      const q = new URLSearchParams(window.location.search);
      const t = q.get('tab');
      if (q.get('add') === '1' || t === 'add') this.tab = 'add';
      else if (t === 'schedule') this.tab = 'schedule';
      else if (t === 'reports') this.tab = 'reports';
    },
    // blankAddForm 返回添加表单的初始/重置态：与 data 里的 addForm 初值保持一致，
    // 保存成功后调用它清空表单（保守限流默认桶 1 / 间隔 1000ms）。
    blankAddForm() {
      return { name: '', display: '', account: '', path: '', method: 'GET', table: '', record_id_fields: '', cron: '', bucket: 1, interval_ms: 1000, multi_interval_ms: 0, window_days: 0, iterate_by_store: false, store_param_name: '', extra_params_text: '' };
    },
    // 本页不再有 5s 轮询：手动 Tab 只做「选接口 → 触发」，触发结果看 toast，任务态看 /logs；
    // 定时调度 Tab 的「上次运行」在 load() 时取一次即够。故无 timer，也就不需要 destroy()。
    switchTab(t) {
      this.tab = t;
    },

    async load() {
      // 完整配置（含 cron/rate/store_sids/iterate_by_store），定时调度 Tab 与 T1/T2 都依赖它
      const cfg = await window.apiGet('/api/config').catch(window.toastError);
      if (cfg) {
        // 归一化每行：保证 rate 存在、补 store_sids_text 供内联输入；随后建立 dirty 基线。
        this.schedule = (cfg.endpoints || []).map(e => this.normalizeRow(e));
        this.scheduleBaseline = {};
        for (const e of this.schedule) this.scheduleBaseline[e.name] = this.rowSnap(e);
        // 保留 path（矩阵去重键）/iterate_by_store/window_days：T1 类型合并与 T2 网格判定都需要
        this.endpoints = this.schedule.map(e => ({
          name: e.name, display: e.display, account_id: e.account, path: e.path,
          iterate_by_store: !!e.iterate_by_store, store_type: e.store_type || '', window_days: e.window_days || 0,
          date_field: e.date_field || '', date_offset_days: e.date_offset_days || 0,
          date_range_capable: !!e.date_range_capable
        }));
        // 账号 ID→名称映射：勾选项与店铺网格显示账号名（自营领星）而非机器 ID（sc_us_1）。
        this.accountNames = {};
        for (const a of (cfg.accounts || [])) this.accountNames[a.id] = a.name || a.id;
        const seen = new Set();
        this.accounts = [];
        for (const e of this.endpoints) {
          if (e.account_id && !seen.has(e.account_id)) { seen.add(e.account_id); this.accounts.push(e.account_id); }
        }
        // 配置若变了，已选账号/类型可能失效，剔除不再存在的（选中态不跨次保留属决策③的自然结果）
        this.form.accounts = this.form.accounts.filter(a => this.accounts.includes(a));
        const validPaths = new Set(this.endpoints.map(e => e.path));
        this.form.types = this.form.types.filter(p => validPaths.has(p));
        this.pruneUnavailableTypes();
      }
      await this.loadCatalog();
      await this.loadRecentTasks();
      await this.loadReportExport();
    },
    async loadReportExport() {
      this.reportExportLoading = true;
      this.reportExportError = '';
      try {
        const response = await window.apiGet('/api/report-exports/config') || {};
        this.reportExportConfigs = Array.isArray(response.report_exports) ? response.report_exports.slice() : [];
        this.reportAvailableTypes = Array.isArray(response.available_types) && response.available_types.length ? response.available_types.slice() : ['fba_customer_returns'];
        this.reportStatuses = {};
        const statuses = await Promise.all(this.reportExportConfigs.map(async row => {
          const query = new URLSearchParams({ type: row.type || 'fba_customer_returns', account: row.account, store_id: row.store_id });
          const status = await window.apiGet('/api/report-exports/status?' + query.toString());
          return [this.reportScopeKey(row), status];
        }));
        for (const [key, status] of statuses) this.reportStatuses[key] = status;
      } catch (error) {
        this.reportExportError = errorMessage(error, '未能读取正式报表');
      } finally {
        this.reportExportLoading = false;
      }
    },
    reportScopeKey(row) { return [row.type, row.account, row.store_id].join('|'); },
    reportTypeLabel(type) {
      return { fba_customer_returns: 'FBA 退货', fba_customer_shipment_sales: 'FBA 发货销售' }[type] || type || '未知正式报表';
    },
    reportStatusFor(row) { return this.reportStatuses[this.reportScopeKey(row)] || { latest_task: null, differences: {} }; },
    reportStatusText(row) {
      const task = this.reportStatusFor(row).latest_task;
      if (!task) return row.enabled ? '等待运行' : '未启用';
      const labels = { success: '已完成', done: '已完成', completed: '已完成', error: '失败', failed: '失败', running: '运行中', pending: '等待中' };
      return labels[task.status] || task.status || '未知';
    },
    reportDifferenceFor(row, name) {
      const differences = this.reportStatusFor(row).differences || {};
      if (differences.error) return '—';
      return differences[name] === null || differences[name] === undefined ? 0 : differences[name];
    },
    async selectReportAccount(account) {
      this.reportBatch.account = account;
      this.reportBatch.store_sids = [];
      if (account) await this.ensureStores(account);
    },
    toggleReportStore(sid) {
      const selected = this.reportBatch.store_sids;
      const index = selected.indexOf(sid);
      if (index >= 0) selected.splice(index, 1);
      else selected.push(sid);
    },
    selectAllReportStores() {
      const slot = this.storesByAccount[this.reportBatch.account];
      if (!slot || !slot.loaded) return;
      const valid = slot.items.filter(store => store.store_type === 'SC' && store.seller_id && store.marketplace_id).map(store => store.sid);
      this.reportBatch.store_sids = this.reportBatch.store_sids.length === valid.length ? [] : valid;
    },
    async saveReportExportBatch() {
      if (this.reportExportSaving) return;
      const account = this.reportBatch.account;
      const slot = this.storesByAccount[account];
      const selected = new Set(this.reportBatch.store_sids);
      const stores = slot && slot.loaded ? slot.items.filter(store => selected.has(store.sid)) : [];
      if (!account || !stores.length) {
        this.reportExportError = '请选择账号和至少一个店铺';
        return;
      }
      const incomplete = stores.filter(store => store.store_type !== 'SC' || !store.seller_id || !store.marketplace_id);
      if (incomplete.length) {
        this.reportExportError = '所选店铺缺少 Seller ID 或 Marketplace ID：' + incomplete.map(store => store.store_name || store.sid).join('、');
        return;
      }
      const reportType = this.reportBatch.type || 'fba_customer_returns';
      const existingScopes = new Set(stores.map(store => [reportType, account, store.sid].join('|')));
      const keep = this.reportExportConfigs.filter(row => !existingScopes.has([row.type, row.account, row.store_id].join('|')));
      const additions = stores.map(store => ({
        type: reportType, enabled: !!this.reportBatch.enabled, account,
        seller_id: store.seller_id, store_id: store.sid, region: this.reportBatch.region,
        marketplace_ids: [store.marketplace_id], cron: this.reportBatch.cron,
        window_days: Number(this.reportBatch.window_days),
      }));
      this.reportExportSaving = true;
      this.reportExportError = '';
      try {
        const result = await window.apiPut('/api/report-exports/config', { report_exports: keep.concat(additions) });
        window.toast('success', (result && result.message) || '正式报表配置已保存');
        await this.loadReportExport();
      } catch (error) {
        this.reportExportError = errorMessage(error, '保存正式报表配置失败');
      } finally {
        this.reportExportSaving = false;
      }
    },
    async deleteReportExport(row) {
      if (this.reportExportSaving) return;
      const ok = await window.syncConfirm('删除「' + this.accountName(row.account) + ' / ' + row.store_id + '」的报表校验配置？', '删除报表配置');
      if (!ok) return;
      const key = this.reportScopeKey(row);
      const reportExports = this.reportExportConfigs.filter(item => this.reportScopeKey(item) !== key);
      this.reportExportSaving = true;
      this.reportExportError = '';
      try {
        const result = await window.apiPut('/api/report-exports/config', { report_exports: reportExports });
        window.toast('success', (result && result.message) || '报表配置已删除');
        await this.loadReportExport();
      } catch (error) {
        this.reportExportError = errorMessage(error, '删除报表配置失败');
      } finally {
        this.reportExportSaving = false;
      }
    },
    // 接口清单（从清单添加的主路径数据）。失败静默：清单拉不到不影响调度表。
    async loadCatalog() {
      const d = await window.apiGet('/api/catalog').catch(window.toastError);
      if (d) {
        this.catalog = { templates: d.templates || [], accounts: d.accounts || [] };
        const keys = new Set(this.catalog.templates.map(t => t.key));
        this.catalogBatchKeys = this.catalogBatchKeys.filter(key => keys.has(key));
        if (this.catalogBatchAccount && !this.catalog.accounts.some(a => a.id === this.catalogBatchAccount)) {
          this.catalogBatchAccount = '';
          this.catalogBatchKeys = [];
        }
      }
    },
    catalogAccountName(id) {
      const account = this.catalog.accounts.find(a => a.id === id);
      return account ? (account.name || account.id) : id;
    },
    catalogEnabledLabel(t) {
      const ids = t.enabled_accounts || [];
      return ids.length ? '已启用：' + ids.map(id => this.catalogAccountName(id)).join('、') : '未启用';
    },
    catalogBatchAvailable(t) {
      return !!this.catalogBatchAccount && !(t.enabled_accounts || []).includes(this.catalogBatchAccount);
    },
    isCatalogBatchPicked(key) {
      return this.catalogBatchKeys.includes(key);
    },
    toggleCatalogBatch(key) {
      const template = this.catalog.templates.find(t => t.key === key);
      if (!template || !this.catalogBatchAvailable(template)) return;
      const i = this.catalogBatchKeys.indexOf(key);
      if (i >= 0) this.catalogBatchKeys.splice(i, 1);
      else this.catalogBatchKeys.push(key);
    },
    selectAllCatalogPending() {
      this.catalogBatchKeys = this.catalog.templates
        .filter(t => this.catalogBatchAvailable(t))
        .map(t => t.key);
    },
    clearCatalogBatch() {
      this.catalogBatchKeys = [];
    },
    catalogBatchDisabled() {
      return !this.catalogBatchAccount || this.catalogBatchKeys.length === 0;
    },
    // 一次选择账号并批量提交清单接口；复用现有单接口启用 API，保持逐项校验、备份和重启提示。
    async enableCatalogBatch() {
      const account = this.catalogBatchAccount;
      const keys = this.catalogBatchKeys.filter(key => {
        const template = this.catalog.templates.find(t => t.key === key);
        return template && this.catalogBatchAvailable(template);
      });
      if (!account) { window.toast('warn', '请先选择目标账号'); return; }
      if (!keys.length) { window.toast('warn', '请至少选择一个未启用接口'); return; }

      let success = 0;
      const failed = [];
      for (const key of keys) {
        try {
          const r = await window.apiPost('/api/catalog/enable', { key, account });
          if (!r) { failed.push(key); continue; }
          success++;
          if (r.need_restart) this.needRestart = true;
        } catch (err) {
          failed.push(key);
        }
      }
      this.clearCatalogBatch();
      await this.load();
      if (failed.length === 0) {
        window.toast('success', '已启用 ' + success + ' 个接口，重启后生效');
      } else if (success > 0) {
        window.toast('warn', '已启用 ' + success + ' 个，失败 ' + failed.length + ' 个');
      } else {
        window.toast('error', '批量启用失败');
      }
    },
    // 只为定时调度 Tab 的 lastRunOf()（每行「上次运行」）取一批最近任务，不做任何列表渲染。
    // 取 50 条而非 8 条：8 条覆盖不到全部接口，lastRunOf 会大面积显示「—」。
    async loadRecentTasks() {
      const d = await window.apiGet('/api/tasks?page=1&page_size=50').catch(window.toastError);
      if (d) this.recentTasks = d.items || [];
    },

    // ---- T1：账号 × 数据类型矩阵辅助 ----
    // 账号显示名：优先账号名称，取不到回退 ID。
    accountName(id) { return this.accountNames[id] || id; },
    // 账号勾选（根维度）。切换 / 判定选中 / 全选清空。
    toggleAccount(acc) {
      const i = this.form.accounts.indexOf(acc);
      if (i >= 0) this.form.accounts.splice(i, 1); else this.form.accounts.push(acc);
      this.pruneUnavailableTypes();
    },
    isAccountPicked(acc) { return this.form.accounts.includes(acc); },
    selectAllAccounts() { this.form.accounts = this.accounts.slice(); this.pruneUnavailableTypes(); },
    clearAccounts() { this.form.accounts = []; this.clearTypes(); },

    // 数据类型目录按所有配置生成，位置不随账号切换而变化。当前账号能否触发
    // 由 isTypeAvailable 单独决定：不可用类型保持原位、禁用，避免选择区重排。
    get dataTypes() {
      const byPath = new Map();
      for (const e of this.endpoints) {
        const label = (e.display || e.name).replace(/（[^）]*）\s*$/, '').trim();
        const cur = byPath.get(e.path);
        if (cur) {
          cur.iterate_by_store = cur.iterate_by_store || !!e.iterate_by_store;
          if (!cur.account_ids.includes(e.account_id)) cur.account_ids.push(e.account_id);
        } else {
          byPath.set(e.path, { key: e.path, label, iterate_by_store: !!e.iterate_by_store, account_ids: [e.account_id] });
        }
      }
      return Array.from(byPath.values());
    },
    // 按分类切片（SC / VC / 其他），供模板一行一类渲染。空类由模板过滤。
    dataTypesByCategory() {
      const t = this.dataTypes;
      return [
        { label: 'SC 链接', items: t.filter(x => /^SC/i.test(x.label)) },
        { label: 'VC 链接', items: t.filter(x => /^VC/i.test(x.label)) },
        { label: '其他', items: t.filter(x => !/^(SC|VC)/i.test(x.label)) },
      ];
    },
    typeForKey(key) { return this.dataTypes.find(t => t.key === key); },
    isTypeAvailable(key) {
      const type = this.typeForKey(key);
      return !!type && this.form.accounts.some(acc => type.account_ids.includes(acc));
    },
    pruneUnavailableTypes() {
      this.form.types = this.form.types.filter(key => this.isTypeAvailable(key));
      if (!this.dateControlsEnabled) {
        this.form.date_from = '';
        this.form.date_to = '';
      }
    },
    toggleType(key) {
      if (!this.isTypeAvailable(key)) return;
      const i = this.form.types.indexOf(key);
      if (i >= 0) this.form.types.splice(i, 1); else this.form.types.push(key);
      this.pruneUnavailableTypes();
    },
    isTypePicked(key) { return this.form.types.includes(key); },
    selectAllTypes() {
      this.form.types = this.dataTypes.filter(t => this.isTypeAvailable(t.key)).map(t => t.key);
      this.pruneUnavailableTypes();
    },
    clearTypes() { this.form.types = []; this.form.date_from = ''; this.form.date_to = ''; },

    // 解析：选中账号 × 选中类型(path) → 真实接口对象数组（笛卡尔积，跳过不存在的组合）。
    // 这是矩阵模型与后端「按 name 触发」之间的唯一映射点，所有下游逻辑都读它。
    get resolvedEndpoints() {
      const accs = new Set(this.form.accounts);
      const paths = new Set(this.form.types);
      return this.endpoints.filter(e => accs.has(e.account_id) && paths.has(e.path));
    },
    // 已选组合数（真实要触发的接口数），顶部计数用。
    get selectedCount() { return this.resolvedEndpoints.length; },

    get dateRangeIssue() {
      const from = this.form.date_from;
      const to = this.form.date_to;
      if (!from && !to) return '';
      if (!from || !to) return '同步日期必须同时填写开始和结束日期';
      if (from > to) return '结束日期不能早于开始日期';
      const unsupported = this.resolvedEndpoints.filter(e =>
        !e.date_range_capable && !(e.date_field && from === to));
      if (unsupported.length) return '所选接口不支持此日期范围：' + unsupported.map(e => e.display || e.name).join('、');
      return '';
    },
    get dateControlsEnabled() {
      return this.resolvedEndpoints.some(e => e.date_range_capable || e.date_field);
    },
    get dateRangeHint() {
      const selected = this.resolvedEndpoints;
      if (!selected.length) return '选择接口后可设置本次同步日期；不填则沿用接口默认策略。';
      const ranges = selected.filter(e => e.date_range_capable).length;
      const singles = selected.filter(e => e.date_field).length;
      if (!ranges && !singles) return '所选接口是快照/全量接口，日期范围不适用。';
      if (singles && !ranges) return '所选接口按单日同步，只能选择同一天。';
      if (singles) return '范围接口可选起止日期；单日接口只能选择同一天。';
      return '仅本次生效，不修改定时同步的默认窗口。';
    },

    // ---- T2：店铺勾选网格辅助 ----
    // 解析出的接口里存在 iterate_by_store:true → 才显示网格
    get showStoreGrid() {
      return this.resolvedEndpoints.some(e => e.iterate_by_store);
    },
    // 需要加载店铺的账号集合 = 解析出的、iterate_by_store 接口所属账号（去重保序）
    get storeAccounts() {
      const out = [];
      for (const e of this.resolvedEndpoints) {
        if (e.iterate_by_store && !out.includes(e.account_id)) out.push(e.account_id);
      }
      return out;
    },
    get selectedStoreCount() {
      return this.storeAccounts.reduce((count, acc) => count + this.accountSelectedCount(acc), 0);
    },
    get manualSubmitDisabled() {
      return this.selectedCount === 0 || !!this.dateRangeIssue;
    },
    get manualSubmitHint() {
      if (!this.form.accounts.length) return '先选择账号';
      if (!this.form.types.length) return '再选择数据类型';
      if (this.dateRangeIssue) return this.dateRangeIssue;
      if (!this.selectedCount) return '所选账号没有可触发接口';
      return '确认后创建同步任务';
    },
    manualSyncSummary() {
      const accounts = this.form.accounts.map(acc => this.accountName(acc)).join('、');
      const selectedPaths = new Set(this.form.types);
      const types = this.dataTypes.filter(t => selectedPaths.has(t.key)).map(t => t.label).join('、');
      const dates = this.form.date_from && this.form.date_to
        ? this.form.date_from + ' 至 ' + this.form.date_to
        : '接口默认策略';
      const stores = this.showStoreGrid
        ? (this.selectedStoreCount ? '已选 ' + this.selectedStoreCount + ' 家店铺' : '按接口配置全量')
        : '不按店铺';
      return '账号：' + accounts + '\n数据类型：' + types + '\n任务：' + this.selectedCount + ' 个\n日期：' + dates + '\n店铺：' + stores;
    },
    // 某 account 是否存在勾选子集（UI 提示用）；无 = 不传 store_sids（等价全部）
    accountSelectedCount(acc) {
      const slot = this.storesByAccount[acc];
      return slot ? Object.keys(slot.selected || {}).filter(k => slot.selected[k]).length : 0;
    },
    // 某账号当前选中的迭代接口需要哪些店铺类型（SC/VC）。返回 Set；含 '' 表示某接口不限类型 → 全放行。
    neededStoreTypes(acc) {
      const types = new Set();
      for (const e of this.resolvedEndpoints) {
        if (e.account_id === acc && e.iterate_by_store) types.add(e.store_type || '');
      }
      return types;
    },
    // 计算某账号的可见（过滤后）店铺列表：先按迭代接口的 store_type 过滤（SC 接口不显示 VC 店铺），
    // 再按 store_name / sid 搜索过滤。
    visibleStores(acc) {
      const slot = this.storesByAccount[acc];
      if (!slot || !slot.loaded) return [];
      const types = this.neededStoreTypes(acc);
      // 含 '' = 有接口不限类型 → 不按类型过滤；否则只留 store_type 命中的店铺
      let items = slot.items;
      if (types.size && !types.has('')) {
        items = items.filter(s => types.has(s.store_type));
      }
      const q = (slot.query || '').trim().toLowerCase();
      if (!q) return items;
      return items.filter(s =>
        (s.store_name && s.store_name.toLowerCase().includes(q)) ||
        (s.sid && s.sid.toLowerCase().includes(q)));
    },
    // 懒加载某账号店铺（参照 settingsApi.loadStores 写法）
    async ensureStores(acc) {
      if (!acc) return;
      if (this.storesByAccount[acc] && this.storesByAccount[acc].loaded) return;
      // 占位，避免并发重复请求
      this.storesByAccount[acc] = Object.assign({}, this.storesByAccount[acc] || {}, { loading: true, loaded: false, selected: (this.storesByAccount[acc] || {}).selected || {}, query: (this.storesByAccount[acc] || {}).query || '', items: (this.storesByAccount[acc] || {}).items || [] });
      const d = await window.apiGet('/api/accounts/' + encodeURIComponent(acc) + '/stores').catch(window.toastError);
      const slot = this.storesByAccount[acc];
      slot.loading = false;
      if (d) {
        slot.items = d.items || [];
        slot.loaded = true;
      }
    },
    // 切换某店铺勾选
    toggleStore(acc, sid) {
      const slot = this.storesByAccount[acc];
      if (!slot) return;
      slot.selected[sid] = !slot.selected[sid];
      if (!slot.selected[sid]) delete slot.selected[sid];
    },
    isStoreSelected(acc, sid) {
      const slot = this.storesByAccount[acc];
      return !!(slot && slot.selected && slot.selected[sid]);
    },
    // 全选/反选当前账号分区的「可见（过滤后）」项
    selectVisibleStores(acc) {
      const slot = this.storesByAccount[acc];
      if (!slot) return;
      const vis = this.visibleStores(acc);
      const allSel = vis.length > 0 && vis.every(s => slot.selected[s.sid]);
      for (const s of vis) {
        if (allSel) { delete slot.selected[s.sid]; }
        else { slot.selected[s.sid] = true; }
      }
    },
    visibleStoresAllSelected(acc) {
      const slot = this.storesByAccount[acc];
      const vis = this.visibleStores(acc);
      return vis.length > 0 && vis.every(s => slot && slot.selected && slot.selected[s.sid]);
    },
    // 取某账号当前勾选的 sid 数组
    selectedSidsOf(acc) {
      const slot = this.storesByAccount[acc];
      if (!slot || !slot.selected) return [];
      return Object.keys(slot.selected).filter(k => slot.selected[k]);
    },

    // ---- 立即同步（T1 fan-out + T2 按接口账号切分 store_sids）----
    async triggerSync() {
      const resolved = this.resolvedEndpoints;
      if (!resolved.length) {
        if (!this.form.accounts.length) { window.toast('warn', '请至少选择一个账号'); return; }
        if (!this.form.types.length) { window.toast('warn', '请至少选择一个数据类型'); return; }
        window.toast('warn', '所选账号与数据类型没有可触发的接口组合'); return;
      }
      const sel = resolved.map(e => e.name);
      if (this.dateRangeIssue) { window.toast('warn', this.dateRangeIssue); return; }
      const confirmed = await window.syncConfirm(this.manualSyncSummary(), '确认同步');
      if (!confirmed) return;
      // 为每个接口构造请求体：只有 iterate_by_store 且该账号有勾选时才带 store_sids；
      // 不勾 = 不传 = 后端按配置白名单（决策③：每次进页面空选）
      const buildReq = (name) => {
        const e = this.endpoints.find(x => x.name === name);
        const body = {};
        if (this.form.date_from && this.form.date_to) {
          body.date_from = this.form.date_from;
          body.date_to = this.form.date_to;
        }
        if (e && e.iterate_by_store) {
          const sids = this.selectedSidsOf(e.account_id);
          if (sids.length) body.store_sids = sids;
        }
        return body;
      };
      // Promise.allSettled：一个接口 409（已在跑）不阻断其它（决策①）
      const results = await Promise.allSettled(sel.map(name =>
        window.apiPost('/api/sync/' + encodeURIComponent(name), buildReq(name))
          .then(r => ({ name, r }))
          .catch(err => Promise.reject({ name, err }))));
      let okN = 0, failN = 0; const failed = [];
      for (const res of results) {
        if (res.status === 'fulfilled') okN++;
        else { failN++; failed.push(res.reason && res.reason.name ? res.reason.name : '?'); }
      }
      if (okN) window.toast('success', '已入队 ' + okN + ' 个接口');
      if (failN) window.toast('error', '失败 ' + failN + ' 个：' + failed.join(', '));
      setTimeout(() => this.load(), 500);
    },
    lastRunOf(name) {
      const task = this.recentTasks.find(x => x.endpoint === name);
      return task ? window.fmtRel(task.started_at) : '—';
    },

    // ---- 定时调度：内联直接编辑（cron / bucket / interval / store_sids / enabled）----
    // 归一化一行，使内联输入可直接 x-model 绑定：保证 rate 对象存在、补 store_sids_text。
    normalizeRow(e) {
      const row = Object.assign({}, e);
      row.rate = Object.assign({ bucket: 1, interval_ms: 200 }, e.rate || {});
      row.store_sids_text = (e.store_sids || []).join(',');
      return row;
    },
    get filteredSchedule() {
      const account = this.scheduleFilter.account;
      const query = (this.scheduleFilter.query || '').trim().toLowerCase();
      return this.schedule.filter(row => {
        if (account && row.account !== account) return false;
        if (!query) return true;
        return [row.display, row.name, row.path].some(value => String(value || '').toLowerCase().includes(query));
      });
    },
    isScheduleSelected(name) { return this.scheduleSelected.includes(name); },
    toggleSchedule(name) {
      this.scheduleSelected = this.isScheduleSelected(name)
        ? this.scheduleSelected.filter(item => item !== name)
        : this.scheduleSelected.concat(name);
    },
    get allVisibleScheduleSelected() {
      return this.filteredSchedule.length > 0 && this.filteredSchedule.every(row => this.isScheduleSelected(row.name));
    },
    toggleAllVisibleSchedule() {
      const visible = new Set(this.filteredSchedule.map(row => row.name));
      if (this.allVisibleScheduleSelected) {
        this.scheduleSelected = this.scheduleSelected.filter(name => !visible.has(name));
        return;
      }
      this.scheduleSelected = Array.from(new Set(this.scheduleSelected.concat(Array.from(visible))));
    },
    async saveScheduleBatch() {
      if (this.scheduleBatchSaving || this.scheduleSelected.length === 0) return;
      const selected = new Set(this.scheduleSelected);
      const rows = this.schedule.filter(row => selected.has(row.name));
      const cron = (this.scheduleBatch.cron || '').trim();
      const hasWindow = this.scheduleBatch.window_days !== '' && this.scheduleBatch.window_days !== null;
      const windowDays = Number(this.scheduleBatch.window_days);
      for (const row of rows) {
        if (this.scheduleBatch.enabled === 'enabled') row.enabled = true;
        if (this.scheduleBatch.enabled === 'disabled') row.enabled = false;
        if (cron) row.cron = cron;
        if (hasWindow && row.date_range_capable) row.window_days = windowDays;
      }
      this.scheduleBatchSaving = true;
      try {
        const failed = [];
        for (const row of rows) {
          if (!(await this.saveRow(row, false))) failed.push(row.name);
        }
        this.scheduleSelected = failed;
        if (failed.length) {
          window.toast('warn', '已保存 ' + (rows.length - failed.length) + ' 个，失败 ' + failed.length + ' 个');
          return;
        }
        window.toast('success', '已保存 ' + rows.length + ' 个接口');
      } finally {
        this.scheduleBatchSaving = false;
      }
    },
    // 该行「可编辑字段」的可比较快照，用于 dirty 判定与取消回滚（复用店铺选择的基线模式）。
    rowSnap(e) {
      return JSON.stringify({
        cron: e.cron || '',
        bucket: e.rate ? Number(e.rate.bucket) : 1,
        interval_ms: e.rate ? Number(e.rate.interval_ms) : 0,
        window_days: Number(e.window_days) || 0,
        date_offset_days: Number(e.date_offset_days) || 0,
        store_sids_text: (e.store_sids_text || '').split(',').map(s => s.trim()).filter(Boolean).join(','),
        enabled: !!e.enabled
      });
    },
    // 整行是否有未保存改动：当前快照 ≠ 基线快照。任意字段（含勾选启用）变化即为脏。
    rowDirty(e) {
      const base = this.scheduleBaseline[e.name];
      return base !== undefined && this.rowSnap(e) !== base;
    },
    // 行底色优先级：未保存改动 > 硬故障 > 告警 > 正常。
    // dirty 排在最前是因为它对应「你有改动待保存」这个即时动作，比长期存在的健康状态更急；
    // fatal（表没建）与 warn（缺声明列）都是重启前不会自行消失的状态，让位给 dirty 不会丢信息
    // ——原因文字仍在名称格里常显。
    rowClass(e) {
      if (this.rowDirty(e)) return 'bg-amber-50 ring-1 ring-inset ring-amber-200';
      if (e.fatal_error) return 'bg-red-50 ring-1 ring-inset ring-red-200';
      if ((e.warnings || []).length) return 'bg-amber-50/40';
      return 'hover:bg-slate-50/70';
    },
    // 取消：回滚该行到基线（cron/bucket/interval/store_sids_text/enabled）。
    revertRow(e) {
      const base = this.scheduleBaseline[e.name];
      if (base === undefined) return;
      const b = JSON.parse(base);
      e.cron = b.cron;
      e.rate.bucket = b.bucket;
      e.rate.interval_ms = b.interval_ms;
      e.window_days = b.window_days;
      e.date_offset_days = b.date_offset_days;
      e.store_sids_text = b.store_sids_text;
      e.enabled = b.enabled;
    },
    // 保存该行：沿用既有契约 PUT /api/endpoints/{name}；成功后仅更新本行基线（不整表 reload，
    // 避免连带丢弃其他行的未保存编辑）。
    async saveRow(e, notify = true) {
      const sids = (e.store_sids_text || '').split(',').map(s => s.trim()).filter(Boolean);
      // 后端 DisallowUnknownFields：剔除仅前端用的 store_sids_text 辅助字段。
      const body = Object.assign({}, e, {
        rate: Object.assign({}, e.rate),
        store_sids: sids
      });
      delete body.store_sids_text;
      const r = await window.apiPut('/api/endpoints/' + encodeURIComponent(e.name), body).catch(window.toastError);
      if (r) {
        if (r.need_restart) {
          this.needRestart = true;
          if (notify) window.toast('info', r.message || '已保存，需重启生效');
        } else if (notify) window.toast('success', r.message || '已热加载生效');
        // 以规范化后的当前值刷新本行（store_sids 数组与文本对齐），并把基线设为当前 → dirty 归零。
        e.store_sids = sids;
        e.store_sids_text = sids.join(',');
        this.scheduleBaseline[e.name] = this.rowSnap(e);
        return true;
      }
      return false;
    },
    async deleteEndpoint(e) {
      const ok = await window.syncConfirm('删除接口「' + (e.display || e.name) + '」？不会删除已同步的数据。', '删除接口');
      if (!ok) return;
      const r = await window.apiDelete('/api/endpoints/' + encodeURIComponent(e.name)).catch(window.toastError);
      if (r) { if (r.need_restart) this.needRestart = true; window.toast('success', r.message || '已删除'); await this.load(); }
    },
    async addEndpoint() {
      const f = this.addForm;
      if (!f.name || !f.account || !f.path || !f.table || !f.record_id_fields || !f.cron) {
        window.toast('warn', '请填写 标识/账号/Path/表/唯一键/Cron'); return;
      }
      // extra_params 选填：留空 = 不带固定参数；填了必须是合法 JSON 对象（如 {"type":1}）。
      let extraParams;
      const raw = (f.extra_params_text || '').trim();
      if (raw) {
        try {
          extraParams = JSON.parse(raw);
        } catch (e) {
          window.toast('warn', 'extra_params 不是合法 JSON：' + e.message); return;
        }
        if (extraParams === null || typeof extraParams !== 'object' || Array.isArray(extraParams)) {
          window.toast('warn', 'extra_params 必须是 JSON 对象，如 {"type":1}'); return;
        }
      }
      const body = {
        name: f.name, display: f.display || f.name, account: f.account,
        path: f.path, method: f.method, table: f.table,
        record_id_fields: f.record_id_fields.split(',').map(s => s.trim()).filter(Boolean),
        cron: f.cron, enabled: true, window_days: Number(f.window_days) || 0,
        rate: { bucket: Number(f.bucket), interval_ms: Number(f.interval_ms), multi_interval_ms: Number(f.multi_interval_ms), dimension: 'account+path' },
        iterate_by_store: !!f.iterate_by_store,
        store_param_name: f.iterate_by_store ? (f.store_param_name || '') : '',
        store_sids: []
      };
      if (extraParams) body.extra_params = extraParams;
      const r = await window.apiPost('/api/endpoints', body).catch(window.toastError);
      if (r) {
        if (r.need_restart) this.needRestart = true;
        window.toast('success', r.message || '已添加，需重启生效');
        this.advancedAdd = false;
        this.addForm = this.blankAddForm();
        await this.load();
      }
    },
    async restartNow() {
      const ok = await window.syncConfirm('立即重启进程使结构性变更生效？重启期间短暂不可用。', '重启');
      if (!ok) return;
      await window.apiPost('/api/settings/restart').catch(window.toastError);
      window.toast('info', '正在重启，3 秒后自动刷新…');
      setTimeout(() => window.location.reload(), 3000);
    }
  };
};

/* ------------------------------------------------------------------ *
 * 3c. logsPage — 同步日志（/logs）表格 + 分页
 * ------------------------------------------------------------------ */
window.logsPage = function () {
  // 从 URL query 预填筛选（同步中心点击单元格跳转过来）
  const q = new URLSearchParams(window.location.search);
  return {
    tasks: [],
    total: 0,
    endpointNames: (window.__PAGE__ && window.__PAGE__.endpointNames) || [],
    accountIDs: (window.__PAGE__ && window.__PAGE__.accountIDs) || [],
    accountOptions: (window.__PAGE__ && window.__PAGE__.accountOptions) || [],
    polling: null,        // T3：5s 轮询句柄
    refreshing: false,    // 手动刷新按钮态（转圈 + 禁用）
    requesting: false,    // 请求互斥，避免轮询和手动刷新重叠
    lastUpdatedAt: '',
    datePreset: '',
    dateRangeOpen: false,
    dateRangeError: '',
    filters: {
      endpoint: q.get('endpoint') || '',
      account: q.get('account') || '',
      status: q.get('status') || '',
      date_from: q.get('date_from') || '',
      date_to: q.get('date_to') || '',
      page: 1,
      page_size: 20
    },

    init() {
      // T3：每 5s 轮询当前筛选条件下的 load()。
      // 关键：不复位 filters.page —— 分页停留在用户所在页，不跳回第 1 页。
      // 轮询失败由 load 内的 toastError 静默吞掉。
      this.polling = setInterval(() => this.load(), 5000);
    },
    destroy() {
      // Alpine 组件卸载时清 timer，避免切页泄漏
      if (this.polling) { clearInterval(this.polling); this.polling = null; }
    },

    async load(showFeedback = false) {
      if (this.requesting) return;
      this.requesting = true;
      this.refreshing = showFeedback;
      try {
        const params = new URLSearchParams();
        for (const k of ['endpoint', 'account', 'status', 'date_from', 'date_to', 'page', 'page_size']) {
          const v = this.filters[k];
          if (v !== '' && v != null) params.set(k, v);
        }
        const d = await window.apiGet('/api/tasks?' + params.toString()).catch(window.toastError);
        if (!d) return;
        this.tasks = d.items || [];
        this.total = d.total || 0;
        this.lastUpdatedAt = new Date().toLocaleTimeString('zh-CN', {
          timeZone: 'Asia/Shanghai', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
        });
      } finally {
        this.requesting = false;
        this.refreshing = false;
      }
    },
    refreshList() {
      return this.load(true);
    },
    accountLabel(id) {
      const account = this.accountOptions.find(item => item.id === id);
      return account ? account.name : id;
    },
    dateRangeLabel() {
      if (this.filters.date_from && this.filters.date_to) {
        return this.filters.date_from.replaceAll('-', '/') + ' - ' + this.filters.date_to.replaceAll('-', '/');
      }
      if (this.filters.date_from) return this.filters.date_from.replaceAll('-', '/') + ' - 结束日期';
      if (this.filters.date_to) return '开始日期 - ' + this.filters.date_to.replaceAll('-', '/');
      return '选择日期范围';
    },
    formatDate(date) {
      const month = String(date.getMonth() + 1).padStart(2, '0');
      const day = String(date.getDate()).padStart(2, '0');
      return date.getFullYear() + '-' + month + '-' + day;
    },
    async applyDatePreset(preset) {
      const today = new Date();
      today.setHours(0, 0, 0, 0);
      const start = new Date(today);
      const end = new Date(today);
      const offsets = { yesterday: 1, last_7_days: 6, last_30_days: 29 };

      if (preset === 'this_month') {
        start.setDate(1);
      } else if (offsets[preset]) {
        start.setDate(start.getDate() - offsets[preset]);
        if (preset === 'yesterday') end.setDate(end.getDate() - 1);
      }

      this.filters.date_from = this.formatDate(start);
      this.filters.date_to = this.formatDate(end);
      this.datePreset = preset;
      this.dateRangeError = '';
      this.dateRangeOpen = false;
      this.filters.page = 1;
      await this.load();
    },
    dateRangeChanged() {
      this.datePreset = '';
      this.dateRangeError = '';
      if (!this.filters.date_from || !this.filters.date_to) return;
      if (this.filters.date_from > this.filters.date_to) {
        this.dateRangeError = '结束日期不能早于开始日期';
        return;
      }
      this.dateRangeOpen = false;
      this.filters.page = 1;
      this.load();
    },
    clearDateRange() {
      this.filters.date_from = '';
      this.filters.date_to = '';
      this.datePreset = '';
      this.dateRangeError = '';
      this.dateRangeOpen = false;
      this.filters.page = 1;
      this.load();
    },
    gotoPage(p) {
      if (p < 1) p = 1;
      this.filters.page = p;
      this.load();
    },
    statusClass(s) {
      switch (s) {
        case 'success': return 'bg-emerald-50 text-emerald-700';
        case 'running': return 'bg-blue-50 text-blue-700';
        case 'error': return 'bg-red-50 text-red-700';
        case 'cancelled': return 'bg-slate-100 text-slate-500';
        default: return 'bg-slate-50 text-slate-500';
      }
    },
    statusText(s) {
      return ({ success: '成功', running: '运行中', error: '失败', cancelled: '已取消' })[s] || s;
    },
    openDetail(task) {
      // 把整个 task 交给详情抽屉组件（用全局事件解耦两个 x-data）。
      // 传整个对象而非仅 id：抽屉的「重新触发」需要 task.endpoint。
      const t = (typeof task === 'object' && task) ? task : { id: task };
      window.dispatchEvent(new CustomEvent('task-detail-open', { detail: { id: t.id, task: t } }));
    },

    // ---- T4：日志行内 取消/重试（仅单条，决策⑥）----
    // running 行 → 取消：POST /api/sync/{endpoint}/cancel body {task_id}（先确认）
    async cancelRow(t) {
      const ok = await window.syncConfirm('确定取消 ' + t.endpoint + ' 的运行任务吗？');
      if (!ok) return;
      const r = await window.apiPost('/api/sync/' + encodeURIComponent(t.endpoint) + '/cancel', { task_id: t.id }).catch(window.toastError);
      if (r) {
        window.toast('success', r.message || '已请求取消');
        setTimeout(() => this.load(), 500);
      }
    },
    // error 行 → 重试：复用触发 POST /api/sync/{endpoint}（后端零改动）
    async retryRow(t) {
      const r = await window.apiPost('/api/sync/' + encodeURIComponent(t.endpoint), {}).catch(window.toastError);
      if (r) {
        window.toast('success', r.message || '已重新触发');
        setTimeout(() => this.load(), 500);
      }
    }
  };
};

/* ------------------------------------------------------------------ *
 * 3d. taskDetail — 日志详情抽屉（与 logsPage 通过事件解耦）
 * ------------------------------------------------------------------ */
window.taskDetail = function () {
  return {
    open: false,
    loading: false,
    taskId: null,
    detail: [],
    canRetry: false,
    currentTask: null,

    init() {
      window.addEventListener('task-detail-open', (e) => {
        this.taskId = e.detail.id;
        this.currentTask = e.detail.task || null;
        this.open = true;
        this.load();
      });
    },
    async load() {
      this.loading = true;
      this.detail = [];
      // 加载该任务的逐页请求日志（/api/tasks/:id/logs）
      const logs = await window.apiGet('/api/tasks/' + this.taskId + '/logs').catch(window.toastError);
      this.loading = false;
      this.detail = logs || [];
      // 重新触发仅在失败态显示：task 状态为 error，或任一页带 error_raw
      const st = this.currentTask && this.currentTask.status;
      this.canRetry = st === 'error' || this.detail.some(l => l.error_raw);
    },
    async retry() {
      // currentTask 由 openDetail 事件注入；endpoint 是触发同步的必填路径参数
      const ep = this.currentTask && this.currentTask.endpoint;
      if (!ep) { window.toast('warn', '缺少接口信息，无法重新触发'); return; }
      const r = await window.apiPost('/api/sync/' + encodeURIComponent(ep), {}).catch(window.toastError);
      if (r) {
        window.toast('success', '已重新触发');
        this.close();
      }
    },
    close() { this.open = false; this.taskId = null; this.detail = []; this.canRetry = false; }
  };
};

/* ------------------------------------------------------------------ *
 * 3e. dataSources — 数据源（/datasources）
 * ------------------------------------------------------------------ */
// 数据集字段路径集中在这一处，避免页面拼接任意表名或动态路由。
function datasetProjectKey(projectId, tokenId) {
  return JSON.stringify([projectId, tokenId]);
}

function listingDailyFieldsPath(projectId, tokenId) {
  const path = '/api/datasources/datasets/listing-daily-v1/fields';
  if (!projectId && !tokenId) return path;
  return path + '?' + new URLSearchParams({ project_id: projectId, token_id: tokenId }).toString();
}

function normalizeDatasetProjects(data) {
  if (data && typeof data.project_id === 'string' && typeof data.token_id === 'string' &&
      Array.isArray(data.available_fields) && Array.isArray(data.fields)) {
    const projectId = data.project_id.trim();
    const tokenId = data.token_id.trim();
    if (!projectId || !tokenId) throw new Error('项目或令牌 ID 选项格式错误');
    const key = datasetProjectKey(projectId, tokenId);
    return {
      projects: [{ project_id: projectId, token_id: tokenId, key, label: projectId + ' / ' + tokenId }],
      detail: { ...data, project_id: projectId, token_id: tokenId },
    };
  }
  const raw = data && data.projects;
  if (!Array.isArray(raw)) throw new Error('项目与令牌 ID 响应格式错误');
  const seen = new Set();
  const projects = raw.map((item) => {
    if (!item || typeof item.project_id !== 'string' || typeof item.token_id !== 'string') {
      throw new Error('项目或令牌 ID 选项格式错误');
    }
    const projectId = item.project_id.trim();
    const tokenId = item.token_id.trim();
    if (!projectId || !tokenId) throw new Error('项目或令牌 ID 选项格式错误');
    const key = datasetProjectKey(projectId, tokenId);
    if (seen.has(key)) throw new Error('项目与令牌 ID 选项重复');
    seen.add(key);
    return {
      project_id: projectId,
      token_id: tokenId,
      store_scopes: Array.isArray(item.store_scopes) ? item.store_scopes.filter((scope) => typeof scope === 'string' && scope.trim()) : [],
      key,
      label: typeof item.label === 'string' && item.label.trim() ? item.label.trim() : projectId + ' / ' + tokenId,
    };
  });
  return { projects, detail: null };
}

function normalizeListingDailyCatalog(data) {
  if (!data || data.dataset_id !== 'listing-daily-v1' || typeof data.dataset_name !== 'string' || !data.dataset_name.trim() ||
      !Array.isArray(data.fixed_fields) || !Array.isArray(data.available_fields)) {
    throw new Error('数据表配置响应格式错误');
  }
  const fixedLabels = {
    store: '店铺', channel: '渠道', asin: 'ASIN', sku: 'SKU', business_date: '业务日期', updated_at: '更新时间',
    is_provisional: '是否临时数据', verification_status: '验证状态',
  };
  const seen = new Set();
  const fixedFields = data.fixed_fields.map((name) => {
    if (typeof name !== 'string' || !fixedLabels[name] || seen.has(name)) throw new Error('固定字段响应格式错误');
    seen.add(name);
    return { name, label: fixedLabels[name] };
  });
  const fields = normalizeListingDailyFields({
    dataset_id: data.dataset_id, project_id: '', token_id: '', available_fields: data.available_fields, fields: [],
  }, '', '');
  return {
    datasetID: data.dataset_id,
    datasetName: data.dataset_name.trim(),
    fixedFields,
    fieldGroups: fields.groups,
    projects: normalizeDatasetProjects(data).projects,
  };
}

function normalizeListingDailyFields(data, projectId, tokenId) {
  if (!data || data.dataset_id !== 'listing-daily-v1' || data.project_id !== projectId || data.token_id !== tokenId ||
      !Array.isArray(data.available_fields) || !Array.isArray(data.fields)) {
    throw new Error('数据集字段响应格式错误');
  }
  const names = new Set();
  const fieldLabels = {
    sales_units: '日销量', sales_amount: '日销售额', returns_qty: '日退货量',
    inventory_sellable: '可售库存', inventory_inbound: '在途库存', inventory_reserved: '预留库存', inventory_unfulfillable: '不可售库存', inventory_local_warehouse: '本地仓库存',
    inventory_unhealthy_units: '不健康库存', inventory_aged90_sellable_units: '90天以上可售库存', inventory_sell_through_rate: '售罄率', inventory_receive_fill_rate: '收货填充率',
    inventory_vendor_confirmation_rate: '供应商确认率', inventory_avg_lead_time_days: '平均交付天数', inventory_sellable_cost: '可售库存成本', inventory_unfulfillable_cost: '不可售库存成本',
    inventory_aged90_cost: '90天以上库存成本', inventory_unhealthy_cost: '不健康库存成本', inventory_inbound_cost: '在途库存成本', inventory_currency: '库存币种',
    inventory_inbound_receiving: '在途接收中', inventory_inbound_shipped: '在途已发货', inventory_inbound_working: '在途处理中', inventory_reserved_customer_orders: '预留客户订单', inventory_reserved_fc_processing: '预留仓内处理中', inventory_reserved_fc_transfers: '预留仓间调拨',
    sessions_desktop: '桌面 Sessions', sessions_mobile: '移动 Sessions', sessions_total: '总 Sessions', review_count: '评价数', rating: '评分',
    sp_spend: 'SP 花费', sp_sales: 'SP 销售额', sp_orders: 'SP 订单量', sd_spend: 'SD 花费', sd_sales: 'SD 销售额', sd_orders: 'SD 订单量',
    hsa_spend: 'HSA 花费', hsa_sales: 'HSA 销售额', hsa_orders: 'HSA 订单量', sb_spend: 'SB 花费', sb_sales: 'SB 销售额', sb_orders: 'SB 订单量',
  };
  const fields = data.available_fields.map((name) => {
    if (typeof name !== 'string' || !name.trim()) throw new Error('数据集字段项格式错误');
    const normalized = name.trim();
    if (names.has(normalized)) throw new Error('数据集字段重复: ' + normalized);
    names.add(normalized);
    return { name: normalized, label: fieldLabels[normalized] || normalized };
  });
  const selectedFields = data.fields.map((name) => {
    if (typeof name !== 'string' || !names.has(name)) {
      throw new Error('数据集已选字段不在登记清单中');
    }
    return name;
  });
  if (new Set(selectedFields).size !== selectedFields.length) {
    throw new Error('数据集已选字段重复');
  }
  const groupPrefixes = [
    { source: '销量', prefixes: ['sales_', 'returns_'] },
    { source: '库存', prefixes: ['inventory_'] },
    { source: 'Performance', prefixes: ['sessions_', 'review_count', 'rating'] },
    { source: 'SP', prefixes: ['sp_'] },
    { source: 'SD', prefixes: ['sd_'] },
    { source: 'HSA', prefixes: ['hsa_'] },
    { source: 'SB', prefixes: ['sb_'] },
  ];
  const groups = groupPrefixes.map((group) => ({
    source: group.source,
    fields: fields.filter((field) => group.prefixes.some((prefix) => field.name === prefix || field.name.startsWith(prefix))),
  }));
  const groupedNames = new Set(groups.flatMap((group) => group.fields.map((field) => field.name)));
  const remaining = fields.filter((field) => !groupedNames.has(field.name));
  if (remaining.length > 0) groups.push({ source: '状态', fields: remaining });
  return { groups: groups.filter((group) => group.fields.length > 0), selectedFields };
}

function errorMessage(error, fallback) {
  return error && error.message ? error.message : (String(error || '') || fallback);
}

window.dataSources = function () {
  return {
    endpoints: [],
    expanded: null,
    metaLoading: false,
    columns: [],
    colError: '',       // 读字段失败时的提示（不静默空白）
    refreshing: false,  // 刷新主表工作态（只重拉 /api/endpoints，不动整页）
    datasetTab: 'table',
    datasetID: 'listing-daily-v1',
    datasetName: '',
    fixedFields: [],
    fieldGroups: [],
    selectedFields: [],
    savedFields: [],
    datasetProjects: [],
    datasetStores: [],
    datasetStoresLoading: false,
    datasetStoresError: '',
    datasetStoreSelection: {},
    selectedProjectKey: '',
    selectedProjectId: '',
    selectedTokenId: '',
    fieldStateByProject: {},
    fieldsLoading: false,
    fieldsSaving: false,
    fieldsError: '',
    fieldsSaveError: '',
    fieldsRequestVersion: 0,
    datasetCreateOpen: false,
    datasetCreating: false,
    fieldsCompleting: false,
    datasetCreateError: '',
    datasetCreateResult: null,
    datasetCreateForm: { project_id: '' },
    dailyPreviewFilters: { date_from: '', date_to: '', store: '', asin: '', sku: '', page: 1, page_size: 20 },
    dailyPreviewItems: [],
    dailyPreviewTotal: 0,
    dailyPreviewLoading: false,
    dailyPreviewLoaded: false,
    dailyPreviewError: '',

    get fieldsDirty() {
      return JSON.stringify(this.selectedFields) !== JSON.stringify(this.savedFields);
    },
    get catalogFieldCount() {
      return this.fieldGroups.reduce((count, group) => count + this.displayFields(group).length, 0);
    },
    get selectedFieldCount() { return this.selectedFields.length; },
    get hasDatasetSelection() { return Boolean(this.selectedProjectKey && this.selectedProjectId && this.selectedTokenId); },
    get projectOptions() { return this.datasetProjects; },
    get datasetSelectedStoreCount() {
      return this.datasetStores.filter((store) => this.isDatasetStoreSelected(store)).length;
    },
    get dailyPreviewPages() {
      return Math.max(1, Math.ceil(this.dailyPreviewTotal / this.dailyPreviewFilters.page_size));
    },

    async load() {
      await this.loadEndpoints();
      await this.loadDatasetCatalog();
    },
    async loadDatasetCatalog() {
      const requestVersion = ++this.fieldsRequestVersion;
      this.fieldsLoading = true;
      this.fieldsError = '';
      try {
        const data = await window.apiGet(listingDailyFieldsPath());
        if (requestVersion !== this.fieldsRequestVersion) return;
        const catalog = normalizeListingDailyCatalog(data);
        this.datasetID = catalog.datasetID;
        this.datasetName = catalog.datasetName;
        this.fixedFields = catalog.fixedFields;
        this.fieldGroups = catalog.fieldGroups;
        this.datasetProjects = catalog.projects;
      } catch (error) {
        if (requestVersion === this.fieldsRequestVersion) this.fieldsError = errorMessage(error, '未能读取数据表配置');
      } finally {
        if (requestVersion === this.fieldsRequestVersion) this.fieldsLoading = false;
      }
    },
    async loadDatasetStores() {
      this.datasetStoresLoading = true;
      this.datasetStoresError = '';
      try {
        const config = await window.apiGet('/api/config');
        const accounts = Array.isArray(config && config.accounts) ? config.accounts : [];
        const batches = await Promise.all(accounts.map(async (account) => {
          const data = await window.apiGet('/api/accounts/' + encodeURIComponent(account.id) + '/stores');
          const items = Array.isArray(data && data.items) ? data.items : [];
          return items.map((store) => ({
            account_id: account.id,
            account_name: account.name || account.id,
            sid: String(store.sid || '').trim(),
            store_name: String(store.store_name || '').trim(),
            store_type: String(store.store_type || '').trim(),
            country: String(store.country || '').trim(),
          }));
        }));
        const stores = batches.flat();
        if (stores.some((store) => !store.sid)) throw new Error('店铺目录响应缺少店铺 ID');
        const owners = new Map();
        stores.forEach((store) => {
          const owner = owners.get(store.sid);
          if (owner && owner !== store.account_id) throw new Error('店铺 ID ' + store.sid + ' 在多个账号中重复，无法安全限定访问范围');
          owners.set(store.sid, store.account_id);
        });
        this.datasetStores = stores;
        this.datasetStoreSelection = {};
      } catch (error) {
        this.datasetStores = [];
        this.datasetStoresError = errorMessage(error, '未能读取店铺目录');
      } finally {
        this.datasetStoresLoading = false;
      }
    },
    datasetStoreKey(store) {
      return [store && store.account_id, store && store.sid].join('|');
    },
    isDatasetStoreSelected(store) {
      return !!this.datasetStoreSelection[this.datasetStoreKey(store)];
    },
    toggleDatasetStore(store) {
      const key = this.datasetStoreKey(store);
      const next = { ...this.datasetStoreSelection };
      if (next[key]) delete next[key];
      else next[key] = true;
      this.datasetStoreSelection = next;
    },
    toggleAllDatasetStores(checked) {
      this.datasetStoreSelection = checked
        ? Object.fromEntries(this.datasetStores.map((store) => [this.datasetStoreKey(store), true]))
        : {};
    },
    datasetStoreScopes() {
      const scopes = [];
      const seen = new Set();
      this.datasetStores.forEach((store) => {
        if (!this.isDatasetStoreSelected(store) || seen.has(store.sid)) return;
        seen.add(store.sid);
        scopes.push(store.sid);
      });
      return scopes;
    },
    datasetStoreLabel(scope) {
      const store = this.datasetStores.find((item) => item.sid === scope);
      return store ? (store.store_name ? store.store_name + '（' + scope + '）' : scope) : scope;
    },
    formatDatasetStoreScopes(scopes) {
      return (Array.isArray(scopes) ? scopes : []).map((scope) => this.datasetStoreLabel(scope)).join('、') || '—';
    },
    async createDatasetProjectToken() {
      if (this.datasetCreating) return;
      this.datasetCreateError = '';
      const storeScopes = this.datasetStoreScopes();
      if (storeScopes.length === 0) {
        this.datasetCreateError = '请至少选择一个可读取店铺';
        return;
      }
      const body = {
        project_id: this.datasetCreateForm.project_id,
        store_scopes: storeScopes,
      };
      this.datasetCreating = true;
      try {
        this.datasetCreateResult = await window.apiPost('/api/datasources/datasets/listing-daily-v1/projects', body);
        this.datasetProjects.push({
          project_id: this.datasetCreateResult.project_id,
          token_id: this.datasetCreateResult.token_id,
          store_scopes: [...body.store_scopes],
          key: datasetProjectKey(this.datasetCreateResult.project_id, this.datasetCreateResult.token_id),
          label: this.datasetCreateResult.project_id + ' / ' + this.datasetCreateResult.token_id,
        });
        this.datasetCreateForm = { project_id: '' };
        this.datasetStoreSelection = {};
        window.toast('success', '项目 Token 已创建，请先复制明文 Token');
      } catch (error) {
        this.datasetCreateError = errorMessage(error, '创建项目 Token 失败');
      } finally {
        this.datasetCreating = false;
      }
    },
    async restartAfterDatasetToken() {
      const result = await window.apiPost('/api/settings/restart', {}).catch(window.toastError);
      if (result) {
        window.toast('success', result.message || '同步机正在重启');
        setTimeout(() => window.location.reload(), 3000);
      }
    },
    async completeDatasetFields() {
      if (this.fieldsCompleting) return;
      this.fieldsCompleting = true;
      try {
        const result = await window.apiPost('/api/datasources/datasets/listing-daily-v1/fields/complete', {});
        if (!result) return;
        await window.apiPost('/api/settings/restart', {});
        window.toast('success', '可选字段已补全，正在重启…');
        setTimeout(() => window.location.reload(), 3000);
      } catch (error) {
        window.toast('error', errorMessage(error, '补全可选字段失败'));
      } finally {
        this.fieldsCompleting = false;
      }
    },
    async loadEndpoints() {
      const eps = await window.apiGet('/api/endpoints').catch(window.toastError);
      this.endpoints = eps || [];
      return eps;
    },
    async loadDailyPreview() {
      if (this.dailyPreviewLoading) return;
      const filters = this.dailyPreviewFilters;
      const params = new URLSearchParams({
        date_from: filters.date_from || '', date_to: filters.date_to || '', store: filters.store || '',
        asin: filters.asin || '', sku: filters.sku || '', page: String(filters.page), page_size: String(filters.page_size),
      });
      this.dailyPreviewLoading = true;
      this.dailyPreviewError = '';
      try {
        const data = await window.apiGet('/api/datasets/listing-daily-v1/preview?' + params.toString());
        if (!data || !Array.isArray(data.items)) throw new Error('日维预览响应格式错误');
        this.dailyPreviewItems = data.items;
        this.dailyPreviewTotal = Number(data.total) || 0;
        this.dailyPreviewFilters.page = Number(data.page) || filters.page;
        this.dailyPreviewFilters.page_size = Number(data.page_size) || filters.page_size;
      } catch (error) {
        this.dailyPreviewItems = [];
        this.dailyPreviewTotal = 0;
        this.dailyPreviewError = errorMessage(error, '未能读取日维数据');
      } finally {
        this.dailyPreviewLoaded = true;
        this.dailyPreviewLoading = false;
      }
    },
    applyDailyPreviewFilters() {
      this.dailyPreviewFilters.page = 1;
      return this.loadDailyPreview();
    },
    changeDailyPreviewPage(page) {
      if (page < 1 || page > this.dailyPreviewPages || page === this.dailyPreviewFilters.page) return;
      this.dailyPreviewFilters.page = page;
      return this.loadDailyPreview();
    },
    dailyPreviewValue(value) { return value === null || value === undefined || value === '' ? '—' : String(value); },
    dailyPreviewIdentity(row, primary, fallback) {
      const value = row[primary];
      return value === null || value === undefined ? row[fallback] : value;
    },
    dailyPreviewRowKey(row) {
      return [row.business_date, this.dailyPreviewIdentity(row, 'store', 'store_id'), row.asin, this.dailyPreviewIdentity(row, 'sku', 'listing_sku')].join('|');
    },
    dailyPreviewStatusText(row) {
      if (row.is_provisional === true || row.is_verified === false) return '未验证';
      if (row.is_verified === true || row.is_provisional === false) return '已验证';
      return '未知';
    },
    dailyPreviewStatusClass(row) {
      const status = this.dailyPreviewStatusText(row);
      if (status === '已验证') return 'bg-emerald-50 text-emerald-700';
      if (status === '未验证') return 'bg-amber-50 text-amber-700';
      return 'bg-slate-100 text-slate-500';
    },
    // 只刷新主表（数据源列表），不重载页面、不影响已展开字段外的状态。
    async refresh() {
      if (this.refreshing) return;
      this.refreshing = true;
      try {
        const eps = await this.loadEndpoints();
        if (eps) {
          this.endpoints = eps;
          this.expanded = null; // 列表刷新后收起展开行，避免 idx 错位
        }
      } finally {
        this.refreshing = false;
      }
    },
    accountOf(e) { return e.account_id || '—'; },
    // 表内最近更新时间：e.last_sync 来自整张表 MAX(synced_at)，不是单个账号或任务的成功时间。
    fmtLastSync(e) { return e.last_sync ? window.fmtRel(e.last_sync) : '从未'; },
    async toggleExpand(idx) {
      if (this.expanded === idx) { this.expanded = null; return; }
      this.expanded = idx;
      this.columns = [];
      this.colError = '';
      this.metaLoading = true;
      const table = (this.endpoints[idx] || {}).table;
      if (!table) { this.metaLoading = false; this.colError = '该数据源未配置表名'; return; }
      // 真实读取目标表列结构（GET /api/datasources/:table/columns）
      const d = await window.apiGet('/api/datasources/' + encodeURIComponent(table) + '/columns').catch(() => null);
      this.metaLoading = false;
      if (!d || !d.columns) { this.colError = '未能读取字段结构'; return; }
      this.columns = d.columns; // [{name,type,is_primary}]
    },
    rememberCurrentFields() {
      if (!this.hasDatasetSelection) return;
      this.fieldStateByProject[this.selectedProjectKey] = {
        groups: this.fieldGroups.map((group) => ({ ...group, fields: [...group.fields] })),
        selected: [...this.selectedFields],
        saved: [...this.savedFields],
      };
    },
    async selectProject(projectKey) {
      const next = this.datasetProjects.find((target) => target.key === projectKey);
      if (!next) throw new Error('项目与令牌 ID 不在可选清单中');
      this.rememberCurrentFields();
      this.fieldsRequestVersion += 1;
      this.selectedProjectKey = next.key;
      this.selectedProjectId = next.project_id;
      this.selectedTokenId = next.token_id;
      const saved = this.fieldStateByProject[next.key];
      if (saved) {
        this.fieldGroups = saved.groups.map((group) => ({ ...group, fields: [...group.fields] }));
        this.selectedFields = [...saved.selected];
        this.savedFields = [...saved.saved];
        this.fieldsError = '';
        this.fieldsSaveError = '';
        return;
      }
      this.fieldGroups = [];
      this.selectedFields = [];
      this.savedFields = [];
      this.fieldsError = '';
      await this.loadDatasetFields();
    },
    async loadDatasetProjects() {
      const requestVersion = ++this.fieldsRequestVersion;
      this.fieldsLoading = true;
      this.fieldsError = '';
      try {
        const data = await window.apiGet(listingDailyFieldsPath());
        if (requestVersion !== this.fieldsRequestVersion) return;
        const normalized = normalizeDatasetProjects(data);
        this.datasetProjects = normalized.projects;
        const first = normalized.projects[0];
        if (!first) return;
        this.selectedProjectKey = first.key;
        this.selectedProjectId = first.project_id;
        this.selectedTokenId = first.token_id;
        if (normalized.detail) {
          const fields = normalizeListingDailyFields(normalized.detail, first.project_id, first.token_id);
          this.applyDatasetFields(fields);
        } else {
          await this.loadDatasetFields();
        }
      } catch (error) {
        if (requestVersion === this.fieldsRequestVersion) this.fieldsError = errorMessage(error, '未能读取项目 ID');
      } finally {
        if (requestVersion === this.fieldsRequestVersion) this.fieldsLoading = false;
      }
    },
    applyDatasetFields(normalized) {
      this.fieldGroups = normalized.groups;
      this.selectedFields = [...normalized.selectedFields];
      this.savedFields = [...normalized.selectedFields];
      this.rememberCurrentFields();
      this.fieldsSaveError = '';
    },
    async loadDatasetFields() {
      if (!this.hasDatasetSelection) {
        this.fieldsError = '请选择项目 ID';
        return;
      }
      const requestVersion = ++this.fieldsRequestVersion;
      const projectKey = this.selectedProjectKey;
      const projectId = this.selectedProjectId;
      const tokenId = this.selectedTokenId;
      this.fieldsLoading = true;
      this.fieldsError = '';
      try {
        const data = await window.apiGet(listingDailyFieldsPath(projectId, tokenId));
        if (requestVersion !== this.fieldsRequestVersion || projectKey !== this.selectedProjectKey) return;
        const normalized = normalizeListingDailyFields(data, projectId, tokenId);
        this.applyDatasetFields(normalized);
      } catch (error) {
        if (requestVersion === this.fieldsRequestVersion && projectKey === this.selectedProjectKey) {
          this.fieldsError = errorMessage(error, '未能读取数据集字段');
        }
      } finally {
        if (requestVersion === this.fieldsRequestVersion) this.fieldsLoading = false;
      }
    },
    isSelected(name) { return this.selectedFields.includes(name); },
    displayFields(group) {
      return (group && Array.isArray(group.fields)) ? group.fields : [];
    },
    fieldMeta(name) {
      for (const group of this.fieldGroups) {
        const field = group.fields.find((item) => item.name === name);
        if (field) return field;
      }
      return null;
    },
    addField(name) {
      if (!this.isSelected(name) && this.fieldMeta(name)) {
        this.selectedFields.push(name);
        this.rememberCurrentFields();
      }
    },
    removeField(name) {
      this.selectedFields = this.selectedFields.filter((field) => field !== name);
      this.rememberCurrentFields();
    },
    async saveDatasetFields() {
      if (this.fieldsSaving || this.fieldsError || !this.hasDatasetSelection || !this.fieldsDirty) return;
      this.fieldsSaving = true;
      this.fieldsSaveError = '';
      const fields = [...this.selectedFields];
      try {
        await window.apiPut(listingDailyFieldsPath(this.selectedProjectId, this.selectedTokenId), {
          project_id: this.selectedProjectId,
          token_id: this.selectedTokenId,
          fields,
        });
        this.savedFields = [...fields];
        this.rememberCurrentFields();
      } catch (error) {
        this.fieldsSaveError = errorMessage(error, '保存数据集字段失败');
      } finally {
        this.fieldsSaving = false;
      }
    },
  };
};

/* ------------------------------------------------------------------ *
 * 3f. settingsApi — API 配置（/settings/api）
 * ------------------------------------------------------------------ */
window.settingsApi = function () {
  return {
    info: { version: '', uptime_sec: 0, db_connected: false, base_url: '' },
    accounts: [],
    selectedAccountId: '',
    accountForm: { id: '', name: '', quota_group: '', app_key: '', app_secret: '' },
    newForm: { id: '', name: '', quota_group: '', app_key: '', app_secret: '' },
    connectionCheck: { cron: '', enabled: false },
    storeSummary: { total: 0, last_synced_at: null, items: [] },
    storesLoading: false,
    storeSyncing: false,
    profileSaving: {},
    storeSel: {},          // sid -> bool，复选框工作态
    storeSelBaseline: {},  // 上次加载/保存后的基线，用于 dirty 判定与取消回滚
    storeSaving: false,
    needRestart: false,
    egress: { ip: null, source: null, sources: [], checked_at: null, error: null }, // 同步机出口 IP（从数据源页迁入）
    egressTestSource: '',  // 下拉选中的探测源
    egressTesting: false,

    async load() {
      this.loadEgress(); // 出口 IP 独立拉取（走外网，慢也不阻塞主配置加载）
      const [settings, cfg] = await Promise.all([
        window.apiGet('/api/settings').catch(window.toastError),
        window.apiGet('/api/config').catch(window.toastError)
      ]);
      if (!settings || !cfg) return;
      this.info = {
        version: settings.version || '', uptime_sec: settings.uptime_sec || 0,
        db_connected: !!settings.db_connected, base_url: settings.base_url || ''
      };
      const statusByID = new Map((settings.accounts || []).map(a => [a.id, a]));
      this.accounts = (cfg.accounts || []).map(a => Object.assign({}, a, statusByID.get(a.id) || {}));
      const next = this.accounts.find(a => a.id === this.selectedAccountId) || this.accounts[0];
      if (next) {
        await this.selectAccount(next.id);
      } else {
        this.selectNew();
      }
    },
    selectAccount(id) {
      const account = this.accounts.find(a => a.id === id);
      if (!account) return;
      this.selectedAccountId = id;
      this.accountForm = {
        id: account.id, name: account.name, quota_group: account.quota_group || account.id,
        app_key: account.app_key || '', app_secret: ''
      };
      this.connectionCheck = Object.assign({ cron: '*/20 * * * *', enabled: true }, account.connection_check || {});
      return this.loadStores();
    },
    selectNew() {
      this.selectedAccountId = '';
      this.accountForm = { id: '', name: '', quota_group: '', app_key: '', app_secret: '' };
      this.connectionCheck = { cron: '', enabled: false };
      this.storeSummary = { total: 0, last_synced_at: null, items: [] };
      this.profileSaving = {};
      this.storeSel = {};
      this.storeSelBaseline = {};
    },
    get selectedAccount() { return this.accounts.find(a => a.id === this.selectedAccountId) || null; },
    statusText(a) {
      if (!a || !a.token_known) return '未验证';
      return a.token_valid ? 'Token 有效' : 'Token 失效';
    },
    statusClass(a) {
      if (!a || !a.token_known) return 'bg-slate-100 text-slate-600';
      return a.token_valid ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700';
    },
    // 店铺状态展示：领星 status 是 int，落库不翻译；仅在页面按语义映射中文。
    // 0 停止同步 / 1 正常 / 2 授权异常 / 3 欠费停服；未知值原样显示。
    storeStatusText(s) {
      const map = { '0': '停止同步', '1': '正常', '2': '授权异常', '3': '欠费停服' };
      const key = (s === null || s === undefined) ? '' : String(s);
      if (key === '') return '-';
      return map[key] || key;
    },
    async loadEgress() {
      const egress = await window.apiGet('/api/egress-ip').catch(window.toastError);
      if (egress) {
        this.egress = egress;
        if (!this.egressTestSource && egress.sources && egress.sources.length) {
          this.egressTestSource = egress.sources[0];
        }
      }
    },
    async testEgressSource() {
      if (!this.egressTestSource || this.egressTesting) return;
      this.egressTesting = true;
      const r = await window.apiGet('/api/egress-ip?source=' + encodeURIComponent(this.egressTestSource)).catch(window.toastError);
      this.egressTesting = false;
      if (!r) return;
      this.egress = r;
      window.toast(r.ip ? 'success' : 'error', r.ip ? ('出口 IP: ' + r.ip) : (r.error || '探测失败'));
    },
    async testDB() {
      const r = await window.apiPost('/api/settings/test-db', {}).catch(window.toastError);
      if (r) window.toast('success', '连接正常，延迟 ' + r.latency_ms + ' ms');
    },
    async testAccount(a) {
      const r = await window.apiPost('/api/settings/test-connection?account=' + encodeURIComponent(a.id)).catch(window.toastError);
      if (r) {
        this.updateToken(a.id, r.expires_in_sec);
        window.toast('success', '账号 ' + a.name + ' Token 有效，剩 ' + window.fmtExpire(r.expires_in_sec));
      }
    },
    async saveAccount() {
      const f = this.accountForm;
      if (!f.name || !f.app_key) {
        window.toast('warn', '请填写账号名称和 AppKey');
        return;
      }
      const body = {
        id: f.id, name: f.name, quota_group: f.quota_group || f.id,
        app_key: f.app_key, app_secret: f.app_secret
      };
      const r = await window.apiPut('/api/accounts/' + encodeURIComponent(f.id), body).catch(window.toastError);
      if (!r) return;
      if (r.need_restart) this.needRestart = true;
      window.toast('success', r.message || '连接配置已保存');
      await this.load();
    },
    async saveConnectionCheck() {
      if (!this.selectedAccountId || !this.connectionCheck.cron) {
        window.toast('warn', '请填写 Cron 表达式');
        return;
      }
      const body = { cron: this.connectionCheck.cron, enabled: !!this.connectionCheck.enabled };
      const r = await window.apiPut('/api/accounts/' + encodeURIComponent(this.selectedAccountId) + '/connection-check', body).catch(window.toastError);
      if (!r) return;
      window.toast('success', r.message || '连接续租计划已保存');
      await this.load();
    },
    async removeAccount(a) {
      const ok = await window.syncConfirm('删除账号「' + a.name + '」，确认？', '删除账号');
      if (!ok) return;
      // 后端若返回 409（该账号仍被接口引用），apiDelete 抛错 → toastError 显示原始提示
      const r = await window.apiDelete('/api/accounts/' + encodeURIComponent(a.id)).catch(window.toastError);
      if (r) {
        window.toast('success', r.message || '已删除');
        if (r.need_restart) this.needRestart = true;
        await this.load();
      }
    },
    async addAccount() {
      const f = this.newForm;
      if (!f.id || !f.name || !f.app_key || !f.app_secret) {
        window.toast('warn', '请填写 ID / 名称 / AppKey / AppSecret');
        return;
      }
      const body = { id: f.id, name: f.name, quota_group: f.quota_group || f.id, app_key: f.app_key, app_secret: f.app_secret };
      const r = await window.apiPost('/api/accounts', body).catch(window.toastError);
      if (!r) return;
      if (r.need_restart) this.needRestart = true;
      this.newForm = { id: '', name: '', quota_group: '', app_key: '', app_secret: '' };
      // 后端撞名（大小写不敏感）时会自动改配 ID，用响应回显的 account_id，不能沿用填入值。
      this.selectedAccountId = r.account_id || body.id;
      window.toast('success', r.message || '账号已保存');
      await this.load();
    },
    updateToken(id, expiresInSec) {
      const account = this.accounts.find(a => a.id === id);
      if (!account) return;
      account.token_known = true;
      account.token_valid = true;
      account.token_expires_in_sec = expiresInSec;
    },
    async loadStores() {
      if (!this.selectedAccountId) return;
      this.storesLoading = true;
      const d = await window.apiGet('/api/accounts/' + encodeURIComponent(this.selectedAccountId) + '/stores').catch(window.toastError);
      this.storesLoading = false;
      if (!d) return;
      this.storeSummary = d;
      // 用后端注解的 enabled 回填复选框；预先给每个 sid 建键，x-model 才有稳定的响应式属性。
      const sel = {};
      (d.items || []).forEach(s => { sel[s.sid] = !!s.enabled; });
      this.storeSel = sel;
      this.storeSelBaseline = Object.assign({}, sel);
      // 同 storeSel：预先给每个 VC 店铺在 profileSaving 上建键。否则空对象 {} 上的缺失键
      // 会让 :disabled="profileSaving[store.sid]" 这个布尔绑定永远拿到「键不存在」的状态，
      // Alpine 不会随之解禁，保存按钮一直 disabled、点击无反应——表现为填了点保存却没真正落库。
      const saving = Object.assign({}, this.profileSaving);
      (d.items || []).forEach(s => { if (s.store_type === 'VC') saving[s.sid] = !!saving[s.sid]; });
      this.profileSaving = saving;
    },
    async syncStores() {
      if (!this.selectedAccountId || this.storeSyncing) return;
      this.storeSyncing = true;
      const r = await window.apiPost('/api/accounts/' + encodeURIComponent(this.selectedAccountId) + '/stores/sync', {}).catch(window.toastError);
      this.storeSyncing = false;
      if (r) window.toast('success', r.message || '店铺目录刷新已加入队列');
    },
    async saveVCProfile(store) {
      if (!this.selectedAccountId || !store || store.store_type !== 'VC' || this.profileSaving[store.sid]) return;
      const profileID = (store.profile_id || '').trim();
      this.profileSaving[store.sid] = true;
      const r = await window.apiPut(
        '/api/accounts/' + encodeURIComponent(this.selectedAccountId) + '/stores/' + encodeURIComponent(store.sid) + '/vc-profile',
        { profile_id: profileID }
      ).catch(window.toastError);
      this.profileSaving[store.sid] = false;
      if (!r) return;
      store.profile_id = r.profile_id || '';
      window.toast('success', r.message || 'VC 广告 Profile ID 已保存');
    },
    get storeSelectedCount() {
      return Object.values(this.storeSel).filter(Boolean).length;
    },
    get storeAllSelected() {
      const items = this.storeSummary.items || [];
      return items.length > 0 && items.every(store => !!this.storeSel[store.sid]);
    },
    toggleAllStores(checked) {
      const next = {};
      (this.storeSummary.items || []).forEach(store => { next[store.sid] = !!checked; });
      this.storeSel = next;
    },
    get storeDirty() {
      const cur = this.storeSel, base = this.storeSelBaseline;
      const keys = new Set([...Object.keys(cur), ...Object.keys(base)]);
      for (const k of keys) {
        if (!!cur[k] !== !!base[k]) return true;
      }
      return false;
    },
    cancelStoreSelection() {
      this.storeSel = Object.assign({}, this.storeSelBaseline);
    },
    async saveStoreSelection() {
      if (!this.selectedAccountId || this.storeSaving) return;
      const sids = Object.keys(this.storeSel).filter(sid => this.storeSel[sid]);
      this.storeSaving = true;
      const r = await window.apiPost(
        '/api/accounts/' + encodeURIComponent(this.selectedAccountId) + '/stores/selection',
        { sids }
      ).catch(window.toastError);
      this.storeSaving = false;
      if (!r) return;
      window.toast('success', r.message || '店铺同步选择已保存');
      await this.loadStores(); // 以后端为准刷新，基线同步归零 dirty
    },
    async restartNow() {
      const ok = await window.syncConfirm('立即重启进程使结构性变更生效？重启期间短暂不可用。', '重启');
      if (!ok) return;
      await window.apiPost('/api/settings/restart').catch(window.toastError);
      window.toast('info', '正在重启，3 秒后自动刷新…');
      setTimeout(() => window.location.reload(), 3000);
    }
  };
};

/* ------------------------------------------------------------------ *
 * 4. 启动时把页面注入数据挂到 window.__PAGE__（layout 模板可扩展写入）
 * ------------------------------------------------------------------ */
// 由各页面模板的 inline script 注入 window.__PAGE__ = { endpointNames, accountIDs, ... };
// logsPage / dataSources 会读取。这里不强制，缺失则降级为空数组。
