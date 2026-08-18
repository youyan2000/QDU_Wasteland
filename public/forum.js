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
const isNewDraft = new URLSearchParams(location.search).get('newdraft') === '1';
function openPostModal() {
  if (window.loadPfCaptcha) window.loadPfCaptcha(document.getElementById('pfCapImg'));
  // 新建草稿模式：加"存草稿"按钮
  const actions = document.querySelector('.pf-actions');
  if (isNewDraft && actions && !document.getElementById('pfDraftBtn')) {
    const dBtn = document.createElement('button');
    dBtn.id = 'pfDraftBtn';
    dBtn.className = 'btn-submit';
    dBtn.style.cssText = 'background:var(--track,#666);margin-left:8px';
    dBtn.textContent = '存草稿';
    actions.appendChild(dBtn);
    dBtn.onclick = savePostDraft;
  }
  if (window.bindPfImageUpload) bindPfImageUpload();
  modal.style.display = 'flex';
}
document.getElementById('newPostBtn').onclick = async () => {
  const r = await fetch('/api/me');
  const d = await r.json();
  if (!d.loggedIn) { location.href = '/login.html'; return; }
  openPostModal();
};
if (isNewDraft) {
  (async () => {
    const r = await fetch('/api/me');
    const d = await r.json();
    if (!d.loggedIn) { location.href = '/login.html'; return; }
    openPostModal();
  })();
}
async function savePostDraft() {
  const err = document.getElementById('pfErr');
  const title = document.getElementById('pfTitle').value.trim();
  const content = document.getElementById('pfContent').value.trim();
  if (!title && !content) { err.textContent = '请至少填写标题或内容'; return; }
  const res = await fetch('/api/forum/post/create', {
    method: 'POST', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({
      forum: document.getElementById('pfForum').value, title, content,
      anonymous: document.getElementById('pfAnon').checked,
      captcha: '', captchaId: '', status: 'draft'
    })
  });
  const d = await res.json();
  if (!res.ok) { err.textContent = d.error || '保存失败'; return; }
  modal.style.display = 'none';
  location.href = '/me.html#drafts';
}
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


// 发帖图片上传：选择图片→上传→插入 ![](url) 到内容
let pfImgBound = false;
function bindPfImageUpload() {
  const btn = document.getElementById('pfImgBtn');
  const file = document.getElementById('pfImgFile');
  if (!btn || !file || pfImgBound) return;
  pfImgBound = true;
  btn.onclick = () => file.click();
  file.onchange = async () => {
    const msg = document.getElementById('pfImgMsg');
    const ta = document.getElementById('pfContent');
    const files = Array.from(file.files || []);
    if (!files.length) return;
    for (const f of files) {
      if (f.size > 8 * 1024 * 1024) { msg.textContent = f.name + ' 超过 8MB 跳过'; continue; }
      const fd = new FormData();
      fd.append('file', f);
      msg.textContent = '上传 ' + f.name + '…';
      try {
        const r = await fetch('/api/upload-image', { method: 'POST', body: fd });
        const d = await r.json();
        if (!r.ok) { msg.textContent = d.error || '上传失败'; continue; }
        // 插入 Markdown 图片语法
        ta.value = ta.value + (ta.value && !ta.value.endsWith('\n') ? '\n' : '') + '![图片](' + d.url + ')\n';
        msg.textContent = '✅ 已插入 ' + f.name;
      } catch (e) { msg.textContent = '上传失败'; }
    }
    file.value = '';
    setTimeout(() => { msg.textContent = ''; }, 2500);
  };
}

// 论坛发帖：从相册选图插入
(function () {
  const btn = document.getElementById('pfAlbumBtn');
  if (!btn) return;
  btn.onclick = async () => {
    const photos = [];
    try {
      const albs = await (await fetch('/api/albums?mine=1')).json();
      for (const a of (albs.albums || [])) {
        const items = await (await fetch('/api/albums/items?album_id=' + a.id)).json();
        (items.items || []).filter(i => i.targetType === 'photo').forEach(p => photos.push({ url: p.photoUrl, album: a.title }));
      }
    } catch (e) {}
    if (!photos.length) { alert('相册里还没有照片，请先上传或保存图片到相册'); return; }
    const overlay = document.createElement('div');
    overlay.className = 'modal';
    overlay.style.display = 'flex';
    overlay.innerHTML = '<div class="modal-box" style="width:min(640px,94vw);max-height:86vh;overflow:auto">' +
      '<div class="modal-head"><h3>从相册选择图片</h3><button class="modal-close" id="apClose2"><i class="icon-fill" data-icon="close"></i></button></div>' +
      '<div style="display:grid;grid-template-columns:repeat(3,1fr);gap:10px;padding:10px 0">' +
      photos.map((p, i) => '<div style="cursor:pointer;border:1px solid var(--border,#eee);border-radius:8px;overflow:hidden" data-pfpick="' + i + '" title="' + p.album + '"><img src="' + p.url + '" style="width:100%;height:110px;object-fit:cover;display:block"></div>').join('') +
      '</div></div>';
    document.body.appendChild(overlay);
    overlay.querySelector('#apClose2').onclick = () => overlay.remove();
    overlay.addEventListener('click', function (e) { if (e.target === overlay) overlay.remove(); });
    overlay.querySelectorAll('[data-pfpick]').forEach(el => {
      el.onclick = () => {
        const url = photos[Number(el.dataset.pfpick)].url;
        const ta = document.getElementById('pfContent');
        ta.value = ta.value + (ta.value && !ta.value.endsWith('\n') ? '\n' : '') + '![图片](' + url + ')\n';
        overlay.remove();
      };
    });
  };
})();