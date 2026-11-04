(() => {
  "use strict";

  const API = "/tool/history/api";
  const PAGE_SIZE = 50;
  const CURVE_DAYS = 60;
  const CALENDAR_DAYS = 182;
  const DOMAIN_LIMIT = 60;
  const app = document.getElementById("app");

  const state = {
    me: null,
    meError: "",
    meLoading: true,
    page: "site",
    site: {
      items: [],
      loading: false,
      loaded: false,
      error: "",
      imported: null,
    },
    history: {
      source: "browser",
      q: "",
      // 筛选条件：日期窗口按本地日界，domain 为主机名。
      from: "",
      to: "",
      domain: "",
      domainQuery: "",
      items: [],
      total: 0,
      nextCursor: null,
      loading: false,
      loaded: false,
      error: "",
      stats: null,
      statsError: "",
      statsLoading: false,
    },
    favorite: {
      q: "",
      folder: "",
      sub: true,
      status: "active",
      snapshot: "",
      snapshots: [],
      snapshotsError: "",
      snapshotsLoading: false,
      // 默认全部展开，只记录被折叠的路径。
      collapsed: new Set(),
      items: [],
      total: 0,
      nextCursor: null,
      loading: false,
      loaded: false,
      error: "",
      folders: [],
      foldersTotal: 0,
      foldersLoose: 0,
      foldersLoading: false,
      foldersError: "",
    },
    backup: {
      file: null,
      preview: null,
      result: null,
      loading: false,
      error: "",
    },
    devices: {
      items: [],
      loading: false,
      loaded: false,
      error: "",
      newToken: "",
      busy: false,
    },
  };

  function node(tag, className, text) {
    const item = document.createElement(tag);
    if (className) item.className = className;
    if (text instanceof Node) item.append(text);
    else if (text !== undefined && text !== null) item.textContent = String(text);
    return item;
  }

  const SVG_NS = "http://www.w3.org/2000/svg";
  function svg(tag, attrs = {}) {
    const element = document.createElementNS(SVG_NS, tag);
    for (const [name, value] of Object.entries(attrs)) element.setAttribute(name, String(value));
    return element;
  }

  function safeHttpUrl(value) {
    if (typeof value !== "string" || !value.trim()) return null;
    try {
      const parsed = new URL(value.trim());
      if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return null;
      return parsed.href;
    } catch (_error) {
      return null;
    }
  }

  function safeString(value, fallback = "") {
    return typeof value === "string" || typeof value === "number" ? String(value) : fallback;
  }

  function getRoute() {
    const path = window.location.pathname.replace(/\/+$/, "") || "/";
    if (window.location.hash === "#backup") return "backup";
    if (path === "/tool/history/bilibili") return "bilibili";
    if (path === "/tool/history/favorite") return "favorite";
    if (path === "/tool/history/backup") return "backup";
    if (path === "/tool/history" || path === "/tool/history/index.html") return "site";
    return "browser";
  }

  function apiErrorMessage(data, fallback) {
    if (data && typeof data === "object") {
      const message = data.message || data.error || data.detail;
      if (typeof message === "string" && message.trim()) return message;
    }
    return fallback;
  }

  async function request(path, options = {}) {
    const method = (options.method || "GET").toUpperCase();
    const headers = new Headers(options.headers || {});
    const init = { method, headers, credentials: "same-origin" };

    if (method !== "GET" && method !== "HEAD") {
      headers.set("Content-Type", "application/json");
      init.body = JSON.stringify(options.body === undefined ? {} : options.body);
    }

    let response;
    try {
      response = await fetch(path, init);
    } catch (_error) {
      throw new Error("连接中断：可能是文件过大或服务刚重启，请刷新页面后重试；大于 20MB 的备份请分批导入。");
    }

    const contentType = response.headers.get("content-type") || "";
    let data = null;
    if (response.status !== 204) {
      try {
        data = contentType.includes("application/json")
          ? await response.json()
          : await response.text();
      } catch (_error) {
        data = null;
      }
    }
    if (!response.ok) {
      throw new Error(apiErrorMessage(data, `请求失败（HTTP ${response.status}）`));
    }
    return { response, data };
  }

  const DATE_FORMAT = new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
  const TIME_FORMAT = new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit" });
  const WEEKDAYS = ["星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"];

  function formatDate(value) {
    const timestamp = typeof value === "number" ? value : Number(value);
    if (!Number.isFinite(timestamp) || timestamp <= 0) return "时间未知";
    try {
      return DATE_FORMAT.format(new Date(timestamp));
    } catch (_error) {
      return "时间未知";
    }
  }

  function formatTime(value) {
    const timestamp = typeof value === "number" ? value : Number(value);
    if (!Number.isFinite(timestamp) || timestamp <= 0) return "—";
    try {
      return TIME_FORMAT.format(new Date(timestamp));
    } catch (_error) {
      return "—";
    }
  }

  function localDayKey(timestamp) {
    const date = new Date(timestamp);
    return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(
      date.getDate(),
    ).padStart(2, "0")}`;
  }

  function dayLabel(key) {
    const [year, month, day] = key.split("-").map(Number);
    const date = new Date(year, month - 1, day);
    const today = localDayKey(Date.now());
    const yesterday = localDayKey(Date.now() - 86400000);
    const weekday = WEEKDAYS[date.getDay()];
    const pretty = `${year}年${month}月${day}日`;
    if (key === today) return `今天 · ${pretty} ${weekday}`;
    if (key === yesterday) return `昨天 · ${pretty} ${weekday}`;
    return `${pretty} ${weekday}`;
  }

  function formatNumber(value) {
    const number = Number(value);
    return Number.isFinite(number) ? number.toLocaleString("zh-CN") : "0";
  }

  function setPageTitle(title, description) {
    const heading = node("div", "page-heading");
    heading.append(node("h1", "", title), node("p", "", description));
    return heading;
  }

  function icon(text, className = "nav-icon") {
    return node("span", className, text);
  }

  function makeLink(href, label, className = "") {
    const link = node("a", className, label);
    link.href = href;
    return link;
  }

  function makeButton(label, className = "button button-secondary", type = "button") {
    const button = node("button", className, label);
    button.type = type;
    return button;
  }

  function navLink(href, symbol, label, active) {
    const link = makeLink(href, "", `nav-link${active ? " active" : ""}`);
    link.append(icon(symbol), node("span", "", label));
    if (active) link.setAttribute("aria-current", "page");
    return link;
  }

  function makeShell() {
    const shell = node("div", "app-shell");
    const sidebar = node("aside", "sidebar");
    sidebar.setAttribute("aria-label", "主导航");

    const brand = makeLink("/tool/history/browser/", "", "brand");
    brand.append(node("span", "brand-mark", "UH"));
    const brandText = node("span", "brand-copy");
    brandText.append(node("strong", "", "浏览足迹"), node("small", "", "WASTELAND"));
    brand.append(brandText);
    sidebar.append(brand);

    const nav = node("nav", "primary-nav");
    nav.append(
      navLink("/tool/history/", "◈", "本站足迹", state.page === "site"),
      navLink("/tool/history/browser/", "◉", "浏览器历史", state.page === "browser"),
      navLink("/tool/history/bilibili/", "▶", "B站历史", state.page === "bilibili"),
      navLink("/tool/history/favorite/", "◇", "收藏列表", state.page === "favorite"),
      navLink("/tool/history/backup/", "↧", "数据备份", state.page === "backup"),
    );
    sidebar.append(nav);

    const sideFoot = node("div", "sidebar-foot");
    sideFoot.append(node("span", "side-foot-dot"), node("span", "", "账号专属 · 导入内容仅本人可见"));
    sidebar.append(sideFoot);

    const main = node("main", "main-area");
    const topbar = node("header", "topbar");
    const status = node("div", `local-badge${state.meError ? " is-error" : ""}`);
    status.append(node("span", "status-dot"));
    status.append(node("span", "", state.meError ? "服务暂不可用" : "账号档案"));
    topbar.append(status);

    const content = node("div", "content-wrap");
    const notice = node("section", `demo-notice${state.meError ? " error" : ""}`);
    notice.setAttribute("role", state.meError ? "alert" : "note");
    notice.append(icon(state.meError ? "!" : "i", "notice-icon"));
    const noticeCopy = node("div", "notice-copy");
    noticeCopy.append(
      node("strong", "", state.meError ? "浏览足迹暂不可用" : "个人浏览档案"),
      node(
        "span",
        "",
        state.meError ? state.meError : "这里汇总你在本站的浏览足迹，以及你主动导入的浏览器或 B 站历史。",
      ),
    );
    notice.append(noticeCopy);
    if (state.me && typeof state.me.warning === "string" && state.me.warning.trim()) {
      noticeCopy.append(node("span", "server-warning", state.me.warning));
    }
    if (state.meLoading && !state.meError) {
      noticeCopy.append(node("span", "server-status", "正在加载账号档案…"));
    }
    content.append(notice);

    const page = node("div", "page-content");
    content.append(page);
    main.append(topbar, content);
    shell.append(sidebar, main);
    // 本站足迹页用标题 + 使用说明表达，不再叠一层提示框。
    if (state.page === "site") notice.remove();
    return { shell, page };
  }

  function makeMessage(kind, message) {
    return node("p", `message ${kind}`, message);
  }

  function makeEmptyState(symbol, title, description) {
    const box = node("div", "empty-state");
    box.append(node("span", "empty-symbol", symbol), node("strong", "", title));
    if (description) box.append(node("p", "", description));
    return box;
  }

  /* ---------- 筛选条 ---------- */

  function makeFilterBar() {
    const history = state.history;
    const chips = [];
    if (history.from || history.to) {
      const label =
        history.from && history.to
          ? history.from === history.to
            ? history.from
            : `${history.from} → ${history.to}`
          : history.from
            ? `${history.from} 起`
            : `至 ${history.to}`;
      chips.push({ key: "date", label: `日期 ${label}` });
    }
    if (history.domain) chips.push({ key: "domain", label: `网站 ${history.domain}` });

    if (!chips.length && !history.q) {
      const bar = node("div", "filter-bar is-idle");
      bar.append(
        node(
          "span",
          "filter-hint",
          state.page === "bilibili"
            ? "点日历格或曲线上的点按日期筛选。"
            : "点日历格或曲线上的点按日期筛选；点左侧网站按站点筛选。",
        ),
      );
      return bar;
    }
    const bar = node("div", "filter-bar");
    bar.append(node("span", "filter-label", "筛选"));
    for (const chip of chips) {
      const item = makeButton("", "filter-chip");
      item.append(node("span", "", chip.label), node("span", "chip-x", "×"));
      item.title = "点击移除该筛选";
      item.addEventListener("click", () => {
        if (chip.key === "date") {
          history.from = "";
          history.to = "";
        } else {
          history.domain = "";
        }
        loadHistory(false);
        loadHistoryStats();
      });
      bar.append(item);
    }
    if (history.q) {
      const item = makeButton("", "filter-chip");
      item.append(node("span", "", `关键词 ${history.q}`), node("span", "chip-x", "×"));
      item.addEventListener("click", () => {
        history.q = "";
        loadHistory(false);
      });
      bar.append(item);
    }
    const clear = makeButton("清除全部", "button button-soft button-tiny");
    clear.addEventListener("click", () => {
      history.from = "";
      history.to = "";
      history.domain = "";
      history.q = "";
      loadHistory(false);
      loadHistoryStats();
    });
    bar.append(clear);
    return bar;
  }

  /* ---------- 图表（可点击筛选） ---------- */

  function isPickedDay(day) {
    return state.history.from === day && state.history.to === day;
  }

  function pickDay(day) {
    const history = state.history;
    const same = isPickedDay(day);
    history.from = same ? "" : day;
    history.to = same ? "" : day;
    render();
    loadHistory(false);
    loadHistoryStats();
  }

  /** 访问曲线：按天计数折线 + 面积；每个点都能点选筛选。 */
  function makeVisitCurve(days) {
    const width = 760;
    const height = 150;
    const padX = 6;
    const padY = 12;
    const max = Math.max(1, ...days.map((day) => day.count));
    const step = days.length > 1 ? (width - padX * 2) / (days.length - 1) : 0;
    const xAt = (index) => padX + index * step;
    const yAt = (count) => height - padY - (count / max) * (height - padY * 2);

    const chart = svg("svg", {
      viewBox: `0 0 ${width} ${height}`,
      class: "curve-chart is-clickable",
      role: "img",
      "aria-label": `最近 ${days.length} 天访问量曲线，最高 ${max} 次；可直接点击某天筛选`,
    });
    for (const ratio of [0, 0.5, 1]) {
      chart.append(
        svg("line", {
          x1: padX,
          x2: width - padX,
          y1: yAt(max * ratio),
          y2: yAt(max * ratio),
          class: "curve-grid",
        }),
      );
    }
    const points = days.map((day, index) => `${xAt(index)},${yAt(day.count)}`).join(" ");
    chart.append(
      svg("polygon", {
        points: `${padX},${height - padY} ${points} ${xAt(days.length - 1)},${height - padY}`,
        class: "curve-area",
      }),
      svg("polyline", { points, class: "curve-line" }),
    );
    // 透明命中区，让"点某一天"可操作；每格至少 8px 宽。
    const hitWidth = Math.max(step, 8);
    for (const [index, day] of days.entries()) {
      const hit = svg("rect", {
        x: xAt(index) - hitWidth / 2,
        y: 0,
        width: hitWidth,
        height,
        class: `curve-hit${isPickedDay(day.date) ? " is-active" : ""}`,
      });
      hit.append(svg("title", {}, `${day.date}：${day.count} 次（点击筛选）`));
      hit.addEventListener("click", () => pickDay(day.date));
      chart.append(hit);
    }
    return chart;
  }

  /** 日历热力图：每列一周，周一起始；点格子按当天筛选。 */
  function makeCalendar(days) {
    const cells = days.slice(-CALENDAR_DAYS);
    // 用非零天的分位数分级，而不是拿最大值当基准：否则只要有一天特别高，
    // 其余所有天都会被压进最浅的一档，颜色就看不出差别了。
    const nonZero = cells
      .map((day) => day.count)
      .filter((count) => count > 0)
      .sort((a, b) => a - b);
    const quantile = (q) => {
      if (!nonZero.length) return Number.POSITIVE_INFINITY;
      return nonZero[Math.min(nonZero.length - 1, Math.floor(q * nonZero.length))];
    };
    const t1 = quantile(0.25);
    const t2 = quantile(0.5);
    const t3 = quantile(0.75);
    const level = (count) => {
      if (!count) return 0;
      if (count > t3) return 4;
      if (count > t2) return 3;
      if (count > t1) return 2;
      return 1;
    };

    const wrap = node("div", "calendar-wrap");
    const grid = node("div", "calendar-grid");
    const first = cells[0];
    if (first) {
      const [year, month, day] = first.date.split("-").map(Number);
      const lead = (new Date(year, month - 1, day).getDay() + 6) % 7;
      for (let index = 0; index < lead; index++) grid.append(node("span", "cal-cell is-empty"));
    }
    for (const day of cells) {
      const cell = node(
        "button",
        `cal-cell level-${level(day.count)}${isPickedDay(day.date) ? " is-active" : ""}`,
      );
      cell.type = "button";
      cell.title = `${day.date}：${day.count} 次访问（点击只看这一天）`;
      cell.setAttribute("aria-label", cell.title);
      cell.addEventListener("click", () => pickDay(day.date));
      grid.append(cell);
    }
    wrap.append(grid);

    const legend = node("div", "cal-legend");
    legend.append(node("span", "", "少"));
    const legendLabels = [
      "0 次",
      `1–${Number.isFinite(t1) ? t1 : 0} 次`,
      `${Number.isFinite(t1) ? t1 + 1 : 0}–${Number.isFinite(t2) ? t2 : 0} 次`,
      `${Number.isFinite(t2) ? t2 + 1 : 0}–${Number.isFinite(t3) ? t3 : 0} 次`,
      `${Number.isFinite(t3) ? t3 + 1 : 0} 次以上`,
    ];
    for (let step = 0; step <= 4; step++) {
      const swatch = node("span", `cal-cell level-${step}`);
      swatch.title = legendLabels[step];
      legend.append(swatch);
    }
    legend.append(node("span", "", "多"));
    wrap.append(legend);
    return wrap;
  }

  /** 时段分布：0-23 时。 */
  function makeHourChart(byHour) {
    const max = Math.max(1, ...byHour);
    const wrap = node("div", "hour-chart");
    for (let hour = 0; hour < 24; hour++) {
      const column = node("div", "hour-column");
      column.title = `${hour} 时：${byHour[hour]} 次`;
      const bar = node("span", "hour-bar");
      bar.style.height = `${Math.max(2, (byHour[hour] / max) * 100)}%`;
      column.append(bar, node("span", "hour-label", hour % 3 === 0 ? String(hour) : ""));
      wrap.append(column);
    }
    return wrap;
  }

  /* ---------- 历史页 ---------- */

  /* ---------- 本站足迹 ---------- */

  async function loadSiteVisits() {
    const site = state.site;
    site.loading = true;
    site.error = "";
    render();
    try {
      const { data } = await request(`${API}/site-visits`);
      const items = data && Array.isArray(data.items) ? data.items : [];
      site.items = items;
      site.error = "";
    } catch (error) {
      site.items = [];
      site.error = error instanceof Error ? error.message : "读取失败";
    } finally {
      site.loading = false;
      site.loaded = true;
      render();
      loadImportedCounts();
    }
  }

  // 本站足迹页顺带拉一下另外三个来源的数量：导入过数据的人能马上看到入口，
  // 不会以为"导入完什么都没有"。
  async function loadImportedCounts() {
    try {
      const [browser, bili, fav] = await Promise.all([
        request(`${API}/history?source=browser&limit=1`),
        request(`${API}/history?source=bilibili&limit=1`),
        request(`${API}/favorite?origin=browser&limit=1`),
      ]);
      state.site.imported = {
        browser: Number((browser.data && browser.data.total) || 0),
        bilibili: Number((bili.data && bili.data.total) || 0),
        favorites: Number((fav.data && fav.data.total) || 0),
      };
    } catch (_error) {
      state.site.imported = null;
    }
    render();
  }

  function renderSitePage(page) {
    const site = state.site;
    page.append(
      setPageTitle(
        "浏览足迹",
        "登录后在本站打开过的文章、帖子、课程和资料会自动记在这里，只有你自己能看到。",
      ),
    );

    // 使用说明：很多人第一次进来不知道"为什么是空的"，这里把三条路径讲清楚。
    const guide = node("details", "history-guide");
    guide.open = state.site.loaded && state.site.items.length === 0;
    const summary = node("summary", "", "怎么用（30 秒看完）");
    const steps = node("ol", "history-guide-steps");
    steps.append(
      node("li", "", "保持登录，正常浏览课程、文章、帖子或资料页面；每打开一个内容，系统自动记一条。"),
      node("li", "", "回到本页按天查看，点右侧「打开」可以直接回到那个页面。"),
      node(
        "li",
        "",
        "「浏览器历史 / B站历史 / 导入的书签」需要安装一次浏览器扩展（到「数据备份」页下载并生成配对码），之后每 30 分钟自动同步；没有配对时为空是正常的。",
      ),
    );
    guide.append(summary, steps);
    page.append(guide);

    // 三个导入来源的入口：哪怕本站足迹还是空的，也能看到导入的数据在哪。
    const imported = state.site.imported;
    if (imported && (imported.browser || imported.bilibili || imported.favorites)) {
      const bar = node("div", "imported-links");
      bar.append(node("span", "imported-label", "已导入："));
      const add = (href, label, count, unit) => {
        if (!count) return;
        const link = makeLink(href, `${label} ${formatNumber(count)} ${unit}`, "imported-link");
        bar.append(link);
      };
      add("/tool/history/browser/", "浏览器历史", imported.browser, "条");
      add("/tool/history/bilibili/", "B站历史", imported.bilibili, "条");
      add("/tool/history/favorite/", "书签", imported.favorites, "个");
      page.append(bar);
    }

    if (site.error) {
      page.append(makeMessage("error", site.error));
      const retry = makeButton("重新读取", "button button-outline");
      retry.addEventListener("click", () => loadSiteVisits());
      page.append(retry);
      return;
    }

    if (!site.loaded || site.loading) {
      page.append(makeMessage("info", "正在读取足迹…"));
      return;
    }

    const items = site.items;
    const listSummary = node("div", "list-summary");
    listSummary.append(
      node("span", "", `共 ${formatNumber(items.length)} 条本站记录`),
      node("span", "summary-source", "只保留最近 200 条 · 同一页面 1 分钟内不重复记录"),
    );
    page.append(listSummary);

    if (!items.length) {
      page.append(
        makeEmptyState(
          "◈",
          "还没有本站足迹",
          "保持登录，去课程、文章或论坛打开一篇内容，再回到这里就能看到记录。",
        ),
      );
      return;
    }

    const list = node("div", "record-list");
    let currentDay = "";
    let daySection = null;
    for (const item of items) {
      const key = item && item.visited_at ? localDayKey(item.visited_at) : "";
      if (key !== currentDay) {
        currentDay = key;
        daySection = node("section", "day-group");
        daySection.append(
          node("div", "day-heading", node("span", "day-title", key ? dayLabel(key) : "时间未知")),
        );
        list.append(daySection);
      }
      const card = node("article", "record-card");
      const visual = node("div", "record-mark site", "◈");
      const body = node("div", "record-body");
      const title = safeString(item && item.title, "未命名页面").trim() || "未命名页面";
      const path = safeString(item && item.url, "").trim();
      const isInternal = path.startsWith("/") && !path.startsWith("//");
      const titleNode = isInternal ? makeLink(path, title, "record-title") : node("span", "record-title", title);
      const meta = node("div", "record-meta");
      meta.append(node("span", "record-time", formatTime(item && item.visited_at)));
      if (path) meta.append(metaDot(), node("span", "record-host", path.split("?")[0]));
      body.append(titleNode, meta);
      card.append(visual, body);
      if (isInternal) card.append(makeLink(path, "打开", "record-open"));
      daySection.append(card);
    }
    for (const group of list.querySelectorAll(".day-group")) {
      const count = group.querySelectorAll(".record-card").length;
      group.querySelector(".day-heading").append(node("span", "day-count", `${count} 条`));
    }
    page.append(list);
  }

  async function ensureSitePage() {
    if (!state.site.loaded && !state.site.loading) await loadSiteVisits();
  }

  function renderHistoryPage(page, source) {
    const history = state.history;
    const isBili = source === "bilibili";
    page.append(
      isBili
        ? setPageTitle("B站历史", "按观看账号归档的 B站观看记录，只增不改。")
        : setPageTitle("浏览器历史", "浏览器每次访问独立归档，只增不改；可装扩展自动同步。"),
    );
    page.append(makeFilterBar());

    if (history.statsError) page.append(makeMessage("error", history.statsError));
    if (history.stats) {
      const stats = history.stats;
      const curvePanel = node("section", "chart-panel");
      curvePanel.append(
        node("div", "panel-heading", node("h2", "", `最近 ${CURVE_DAYS} 天访问曲线`)),
        makeVisitCurve(stats.days.slice(-CURVE_DAYS)),
      );
      page.append(curvePanel);

      const lower = node("div", "chart-row");
      const calPanel = node("section", "chart-panel");
      calPanel.append(
        node("div", "panel-heading", node("h2", "", `日历热力图（最近 ${CALENDAR_DAYS} 天）`)),
        makeCalendar(stats.days),
      );
      const hourPanel = node("section", "chart-panel");
      // 标题里写明统计窗口：不然选中某一天后这张图变了，用户也不知道变的是哪一段。
      const hourScope = history.from
        ? history.from === history.to
          ? history.from
          : `${history.from} → ${history.to}`
        : history.domain
          ? `网站 ${history.domain}`
          : `最近 ${CALENDAR_DAYS} 天`;
      hourPanel.append(
        node("div", "panel-heading", node("h2", "", `时段分布 · ${hourScope}`)),
        makeHourChart(stats.byHour),
      );
      lower.append(calPanel, hourPanel);
      page.append(lower);
    }

    // B站历史没有"网站"这个概念（只有 bilibili.com 和 live.bilibili.com），
    // 所以不给它网站筛选面板；只有浏览器历史才有。
    const main = node("div", "domain-content");
    const toolbar = node("div", "list-toolbar");
    toolbar.append(
      makeSearchForm(history.q, "搜索标题或网址…", (query) => {
        history.q = query;
        loadHistory(false);
      }),
    );
    main.append(toolbar);

    const listSummary = node("div", "list-summary");
    listSummary.append(
      node(
        "span",
        "",
        history.loading ? "正在读取账号档案…" : `${formatNumber(history.total)} 条记录`,
      ),
      node(
        "span",
        "summary-source",
        [isBili ? "来源 · B站" : "来源 · 浏览器", history.domain, history.from]
          .filter(Boolean)
          .join(" · "),
      ),
    );
    main.append(listSummary);

    const list = node("div", isBili ? "record-list bili-grid" : "record-list");
    main.append(list);
    if (history.error) list.append(makeMessage("error", history.error));
    if (!history.loading && !history.error && history.loaded && history.items.length === 0) {
      list.append(
        makeEmptyState(
          "⌕",
          "没有符合条件的记录",
          history.q || history.domain || history.from
            ? "试试清除筛选条件。"
            : isBili
              ? "B站历史需要浏览器里有 B站 登录状态；在扩展设置里打开「同步 B站历史」即可。"
              : "这个来源还没有记录；装扩展同步，或到「数据备份」导入 JSON。",
        ),
      );
    }

    let currentDay = "";
    let daySection = null;
    for (const item of history.items) {
      const key = item && item.visited_at ? localDayKey(item.visited_at) : "";
      if (key !== currentDay) {
        currentDay = key;
        daySection = node("section", "day-group");
        daySection.append(
          node("div", "day-heading", node("span", "day-title", key ? dayLabel(key) : "时间未知")),
        );
        list.append(daySection);
      }
      daySection.append(makeHistoryItem(item, source));
    }
    for (const group of list.querySelectorAll(".day-group")) {
      // 历史列表用 .record-card，B站历史用 .bili-card；两种都要算进当天条数。
      const count = group.querySelectorAll(".record-card, .bili-card").length;
      group.querySelector(".day-heading").append(node("span", "day-count", `${count} 条`));
    }

    if (
      !history.loading &&
      !history.error &&
      history.items.length > 0 &&
      hasNext(history.nextCursor)
    ) {
      const more = makeButton("加载更多", "button button-outline load-more");
      more.addEventListener("click", () => loadHistory(true));
      list.append(more);
    }

    if (isBili) {
      page.append(main);
    } else {
      const layout = node("div", "domain-layout");
      layout.append(makeDomainPanel(), main);
      page.append(layout);
    }
  }

  /** 网站筛选面板：只用于浏览器历史；B站历史没有"网站"这个概念。 */
  function makeDomainPanel() {
    const history = state.history;
    const aside = node("aside", "domain-panel");
    aside.setAttribute("aria-label", "网站筛选");
    const head = node("div", "folder-heading");
    const domainHeadCount = history.domain
      ? `${formatNumber(history.stats?.total ?? 0)} / ${formatNumber(history.stats?.domainTotal ?? 0)}`
      : formatNumber(history.stats?.domainTotal ?? 0);
    head.append(node("h2", "", "网站"), node("span", "", domainHeadCount));
    aside.append(head);

    const allRow = makeButton("", `folder-row folder-all${history.domain ? "" : " active"}`);
    allRow.append(
      node("span", "folder-name", "全部网站"),
      node("span", "folder-count", formatNumber(history.stats?.domainTotal ?? 0)),
    );
    allRow.addEventListener("click", () => {
      history.domain = "";
      loadHistory(false);
      loadHistoryStats();
    });
    aside.append(allRow);

    const search = node("label", "search-box search-compact");
    search.append(icon("⌕", "search-icon"));
    const searchInput = node("input", "");
    searchInput.type = "search";
    searchInput.value = history.domainQuery;
    searchInput.placeholder = "过滤网站…";
    searchInput.setAttribute("aria-label", "过滤网站列表");
    searchInput.addEventListener("input", () => {
      history.domainQuery = searchInput.value.trim().toLowerCase();
      render();
      const next = document.querySelector(".domain-panel .search-compact input");
      if (next) {
        next.focus();
        next.setSelectionRange(next.value.length, next.value.length);
      }
    });
    search.append(searchInput);
    aside.append(search);

    const domains = (history.stats?.domains ?? []).filter((entry) =>
      history.domainQuery ? entry.host.includes(history.domainQuery) : true,
    );
    if (!domains.length) {
      aside.append(
        node("p", "section-help", history.domainQuery ? "没有匹配的网站。" : "暂无数据。"),
      );
    }
    for (const entry of domains.slice(0, DOMAIN_LIMIT)) {
      const row = makeButton("", `folder-row${history.domain === entry.host ? " active" : ""}`);
      row.append(
        node("span", "folder-name", entry.host),
        node("span", "folder-count", formatNumber(entry.count)),
      );
      row.title = `只看 ${entry.host}`;
      row.addEventListener("click", () => {
        history.domain = history.domain === entry.host ? "" : entry.host;
        loadHistory(false);
        loadHistoryStats();
      });
      aside.append(row);
    }
    if (domains.length > DOMAIN_LIMIT) {
      aside.append(
        node(
          "p",
          "section-help",
          `另有 ${domains.length - DOMAIN_LIMIT} 个网站未列出，可用上方搜索。`,
        ),
      );
    }
    return aside;
  }

  const BUSINESS_LABELS = {
    archive: "视频",
    pgc: "番剧",
    article: "专栏",
    "article-list": "文集",
    live: "直播",
    cheese: "课程",
  };

  function formatDuration(seconds) {
    const total = Math.max(0, Math.floor(Number(seconds) || 0));
    const hours = Math.floor(total / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    const secs = total % 60;
    const pad = (value) => String(value).padStart(2, "0");
    return hours > 0 ? `${hours}:${pad(minutes)}:${pad(secs)}` : `${pad(minutes)}:${pad(secs)}`;
  }

  /**
   * B站历史卡片：16:9 封面 + 观看进度条 + 类型/已看完/已收藏角标 + UP主。
   * 刻意不复用浏览器历史的行式布局，两者内容差别很大。
   */
  function makeBiliCard(item) {
    const card = node("article", "bili-card");
    const url = safeHttpUrl(item && item.url);
    const title = safeString(item && item.title, "未命名内容").trim() || "未命名内容";
    const cover = safeHttpUrl(upgradeHttps(safeString(item && item.cover, "")));
    const progress = Number(item && item.progress);
    const duration = Number(item && item.duration);
    const hasProgress = progress > 0 && duration > 0;
    const percent = hasProgress ? Math.min(100, Math.round((progress / duration) * 100)) : 0;

    const thumb = node(url ? "a" : "div", "bili-cover");
    if (url) {
      thumb.href = url;
      thumb.target = "_blank";
      thumb.rel = "noopener noreferrer";
      thumb.setAttribute("aria-label", `打开：${title}`);
    }
    if (cover) {
      const image = node("img", "bili-cover-img");
      image.src = cover;
      image.alt = "";
      image.loading = "lazy";
      image.decoding = "async";
      // 不把本地页面的地址作为 referrer 发给 B站 CDN。
      image.referrerPolicy = "no-referrer";
      image.addEventListener("error", () => {
        image.remove();
      });
      thumb.append(image);
    } else {
      thumb.append(node("span", "bili-cover-fallback", "▶"));
    }

    if (hasProgress) {
      const bar = node("span", "bili-progress");
      const fill = node("span", "bili-progress-fill");
      fill.style.width = `${percent}%`;
      bar.append(fill);
      thumb.append(bar);
    }

    const badges = node("div", "bili-badges");
    if (progress === -1) badges.append(node("span", "bili-badge is-done", "已看完"));
    else if (hasProgress) {
      badges.append(
        node(
          "span",
          "bili-badge",
          `${formatDuration(progress)} / ${formatDuration(duration)} · ${percent}%`,
        ),
      );
    }
    if (item && item.business && item.business !== "archive") {
      badges.append(
        node("span", "bili-badge is-type", BUSINESS_LABELS[item.business] ?? item.business),
      );
    }
    if (item && item.tagName) badges.append(node("span", "bili-badge", safeString(item.tagName)));
    if (item && item.isFav) badges.append(node("span", "bili-badge", "已收藏"));
    if (badges.childElementCount) thumb.append(badges);
    card.append(thumb);

    const body = node("div", "bili-body");
    const titleNode = url ? makeLink(url, title, "bili-title") : node("h3", "bili-title", title);
    if (url) {
      titleNode.target = "_blank";
      titleNode.rel = "noopener noreferrer";
    }
    body.append(titleNode);

    const meta = node("div", "bili-meta");
    const authorName = safeString(item && item.authorName, "").trim();
    const authorMid = Number(item && item.authorMid);
    if (authorName || authorMid) {
      const authorUrl =
        Number.isFinite(authorMid) && authorMid > 0
          ? `https://space.bilibili.com/${authorMid}`
          : null;
      const author = authorUrl
        ? makeLink(authorUrl, authorName || `UID ${authorMid}`, "bili-author")
        : node("span", "bili-author", authorName || `UID ${authorMid}`);
      if (authorUrl) {
        author.target = "_blank";
        author.rel = "noopener noreferrer";
      }
      meta.append(author);
    } else {
      meta.append(node("span", "bili-author is-muted", "UP主未知"));
    }
    meta.append(node("time", "bili-time", formatDate(item && item.visited_at)));
    body.append(meta);
    card.append(body);
    return card;
  }

  function makeHistoryItem(item, source) {
    if (source === "bilibili") return makeBiliCard(item);
    const record = node("article", "record-card");
    const visual = node("div", "record-mark browser", "◉");
    const body = node("div", "record-body");
    const title = safeString(item && item.title, "未命名记录").trim() || "未命名记录";
    const url = safeHttpUrl(item && item.url);
    const titleNode = url
      ? makeLink(url, title, "record-title")
      : node("span", "record-title", title);
    if (url) {
      titleNode.target = "_blank";
      titleNode.rel = "noopener noreferrer";
    }
    const meta = node("div", "record-meta");
    meta.append(node("span", "record-time", formatTime(item && item.visited_at)));
    if (url) meta.append(metaDot(), node("span", "record-host", new URL(url).hostname));
    if (item && item.deviceName)
      meta.append(metaDot(), node("span", "", safeString(item.deviceName)));
    body.append(titleNode, meta);
    record.append(visual, body);
    if (url) record.append(node("span", "record-open", "↗"));
    return record;
  }

  // 旧数据可能存了 http:// 或 // 的封面，站点 CSP 只放行 https，这里统一升级。
  function upgradeHttps(value) {
    if (!value) return null;
    if (value.indexOf('//') === 0) return 'https:' + value;
    if (value.indexOf('http://') === 0) return 'https://' + value.slice(7);
    return value;
  }

  function metaDot() {
    return node("span", "meta-dot", "·");
  }

  function hasNext(cursor) {
    return (
      cursor !== null && cursor !== undefined && cursor !== "" && cursor !== false && cursor !== 0
    );
  }

  function makeSearchForm(value, placeholder, onSubmit) {
    const form = node("form", "toolbar-form");
    const search = node("label", "search-box");
    search.append(icon("⌕", "search-icon"));
    const input = node("input", "");
    input.type = "search";
    input.value = value;
    input.placeholder = placeholder;
    input.setAttribute("aria-label", "搜索档案");
    search.append(input);
    form.append(search);
    form.append(makeButton("搜索", "button button-primary button-compact", "submit"));
    form.addEventListener("submit", (event) => {
      event.preventDefault();
      onSubmit(input.value.trim());
    });
    return form;
  }

  async function loadHistoryStats() {
    const history = state.history;
    if (history.statsLoading) return;
    history.statsLoading = true;
    history.statsError = "";
    try {
      const params = new URLSearchParams({
        source: history.source,
        days: String(CALENDAR_DAYS),
      });
      if (history.domain) params.set("domain", history.domain);
      // 时段分布要跟着日期窗口走，否则选中某一天时它会一动不动。
      if (history.from) params.set("from", history.from);
      if (history.to) params.set("to", history.to);
      const { data } = await request(`${API}/stats?${params.toString()}`);
      history.stats = data && typeof data === "object" ? data : null;
    } catch (error) {
      history.statsError = error instanceof Error ? error.message : "读取统计失败。";
    } finally {
      history.statsLoading = false;
      render();
    }
  }

  async function loadHistory(append) {
    const history = state.history;
    if (history.loading) return;
    const cursor = append ? history.nextCursor : 0;
    history.loading = true;
    history.error = "";
    if (!append) {
      history.items = [];
      history.nextCursor = null;
    }
    render();
    try {
      const params = new URLSearchParams({
        source: history.source,
        q: history.q,
        cursor: String(cursor ?? 0),
        limit: String(PAGE_SIZE),
      });
      if (history.from) params.set("from", history.from);
      if (history.to) params.set("to", history.to);
      if (history.domain) params.set("domain", history.domain);
      const { data } = await request(`${API}/history?${params.toString()}`);
      const items = Array.isArray(data && data.items) ? data.items : [];
      history.items = append ? history.items.concat(items) : items;
      history.total = Number.isFinite(Number(data && data.total))
        ? Number(data.total)
        : history.items.length;
      history.nextCursor = data && data.nextCursor !== undefined ? data.nextCursor : null;
      history.loaded = true;
    } catch (error) {
      history.error = error instanceof Error ? error.message : "读取历史失败。";
      history.loaded = true;
    } finally {
      history.loading = false;
      render();
    }
  }

  /* ---------- 收藏页 ---------- */

  /** 把扁平的 "toolbar / a / b" 路径折叠成树，父节点累计子节点数量。 */
  function buildFolderTree(folders, root) {
    const base = { name: root, path: root, direct: 0, count: 0, children: new Map() };
    for (const entry of folders) {
      const parts = (entry.path || "").split(" / ").filter(Boolean);
      let cursor = base;
      let path = "";
      for (const part of parts) {
        path = path ? `${path} / ${part}` : part;
        if (!cursor.children.has(part)) {
          cursor.children.set(part, { name: part, path, direct: 0, count: 0, children: new Map() });
        }
        cursor = cursor.children.get(part);
      }
      cursor.direct += entry.count;
    }
    const total = (folder) => {
      let sum = folder.direct;
      for (const child of folder.children.values()) sum += total(child);
      folder.count = sum;
      return sum;
    };
    total(base);
    return base;
  }

  function renderFolderTree(folder, depth, container) {
    const entries = [...folder.children.values()].sort(
      (a, b) => b.count - a.count || a.name.localeCompare(b.name, "zh-CN"),
    );
    for (const entry of entries) {
      const hasChildren = entry.children.size > 0;
      const collapsed = state.favorite.collapsed.has(entry.path);
      const line = node("div", "folder-line");
      line.style.paddingLeft = `${6 + depth * 13}px`;

      const caret = node("span", "folder-caret", hasChildren ? (collapsed ? "▸" : "▾") : "·");
      if (hasChildren) {
        caret.title = collapsed ? "展开" : "折叠";
        caret.setAttribute("role", "button");
        caret.setAttribute("tabindex", "0");
        const toggle = () => {
          if (collapsed) state.favorite.collapsed.delete(entry.path);
          else state.favorite.collapsed.add(entry.path);
          render();
        };
        caret.addEventListener("click", toggle);
        caret.addEventListener("keydown", (event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            toggle();
          }
        });
      } else {
        caret.classList.add("is-leaf");
      }
      line.append(caret);

      const row = makeButton(
        "",
        `folder-row${state.favorite.folder === entry.path ? " active" : ""}`,
      );
      row.append(
        node("span", "folder-name", entry.name),
        node("span", "folder-count", formatNumber(entry.count)),
      );
      row.title = `只看 ${entry.path}`;
      row.addEventListener("click", () => {
        state.favorite.folder = state.favorite.folder === entry.path ? "" : entry.path;
        loadFavorites(false);
      });
      line.append(row);
      container.append(line);

      if (hasChildren && !collapsed) renderFolderTree(entry, depth + 1, container);
    }
  }

  function makeSnapshotBar() {
    const favorites = state.favorite;
    const bar = node("div", "snapshot-bar");
    const label = node("label", "snapshot-control");
    label.append(node("span", "select-label", "列表版本"));
    const select = node("select", "");
    const currentOption = node("option", "", "当前（最新）");
    currentOption.value = "";
    select.append(currentOption);
    for (const snapshot of favorites.snapshots) {
      const option = node(
        "option",
        "",
        `${formatDate(snapshot.createdAt)} · ${formatNumber(snapshot.count)} 个书签`,
      );
      option.value = snapshot.id;
      select.append(option);
    }
    select.value = favorites.snapshot;
    select.addEventListener("change", () => {
      favorites.snapshot = select.value;
      loadFavorites(false);
    });
    label.append(select);
    bar.append(label);

    if (favorites.snapshot) {
      const chosen = favorites.snapshots.find((item) => item.id === favorites.snapshot);
      const banner = node("div", "snapshot-banner");
      const copy = node("div", "", "");
      copy.append(
        node("strong", "", `正在查看历史版本${chosen ? ` · ${formatDate(chosen.createdAt)}` : ""}`),
        node("span", "section-help", "这是整份收藏列表的快照，只读。"),
      );
      banner.append(copy);
      const back = makeButton("回到最新", "button button-soft button-tiny");
      back.addEventListener("click", () => {
        favorites.snapshot = "";
        loadFavorites(false);
      });
      banner.append(back);
      bar.append(banner);
    } else if (!favorites.snapshots.length) {
      bar.append(
        node("span", "section-help", "还没有历史版本；每次导入书签后会留下整份列表的快照。"),
      );
    }
    return bar;
  }

  function renderFavorite(page) {
    const favorites = state.favorite;
    page.append(
      setPageTitle("收藏列表", "按 Firefox 书签文件夹归类；每次导入都会留下整份列表的快照。"),
    );
    page.append(makeSnapshotBar());

    const layout = node("div", "folder-layout");
    const aside = node("aside", "folder-panel");
    aside.setAttribute("aria-label", "书签文件夹");
    const head = node("div", "folder-heading");
    head.append(node("h2", "", "文件夹"), node("span", "", formatNumber(favorites.foldersTotal)));
    aside.append(head);

    const allRow = makeButton("", `folder-row folder-all${favorites.folder ? "" : " active"}`);
    allRow.append(
      node("span", "folder-name", "全部书签"),
      node("span", "folder-count", formatNumber(favorites.foldersTotal)),
    );
    allRow.addEventListener("click", () => {
      favorites.folder = "";
      loadFavorites(false);
    });
    aside.append(allRow);

    const bulk = node("div", "folder-bulk");
    const expandAll = makeButton("展开全部", "button button-soft button-tiny");
    expandAll.addEventListener("click", () => {
      favorites.collapsed.clear();
      render();
    });
    const collapseAll = makeButton("折叠全部", "button button-soft button-tiny");
    collapseAll.addEventListener("click", () => {
      const tree = buildFolderTree(favorites.folders, "toolbar");
      const walk = (folder) => {
        for (const child of folder.children.values()) {
          if (child.children.size) favorites.collapsed.add(child.path);
          walk(child);
        }
      };
      walk(tree);
      render();
    });
    bulk.append(expandAll, collapseAll);
    aside.append(bulk);

    if (favorites.foldersLoading) aside.append(node("p", "section-help", "正在统计文件夹…"));
    if (favorites.foldersError) aside.append(makeMessage("error", favorites.foldersError));
    // 只渲染 toolbar 子树本身，因为归档范围内的书签都在它下面。
    renderFolderTree(buildFolderTree(favorites.folders, "toolbar"), 0, aside);
    if (!favorites.folders.length && !favorites.foldersLoading) {
      aside.append(node("p", "section-help", "toolbar 下暂无书签。"));
    }

    const main = node("div", "folder-content");
    const toolbar = node("div", "list-toolbar");
    toolbar.append(
      makeSearchForm(favorites.q, "搜索标题、网址或目录…", (query) => {
        favorites.q = query;
        loadFavorites(false);
      }),
      makeSelect(
        "归档范围",
        favorites.status,
        [
          { value: "active", label: "仍在 Firefox 中" },
          { value: "deleted", label: "已在 Firefox 删除" },
          { value: "all", label: "两者都显示" },
        ],
        (value) => {
          favorites.status = value;
          loadFavorites(false);
        },
      ),
    );
    main.append(toolbar);

    const listSummary = node("div", "list-summary");
    listSummary.append(
      node("span", "", favorites.loading ? "正在读取…" : `${formatNumber(favorites.total)} 个书签`),
      node(
        "span",
        "summary-source",
        favorites.folder ? `目录 · ${favorites.folder}` : "目录 · 全部（仅 toolbar）",
      ),
    );
    main.append(listSummary);

    const list = node("div", "record-list favorite-list");
    main.append(list);
    if (favorites.error) list.append(makeMessage("error", favorites.error));
    if (
      !favorites.loading &&
      !favorites.error &&
      favorites.loaded &&
      favorites.items.length === 0
    ) {
      list.append(
        makeEmptyState(
          "◇",
          "这个范围里没有书签",
          favorites.q ? "试试更短的关键词。" : "换一个文件夹，或先导入 Firefox 书签。",
        ),
      );
    }
    for (const item of favorites.items) list.append(makeFavoriteCard(item));
    if (
      !favorites.loading &&
      !favorites.error &&
      favorites.items.length > 0 &&
      hasNext(favorites.nextCursor)
    ) {
      const more = makeButton("加载更多", "button button-outline load-more");
      more.addEventListener("click", () => loadFavorites(true));
      list.append(more);
    }

    layout.append(aside, main);
    page.append(layout);
  }

  function makeFavoriteCard(item) {
    const card = node("article", "record-card favorite-card");
    const visual = node("div", "record-mark folder", "◇");
    const body = node("div", "record-body");
    const title = safeString(item && item.title, "未命名书签").trim() || "未命名书签";
    const url = safeHttpUrl(item && item.url);
    const titleNode = url
      ? makeLink(url, title, "record-title")
      : node("span", "record-title", title);
    if (url) {
      titleNode.target = "_blank";
      titleNode.rel = "noopener noreferrer";
    }
    const chips = node("div", "chip-row");
    chips.append(node("span", "chip", item && item.deletedAt ? "源已删除" : "在档"));
    if (item && item.folder) chips.append(node("span", "chip chip-folder", item.folder));
    if (item && item.deviceName) chips.append(node("span", "chip", item.deviceName));
    const meta = node("div", "record-meta");
    meta.append(node("span", "", `更新 ${formatDate(item && item.updated_at)}`));
    body.append(titleNode, chips, meta);
    card.append(visual, body);
    if (url) card.append(node("span", "record-open", "↗"));
    return card;
  }

  function makeSelect(label, value, options, onChange) {
    const wrap = node("label", "select-control");
    wrap.append(node("span", "select-label", label));
    const select = node("select", "");
    for (const option of options) {
      const item = node("option", "", option.label);
      item.value = option.value;
      if (option.value === value) item.selected = true;
      select.append(item);
    }
    select.addEventListener("change", () => onChange(select.value));
    wrap.append(select);
    return wrap;
  }

  async function loadFavoriteFolders() {
    const favorites = state.favorite;
    if (favorites.foldersLoading) return;
    favorites.foldersLoading = true;
    favorites.foldersError = "";
    try {
      const { data } = await request(`${API}/favorite/folders?origin=browser&root=toolbar`);
      favorites.folders = Array.isArray(data && data.folders) ? data.folders : [];
      favorites.foldersTotal = Number(data && data.total) || 0;
      favorites.foldersLoose = Number(data && data.loose) || 0;
    } catch (error) {
      favorites.foldersError = error instanceof Error ? error.message : "读取文件夹失败。";
    } finally {
      favorites.foldersLoading = false;
      render();
    }
  }

  async function loadSnapshots() {
    const favorites = state.favorite;
    if (favorites.snapshotsLoading) return;
    favorites.snapshotsLoading = true;
    favorites.snapshotsError = "";
    try {
      const { data } = await request(`${API}/favorite/snapshots`);
      favorites.snapshots = Array.isArray(data && data.snapshots) ? data.snapshots : [];
    } catch (error) {
      favorites.snapshotsError = error instanceof Error ? error.message : "读取列表版本失败。";
    } finally {
      favorites.snapshotsLoading = false;
      render();
    }
  }

  async function loadFavorites(append) {
    const favorites = state.favorite;
    if (favorites.loading) return;
    const cursor = append ? favorites.nextCursor : 0;
    favorites.loading = true;
    favorites.error = "";
    if (!append) {
      favorites.items = [];
      favorites.nextCursor = null;
    }
    render();
    try {
      const params = new URLSearchParams({
        origin: "browser",
        status: favorites.status,
        root: "toolbar",
        q: favorites.q,
        cursor: String(cursor ?? 0),
        limit: String(PAGE_SIZE),
      });
      if (favorites.folder) {
        params.set("folder", favorites.folder);
        if (favorites.sub) params.set("sub", "1");
      }
      if (favorites.snapshot) params.set("snapshot", favorites.snapshot);
      const { data } = await request(`${API}/favorite?${params.toString()}`);
      const items = Array.isArray(data && data.items) ? data.items : [];
      favorites.items = append ? favorites.items.concat(items) : items;
      favorites.total = Number.isFinite(Number(data && data.total))
        ? Number(data.total)
        : favorites.items.length;
      favorites.nextCursor = data && data.nextCursor !== undefined ? data.nextCursor : null;
      favorites.loaded = true;
    } catch (error) {
      favorites.error = error instanceof Error ? error.message : "读取收藏失败。";
      favorites.loaded = true;
    } finally {
      favorites.loading = false;
      render();
    }
  }

  /* ---------- 数据备份 ---------- */

  async function loadDevices() {
    state.devices.loading = true;
    state.devices.error = "";
    render();
    try {
      const { data } = await request(`${API}/devices`);
      state.devices.items = Array.isArray(data && data.devices) ? data.devices : [];
    } catch (error) {
      state.devices.error = error instanceof Error ? error.message : "读取配对设备失败";
    } finally {
      state.devices.loading = false;
      state.devices.loaded = true;
      render();
    }
  }

  async function createDeviceToken() {
    state.devices.busy = true;
    state.devices.error = "";
    render();
    try {
      const { data } = await request(`${API}/devices`, { method: "POST", body: { label: "浏览器扩展" } });
      state.devices.newToken = (data && data.token) || "";
      await loadDevices();
    } catch (error) {
      state.devices.error = error instanceof Error ? error.message : "生成配对码失败";
    } finally {
      state.devices.busy = false;
      render();
    }
  }

  async function revokeDevice(id) {
    state.devices.busy = true;
    try {
      await request(`${API}/devices?id=${encodeURIComponent(id)}`, { method: "DELETE" });
      state.devices.newToken = "";
      await loadDevices();
    } catch (error) {
      state.devices.error = error instanceof Error ? error.message : "撤销失败";
    } finally {
      state.devices.busy = false;
      render();
    }
  }

  function formatSyncTime(value) {
    return value ? formatDate(value) : "还没同步过";
  }

  function renderExtensionPanel(panel) {
    const devices = state.devices;
    const head = node("div", "panel-heading");
    head.append(
      node("span", "panel-icon", "⟳"),
      node("div", "panel-heading-copy", node("h2", "", "浏览器扩展 · 自动同步")),
    );
    head
      .querySelector(".panel-heading-copy")
      .append(node("p", "", "装一次扩展，之后每 30 分钟自动同步浏览器历史和书签，不用再手动导入。"));
    panel.append(head);

    const steps = node("ol", "extension-steps");
    steps.append(
      node("li", "", "按你的浏览器下载对应的扩展包，并解压到一个固定文件夹（别删，删了扩展就失效）。"),
      node("li", "", "Chrome / Edge：打开扩展页开启「开发者模式」→「加载已解压的扩展程序」。Firefox：打开 about:debugging#/runtime/this-firefox →「临时载入附加组件」选中解压后的 manifest.json。"),
      node("li", "", "点下面的「生成配对码」，把站点地址和配对码填进扩展的「设置」页，再点「立即同步」。"),
    );
    panel.append(steps);

    const actions = node("div", "extension-actions");
    const chromeZip = makeLink(`${API}/extension.zip?browser=chrome`, "下载 Chrome / Edge 版", "button button-primary button-compact");
    chromeZip.setAttribute("download", "wasteland-history-sync-chrome.zip");
    const firefoxZip = makeLink(`${API}/extension.zip?browser=firefox`, "下载 Firefox 版", "button button-outline button-compact");
    firefoxZip.setAttribute("download", "wasteland-history-sync-firefox.zip");
    actions.append(chromeZip, firefoxZip);

    const gen = makeButton(devices.busy ? "生成中…" : "生成配对码", "button button-outline button-compact");
    gen.disabled = devices.busy;
    gen.addEventListener("click", () => createDeviceToken());
    actions.append(gen);
    panel.append(actions);

    panel.append(
      node(
        "p",
        "section-help",
        "Firefox 正式版只允许临时载入未签名扩展：重启浏览器后需要在 about:debugging 里再载入一次（不需要重新配对）；Firefox Developer Edition / Nightly 可关闭签名校验后永久安装。Chrome / Edge 装一次即可长期有效。",
      ),
    );

    if (devices.error) panel.append(makeMessage("error", devices.error));

    if (devices.newToken) {
      const tokenBox = node("div", "token-box");
      tokenBox.append(node("span", "token-label", "配对码（只显示这一次，请立刻填进扩展）"));
      const code = node("code", "token-value", devices.newToken);
      const copy = makeButton("复制", "button button-outline button-compact");
      copy.addEventListener("click", () => {
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(devices.newToken).then(() => {
            copy.textContent = "已复制";
            window.setTimeout(() => (copy.textContent = "复制"), 1500);
          });
        }
      });
      tokenBox.append(code, copy);
      const origin = node("p", "section-help", `站点地址填：${location.origin}`);
      tokenBox.append(origin);
      panel.append(tokenBox);
    }

    const list = node("div", "device-list");
    if (!devices.loading && !devices.loaded) {
      list.append(node("p", "section-help", "正在读取配对设备…"));
    } else if (!devices.items.length) {
      list.append(node("p", "section-help", "还没有配对设备。生成配对码后，扩展才能同步到你的账号。"));
    } else {
      for (const d of devices.items) {
        const row = node("div", "device-row");
        row.append(
          node("span", "device-name", d.label || "浏览器扩展"),
          node("span", "device-sync", `最近同步：${formatSyncTime(d.lastSync)}`),
        );
        const revoke = makeButton("撤销", "button button-outline button-compact");
        revoke.disabled = devices.busy;
        revoke.addEventListener("click", () => revokeDevice(d.id));
        row.append(revoke);
        list.append(row);
      }
    }
    panel.append(list);
  }

  function renderBackup(page) {
    const backup = state.backup;
    page.append(setPageTitle("数据备份", "推荐装扩展自动同步；也可以手动导入 JSON 档案。"));

    const backupPanel = node("section", "backup-panel");
    renderExtensionPanel(backupPanel);
    backupPanel.append(node("div", "panel-divider"));
    const importSection = node("div", "backup-section");
    importSection.append(
      node("h3", "", "导入 JSON 档案"),
      node("p", "section-help", "选择 JSON 文件后先查看内容摘要与服务端提示，再决定是否确认导入。"),
    );
    const importForm = node("form", "import-form");
    const fileLabel = node("label", "file-picker");
    const fileInput = node("input", "file-input");
    fileInput.type = "file";
    fileInput.accept = ".json,application/json";
    fileInput.setAttribute("aria-label", "选择 JSON 备份文件");
    const fileCopy = node("span", "file-picker-copy");
    fileCopy.append(
      node("span", "file-picker-icon", "＋"),
      node("strong", "", backup.file ? backup.file.name : "选择 JSON 文件"),
      node(
        "small",
        "",
        backup.file ? formatFileSize(backup.file.size) : "仅在你点击预览后读取文件",
      ),
    );
    fileLabel.append(fileInput, fileCopy);
    fileInput.addEventListener("change", () => {
      backup.file = fileInput.files && fileInput.files[0] ? fileInput.files[0] : null;
      backup.preview = null;
      backup.result = null;
      backup.error = "";
      render();
    });
    importForm.append(fileLabel);
    const preview = makeButton(
      backup.loading ? "处理中…" : "预览导入内容",
      "button button-primary button-compact",
      "submit",
    );
    preview.disabled = backup.loading || !backup.file;
    importForm.append(preview);
    importForm.addEventListener("submit", (event) => {
      event.preventDefault();
      previewImport();
    });
    importSection.append(importForm);
    if (backup.error) importSection.append(makeMessage("error", backup.error));
    if (backup.preview) importSection.append(makeImportPreview(backup.preview, backup.loading));
    if (backup.result) importSection.append(makeImportResult(backup.result));
    backupPanel.append(importSection);

    backupPanel.append(node("div", "panel-divider"));
    const exportSection = node("div", "backup-section export-section");
    exportSection.append(node("div", "export-copy", node("h3", "", "下载完整备份")));
    exportSection
      .querySelector(".export-copy")
      .append(
        node(
          "p",
          "section-help",
          "下载当前档案的完整 JSON。注意：收藏列表的整表快照只存在本机 SQLite，不进入这份 JSON。",
        ),
      );
    const download = makeButton(
      backup.loading ? "正在处理…" : "下载 JSON 备份  ↗",
      "button button-outline button-compact",
    );
    download.disabled = backup.loading;
    download.addEventListener("click", downloadBackup);
    exportSection.append(download);
    backupPanel.append(exportSection);
    page.append(backupPanel);

    page.append(
      node(
        "p",
        "backup-footnote",
        "导入内容会归入当前账号；备份文件下载到你的设备，请妥善保管。",
      ),
    );
  }

  function formatFileSize(size) {
    if (!Number.isFinite(size)) return "JSON 文件";
    if (size < 1024) return `${size} B`;
    return `${(size / 1024).toFixed(size < 1024 * 1024 ? 1 : 0)} KB`;
  }

  function makeImportPreview(preview, loading) {
    const box = node("div", "import-preview");
    const head = node("div", "preview-heading");
    head.append(node("span", "preview-check", "✓"), node("div", "", ""));
    head
      .querySelector("div")
      .append(
        node("strong", "", "服务端预览已就绪"),
        node("span", "", "尚未导入；请核对数量与提示"),
      );
    box.append(head);

    const counts = preview.counts && typeof preview.counts === "object" ? preview.counts : {};
    const countGrid = node("div", "count-grid");
    countGrid.append(
      makeCount("B站历史", counts.bilibili),
      makeCount("浏览器记录", counts.browser),
      makeCount("浏览器书签", counts.favorites),
    );
    box.append(countGrid);
    if (Array.isArray(preview.warnings) && preview.warnings.length) {
      const warningList = node("ul", "warning-list");
      warningList.append(node("li", "warning-title", "服务端提示"));
      for (const warning of preview.warnings)
        warningList.append(node("li", "", safeString(warning, "未说明的提示")));
      box.append(warningList);
    } else {
      box.append(node("p", "no-warnings", "服务端未返回额外提示。"));
    }
    const previewId = safeString(preview.previewId, "");
    if (!previewId) {
      box.append(makeMessage("error", "服务端没有返回有效的预览编号，无法确认导入。"));
      return box;
    }
    const confirmRow = node("div", "confirm-import-row");
    confirmRow.append(node("span", "", `预览编号：${previewId}`));
    const confirm = makeButton(
      loading ? "处理中…" : "确认导入这些内容",
      "button button-primary button-compact",
    );
    confirm.disabled = loading;
    confirm.addEventListener("click", confirmImport);
    confirmRow.append(confirm);
    box.append(confirmRow);
    return box;
  }

  function makeCount(label, value) {
    const card = node("div", "count-card");
    card.append(
      node("strong", "", Number.isFinite(Number(value)) ? String(Number(value)) : "—"),
      node("span", "", label),
    );
    return card;
  }

  function makeImportResult(result, title = "导入完成") {
    const box = node("div", "result-box");
    box.append(node("strong", "", title));
    const values = result && typeof result === "object" ? result : {};
    const line = node("div", "result-counts");
    line.append(
      node("span", "", `新增 ${safeString(values.added, "0")}`),
      node("span", "", `更新 ${safeString(values.updated, "0")}`),
      node("span", "", `跳过 ${safeString(values.skipped, "0")}`),
    );
    if (Number(values.snapshots) > 0) {
      line.append(node("span", "result-versioned", "新增列表版本 1"));
    }
    box.append(line);
    return box;
  }

  function invalidateAfterWrite() {
    state.favorite.loaded = false;
    state.favorite.folders = [];
    state.favorite.snapshots = [];
    state.favorite.snapshot = "";
    state.history.loaded = false;
    state.history.stats = null;
  }

  async function previewImport() {
    const backup = state.backup;
    if (!backup.file || backup.loading) return;
    backup.loading = true;
    backup.preview = null;
    backup.result = null;
    backup.error = "";
    render();
    try {
      const raw = await backup.file.text();
      // 服务端对导入接口的上限是 20MB；提前拦住，避免浏览器只报"无法连接"。
      if (raw.length > 20 * 1024 * 1024) {
        throw new Error(
          `文件约 ${(raw.length / 1048576).toFixed(1)}MB，超过单次导入上限 20MB，请分批导出后再导入。`,
        );
      }
      let parsed;
      try {
        parsed = JSON.parse(raw);
      } catch (_error) {
        throw new Error("文件不是有效的 JSON。请检查文件内容后重试。");
      }
      const { data } = await request(`${API}/import/preview`, {
        method: "POST",
        body: { data: parsed },
      });
      if (!data || typeof data !== "object") throw new Error("本地服务返回的预览结果无效。");
      backup.preview = data;
    } catch (error) {
      backup.error = error instanceof Error ? error.message : "读取文件预览失败。";
    } finally {
      backup.loading = false;
      render();
    }
  }

  async function confirmImport() {
    const backup = state.backup;
    const previewId = backup.preview && backup.preview.previewId;
    if (!previewId || backup.loading) return;
    if (!window.confirm("确认将预览中的内容导入你的账号档案吗？")) return;
    backup.loading = true;
    backup.error = "";
    backup.result = null;
    render();
    try {
      const { data } = await request(`${API}/import/confirm`, {
        method: "POST",
        body: { previewId },
      });
      backup.result = data || {};
      backup.preview = null;
      invalidateAfterWrite();
    } catch (error) {
      backup.error = error instanceof Error ? error.message : "确认导入失败。";
    } finally {
      backup.loading = false;
      render();
    }
  }

  async function downloadBackup() {
    const backup = state.backup;
    if (backup.loading) return;
    backup.loading = true;
    backup.error = "";
    render();
    try {
      const { response, data } = await request(`${API}/backup`);
      if (!data || typeof data !== "object") throw new Error("本地服务返回的备份不是有效的 JSON。");
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
      const disposition = response.headers.get("content-disposition") || "";
      const match = disposition.match(/filename\*?=(?:UTF-8''|")?([^;"]+)/i);
      const filename = match
        ? decodeURIComponent(match[1].replace(/"/g, "").trim())
        : "unlimited-history-backup.json";
      const downloadUrl = URL.createObjectURL(blob);
      const link = node("a", "");
      link.href = downloadUrl;
      link.download = filename.replace(/[\\/:*?"<>|]/g, "_");
      link.hidden = true;
      document.body.append(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(downloadUrl), 1000);
    } catch (error) {
      backup.error = error instanceof Error ? error.message : "下载备份失败。";
    } finally {
      backup.loading = false;
      render();
    }
  }

  /* ---------- 引导 ---------- */

  function render() {
    state.page = getRoute();
    const { shell, page } = makeShell();
    if (state.page === "site") renderSitePage(page);
    else if (state.page === "favorite") renderFavorite(page);
    else if (state.page === "backup") renderBackup(page);
    else renderHistoryPage(page, state.page);
    app.replaceChildren(shell);
  }

  async function ensureHistoryPage() {
    const wanted = state.page === "bilibili" ? "bilibili" : "browser";
    if (state.history.source !== wanted) {
      state.history.source = wanted;
      state.history.items = [];
      state.history.loaded = false;
      state.history.stats = null;
      state.history.q = "";
      state.history.from = "";
      state.history.to = "";
      state.history.domain = "";
      state.history.domainQuery = "";
    }
    if (!state.history.loaded) await loadHistory(false);
    if (!state.history.stats) await loadHistoryStats();
  }

  async function ensureFavoritePage() {
    if (!state.favorite.snapshots.length && !state.favorite.snapshotsLoading) void loadSnapshots();
    if (!state.favorite.folders.length) await loadFavoriteFolders();
    if (!state.favorite.loaded) await loadFavorites(false);
  }

  async function initialize() {
    render();
    try {
      const { data } = await request(`${API}/me`);
      state.me = data && typeof data === "object" ? data : null;
      state.meError = "";
    } catch (error) {
      state.meError = error instanceof Error ? error.message : "服务暂不可用。";
    } finally {
      state.meLoading = false;
      render();
    }

    if (state.page === "favorite") await ensureFavoritePage();
    else if (state.page === "site") await ensureSitePage();
    else if (state.page === "backup") {
      if (!state.devices.loaded) await loadDevices();
    } else await ensureHistoryPage();
  }

  window.addEventListener("hashchange", () => {
    const next = getRoute();
    if (next === state.page) return;
    state.page = next;
    render();
    if (next === "favorite") void ensureFavoritePage();
    else if (next === "site") void ensureSitePage();
    else if (next === "backup") {
      if (!state.devices.loaded) void loadDevices();
    } else void ensureHistoryPage();
  });

  initialize();
})();
