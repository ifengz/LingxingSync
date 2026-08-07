/* web/static/app.js — 公共 Alpine.js 逻辑 + fetch 封装 + 各页组件工厂。
 *
 * 宪法对应：doc/04-api.md（fetch 路径）、doc/05-pages.md（每页 Alpine 组件）。
 *
 * 组织：
 *   1. 全局工具：apiGet / apiPost / 时间格式化 / 脱敏
 *   2. AppRoot：挂在 body 上的全局根组件，维护 toasts / confirmState，供所有页面共享
 *   3. 每页一个独立组件工厂函数（syncCenter / syncManage / logsPage / taskDetail /
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
  return t.toLocaleDateString();
};

// 绝对时间：RFC3339 → "2026-08-06 12:34:56"。
window.fmtTime = function (iso) {
  if (!iso) return '-';
  const t = new Date(iso);
  if (isNaN(t.getTime())) return '-';
  const pad = (n) => String(n).padStart(2, '0');
  return t.getFullYear() + '-' + pad(t.getMonth() + 1) + '-' + pad(t.getDate()) +
    ' ' + pad(t.getHours()) + ':' + pad(t.getMinutes()) + ':' + pad(t.getSeconds());
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
 * 3a. syncCenter — 同步中心（/）
 * ------------------------------------------------------------------ */
window.syncCenter = function () {
  return {
    summary: { total: 0, running: 0, error: 0, disabled: 0 },
    rows: [],      // [{name, display, account_id, status, last_status, ...}]
    accounts: [],  // 去重后的 account_id 列表
    polling: null,
    todayRecords: 0,
    todayErrors: 0,

    async load() {
      const d = await window.apiGet('/api/status').catch(window.toastError);
      if (!d) return;
      this.summary = d.summary || this.summary;
      this.rows = d.workers || [];
      this.todayRecords = this.rows.reduce((total, w) => total + (Number(w.today_records) || 0), 0);
      this.todayErrors = this.rows.reduce((total, w) => total + (Number(w.today_errors) || 0), 0);
      // 从 workers 反推出 account 列表（去重保序）
      const seen = new Set();
      this.accounts = [];
      for (const w of this.rows) {
        if (w.account_id && !seen.has(w.account_id)) {
          seen.add(w.account_id);
          this.accounts.push(w.account_id);
        }
      }
    },

    // 单元格样式（宪法 §5 颜色）。
    // worker.Status 取值：idle|running|error|disabled；上次结果看 last_status（success|error|cancelled）。
    cellClass(ep, acc) {
      if (ep.account_id !== acc) return 'bg-slate-100 text-slate-400 cursor-not-allowed';
      switch (ep.status) {
        case 'running': return 'bg-blue-100 text-blue-700 hover:bg-blue-200';
        case 'error':   return 'bg-red-100 text-red-700 hover:bg-red-200';
        case 'disabled':return 'bg-slate-200 text-slate-500 cursor-not-allowed';
        default:
          // idle：再看上次结果决定是「成功绿」还是「空闲灰」
          if (ep.last_status === 'success') return 'bg-emerald-100 text-emerald-700 hover:bg-emerald-200';
          return 'bg-slate-100 text-slate-500 hover:bg-slate-200';
      }
    },
    cellText(ep, acc) {
      if (ep.account_id !== acc) return '—';
      switch (ep.status) {
        case 'running': return '运行';
        case 'error':   return '失败';
        case 'disabled':return '禁用';
        default:
          if (ep.last_status === 'success') return '成功';
          return '空闲';
      }
    },
    // 点击单元格跳到 /logs 带过滤
    cellClick(name, acc) {
      const q = new URLSearchParams({ endpoint: name, account: acc });
      window.location.href = '/logs?' + q.toString();
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
    // form.endpoint(单值) → form.endpoints(数组)（决策①：多选）
    // storeSids 不在 form 上集中存：T2 要求按账号分区，故选择态落在 storesByAccount[acc].selected
    form: { endpoints: [], date_from: '', date_to: '' },
    // ---- T2 店铺网格（按账号分区）----
    // storesByAccount[accountId] = { items:[StoreSummary], loading:false, loaded:true,
    //                                 selected:{sid:true}, query:'' }
    storesByAccount: {},
    runningList: [],
    recentTasks: [],
    needRestart: false,   // 结构性变更后显示重启横幅
    editing: null,        // 正在内联编辑的 endpoint name
    editBuf: {},          // 编辑缓冲
    showAddForm: false,
    addForm: { name: '', display: '', account: '', path: '', method: 'GET', table: '', record_id_fields: '', cron: '', bucket: 1, interval_ms: 200, multi_interval_ms: 0 },
    polling: null,        // T3：5s 轮询句柄（仅手动 Tab 启用）

    // Alpine 自动调一次（早于模板里的 x-init="load()"）。
    // 这里只挂 $watch：选中接口变化时，懒加载涉及账号的店铺（T2）。
    init() {
      if (this.$watch) {
        this.$watch('form.endpoints', () => {
          for (const acc of this.storeAccounts) this.ensureStores(acc);
        });
      }
      // T3：默认手动 Tab，进入即每 5s 轮询「最近同步任务」+ 运行态。
      // 轮询失败由 loadRunning/loadRecentTasks 内的 toastError 静默吞掉。
      this.startPolling();
    },
    destroy() {
      // Alpine 组件卸载时清 timer，避免切页泄漏
      this.stopPolling();
    },
    startPolling() {
      this.stopPolling();
      this.polling = setInterval(() => {
        // 仅刷新「最近同步任务」+ 运行态；不重拉整份 /api/config（定时配置无需每 5s 拉）
        this.loadRunning();
        this.loadRecentTasks();
      }, 5000);
    },
    stopPolling() {
      if (this.polling) { clearInterval(this.polling); this.polling = null; }
    },
    // Tab 切换：进 manual 启轮询、进 schedule 停轮询（避免无谓请求）
    switchTab(t) {
      this.tab = t;
      if (t === 'manual') this.startPolling();
      else this.stopPolling();
    },

    async load() {
      // 完整配置（含 cron/rate/store_sids/iterate_by_store），定时调度 Tab 与 T1/T2 都依赖它
      const cfg = await window.apiGet('/api/config').catch(window.toastError);
      if (cfg) {
        this.schedule = cfg.endpoints || [];
        // 保留 iterate_by_store/window_days：T1 分组与 T2 网格判定都需要
        this.endpoints = this.schedule.map(e => ({
          name: e.name, display: e.display, account_id: e.account,
          iterate_by_store: !!e.iterate_by_store, window_days: e.window_days || 0
        }));
        const seen = new Set();
        this.accounts = [];
        for (const e of this.endpoints) {
          if (e.account_id && !seen.has(e.account_id)) { seen.add(e.account_id); this.accounts.push(e.account_id); }
        }
        // 配置若变了，已选接口可能失效；选中态不跨次保留属决策③的自然结果
        this.form.endpoints = this.form.endpoints.filter(n => this.endpoints.some(e => e.name === n));
      }
      await this.loadRunning();
      await this.loadRecentTasks();
    },
    async loadRunning() {
      const d = await window.apiGet('/api/status').catch(window.toastError);
      if (!d) return;
      this.runningList = (d.workers || []).filter(w => w.status === 'running');
    },
    async loadRecentTasks() {
      const d = await window.apiGet('/api/tasks?page=1&page_size=8').catch(window.toastError);
      if (d) this.recentTasks = d.items || [];
    },

    // ---- T1：数据类型多选辅助 ----
    // 按账号分组（账号是根维度），返回 [{account, items:[endpoint]}]，顺序与 accounts 一致
    get endpointsByAccount() {
      const groups = [];
      for (const acc of this.accounts) {
        const items = this.endpoints.filter(e => e.account_id === acc);
        if (items.length) groups.push({ account: acc, items });
      }
      return groups;
    },
    // 已选数据类型数量
    get selectedCount() { return this.form.endpoints.length; },
    // 顶部「全选 / 清空」
    selectAllEndpoints() { this.form.endpoints = this.endpoints.map(e => e.name); },
    clearEndpoints() { this.form.endpoints = []; },

    // ---- T2：店铺勾选网格辅助 ----
    // 当前选中接口里，存在 iterate_by_store:true 的 → 才显示网格
    get showStoreGrid() {
      if (!this.form.endpoints.length) return false;
      const sel = new Set(this.form.endpoints);
      return this.endpoints.some(e => sel.has(e.name) && e.iterate_by_store);
    },
    // 需要加载店铺的账号集合 = 选中且 iterate_by_store 的接口所属账号（去重保序）
    get storeAccounts() {
      const sel = new Set(this.form.endpoints);
      const out = [];
      for (const e of this.endpoints) {
        if (sel.has(e.name) && e.iterate_by_store && !out.includes(e.account_id)) out.push(e.account_id);
      }
      return out;
    },
    // 某 account 是否存在勾选子集（UI 提示用）；无 = 不传 store_sids（等价全部）
    accountSelectedCount(acc) {
      const slot = this.storesByAccount[acc];
      return slot ? Object.keys(slot.selected || {}).filter(k => slot.selected[k]).length : 0;
    },
    // 计算某账号的可见（过滤后）店铺列表：按 store_name / sid 即时过滤
    visibleStores(acc) {
      const slot = this.storesByAccount[acc];
      if (!slot || !slot.loaded) return [];
      const q = (slot.query || '').trim().toLowerCase();
      if (!q) return slot.items;
      return slot.items.filter(s =>
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
      if (!this.form.endpoints.length) { window.toast('warn', '请至少选择一个数据类型'); return; }
      const sel = this.form.endpoints.slice();
      // 为每个接口构造请求体：只有 iterate_by_store 且该账号有勾选时才带 store_sids；
      // 不勾 = 不传 = 后端按配置白名单（决策③：每次进页面空选）
      const buildReq = (name) => {
        const e = this.endpoints.find(x => x.name === name);
        if (e && e.iterate_by_store) {
          const sids = this.selectedSidsOf(e.account_id);
          if (sids.length) return { store_sids: sids };
        }
        return {};
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
    async cancel(name, taskID) {
      const ok = await window.syncConfirm('确定取消 ' + name + ' 的运行任务吗？');
      if (!ok) return;
      const r = await window.apiPost('/api/sync/' + encodeURIComponent(name) + '/cancel', { task_id: taskID }).catch(window.toastError);
      if (r) { window.toast('success', '已请求取消'); setTimeout(() => this.loadRunning(), 500); }
    },
    lastRunOf(name) {
      const task = this.recentTasks.find(x => x.endpoint === name);
      return task ? window.fmtRel(task.started_at) : '—';
    },
    taskStatusText(status) {
      return ({ success: '成功', running: '运行中', error: '失败', cancelled: '已取消' })[status] || status;
    },

    // ---- 定时调度：内联编辑 cron / bucket / interval / store_sids ----
    startEdit(e) {
      this.editing = e.name;
      this.editBuf = {
        cron: e.cron || '',
        bucket: e.rate ? e.rate.bucket : 1,
        interval_ms: e.rate ? e.rate.interval_ms : 200,
        store_sids_text: (e.store_sids || []).join(',')
      };
    },
    cancelEdit() { this.editing = null; this.editBuf = {}; },
    async saveEdit(e) {
      const sids = (this.editBuf.store_sids_text || '').split(',').map(s => s.trim()).filter(Boolean);
      const body = Object.assign({}, e, {
        cron: this.editBuf.cron,
        rate: Object.assign({}, e.rate, { bucket: Number(this.editBuf.bucket), interval_ms: Number(this.editBuf.interval_ms) }),
        store_sids: sids
      });
      const r = await window.apiPut('/api/endpoints/' + encodeURIComponent(e.name), body).catch(window.toastError);
      if (r) {
        if (r.need_restart) { this.needRestart = true; window.toast('info', r.message || '已保存，需重启生效'); }
        else window.toast('success', r.message || '已热加载生效');
        this.cancelEdit();
        await this.load();
      }
    },
    async toggleEnable(e) {
      const body = Object.assign({}, e, { enabled: !e.enabled });
      const r = await window.apiPut('/api/endpoints/' + encodeURIComponent(e.name), body).catch(window.toastError);
      if (r) {
        if (r.need_restart) this.needRestart = true;
        window.toast('success', (e.enabled ? '已停用 ' : '已启用 ') + (e.display || e.name));
        await this.load();
      }
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
      const body = {
        name: f.name, display: f.display || f.name, account: f.account,
        path: f.path, method: f.method, table: f.table,
        record_id_fields: f.record_id_fields.split(',').map(s => s.trim()).filter(Boolean),
        cron: f.cron, enabled: true, window_days: 0,
        rate: { bucket: Number(f.bucket), interval_ms: Number(f.interval_ms), multi_interval_ms: Number(f.multi_interval_ms), dimension: 'account+path' },
        store_sids: []
      };
      const r = await window.apiPost('/api/endpoints', body).catch(window.toastError);
      if (r) {
        if (r.need_restart) this.needRestart = true;
        window.toast('success', r.message || '已添加，需重启生效');
        this.showAddForm = false;
        this.addForm = { name: '', display: '', account: '', path: '', method: 'GET', table: '', record_id_fields: '', cron: '', bucket: 1, interval_ms: 200, multi_interval_ms: 0 };
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
    polling: null,        // T3：5s 轮询句柄
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

    async load() {
      const params = new URLSearchParams();
      for (const k of ['endpoint', 'account', 'status', 'date_from', 'date_to', 'page', 'page_size']) {
        const v = this.filters[k];
        if (v !== '' && v != null) params.set(k, v);
      }
      const d = await window.apiGet('/api/tasks?' + params.toString()).catch(window.toastError);
      if (!d) return;
      this.tasks = d.items || [];
      this.total = d.total || 0;
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
window.dataSources = function () {
  return {
    endpoints: [],
    connOpen: false,
    egress: { ip: null, checked_at: null, error: null },
    expanded: null,
    metaLoading: false,
    columns: [],
    colError: '',       // 读字段失败时的提示（不静默空白）

    async load() {
      const eps = await window.apiGet('/api/endpoints').catch(window.toastError);
      this.endpoints = eps || [];
      const egress = await window.apiGet('/api/egress-ip').catch(window.toastError);
      if (egress) this.egress = egress;
    },
    accountOf(e) { return e.account_id || '—'; },
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
    connStr() {
      // 只读展示用，密码隐藏。真实连接信息由 datasources.html 注入 window.__DB__
      // （pageDataSources → newPageData 从 cfg.Database 填充；密码永不下发）。
      // fallback 仅在注入缺失时兜底，避免空白。
      const db = window.__DB__ || { host: '127.0.0.1', port: 3306, user: 'lingxing', db: 'lingxing' };
      return 'mysql://' + db.user + ':****@' + db.host + ':' + db.port + '/' + db.db;
    },
    async copyConn() {
      try {
        await navigator.clipboard.writeText(this.connStr());
        window.dispatchEvent(new CustomEvent('sync-toast', { detail: { type: 'success', msg: '已复制连接串' } }));
      } catch (e) {
        window.dispatchEvent(new CustomEvent('sync-toast', { detail: { type: 'error', msg: '复制失败：' + e.message } }));
      }
    }
  };
};

/* ------------------------------------------------------------------ *
 * 3f. settingsApi — API 配置（/settings/api）
 * ------------------------------------------------------------------ */
window.settingsApi = function () {
  return {
    info: { version: '', uptime_sec: 0, db_connected: false, base_url: '' },
    accounts: [],
    endpoints: [],
    selectedAccountId: '',
    accountForm: { id: '', name: '', quota_group: '', app_key: '', app_secret: '' },
    newForm: { id: '', name: '', quota_group: '', app_key: '', app_secret: '' },
    storeSummary: { total: 0, last_synced_at: null, items: [] },
    storesLoading: false,
    needRestart: false,

    async load() {
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
      this.endpoints = cfg.endpoints || [];
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
      return this.loadStores();
    },
    selectNew() {
      this.selectedAccountId = '';
      this.accountForm = { id: '', name: '', quota_group: '', app_key: '', app_secret: '' };
      this.storeSummary = { total: 0, last_synced_at: null, items: [] };
    },
    get selectedAccount() { return this.accounts.find(a => a.id === this.selectedAccountId) || null; },
    get schedules() { return this.endpoints.filter(e => e.account === this.selectedAccountId); },
    statusText(a) {
      if (!a || !a.token_known) return '未验证';
      return a.token_valid ? 'Token 有效' : 'Token 失效';
    },
    statusClass(a) {
      if (!a || !a.token_known) return 'bg-slate-100 text-slate-600';
      return a.token_valid ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700';
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
      this.selectedAccountId = body.id;
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
      if (d) this.storeSummary = d;
    },
    async saveSchedule(schedule) {
      const body = Object.assign({}, schedule, {
        rate: Object.assign({}, schedule.rate),
        record_id_fields: (schedule.record_id_fields || []).slice(),
        store_sids: (schedule.store_sids || []).slice(),
        extra_params: Object.assign({}, schedule.extra_params || {})
      });
      const r = await window.apiPut('/api/endpoints/' + encodeURIComponent(schedule.name), body).catch(window.toastError);
      if (!r) return;
      window.toast('success', r.message || 'Cron 已保存');
      await this.load();
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
