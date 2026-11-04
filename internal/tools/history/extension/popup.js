// Firefox 用 browser.*，Chrome 用 chrome.*，两者都返回 Promise。
const api = globalThis.browser || globalThis.chrome;
// popup.js — 显示配对状态并手动触发一次同步。
const state = document.getElementById('state');

function fmt(ts) {
  return ts ? new Date(ts).toLocaleString('zh-CN') : '从未同步';
}

function phaseText(p) {
  if (!p) return '';
  switch (p.phase) {
    case 'history':
      return '读取浏览器历史…';
    case 'bookmarks':
      return '读取书签…（历史 ' + (p.items || 0) + ' 条）';
    case 'bilibili':
      return '补 B站历史：第 ' + (p.pages || 0) + ' 页，已取 ' + (p.items || 0) + ' 条';
    case 'upload':
      return '上传中：' + (p.items || 0) + ' 条';
    default:
      return '处理中…';
  }
}

async function load() {
  const cfg = await api.storage.local.get({
    server: '',
    token: '',
    lastSync: 0,
    lastError: '',
    syncProgress: null,
  });
  if (!cfg.server || !cfg.token) {
    state.className = 'err';
    state.textContent = '还没配对。点「打开设置」填写站点地址和配对码。';
    return;
  }
  if (cfg.syncProgress) {
    state.className = '';
    state.textContent = '同步进行中：' + phaseText(cfg.syncProgress);
    return;
  }
  state.className = '';
  state.textContent = '最近同步：' + fmt(cfg.lastSync) + (cfg.lastError ? '\n上次错误：' + cfg.lastError : '');
}

document.getElementById('now').onclick = async () => {
  state.className = '';
  state.textContent = '正在同步…';
  const job = api.runtime.sendMessage({ type: 'sync-now' });

  // 同步可能持续几十秒到几分钟：先轮询进度，避免弹窗干等或看起来“卡死”。
  const timer = setInterval(async () => {
    const s = await api.storage.local.get({ syncProgress: null });
    if (s.syncProgress) state.textContent = '同步进行中：' + phaseText(s.syncProgress);
  }, 800);

  const raced = await Promise.race([
    job,
    new Promise((resolve) => setTimeout(() => resolve({ pending: true }), 4000)),
  ]);
  clearInterval(timer);

  if (raced && raced.pending) {
    state.className = '';
    state.textContent = '已开始同步，可以关闭这个窗口；进度和结果在「设置」页看。';
    return;
  }
  if (!raced || !raced.ok) {
    state.className = 'err';
    state.textContent = '同步失败：' + ((raced && raced.error) || '未知错误');
    return;
  }
  if (raced.result && raced.result.skipped === 'unconfigured') {
    state.className = 'err';
    state.textContent = '还没配对，请打开设置填写。';
    return;
  }
  state.className = 'ok';
  state.textContent = '同步完成，新增 ' + ((raced.result && raced.result.imported) || 0) + ' 条';
  load();
};

document.getElementById('open').onclick = () => api.runtime.openOptionsPage();

load();
