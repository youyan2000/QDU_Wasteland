// Firefox 用 browser.*，Chrome 用 chrome.*，两者都返回 Promise。
const api = globalThis.browser || globalThis.chrome;
// options.js — 读写配对配置并触发一次同步。
const $ = (id) => document.getElementById(id);

function fmt(ts) {
  if (!ts) return '从未同步';
  return new Date(ts).toLocaleString('zh-CN');
}

function phaseText(p) {
  if (!p) return '正在同步…';
  switch (p.phase) {
    case 'history': return '正在读取浏览器历史…';
    case 'bookmarks': return '正在读取书签…（历史 ' + (p.items || 0) + ' 条）';
    case 'bilibili': return '正在补 B站历史：第 ' + (p.pages || 0) + ' 页，已取 ' + (p.items || 0) + ' 条';
    case 'upload': return '正在上传：' + (p.items || 0) + ' 条';
    default: return '正在同步…';
  }
}

async function load() {
  $('version').textContent = '扩展版本：' + api.runtime.getManifest().version;
  const cfg = await api.storage.local.get({
    server: '',
    token: '',
    syncBili: false,
    enabled: true,
    lastSync: 0,
    lastError: '',
    lastResult: null,
    syncProgress: null,
  });
  $('server').value = cfg.server || '';
  $('token').value = cfg.token || '';
  $('syncBili').checked = !!cfg.syncBili;
  $('enabled').checked = cfg.enabled !== false;

  const lines = ['最近同步：' + fmt(cfg.lastSync)];
  if (cfg.syncProgress) {
    lines.push('当前进度：' + phaseText(cfg.syncProgress));
  }
  if (cfg.lastResult) {
    const r = cfg.lastResult;
    lines.push(
      `上次结果：历史 ${r.history || 0} 条、书签 ${r.favorites || 0} 条、B站 ${r.bili || 0} 条，新增 ${r.imported || 0} 条`,
    );
    if (r.bili) {
      lines.push(
        `本次B站 ${r.bili} 条（翻 ${r.biliPages || 0} 页，${r.biliDone ? '历史已补全' : '还在往更早补'}）`,
      );
    }
    if (r.biliError) lines.push('B站提示：' + r.biliError);
    if (r.biliDebug) {
      const d = r.biliDebug;
      const extra = [
        d.cursorFallback ? `，${d.cursorFallback} 页用兜底游标` : '',
        d.cursorMissing ? `，${d.cursorMissing} 页没有 cursor` : '',
      ].join('');
      const span = d.firstTs && d.lastTs
        ? `，时间范围 ${new Date(d.lastTs).toLocaleDateString('zh-CN')} ~ ${new Date(d.firstTs).toLocaleDateString('zh-CN')}`
        : '';
      lines.push(
        `B站诊断：请求 ${d.pages || 0} 页，接口返回 ${d.raw || 0} 条，入库 ${d.unique || 0} 条${extra}${span}，停止原因 ${d.stopReason || '未知'}，code=${d.code}`,
      );
    }
  }
  if (cfg.lastError) lines.push('上次错误：' + cfg.lastError);
  $('status').textContent = lines.join('\n');
}

$('save').onclick = async () => {
  const server = $('server').value.trim().replace(/\/+$/, '');
  const token = $('token').value.trim();
  if (!/^https?:\/\//i.test(server)) {
    $('status').className = 'status err';
    $('status').textContent = '站点地址要以 http:// 或 https:// 开头';
    return;
  }
  await api.storage.local.set({
    server,
    token,
    syncBili: $('syncBili').checked,
    enabled: $('enabled').checked,
    lastError: '',
  });
  $('status').className = 'status ok';
  $('status').textContent = '已保存。点「立即同步」验证一次。';
};

// 同步可能持续几十秒到几分钟：轮询 storage 里的进度，避免页面一直停在"正在同步…"。
async function runSync(resetFirst) {
  const status = $('status');
  status.className = 'status';
  status.textContent = resetFirst ? '正在全量同步…' : '正在同步…';

  if (resetFirst) {
    await api.storage.local.set({ lastSync: 0, biliCursor: null, biliDone: false, lastError: '' });
  }

  const started = Date.now();
  const timer = setInterval(async () => {
    const s = await api.storage.local.get({ syncProgress: null });
    const secs = Math.round((Date.now() - started) / 1000);
    status.textContent = phaseText(s.syncProgress) + '（' + secs + 's）';
    if (secs > 420) {
      status.className = 'status err';
      status.textContent = '同步超过 7 分钟仍未返回：打开扩展的 Service worker 控制台查看卡在哪一步。';
    }
  }, 800);

  let res = null;
  try {
    res = await api.runtime.sendMessage({ type: 'sync-now' });
  } finally {
    clearInterval(timer);
  }

  if (!res || !res.ok) {
    status.className = 'status err';
    status.textContent = '同步失败：' + ((res && res.error) || '未知错误');
    return;
  }
  const r = res.result || {};
  if (r.skipped === 'unconfigured') {
    status.className = 'status err';
    status.textContent = '还没配置站点地址或配对码';
    return;
  }
  status.className = 'status ok';
  status.textContent =
    (resetFirst ? '全量同步完成' : '同步完成') + '，新增 ' + (r.imported || 0) + ' 条' +
    (r.biliError ? '（B站：' + r.biliError + '）' : '');
  load();
}

$('sync').onclick = () => runSync(false);
$('resync').onclick = () => runSync(true);

load();
