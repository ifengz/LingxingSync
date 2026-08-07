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

void (async () => {
  const calls = [];
  sandbox.window.apiPut = async (url, body) => {
    // body 来自 VM 沙箱上下文，其原型与主上下文不同，
    // 直接用 deepStrictEqual 比较会因跨 realm 原型不等而失败。
    // 这里用 JSON 序列化/反序列化把它归一到主上下文的普通对象。
    calls.push({ url, body: JSON.parse(JSON.stringify(body)) });
    return { profile_id: body.profile_id, message: '已保存' };
  };

  const settings = sandbox.window.settingsApi();
  settings.selectedAccountId = 'account/1';
  const vcStore = { sid: 'vc/store', store_type: 'VC', profile_id: ' 1381344986683897 ' };

  await settings.saveVCProfile(vcStore);
  assert.deepEqual(calls[0], {
    url: '/api/accounts/account%2F1/stores/vc%2Fstore/vc-profile',
    body: { profile_id: '1381344986683897' }
  });
  assert.equal(vcStore.profile_id, '1381344986683897');

  vcStore.profile_id = '  ';
  await settings.saveVCProfile(vcStore);
  assert.deepEqual(calls[1].body, { profile_id: '' });

  await settings.saveVCProfile({ sid: 'sc-1', store_type: 'SC', profile_id: '' });
  assert.equal(calls.length, 2);
})().catch((error) => {
  process.nextTick(() => { throw error; });
});
