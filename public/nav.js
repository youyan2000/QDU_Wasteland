// nav.js — 全站统一顶部导航注入
// 导航项：课程资料 / 论坛 / 文章 / 我的风采
// 右上角：搜索 + 铃铛(公告/通知/私信信息面板) + 登录态
(function () {
  const holder = document.getElementById('site-nav');
  if (!holder) return;

  const path = location.pathname;
  function isActive(prefix) {
    if (prefix === '/login.html' || prefix === '/me.html') return path === prefix;
    return path.startsWith(prefix);
  }

  const links = [
    { href: '/courses.html', label: '课程资料' },
    { href: '/forum.html', label: '论坛' },
    { href: '/articles.html', label: '文章' },
    { href: '/me.html', label: '我的风采' }
  ];

  holder.innerHTML = `
    <nav class="site-nav">
      <div class="nav-box">
        <a class="nav-logo" href="/">Wasteland</a>
        <div class="nav-links">
          ${links.map(l => `<a href="${l.href}" class="${isActive(l.href) ? 'active' : ''}">${l.label}</a>`).join('')}
        </div>
      </div>
      <div class="nav-right">
        <form class="nav-search" action="/search.html" method="get" onsubmit="event.preventDefault();location.href='/search.html?q='+encodeURIComponent(this.q.value.trim())">
          <input type="search" name="q" placeholder="搜索…" autocomplete="off">
        </form>
        <button class="nav-bell" id="navBell" title="公告 / 通知 / 私信">
          ${ICONS.bell(18)}<span id="bellCount" class="bell-count" style="display:none">0</span>
        </button>
        <button class="nav-bell" id="navFeedbackBtn" title="意见反馈">${ICONS.mail(18)}</button>
        <span id="navAuth"></span>
      </div>
    </nav>`;

  // ---- 登录态 ----
  const authEl = document.getElementById('navAuth');
  (async function () {
    let loggedIn = false, email = '';
    try {
      const d = await (await fetch('/api/me')).json();
      loggedIn = d.loggedIn;
      if (loggedIn) email = d.user.nickname || d.user.email;
    } catch (e) {}
    authEl.innerHTML = loggedIn
      ? `<a href="/me.html" class="nav-auth" style="text-decoration:none;color:var(--ink);font-weight:600">${ICONS.user(16)} ${escapeHtml(email)}</a>`
      : `<a href="/login.html" class="nav-auth nav-auth-login">登录</a> <a href="/login.html#register" class="nav-auth nav-auth-register">注册</a>`;
    initBell(loggedIn);
  })();

  // ---- 铃铛信息面板（公告 + 通知 + 私信）----
  function initBell(loggedIn) {
    const bell = document.getElementById('navBell');
    const countEl = document.getElementById('bellCount');
    let open = false;

    // 拉取未读计数
    async function refreshCount() {
      if (!loggedIn) { countEl.style.display = 'none'; return; }
      try {
        const [n, m] = await Promise.all([
          (await fetch('/api/notifications')).json(),
          (await fetch('/api/messages')).json()
        ]);
        const unread = (n.unread || 0) + (m.unread || 0);
        if (unread > 0) { countEl.style.display = 'inline-flex'; countEl.textContent = unread > 99 ? '99+' : unread; }
        else countEl.style.display = 'none';
      } catch (e) {}
    }
    refreshCount();

    bell.onclick = async (e) => {
      e.stopPropagation();
      if (!open) { await openPanel(); open = true; } else { closePanel(); open = false; }
      countEl.style.display = 'none';
    };
    document.addEventListener('click', (e) => { if (open && !e.target.closest('#bellPanel')) { closePanel(); open = false; } });

    function buildPanel(ann, notifs, msgs) {
      // 公告内容
      const annHtml = ann && ann.content
        ? `<div class="nb-ann"><span class="ann-tag">${ICONS.megaphone(14)}</span>${escapeHtml(ann.content)}</div>`
        : '<div class="nb-empty">暂无公告</div>';
      // 通知
      const notifHtml = (notifs || []).slice(0, 20).map(n => `
        <div class="nb-item" data-go="${n.refId && n.type==='message' ? '' : (n.refId ? '/post.html?id='+n.refId : '')}">
          <span class="nb-text">${escapeHtml(n.actor||'')} ${escapeHtml(n.text)}</span>
          <span class="nb-time">${escapeHtml(n.createdAt)}</span>
        </div>`).join('') || '<div class="nb-empty">暂无通知</div>';
      const msgHtml = (msgs || []).slice(0, 20).map(m => `
        <div class="nb-item">
          <span class="nb-text"><b>${escapeHtml(m.fromName)}</b>：${escapeHtml(m.content)}</span>
          <span class="nb-time">${escapeHtml(m.createdAt)}</span>
        </div>`).join('') || '<div class="nb-empty">暂无私信</div>';

      const panel = document.createElement('div');
      panel.id = 'bellPanel';
      panel.className = 'bell-panel';
      panel.innerHTML = `
        <div class="nb-tabs">
          <button class="nb-tab active" data-tab="ann">公告</button>
          <button class="nb-tab" data-tab="notif">通知${(notifs||[]).length?' ('+(notifs||[]).length+')':''}</button>
          <button class="nb-tab" data-tab="msg">私信${(msgs||[]).length?' ('+(msgs||[]).length+')':''}</button>
          <button class="nb-clear nb-tab" id="nbReadAll" title="全部已读">已读</button>
        </div>
        <div class="nb-body">
          <div class="nb-sec" data-sec="ann">${annHtml}</div>
          <div class="nb-sec" data-sec="notif" style="display:none">${notifHtml}</div>
          <div class="nb-sec" data-sec="msg" style="display:none">${msgHtml}</div>
        </div>`;
      document.body.appendChild(panel);
      const bellRect = document.getElementById('navBell').getBoundingClientRect();
      const isLeftNav = document.body.getAttribute('data-nav') === 'left';
      if (isLeftNav) {
        // 左导航：面板贴导航右侧，从铃铛顶部对齐，避免盖住上方内容
        panel.style.right = 'auto';
        panel.style.top = Math.max(72, bellRect.top) + 'px';
        panel.style.left = Math.max(240, bellRect.right + 8) + 'px';
      } else {
        panel.style.top = '64px';
        panel.style.right = 'auto';
        panel.style.left = Math.max(8, bellRect.left - panel.offsetWidth + bellRect.width) + 'px';
      }

      // Tab 切换
      panel.querySelectorAll('.nb-tab[data-tab]').forEach(t => t.onclick = () => {
        panel.querySelectorAll('.nb-tab[data-tab]').forEach(x => x.classList.remove('active'));
        t.classList.add('active');
        const tab = t.dataset.tab;
        panel.querySelectorAll('.nb-sec').forEach(s => s.style.display = s.dataset.sec === tab ? '' : 'none');
      });
      // 已读全部
      panel.querySelector('#nbReadAll').onclick = async () => {
        await fetch('/api/notifications/read', { method: 'POST' });
        await fetch('/api/messages/read', { method: 'POST' });
        refreshCount(); buildPanel(ann, [], []);
      };
    }

    async function openPanel() {
      if (!loggedIn) { location.href = '/login.html'; return; }
      let ann, notifs, msgs;
      try {
        ann = (await (await fetch('/api/announcement')).json()).announcement;
        notifs = (await (await fetch('/api/notifications')).json()).notifications || [];
        msgs = (await (await fetch('/api/messages')).json()).messages || [];
      } catch (e) {}
      buildPanel(ann, notifs, msgs);
    }
    function closePanel() { const p = document.getElementById('bellPanel'); if (p) p.remove(); }
  }

  // ========== 全站自定义设置：主题/导航/主题色/拉黑 (V14-5.3b) ==========
  // 加载主题样式表
  (function loadThemesCss() {
    if (document.getElementById('themesCss')) return;
    const l = document.createElement('link');
    l.id = 'themesCss'; l.rel = 'stylesheet'; l.href = '/themes.css';
    document.head.appendChild(l);
  })();

  // 从 localStorage 应用已保存设置
  function applySettings() {
    const t = localStorage.getItem('qwTheme') || '';
    const nav = localStorage.getItem('qwNav') || '';
    const color = localStorage.getItem('qwColor') || '';
    if (t) document.body.setAttribute('data-theme', t); else document.body.removeAttribute('data-theme');
    if (nav === 'left') document.body.setAttribute('data-nav', 'left'); else document.body.removeAttribute('data-nav');
    if (color) {
      const valid = /^#[0-9a-fA-F]{6}$/.test(color);
      if (!valid) return;
      document.body.style.setProperty('--magenta', color);
      // 派生深色
      document.body.style.setProperty('--magenta-deep', color);
      document.body.style.setProperty('--magenta-soft', hexSoft(color));
    }
  }
  function hexSoft(hex) {
    const n = parseInt(hex.slice(1), 16);
    const r = (n>>16)&255, g=(n>>8)&255, b=n&255;
    const sr = Math.round(r*0.25+230*0.75), sg=Math.round(g*0.1+245*0.9), sb=Math.round(b*0.2+235*0.8);
    return `#${((1<<24)+(sr<<16)+(sg<<8)+sb).toString(16).slice(1)}`;
  }
  applySettings();

  // 拉黑名单（localStorage 数组，存被拉黑用户 id）
  function blacklist() { try { return JSON.parse(localStorage.getItem('qwBlack')||'[]'); } catch(e){ return []; } }
  function isBlocked(id) { return blacklist().includes(String(id)); }

  // 创建设置浮标
  const SAVE_THEMES = [
    { key:'', label:'默认' },
    { key:'dark', label:'暗黑' },
    { key:'cyber', label:'赛博朋克' },
    { key:'github', label:'简约' }
  ];
  const COLORS = ['#ff2e88','#2ec4b6','#6366f1','#f59e0b','#10b981','#ef4444'];

  function initSettings() {
    // 设置只在"我的风采"(me.html) 页面提供入口
    if (location.pathname !== '/me.html') return;
    if (document.getElementById('settingsFab')) return;
    const fab = document.createElement('button');
    fab.id = 'settingsFab'; fab.className = 'settings-fab'; fab.title = '自定义站点'; fab.innerHTML = ICONS.settings(26);
    document.body.appendChild(fab);
    fab.onclick = (e) => { e.stopPropagation(); togglePanel(); };

    document.addEventListener('click', (e) => {
      const pn = document.getElementById('settingsPanel');
      if (pn && !e.target.closest('#settingsFab') && !e.target.closest('#settingsPanel')) { pn.remove(); }
    });
  }

  function togglePanel() {
    const old = document.getElementById('settingsPanel');
    if (old) { old.remove(); return; }
    const curTheme = document.body.getAttribute('data-theme') || '';
    const curNav = document.body.getAttribute('data-nav') || '';
    const curColor = localStorage.getItem('qwColor') || '';
    const panel = document.createElement('div');
    panel.id = 'settingsPanel'; panel.className = 'settings-panel';
    panel.innerHTML = `
      <button class="set-close" id="setClose">${ICONS.close(16)}</button>
      <h4>站点设置</h4>
      <div class="set-group">
        <div class="set-label">主题模式</div>
        <div class="set-row">
          ${SAVE_THEMES.map(t => `<button class="set-theme ${curTheme===t.key?'active':''}" data-key="${t.key}">${t.label}</button>`).join('')}
        </div>
      </div>
      <div class="set-group">
        <div class="set-label">主题色</div>
        <div class="set-row">
          ${COLORS.map(c => `<button class="set-color ${curColor===c?'active':''}" data-color="${c}" style="background:${c}"></button>`).join('')}
        </div>
      </div>
      <div class="set-group">
        <div class="set-label">导航位置</div>
        <div class="set-switch">
          <button data-navsel="top" class="${curNav!=='left'?'active':''}">顶栏</button>
          <button data-navsel="left" class="${curNav==='left'?'active':''}">左侧</button>
        </div>
      </div>`;
    document.body.appendChild(panel);
    panel.style.top = 'auto';

    // 主题
    panel.querySelectorAll('.set-theme[data-key]').forEach(b => b.onclick = () => {
      const k = b.dataset.key;
      if (k) localStorage.setItem('qwTheme', k); else localStorage.removeItem('qwTheme');
      applySettings(); togglePanel();
    });
    // 主题色
    panel.querySelectorAll('.set-color').forEach(b => b.onclick = () => {
      const c = b.dataset.color;
      if (curColor === c) { localStorage.removeItem('qwColor'); }
      else { localStorage.setItem('qwColor', c); }
      applySettings(); togglePanel();
    });
    // 导航
    panel.querySelectorAll('[data-navsel]').forEach(b => b.onclick = () => {
      const n = b.dataset.navsel;
      if (n === 'left') localStorage.setItem('qwNav','left'); else localStorage.removeItem('qwNav');
      applySettings(); togglePanel();
    });
    panel.querySelector('#setClose').onclick = () => panel.remove();
  }
  window.isBlocked = isBlocked;
  // 过滤掉被拉黑用户的内容；idKey 为列表项里的作者id字段名
  window.filterBlocked = function (list, idKey) {
    const bl = blacklist();
    if (!bl.length || !list) return list;
    return list.filter(it => !bl.includes(String(it[idKey])));
  };

  initSettings();

  function escapeHtml(s) {
    return (s==null?'':String(s)).replace(/[&<>"']/g, m => ({ '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]));
  }
})();


  // ---- 意见箱（G0）：点"反馈"弹模态框提交意见 ----
  function initFeedback() {
    const btn = document.getElementById('navFeedbackBtn');
    if (!btn) return;
    btn.onclick = async () => {
      if (document.getElementById('feedbackModal')) { document.getElementById('feedbackModal').remove(); return; }
      const me = await (await fetch('/api/me')).json();
      if (!me.loggedIn) { location.href = '/login.html'; return; }
      const m = document.createElement('div');
      m.id = 'feedbackModal';
      m.className = 'feedback-modal';
      m.innerHTML = `
        <div class="feedback-box">
          <div class="feedback-head"><h4>意见 / 建议</h4><button id="fbClose" style="border:none;background:none;font-size:1.1rem;cursor:pointer;color:var(--ink);opacity:.8">${ICONS.close(16)}</button></div>
          <p style="color:var(--muted);font-size:.85rem;margin:0 0 10px">告诉我们你希望这个网站做成什么样，或遇到了什么问题。</p>
          <textarea id="fbContent" placeholder="写下你的意见…" style="width:100%;min-height:90px;box-sizing:border-box;padding:8px;border:1px solid #ddd;border-radius:8px;font:inherit"></textarea>
          <div id="fbMsg" style="font-size:.85rem;color:var(--success,#2e7d32);min-height:1em"></div>
          <button id="fbSubmit" class="btn btn-primary" style="width:100%;padding:9px;border:none;border-radius:8px;background:var(--magenta);color:#fff;cursor:pointer;font-weight:600">提交</button>
        </div>`;
      document.body.appendChild(m);
      document.getElementById('fbClose').onclick = () => m.remove();
      document.getElementById('fbSubmit').onclick = async () => {
        const content = document.getElementById('fbContent').value.trim();
        if (!content) { document.getElementById('fbMsg').textContent = '请输入内容'; document.getElementById('fbMsg').style.color='#c0392b'; return; }
        document.getElementById('fbMsg').textContent = '提交中…';
        document.getElementById('fbMsg').style.color = 'var(--muted)';
        try {
          const r = await fetch('/api/opinion', { method:'POST', headers:{'Content-Type':'application/json'},
            body: JSON.stringify({ content, anonymous: false }) });
          const d = await r.json();
          if (r.ok && d.ok) {
            // 提交成功：显示成功提示后关闭弹窗
            document.getElementById('fbMsg').innerHTML = ICONS.check(14)+' 已提交，感谢反馈！';
            document.getElementById('fbMsg').style.color='#2e7d32';
            document.getElementById('fbContent').value='';
            setTimeout(() => { const fm = document.getElementById('feedbackModal'); if (fm) fm.remove(); }, 900);
          }
          else { document.getElementById('fbMsg').innerHTML = (ICONS.x(14)+' '+(d.error||'提交失败')); document.getElementById('fbMsg').style.color='#c0392b'; }
        } catch(e) { document.getElementById('fbMsg').innerHTML = ICONS.x(14)+' 提交失败'; document.getElementById('fbMsg').style.color='#c0392b'; }
      };
    };
  }
  initFeedback();

  // 全站统一页脚：除首页/登录页（各自已有 footer）外的其他页面，在底部注入同一行页脚
  (function injectFooter() {
    // 已有专属页脚则跳过（首页 .footer、登录页 .auth-foot）
    if (document.querySelector('.footer') || document.querySelector('.auth-foot')) return;
    // 排除无内容包装的轻页面（预览等）不要也可以——这里统一加，保持"每一页都有"
    var f = document.createElement('footer');
    f.className = 'site-footer';
    f.style.cssText = 'text-align:center;padding:1.4rem 1rem;color:var(--muted);font-size:.82rem;border-top:1px solid var(--border);';
    f.innerHTML = 'Wasteland © 2026  青岛大学学习互助社区（内测） · ' +
      '<a href="/terms.html" style="color:var(--muted)">用户协议</a> · ' +
      '<a href="/privacy.html" style="color:var(--muted)">隐私政策</a> · ' +
      '<a href="https://github.com/youyan2000/QDU_Wasteland" target="_blank" rel="noopener" style="color:var(--muted)">开源仓库</a> · ' +
      '<a href="https://wpa.qq.com/msgrd?v=3&uin=3978279928&site=qq&menu=yes" target="_blank" rel="noopener" style="color:var(--muted)">联系方式</a>' +
      '（QQ 3978279928）';
    document.body.appendChild(f);
  })();