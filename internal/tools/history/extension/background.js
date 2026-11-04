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

// 所有网络请求都带超时：MV3 的 fetch 默认没有超时，B站/站点卡住时会一直挂着，
// 表现就是"同步十分钟没反应"。
async function fetchJSON(url, options = {}, timeoutMs = 15000) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const res = await fetch(url, { ...options, signal: controller.signal });
    return res;
  } catch (error) {
    if (error && error.name === 'AbortError') {
      throw new Error('请求超时（' + Math.round(timeoutMs / 1000) + 's）：' + url);
    }
    throw error;
  } finally {
    clearTimeout(timer);
  }
}
function endpoint(server) {
  return server.replace(/\/+$/, '') + '/api/history/sync';
}

// 浏览器历史分两种量：新增（上次同步之后）和补旧（按 7 天一个时间窗往回翻）。
// 补旧每轮只取一个窗口，把"首次同步很久"摊到后面几次里，避免一次跑几分钟被浏览器挂起。
const HISTORY_WINDOW_MS = 7 * 24 * 3600 * 1000;
const HISTORY_FLOOR_MS = 365 * 24 * 3600 * 1000; // 最多回溯一年，再早的不要
const HISTORY_MAX_ITEMS = 2000;

function historyItem(cfg, item, out, seen) {
  if (!item.url || !/^https?:/i.test(item.url)) return;
  const visited = item.lastVisitTime || Date.now();
  const id = `browser:${cfg.deviceId}:${item.id || visited}`;
  if (seen.has(id)) return;
  seen.add(id);
  out.push({
    id,
    source: 'browser',
    title: item.title || item.url,
    url: item.url,
    visited_at: visited,
    deviceId: cfg.deviceId,
    deviceName: cfg.deviceName,
  });
}

async function collectHistory(cfg, since) {
  const saved = await api.storage.local.get({ histCursor: 0, histDone: false });
  const out = [];
  const seen = new Set();

  // 1) 新增部分：从上次同步前 1 小时开始
  const recent = await api.history.search({
    text: '',
    startTime: since,
    maxResults: HISTORY_MAX_ITEMS,
  });
  for (const item of recent) historyItem(cfg, item, out, seen);
  await api.storage.local.set({ syncProgress: { phase: 'history', items: out.length, at: Date.now() } });

  // 2) 补旧部分：每轮一个 7 天窗口，游标往回推
  let cursor = Number(saved.histCursor || 0) || Date.now();
  let done = !!saved.histDone;
  if (!done) {
    const floor = Date.now() - HISTORY_FLOOR_MS;
    const windowStart = Math.max(cursor - HISTORY_WINDOW_MS, floor);
    const older = await api.history.search({
      text: '',
      startTime: windowStart,
      endTime: cursor,
      maxResults: HISTORY_MAX_ITEMS,
    });
    for (const item of older) historyItem(cfg, item, out, seen);
    cursor = windowStart;
    if (windowStart <= floor) done = true;
    await api.storage.local.set({ histCursor: cursor, histDone: done });
  }
  return { items: out, histDone: done };
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
// 采用"持久化游标 + 渐进补全"：没翻到底就记住位置，下次接着往更早翻。
// 注意：接口不稳时可能出现"返回条数很少 / 游标不前进"，这里都记录 stopReason 便于排查。
const BILI_STATE_VERSION = 3;

async function collectBilibili(cfg, since) {
  const saved = await api.storage.local.get({
    biliCursor: null,
    biliDone: false,
    biliVer: 0,
    forceBackfill: 0,
  });
  // 只有用户明确点「重新全量同步」才做长补全；「立即同步」永远走小步增量，
  // 避免一次跑几十页把 Firefox 的后台事件页拖死（表现就是永远"正在同步"）。
  const forced = Number(saved.forceBackfill || 0) > 0;
  const staleState = Number(saved.biliVer || 0) !== BILI_STATE_VERSION;
  const backfill = forced || (staleState ? false : !saved.biliDone);
  const startCursor = forced || staleState
    ? { max: 0, view_at: 0, business: '' }
    : saved.biliCursor || { max: 0, view_at: 0, business: '' };
  if (staleState || forced) {
    await api.storage.local.set({ biliVer: BILI_STATE_VERSION, biliCursor: null, biliDone: false, forceBackfill: 0 });
  }
  // 单次同步的预算：立即同步 6 页 / 90 秒；全量 30 页 / 4 分钟。
  const maxPages = forced ? 30 : 6;
  const deadline = Date.now() + (forced ? 4 * 60 * 1000 : 90 * 1000);
  let cursor = startCursor;
  let pages = 0;
  let reachedEnd = false;
  let stopReason = 'max-pages';
  let uid = 0;
  const out = [];
  const debug = { pages: 0, raw: 0, unique: 0, cursorMissing: 0, cursorFallback: 0, cursorStuck: 0, cursorRepeat: 0, code: 0, stopReason: '', firstTs: 0, lastTs: 0 };
  const seenCursors = new Set();
  const cursorKey = (c) => `${c.max || 0}:${c.view_at || 0}:${c.business || ''}`;
  seenCursors.add(cursorKey(cursor));

  while (pages < maxPages) {
    if (Date.now() > deadline) {
      stopReason = 'deadline';
      break;
    }
    const url =
      'https://api.bilibili.com/x/web-interface/history/cursor?ps=30' +
      '&max=' + encodeURIComponent(cursor.max || 0) +
      '&view_at=' + encodeURIComponent(cursor.view_at || 0) +
      '&business=' + encodeURIComponent(cursor.business || '');
    const res = await fetchJSON(url, { credentials: 'include' }, 15000);
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
      stopReason = 'empty';
      break;
    }

    let reachedOld = false;
    for (const it of list) {
      const h = it.history || {};
      const business = h.business || 'archive';
      const oid = Number(h.oid || 0);
      const viewAt = Number(it.view_at || 0) * 1000;
      if (!oid || !viewAt) continue;
      if (!debug.firstTs || viewAt > debug.firstTs) debug.firstTs = viewAt;
      if (!debug.lastTs || viewAt < debug.lastTs) debug.lastTs = viewAt;
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
    await api.storage.local.set({ syncProgress: { phase: 'bilibili', pages, items: out.length, at: Date.now() } });

    // 正常情况用接口返回的 cursor；没返回或不前进时，用最后一条的 view_at 兜底。
    let next = data.cursor;
    if (!next || (!next.max && !next.view_at)) {
      const last = list[list.length - 1];
      if (last && Number(last.view_at)) {
        next = {
          max: Number(last.view_at),
          view_at: Number(last.view_at),
          business: (last.history && last.history.business) || 'archive',
        };
        debug.cursorFallback += 1;
      }
    }

    const sameAsSent =
      next && Number(next.max) === Number(cursor.max) && Number(next.view_at) === Number(cursor.view_at);
    if (sameAsSent) {
      // 游标原地踏步：再用本页最后一条的时间兜底一次，仍不前进才判定到底。
      const last = list[list.length - 1];
      if (last && Number(last.view_at) && Number(last.view_at) !== Number(cursor.view_at)) {
        next = {
          max: Number(last.view_at),
          view_at: Number(last.view_at),
          business: (last.history && last.history.business) || 'archive',
        };
        debug.cursorFallback += 1;
      } else {
        debug.cursorStuck += 1;
        reachedEnd = true;
        stopReason = 'cursor-stuck';
        break;
      }
    }
    if (!next || (!next.max && !next.view_at)) {
      reachedEnd = true;
      stopReason = 'no-cursor';
      break;
    }

    const nextKey = cursorKey({ max: next.max || 0, view_at: next.view_at || 0, business: next.business || '' });
    if (seenCursors.has(nextKey)) {
      // 游标在几个值之间来回跳（接口异常）：立即停止，避免无限翻页。
      debug.cursorRepeat += 1;
      reachedEnd = true;
      stopReason = 'cursor-repeat';
      break;
    }
    seenCursors.add(nextKey);
    cursor = { max: next.max || 0, view_at: next.view_at || 0, business: next.business || '' };
    // 每页都落一次游标：万一后台被浏览器挂起，下次同步能从这里继续，不必从头翻。
    await api.storage.local.set({ biliCursor: cursor, biliDone: false, biliVer: BILI_STATE_VERSION, syncHeartbeat: Date.now() });
    if (reachedOld) {
      reachedEnd = true;
      stopReason = 'reached-old';
      break;
    }
    await new Promise((r) => setTimeout(r, 260));
  }

  debug.pages = pages;
  debug.unique = new Set(out.map((x) => x.id)).size;
  debug.stopReason = stopReason;
  if (reachedEnd) {
    await api.storage.local.set({ biliDone: true, biliCursor: null, biliVer: BILI_STATE_VERSION });
  } else {
    await api.storage.local.set({ biliDone: false, biliCursor: cursor, biliVer: BILI_STATE_VERSION });
  }
  return { items: out, pages, done: reachedEnd, backfill, debug };
}
async function sync(trigger) {
  const cfg = await config();
  if (!cfg.enabled) return { ok: false, skipped: 'disabled' };
  if (!cfg.server || !cfg.token) return { ok: false, skipped: 'unconfigured' };

  // 增量窗口：上次同步前 1 小时开始，避免边界漏记录；首次拉 7 天。
  const since = cfg.lastSync ? cfg.lastSync - 3600 * 1000 : Date.now() - 7 * 24 * 3600 * 1000;
  const now = Date.now();

  await api.storage.local.set({ syncProgress: { phase: 'history', at: Date.now() } });
  console.log('[history-sync] phase=history since=' + new Date(since).toISOString());
  const historyRes = await collectHistory(cfg, since);
  const history = historyRes.items;
  console.log('[history-sync] history=' + history.length);
  await api.storage.local.set({ syncProgress: { phase: 'bookmarks', items: history.length, at: Date.now() } });
  const favorites = await collectBookmarks(cfg);
  console.log('[history-sync] favorites=' + favorites.length);

  let bili = [];
  let biliError = '';
  let biliMeta = { pages: 0, done: true, backfill: false };
  if (cfg.syncBili) {
    try {
      const r = await collectBilibili(cfg, since);
      bili = r.items;
      biliMeta = { pages: r.pages, done: r.done, backfill: r.backfill, debug: r.debug };
      console.log('[history-sync] bili=' + bili.length + ' pages=' + r.pages + ' stop=' + (r.debug && r.debug.stopReason));
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
    await api.storage.local.set({ lastSync: now, syncProgress: null, lastError: '' });
    return { ok: true, imported: 0 };
  }

  await api.storage.local.set({ syncProgress: { phase: 'upload', pages: biliMeta.pages || 0, items: data.history.length + data.favorites.length, at: Date.now() } });
  const res = await fetchJSON(endpoint(cfg.server), {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: 'Bearer ' + cfg.token,
    },
    body: JSON.stringify({
      batchId: 'ext-' + new Date(now).toISOString(),
      data,
    }),
  }, 90000);
  console.log('[history-sync] upload status=' + res.status);
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
      histDone: !!historyRes.histDone,
      biliDebug: biliMeta.debug || null,
    },
  });
  await api.storage.local.set({ syncProgress: null, lastError: '' });
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
      api.storage.local.set({ lastError: String((error && error.message) || error), syncProgress: null });
    });
  }
});

// 弹窗/设置页主动触发
api.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg && msg.type === 'sync-now') {
    sync('manual')
      .then((r) => sendResponse({ ok: true, result: r }))
      .catch((e) => {
        const msg = String((e && e.message) || e);
        api.storage.local.set({ lastError: msg, syncProgress: null });
        sendResponse({ ok: false, error: msg });
      });
    return true;
  }
  return false;
});
