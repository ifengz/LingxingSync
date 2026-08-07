const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

const sandbox = {
  window: { addEventListener() {}, dispatchEvent() {} },
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
  const manage = sandbox.window.syncManage();
  manage.load = async () => {};
  await manage.cancel('orders', 42);

  assert.equal(request.url, '/api/sync/orders/cancel');
  assert.equal(request.body.task_id, 42);

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
        accounts: [{ id: 'sc_us', name: '美国领星', quota_group: 'sc_us', app_key: 'ak_old', app_secret: '****' }],
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

  settings.accountForm.name = '美国主账号';
  await settings.saveAccount();
  const accountSave = calls.find(c => c.method === 'PUT' && c.url === '/api/accounts/sc_us');
  assert.equal(JSON.stringify(accountSave.body), JSON.stringify({
    id: 'sc_us', name: '美国主账号', quota_group: 'sc_us', app_key: 'ak_old', app_secret: ''
  }));

  settings.schedules[0].cron = '*/30 * * * *';
  await settings.saveSchedule(settings.schedules[0]);
  const cronSave = calls.find(c => c.method === 'PUT' && c.url === '/api/endpoints/sc_stores');
  assert.equal(cronSave.body.cron, '*/30 * * * *');
  assert.equal(cronSave.body.rate.bucket, 2);
})().catch((error) => {
  process.nextTick(() => { throw error; });
});
