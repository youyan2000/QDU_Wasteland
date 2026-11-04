// Firefox 用 browser.*，Chrome 用 chrome.*，两者都返回 Promise。
const api = globalThis.browser || globalThis.chrome;
// popup.js — 显示配对状态并手动触发一次同步。
const state = document.getElementById('state');

function fmt(ts) {
  return ts ? new Date(ts).toLocaleString('zh-CN') : '从未同步';
}

async function load() {
  const cfg = await api.storage.local.get({ server: '', token: '', lastSync: 0, lastError: '' });
  if (!cfg.server || !cfg.token) {
    state.className = 'err';
    state.textContent = '还没配对。点「打开设置」填写站点地址和配对码。';
    return;
  }
  state.className = '';
  state.textContent = '最近同步：' + fmt(cfg.lastSync) + (cfg.lastError ? '\n上次错误：' + cfg.lastError : '');
}

document.getElementById('now').onclick = async () => {
  state.className = '';
  state.textContent = '正在同步…';
  const res = await api.runtime.sendMessage({ type: 'sync-now' });
  if (!res || !res.ok) {
    state.className = 'err';
    state.textContent = '同步失败：' + ((res && res.error) || '未知错误');
    return;
  }
  if (res.result && res.result.skipped === 'unconfigured') {
    state.className = 'err';
    state.textContent = '还没配对，请打开设置填写。';
    return;
  }
  state.className = 'ok';
  state.textContent = '同步完成，新增 ' + ((res.result && res.result.imported) || 0) + ' 条';
  load();
};

document.getElementById('open').onclick = () => api.runtime.openOptionsPage();

load();
