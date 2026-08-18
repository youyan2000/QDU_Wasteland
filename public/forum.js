// forum.js — 论坛首页（V13 加分页）
let currentForum = ''; // '' = all
let currentSort = 'latest';
let currentPage = 1;
let totalPages = 1;
const pageSize = 15;
let forums = [];

// 加载四广场
async function loadForums() {
  const r = await fetch('/api/forums');
  const d = await r.json();
  forums = d.forums;
  const bar = document.getElementById('forumsBar');
  bar.innerHTML = '';
  forums.forEach(f => {
    const d = document.createElement('button');
    d.className = 'forum-chip' + (f.slug === currentForum ? ' active' : '');
    d.innerHTML = `<span class="fc-name">${f.name}</span><span class="fc-desc">${f.desc}</span>`;
    d.onclick = () => { currentForum = f.slug === currentForum ? '' : f.slug; currentPage = 1; syncForums(); loadPosts(); };
    bar.appendChild(d);
  });
  // 填发帖弹层的广场下拉
  const sel = document.getElementById('pfForum');
  sel.innerHTML = '';
  forums.forEach(f => { const o=document.createElement('option'); o.value=f.slug; o.textContent=f.name; sel.appendChild(o); });
  syncForums();
}
// 同步'全部'与广场的高亮
function syncForums() {
  document.getElementById('allChip').classList.toggle('active', currentForum === '');
  document.querySelectorAll('.forum-chip').forEach(ch => {
    const idx = Array.from(document.querySelectorAll('.forum-chip')).indexOf(ch);
    ch.classList.toggle('active', forums[idx] && forums[idx].slug === currentForum);
  });
}

// 加载帖子（分页）
async function loadPosts() {
  const list = document.getElementById('postList');
  list.innerHTML = '<div class="skeleton-list"><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div></div>';
  let url = '/api/forum/posts?sort=' + currentSort + '&page=' + currentPage + '&size=' + pageSize;
  if (currentForum) url += '&f=' + currentForum;
  let d;
  try {
    const r = await fetch(url + '&_=' + Date.now());
    d = await r.json();
  } catch (e) {
    list.innerHTML = '<div class="empty"><span class="empty-title">加载失败</span><span class="empty-hint">网络异常，请刷新重试</span></div>';
    return;
  }
  totalPages = Math.max(1, Math.ceil((d.total || 0) / (d.pageSize || pageSize)));
  list.innerHTML = '';
  let posts = d.posts || [];
  if (window.filterBlocked) posts = window.filterBlocked(posts, 'authorId');
  if (!posts.length) {
    list.innerHTML = '<div class="empty"><span class="empty-title">这里还很安静</span><span class="empty-hint">来发第一帖吧</span></div>';
    renderPager(0, pageSize);
    return;
  }
  posts.forEach(p => {
    const div = document.createElement('div');
    div.className = 'post-item';
    div.innerHTML = `
      <div class="pi-title">${escape(p.title)}</div>
      <div class="pi-meta">
        <span>${p.authorId ? `<a class="author-link" href="/user.html?id=${p.authorId}">${escape(p.author)}</a>` : escape(p.author)}</span>
        <span>${escape(p.createdAt)}</span>
        <span class="pi-stat">${ICONS.thumb(13)} ${p.likes} · ${ICONS.chat(13)} ${p.commentCount}</span>
      </div>`;
    div.querySelectorAll('.author-link').forEach(a => a.addEventListener('click', e => e.stopPropagation()));
    div.onclick = () => location.href = '/post.html?id=' + p.id;
    list.appendChild(div);
  });
  renderPager(d.total || 0, d.pageSize || pageSize);
}

function renderPager(total, psiz) {
  let pager = document.getElementById('pager');
  if (!pager) {
    pager = document.createElement('div');
    pager.id = 'pager';
    pager.className = 'pager';
    document.getElementById('postList').insertAdjacentElement('afterend', pager);
  }
  if (!total) { pager.innerHTML = ''; return; }
  let h = `<button class="pg-btn" data-pg="${currentPage-1}" ${currentPage<=1?'disabled':''}>上一页</button>
    <span class="pg-info">第 ${currentPage} / ${totalPages} 页 · 共 ${total} 条</span>
    <button class="pg-btn" data-pg="${currentPage+1}" ${currentPage>=totalPages?'disabled':''}>下一页</button>`;
  pager.innerHTML = h;
  pager.querySelectorAll('.pg-btn').forEach(b => {
    b.onclick = () => {
      const pg = +b.dataset.pg;
      if (pg >= 1 && pg <= totalPages) { currentPage = pg; loadPosts(); window.scrollTo({top:0,behavior:'smooth'}); }
    };
  });
}

// "全部"点击
document.getElementById('allChip').addEventListener('click', () => {
  if (currentForum === '') return;
  currentForum = ''; currentPage = 1; syncForums(); loadPosts();
});

// 排序
document.querySelectorAll('.sort-tab').forEach(b => {
  b.onclick = () => {
    currentSort = b.dataset.sort;
    currentPage = 1;
    document.querySelectorAll('.sort-tab').forEach(x => x.classList.toggle('active', x === b));
    loadPosts();
  };
});

// 发帖
const modal = document.getElementById('postModal');
document.getElementById('newPostBtn').onclick = async () => {
  const r = await fetch('/api/me');
  const d = await r.json();
  if (!d.loggedIn) { location.href = '/login.html'; return; }
  if (window.loadPfCaptcha) window.loadPfCaptcha(document.getElementById('pfCapImg'));
  modal.style.display = 'flex';
};
document.getElementById('modalClose').onclick = () => modal.style.display = 'none';
modal.addEventListener('click', e => { if (e.target === modal) modal.style.display='none'; });
document.getElementById('pfSubmit').onclick = async () => {
  const err = document.getElementById('pfErr');
  const title = document.getElementById('pfTitle').value.trim();
  const content = document.getElementById('pfContent').value.trim();
  if (!title) { err.textContent = '请填写标题'; return; }
  const res = await fetch('/api/forum/post/create', {
    method: 'POST', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({
      forum: document.getElementById('pfForum').value, title, content,
      anonymous: document.getElementById('pfAnon').checked,
      captcha: document.getElementById('pfCaptcha').value.trim(),
      captchaId: document.getElementById('pfCaptchaId').value
    })
  });
  const d = await res.json();
  if (!res.ok) { err.textContent = d.error; return; }
  err.textContent = '';
  modal.style.display = 'none';
  document.getElementById('pfTitle').value = '';
  document.getElementById('pfContent').value = '';
  currentPage = 1;
  loadForums(); loadPosts();
};

function escape(s) { return (s==null?'':String(s)).replace(/[&<>"']/g, m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m])); }

loadForums();
loadPosts();
