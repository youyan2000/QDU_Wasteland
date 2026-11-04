// Firefox 用 browser.*，Chrome 用 chrome.*，两者都返回 Promise。
const api = globalThis.browser || globalThis.chrome;
// options.js — 读写配对配置并触发一次同步。
const $ = (id) => document.getElementById(id);

function fmt(ts) {
  if (!ts) return '从未同步';
  return new Date(ts).toLocaleString('zh-CN');
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
  });
  $('server').value = cfg.server || '';
  $('token').value = cfg.token || '';
  $('syncBili').checked = !!cfg.syncBili;
  $('enabled').checked = cfg.enabled !== false;

  const lines = ['最近同步：' + fmt(cfg.lastSync)];
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
      lines.push(
        `B站诊断：请求 ${d.pages || 0} 页，接口共返回 ${d.raw || 0} 条${extra}，code=${d.code}`,
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

$('sync').onclick = async () => {
  $('status').className = 'status';
  $('status').textContent = '正在同步…';
  const res = await api.runtime.sendMessage({ type: 'sync-now' });
  if (!res || !res.ok) {
    $('status').className = 'status err';
    $('status').textContent = '同步失败：' + ((res && res.error) || '未知错误');
    return;
  }
  const r = res.result || {};
  if (r.skipped === 'unconfigured') {
    $('status').className = 'status err';
    $('status').textContent = '还没配置站点地址或配对码';
    return;
  }
  $('status').className = 'status ok';
  $('status').textContent = '同步完成，新增 ' + (r.imported || 0) + ' 条' + (r.biliError ? '（B站：' + r.biliError + '）' : '');
  load();
};

$('resync').onclick = async () => {
  $('status').className = 'status';
  $('status').textContent = '正在全量同步（B站可能要多翻几页，请稍等）…';
  // 清掉增量游标：下一次同步按"首次"处理，B站翻到底，浏览器历史回溯 7 天。
  await api.storage.local.set({ lastSync: 0, biliCursor: null, biliDone: false, lastError: '' });
  const res = await api.runtime.sendMessage({ type: 'sync-now' });
  if (!res || !res.ok) {
    $('status').className = 'status err';
    $('status').textContent = '同步失败：' + ((res && res.error) || '未知错误');
    return;
  }
  const r = res.result || {};
  $('status').className = 'status ok';
  $('status').textContent =
    '全量同步完成，新增 ' + (r.imported || 0) + ' 条' + (r.biliError ? '（B站：' + r.biliError + '）' : '');
  load();
};

load();