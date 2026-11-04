// Firefox 用 browser.*，Chrome 用 chrome.*，两者都返回 Promise。
const api = globalThis.browser || globalThis.chrome;
// background.js — 定时把浏览器历史与书签推送到 Wasteland。
// 只在配置了服务器地址与配对令牌后才工作；令牌由站点「浏览足迹 → 数据备份」生成。

const DEFAULTS = {
  server: '',
  token: '',
  deviceId: '',
  deviceName: '',
  lastSync: 0,
  syncBili: false,
  enabled: true,
};

async function config() {
  const stored = await api.storage.local.get(DEFAULTS);
  if (!stored.deviceId) {
    stored.deviceId = 'dev_' + Math.random().toString(36).slice(2, 10) + Date.now().toString(36);
    await api.storage.local.set({ deviceId: stored.deviceId });
  }
  if (!stored.deviceName) {
    stored.deviceName = navigator.userAgent.includes('Firefox') ? 'Firefox' : navigator.userAgent.includes('Edg') ? 'Edge' : 'Chrome';
    await api.storage.local.set({ deviceName: stored.deviceName });
  }
  return stored;
}

function endpoint(server) {
  return server.replace(/\/+$/, '') + '/api/history/sync';
}

async function collectHistory(cfg, since) {
  const items = await api.history.search({
    text: '',
    startTime: since,
    maxResults: 5000,
  });
  const out = [];
  for (const item of items) {
    if (!item.url || !/^https?:/i.test(item.url)) continue;
    const visited = item.lastVisitTime || Date.now();
    out.push({
      id: `browser:${cfg.deviceId}:${item.id || visited}`,
      source: 'browser',
      title: item.title || item.url,
      url: item.url,
      visited_at: visited,
      deviceId: cfg.deviceId,
      deviceName: cfg.deviceName,
    });
  }
  return out;
}

// Chrome 的三个书签根：1=书签栏 2=其他书签 3=移动设备。站点按 toolbar 分组展示。
function rootKey(id) {
  if (id === '2') return 'other';
  if (id === '3') return 'mobile';
  return 'toolbar';
}

function flattenBookmarks(nodes, path, cfg, out) {
  for (const node of nodes) {
    if (node.url) {
      if (!/^https?:/i.test(node.url)) continue;
      out.push({
        id: `bookmark:${cfg.deviceId}:${node.id}`,
        origin: 'browser',
        title: node.title || node.url,
        url: node.url,
        folder: path.join(' / '),
        updated_at: node.dateGroupModified || node.dateAdded || Date.now(),
        deviceId: cfg.deviceId,
        deviceName: cfg.deviceName,
      });
    } else if (node.children) {
      const next = path.length === 0
        ? [rootKey(node.id)]
        : node.title
          ? path.concat(node.title)
          : path;
      flattenBookmarks(node.children, next, cfg, out);
    }
  }
}

async function collectBookmarks(cfg) {
  const tree = await api.bookmarks.getTree();
  const out = [];
  flattenBookmarks(tree, [], cfg, out);
  return out;
}

// B站历史：用浏览器自身的登录 Cookie 调官方历史接口（可选，默认关闭）。
// B站历史：用浏览器登录态调官方历史接口，按 cursor 翻页。
// 采用"持久化游标 + 渐进补全"：每次最多翻 40 页，没翻到底就记住位置，
// 下次同步接着往更早翻，直到接口返回没有更多；补齐后才切成按时间的增量。
async function collectBilibili(cfg, since) {
  const saved = await api.storage.local.get({ biliCursor: null, biliDone: false });
  const backfill = !saved.biliDone; // 还没补完历史
  const maxPages = 40;
  let cursor = saved.biliCursor || { max: 0, view_at: 0, business: '' };
  let pages = 0;
  let reachedEnd = false;
  let uid = 0;
  const out = [];
  const debug = { pages: 0, raw: 0, cursorMissing: 0, code: 0, lastCursor: null };

  while (pages < maxPages) {
    const url =
      'https://api.bilibili.com/x/web-interface/history/cursor?ps=30' +
      '&max=' + encodeURIComponent(cursor.max || 0) +
      '&view_at=' + encodeURIComponent(cursor.view_at || 0) +
      '&business=' + encodeURIComponent(cursor.business || '');
    const res = await fetch(url, { credentials: 'include' });
    if (!res.ok) throw new Error('B站接口返回 ' + res.status);
    const json = await res.json();
    debug.code = json.code;
    if (json.code !== 0) throw new Error('B站未登录或接口拒绝（code ' + json.code + '）');

    const data = json.data || {};
    uid = Number(data.uid || uid) || 0;
    const list = data.list || [];
    debug.raw += list.length;
    debug.lastCursor = data.cursor || null;
    if (!data.cursor) debug.cursorMissing += 1;
    if (!list.length) {
      reachedEnd = true;
      break;
    }

    let reachedOld = false;
    for (const it of list) {
      const h = it.history || {};
      const business = h.business || 'archive';
      const oid = Number(h.oid || 0);
      const viewAt = Number(it.view_at || 0) * 1000;
      if (!oid || !viewAt) continue;
      // 补历史阶段不看时间窗口；补齐后只处理增量，遇到更早的就收工。
      if (!backfill && since && viewAt < since) {
        reachedOld = true;
        continue;
      }
      out.push({
        id: `bili:${uid}:${business}:${oid}:${Math.floor(viewAt / 1000)}`,
        source: 'bilibili',
        title: it.title || it.long_title || 'B站视频',
        url: 'https://www.bilibili.com/video/' + (h.bvid || ''),
        visited_at: viewAt,
        uid,
        business,
        contentId: oid,
        eventKey: h.bvid || '',
        cover: (it.cover || '').replace(/^http:/, 'https:'),
        authorName: it.author_name || '',
        authorMid: Number(it.author_mid || 0),
        duration: Number(it.duration || 0),
        progress: Number(it.progress || 0),
        tagName: it.tag_name || '',
      });
    }

    pages += 1;
    // 正常情况用接口返回的 cursor；个别情况下接口不返回 cursor，
    // 就用最后一条记录的 view_at 当游标继续翻（B站 的两套参数都接受）。
    let next = data.cursor;
    if (!next || (!next.max && !next.view_at)) {
      const last = list[list.length - 1];
      if (last && Number(last.view_at)) {
        next = {
          max: Number(last.view_at),
          view_at: Number(last.view_at),
          business: (last.history && last.history.business) || 'archive',
        };
        debug.cursorFallback = (debug.cursorFallback || 0) + 1;
      }
    }
    if (!next || (!next.max && !next.view_at)) {
      reachedEnd = true;
      break;
    }
    const advanced = Number(next.max) !== Number(cursor.max) || Number(next.view_at) !== Number(cursor.view_at);
    if (!advanced) {
      // 游标没有前进，避免死循环。
      reachedEnd = true;
      break;
    }
    cursor = { max: next.max || 0, view_at: next.view_at || 0, business: next.business || '' };
    if (reachedOld) {
      reachedEnd = true;
      break;
    }
    await new Promise((r) => setTimeout(r, 260));
  }

  if (reachedEnd) {
    await api.storage.local.set({ biliDone: true, biliCursor: null });
  } else {
    await api.storage.local.set({ biliDone: false, biliCursor: cursor });
  }
  debug.pages = pages;
  return { items: out, pages, done: reachedEnd, backfill, debug };
}

async function sync(trigger) {
  const cfg = await config();
  if (!cfg.enabled) return { ok: false, skipped: 'disabled' };
  if (!cfg.server || !cfg.token) return { ok: false, skipped: 'unconfigured' };

  // 增量窗口：上次同步前 1 小时开始，避免边界漏记录；首次拉 7 天。
  const since = cfg.lastSync ? cfg.lastSync - 3600 * 1000 : Date.now() - 7 * 24 * 3600 * 1000;
  const now = Date.now();

  const history = await collectHistory(cfg, since);
  const favorites = await collectBookmarks(cfg);

  let bili = [];
  let biliError = '';
  let biliMeta = { pages: 0, done: true, backfill: false };
  if (cfg.syncBili) {
    try {
      const r = await collectBilibili(cfg, since);
      bili = r.items;
      biliMeta = { pages: r.pages, done: r.done, backfill: r.backfill, debug: r.debug };
    } catch (error) {
      biliError = String((error && error.message) || error);
    }
  }

  const data = {
    format: 'unlimited-history-local-demo',
    version: 1,
    history: history.concat(bili),
    favorites,
    tombstones: [],
  };
  if (!data.history.length && !data.favorites.length) {
    await api.storage.local.set({ lastSync: now });
    return { ok: true, imported: 0 };
  }

  const res = await fetch(endpoint(cfg.server), {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: 'Bearer ' + cfg.token,
    },
    body: JSON.stringify({
      batchId: 'ext-' + new Date(now).toISOString(),
      data,
    }),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(body.error || '同步失败（HTTP ' + res.status + '）');
  }
  await api.storage.local.set({
    lastSync: now,
    lastResult: {
      at: now,
      trigger,
      history: history.length,
      bili: bili.length,
      favorites: favorites.length,
      imported: body.imported || 0,
      duplicate: !!body.duplicate,
      biliError,
      biliPages: biliMeta.pages,
      biliDone: biliMeta.done,
      biliBackfill: biliMeta.backfill,
      biliDebug: biliMeta.debug || null,
    },
  });
  return { ok: true, imported: body.imported || 0, biliError };
}

api.runtime.onInstalled.addListener(() => {
  api.alarms.create('wasteland-sync', { periodInMinutes: 30 });
});
api.runtime.onStartup.addListener(() => {
  api.alarms.create('wasteland-sync', { periodInMinutes: 30 });
});
api.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === 'wasteland-sync') {
    sync('alarm').catch((error) => {
      api.storage.local.set({ lastError: String((error && error.message) || error) });
    });
  }
});

// 弹窗/设置页主动触发
api.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg && msg.type === 'sync-now') {
    sync('manual')
      .then((r) => sendResponse({ ok: true, result: r }))
      .catch((e) => sendResponse({ ok: false, error: String((e && e.message) || e) }));
    return true;
  }
  return false;
});
