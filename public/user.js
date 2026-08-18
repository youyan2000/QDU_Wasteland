// user.js — 公开个人主页
const wrap = document.getElementById('userWrap');
const id = new URLSearchParams(location.search).get('id');

async function load() {
  if (!id) { wrap.innerHTML = '<div class="empty">缺少用户</div>'; return; }
  try {
    const r = await fetch('/api/user/' + encodeURIComponent(id));
    if (r.status === 404) { wrap.innerHTML = '<div class="empty">用户不存在</div>'; return; }
    const d = await r.json();
    render(d);
  } catch (e) { wrap.innerHTML = '<div class="empty">加载失败</div>'; }
}

function render(d) {
  const u = d.user;
  document.title = u.nickname + ' · QDU Wasteland';
  const avatar = (u.nickname && u.nickname[0]) || '?';
  const posts = (d.posts || []).map(p => `
    <a class="u-item" href="/post.html?id=${p.id}">
      <span class="u-badge badge-post">帖</span>
      <span class="u-title">${escapeHtml(p.title)}</span>
      <span class="u-sub">${escapeHtml(p.forum)} · ${escapeHtml(p.createdAt)}</span>
    </a>`).join('');
  const articles = (d.articles || []).map(a => `
    <a class="u-item" href="/article.html?id=${a.id}">
      <span class="u-badge badge-article">文</span>
      <span class="u-title">${escapeHtml(a.title)}</span>
      <span class="u-sub">${escapeHtml(a.category)} · ${escapeHtml(a.createdAt)}</span>
    </a>`).join('');
  wrap.innerHTML = `
    <div class="u-card">
      <div class="u-head">
        <div class="avatar">${escapeHtml(avatar)}</div>
        <div style="flex:1">
          <div class="u-name">${escapeHtml(u.nickname)}</div>
          <div class="u-major">${escapeHtml(u.college || '—')} · ${escapeHtml(u.major || '—')}</div>
        </div>
      </div>
      <div class="u-stats">
        <div><span class="u-num">${u.postCount||0}</span><span class="u-lbl">帖子</span></div>
        <div><span class="u-num">${u.articleCount||0}</span><span class="u-lbl">文章</span></div>
      </div>
      <div id="msgZone" class="u-msg" style="margin-top:1.2rem"></div>
      <button id="blockUserBtn" class="u-block-btn" style="display:none"></button>
    </div>
    <h2 class="u-h2">TA 的帖子</h2>
    <div class="u-list">${posts || '<div class="empty">还没有公开发帖</div>'}</div>
    <h2 class="u-h2">TA 的文章</h2>
    <div class="u-list">${articles || '<div class="empty">还没有发布文章</div>'}</div>`;
    checkMsgEligible();
    initBlockButton();
}

// D2 私信资格 + 发送
// 拉黑按钮
function initBlockButton() {
  const btn = document.getElementById('blockUserBtn');
  if (!btn) return;
  const me = null;
  fetch('/api/me').then(r=>r.json()).then(d=>{
    if (!d.loggedIn || d.user.id === Number(id) || d.user.isAdmin === 1) return; // 自己/管理员不拉黑
    btn.style.display = 'inline-block';
    // 后端拉黑状态查询
    fetch('/api/block/list').then(r=>r.json()).then(bl => {
      const blocked = (bl.blocked||[]).some(b => b.id === Number(id));
      btn.className = 'u-block-btn' + (blocked ? ' blocked' : '');
      btn.innerHTML = (blocked ? ICONS.check(14)+' 已拉黑' : ICONS.ban(14)+' 拉黑此人');
      btn.onclick = async () => {
        if (!blocked && !confirm('确定拉黑该用户？拉黑后其帖子、文章、评论将在全站对你隐藏。')) return;
        await fetch('/api/block', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ id: Number(id) }) });
      };
    }).catch(() => { btn.style.display = 'none'; });
  });
}
function isBlockedLS(id) {
  try { return JSON.parse(localStorage.getItem('qwBlack')||'[]').includes(String(id)); } catch(e){ return false; }
}

async function checkMsgEligible() {
  const zone = document.getElementById('msgZone');
  if (!zone) return;
  const me = await (await fetch('/api/me')).json();
  if (!me.loggedIn) { zone.innerHTML = ''; return; }
  if (me.user.id === Number(id)) { zone.innerHTML = ''; return; } // 自己
  const r = await fetch('/api/message/eligible?to=' + id);
  if (r.status === 200) {
    const d = await r.json();
    if (!d.eligible) { zone.innerHTML = '<p class="u-msg-hint">你们还没有互动过，暂不能私信</p>'; return; }
    zone.innerHTML = `
      <textarea id="dmInput" rows="2" placeholder="发私信给 TA…" style="width:100%;box-sizing:border-box;padding:.6rem .8rem;border:1px solid rgba(26,26,46,.18);border-radius:9px;font-family:inherit;font-size:.9rem"></textarea>
      <div style="display:flex;align-items:center;gap:10px;margin-top:8px">
        <button class="u-send-btn" id="dmSend">${ICONS.mail(15)} 发送私信</button>
        <span id="dmMsg" style="font-size:.84rem;color:var(--muted)"></span>
      </div>`;
    document.getElementById('dmSend').onclick = async () => {
      const content = document.getElementById('dmInput').value.trim();
      const msg = document.getElementById('dmMsg');
      if (!content) { msg.textContent = '请输入内容'; return; }
      const rr = await fetch('/api/messages/send', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ to: Number(id), content }) });
      const dd = await rr.json();
      if (!rr.ok) { msg.textContent = dd.error || '发送失败'; return; }
      msg.innerHTML = ICONS.check(14)+' 已发送';
      document.getElementById('dmInput').value = '';
    };
  }
}

function escapeHtml(s) {
  return (s == null ? '' : String(s)).replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m]));
}

load();
