// post.js — 帖子详情页（内容 + 评论 + 点赞 + 举报）
const wrap = document.getElementById('postWrap');
const id = new URLSearchParams(location.search).get('id');

async function load() {
  if (!id) { wrap.innerHTML = '<div class="empty">缺少帖子</div>'; return; }
  try {
    const r = await fetch('/api/forum/post?id=' + encodeURIComponent(id));
    if (r.status === 404) { wrap.innerHTML = '<div class="empty">帖子不存在</div>'; return; }
    const p = await r.json();
    render(p);
  } catch (e) { wrap.innerHTML = '<div class="empty">加载失败</div>'; }
}

function render(p) {
  let html = `
    <article class="post-detail">
      <h1 class="post-title">${escape(p.title)}</h1>
      <div class="post-meta">
        <span class="tag">${escape(p.forum)}</span>
        <span>${!p.anonymous && p.ownerId ? `<a class="author-link" href="/user.html?id=${p.ownerId}">${escape(p.author)}</a>` : escape(p.author)}</span>
        <span>${escape(p.createdAt)}</span>
      </div>
      <div class="post-content">${escape(p.content)}</div>
      <div class="post-actions" style="display:flex;align-items:center;gap:16px;flex-wrap:wrap">
        <button id="likeBtn" class="like-btn ${p.liked?'liked':''}">${ICONS.thumb(16, '', p.liked)} ${p.liked?'已赞':'点赞'} <span class="like-count">${p.likes||0}</span></button>
        <button id="favBtn" class="like-btn">☆ 收藏</button>
        <button class="rep-btn-mini" data-report-type="post" data-report-id="${p.id}" data-report-label="${escape(p.title)}">举报</button>
        <span style="margin-left:auto;display:flex;gap:8px" id="ownActions"></span>
      </div>
    </article>
    <h2 class="comments-h2">评论 (${(p.comments||[]).length})</h2>
    <div class="comment-form">
      <textarea id="cmContent" rows="2" placeholder="写下你的评论…"></textarea>
      <div style="display:flex;align-items:center;gap:12px;margin-top:8px">
        <label style="display:flex;align-items:center;gap:4px;font-size:.85rem"><input type="checkbox" id="cmAnon"> 匿名</label>
        <button id="cmSubmit" class="btn-submit">评论</button>
        <span id="cmMsg"></span>
      </div>
    </div>
    <div class="comment-list">${renderComments(p.comments||[])}</div>
    </div>`;
  wrap.innerHTML = html;

  // C2 作者本人或管理员的"撤回草稿/删除"按钮（编辑入口简洁版：删除=撤回草稿，草稿可再发布）
  (async () => {
    const me = await (await fetch('/api/me')).json();
    if (!me.loggedIn) return;
    const isOwner = me.user.id === (p.ownerId || 0) && p.ownerId;
    const meRes = await fetch('/api/me');
    const meD = await meRes.json();
    const canMod = meD.loggedIn && (p.ownerId && meD.user.id === p.ownerId || meD.user.isAdmin === 1);
    if (!canMod) return;
    const box = document.getElementById('ownActions');
    if (box) {
      box.innerHTML = `<button class="me-btn mini" id="postDelBtn">删除(撤回草稿)</button>`;
      document.getElementById('postDelBtn').onclick = async () => {
        if (!confirm('确定删除该帖子？将撤回草稿箱，可在"我的风采-草稿箱"重新发布。')) return;
        await fetch('/api/forum/post/delete', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ id: p.id }) });
        location.href = '/forum.html';
      };
    }
  })();

  // 点赞
  document.getElementById('likeBtn').onclick = async () => {
    const r = await fetch('/api/me');
    const me = await r.json();
    if (!me.loggedIn) { location.href = '/login.html'; return; }
    await fetch('/api/forum/like?id=' + p.id);
    load();
  };

  // 收藏 (J6)
  const favBtn = document.getElementById('favBtn');
  if (favBtn) {
    (async () => {
      const me = await (await fetch('/api/me')).json();
      if (!me.loggedIn) return;
      const r = await fetch('/api/favorites?type=post');
      const d = await r.json();
      const faved = (d.favorites||[]).some(f => f.targetId === String(p.id));
      favBtn.textContent = faved ? '★ 已收藏' : '☆ 收藏';
      favBtn.classList.toggle('liked', faved);
    })();
    favBtn.onclick = async () => {
      const me = await (await fetch('/api/me')).json();
      if (!me.loggedIn) { location.href = '/login.html'; return; }
      const res = await fetch('/api/favorite/toggle', {
        method:'POST', headers:{'Content-Type':'application/json'},
        body: JSON.stringify({ targetType:'post', targetId: String(p.id) })
      });
      const d = await res.json();
      favBtn.textContent = d.favorited ? '★ 已收藏' : '☆ 收藏';
      favBtn.classList.toggle('liked', d.favorited);
    };
  }

  // 评论（含楼中楼回复）
  let replyToId = 0;
  document.querySelectorAll('.cm-reply').forEach(btn => {
    btn.onclick = () => {
      replyToId = Number(btn.dataset.replyTo) || 0;
      const ta = document.getElementById('cmContent');
      ta.focus();
      ta.placeholder = replyToId ? `回复 #${replyToId} 的评论…` : '写下你的评论…';
    };
  });
  document.getElementById('cmSubmit').onclick = async () => {
    const msg = document.getElementById('cmMsg');
    const content = document.getElementById('cmContent').value.trim();
    if (!content) { msg.textContent = '评论不能为空'; return; }
    const me = await (await fetch('/api/me')).json();
    if (!me.loggedIn) { location.href='/login.html'; return; }
    const res = await fetch('/api/forum/comment', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({ postId: p.id, content, anonymous: document.getElementById('cmAnon').checked, parentId: replyToId || undefined })
    });
    const d = await res.json();
    if (!res.ok) { msg.textContent = d.error; return; }
    load();
  };
}

function escape(s) { return (s==null?'':String(s)).replace(/[&<>"']/g, m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m])); }

// 楼中楼渲染：按 parentId 递归嵌套，子评论缩进展示
function renderComments(all) {
  if (!all.length) return '<div class="empty">还没有评论</div>';
  const byParent = {};
  all.forEach(c => {
    const k = c.parentId || 0;
    if (!byParent[k]) byParent[k] = [];
    byParent[k].push(c);
  });
  const depthMap = {};
  const walk = (pid, depth) => {
    const kids = byParent[pid] || [];
    if (!kids.length) return '';
    return kids.map(c => {
      depthMap[c.id] = depth;
      const kidsHtml = walk(c.id, depth + 1);
      return `
        <div class="comment-item" style="margin-left:${Math.min(depth, 3) * 18}px;${depth > 0 ? 'border-left:2px solid var(--line,#eee);padding-left:10px;' : ''}">
          <span class="cm-author">${escape(c.author)}</span>
          <span class="cm-time">${escape(c.createdAt)}</span>
          <button class="cm-reply" data-reply-to="${c.id}">回复</button>
          <button class="rep-btn-mini" data-report-type="comment" data-report-id="${c.id}" data-report-label="${escape(String(c.content).slice(0,30))}">举报</button>
          <p class="cm-content">${escape(c.content)}</p>
          ${kidsHtml}
        </div>`;
    }).join('');
  };
  return walk(0, 0) || '<div class="empty">还没有评论</div>';
}

load();
