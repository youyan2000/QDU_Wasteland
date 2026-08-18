// article-editor.js — 写文章/编辑文章编辑器
// 支持 ?edit=id 编辑模式：加载草稿内容，保存走 update
const edCat = document.getElementById('edCat');
const ta = document.getElementById('edContent');
const prev = document.getElementById('edPreview');
const cnt = document.getElementById('edCount');
const editId = parseInt(new URLSearchParams(location.search).get('edit') || '0', 10);

let initialContent = '';
let currentCover = ''; // J7

ta.addEventListener('input', () => {
  prev.innerHTML = renderMarkdown(ta.value) || '<p class="ph-tip">开始输入即可看到预览…</p>';
  cnt.textContent = ta.value.length;
});

// 工具栏插入
document.getElementById('mdToolbar').addEventListener('click', e => {
  const btn = e.target.closest('button[data-ins]');
  if (!btn) return;
  const s = ta.selectionStart, t = ta.selectionEnd;
  const ins = btn.dataset.ins.replace(/&#10;/g, '\n');
  if (s !== t && ins.startsWith('> ')) {
    // 引用：给选中的每一行加 > 前缀，而不是把选中的内容夹在中间
    const sel = ta.value.slice(s, t);
    const prefixed = sel.split('\n').map(l => '> ' + l).join('\n');
    ta.value = ta.value.slice(0, s) + prefixed + ta.value.slice(t);
    ta.selectionStart = s; ta.selectionEnd = s + prefixed.length;
  } else if (s === t) {
    ta.value = ta.value.slice(0, s) + ins + ta.value.slice(s);
    ta.selectionStart = ta.selectionEnd = s + ins.length;
  } else {
    ta.value = ta.value.slice(0, s) + ins + ta.value.slice(s, t) + ins + ta.value.slice(t);
    ta.selectionStart = s + ins.length;
  }
  ta.focus();
  ta.dispatchEvent(new Event('input'));
});


// J7 封面上传/移除
(async () => {
  const fileEl = document.getElementById('edCoverFile');
  const prev = document.getElementById('edCoverPreview');
  const msg = document.getElementById('edCoverMsg');
  if (!fileEl) return;
  fileEl.onchange = async () => {
    const f = fileEl.files && fileEl.files[0];
    if (!f) return;
    if (f.size > 5*1024*1024) { msg.textContent = '图片不能超过 5MB'; return; }
    const fd = new FormData();
    fd.append('file', f);
    msg.textContent = '上传中…';
    try {
      const r = await fetch('/api/article-cover', { method:'POST', body: fd });
      const d = await r.json();
      if (!r.ok) throw new Error(d.error || '上传失败');
      currentCover = d.cover;
      prev.src = d.cover;
      prev.style.display = 'block';
      msg.textContent = '已上传';
    } catch (e) { msg.textContent = e.message; }
  };
  const rm = document.getElementById('edCoverRemove');
  if (rm) rm.onclick = () => {
    currentCover = '';
    prev.style.display = 'none';
    prev.src = '';
    msg.textContent = '已移除封面';
  };
})();

// 共用保存：mode = 'publish' 发布 | 'draft' 存草稿
async function saveArticle(mode) {
  const errEl = document.getElementById('edErr');
  errEl.textContent = '';
  const title = document.getElementById('edTitle').value.trim();
  const content = ta.value.trim();
  if (mode !== 'draft') {
    if (!title) { errEl.textContent = '请先填写标题'; window.scrollTo({top:0,behavior:'smooth'}); return; }
    if (!content) { errEl.textContent = '正文不能为空'; window.scrollTo({top:0,behavior:'smooth'}); return; }
  }
  const me = await (await fetch('/api/me')).json();
  if (!me.loggedIn) { location.href = '/login.html'; return; }

  // 构造请求体（cover 为空时传空串，避免 undefined 导致序列化差异）
  const payload = { title, category: edCat.value, content, anonymous: document.getElementById('edAnon') ? document.getElementById('edAnon').checked : false, status: mode === 'publish' ? (editId ? '正常' : '') : 'draft', cover: currentCover || '' };
  if (editId) payload.id = editId;

  let res;
  try {
    if (editId) {
      res = await fetch('/api/articles/update', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
    } else {
      res = await fetch('/api/articles/create', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
    }
  } catch (e) {
    errEl.textContent = '网络异常，请检查连接后重试';
    return;
  }
  const d = await res.json().catch(() => ({}));
  if (!res.ok) { errEl.textContent = d.error || '保存失败（' + res.status + '）'; window.scrollTo({top:0,behavior:'smooth'}); return; }
  location.href = '/me.html';
}
document.getElementById('edPublish').addEventListener('click', () => saveArticle('publish'));
document.getElementById('edDraft').addEventListener('click', () => saveArticle('draft'));

// 若为编辑模式：加载草稿内容
async function initEditMode() {
  if (!editId) return;
  try {
    const r = await fetch('/api/articles/' + editId);
    if (r.status !== 200) { alert('无法加载该草稿'); location.href='/me.html'; return; }
    const a = (await r.json()).article;
    if (!a) { alert('草稿不存在'); location.href='/me.html'; return; }
    // 填充
    document.getElementById('edTitle').value = a.title || '';
    if (a.cover) { currentCover = a.cover; const pv = document.getElementById('edCoverPreview'); if (pv) { pv.src = a.cover; pv.style.display = 'block'; } }
    edCat.value = a.category || '生活';
    ta.value = a.content || '';
    initialContent = ta.value;
    ta.dispatchEvent(new Event('input'));
    // 切换按钮 / 加模式选择
    document.getElementById('edPublish').textContent = '保存草稿';
    const pubRow = document.getElementById('edPublishRow');
    if (pubRow) pubRow.innerHTML = `
      <select id="edPublishMode" style="padding:.45rem .7rem;border:1px solid rgba(26,26,46,.15);border-radius:8px;font-size:.85rem">
        <option value="draft">保存为草稿</option>
        <option value="publish">保存并发布</option>
      </select>`;
  } catch (e) { alert('加载失败'); location.href = '/me.html'; }
}

// 未登录跳转
(async () => {
  const me = await (await fetch('/api/me')).json();
  if (!me.loggedIn) { location.href = '/login.html'; return; }
  initEditMode();
})();

// ---- 相册选图（正文插入 & 封面共用）----
async function loadMyAlbumPhotos() {
  const photos = [];
  const albs = await (await fetch('/api/albums?mine=1')).json();
  for (const a of (albs.albums || [])) {
    const items = await (await fetch('/api/albums/items?album_id=' + a.id)).json();
    (items.items || []).filter(i => i.targetType === 'photo').forEach(p => photos.push({ url: p.photoUrl, album: a.title }));
  }
  return photos;
}

function openAlbumPicker(photos, onPick) {
  if (!photos.length) { alert('相册里还没有照片，请先上传或保存图片到相册'); return; }
  const overlay = document.createElement('div');
  overlay.className = 'modal';
  overlay.style.display = 'flex';
  overlay.innerHTML = '<div class="modal-box" style="width:min(640px,94vw);max-height:86vh;overflow:auto">' +
    '<div class="modal-head"><h3>从相册选择图片</h3><button class="modal-close" id="apClose"><i class="icon-fill" data-icon="close"></i></button></div>' +
    '<div style="display:grid;grid-template-columns:repeat(3,1fr);gap:10px;padding:10px 0">' +
    photos.map((p, i) => '<div style="cursor:pointer;border:1px solid var(--border,#eee);border-radius:8px;overflow:hidden" data-albumpick="' + i + '" title="' + p.album + '"><img src="' + p.url + '" style="width:100%;height:110px;object-fit:cover;display:block"></div>').join('') +
    '</div></div>';
  document.body.appendChild(overlay);
  overlay.querySelector('#apClose').onclick = () => overlay.remove();
  overlay.addEventListener('click', function (e) { if (e.target === overlay) overlay.remove(); });
  overlay.querySelectorAll('[data-albumpick]').forEach(el => {
    el.onclick = () => {
      const url = photos[Number(el.dataset.albumpick)].url;
      overlay.remove();
      onPick(url);
    };
  });
}

// 从相册选图插入正文
(function () {
  const btn = document.getElementById('edAlbumPick');
  if (!btn) return;
  btn.onclick = async () => {
    const me = await (await fetch('/api/me')).json();
    if (!me.loggedIn) { location.href = '/login.html'; return; }
    try {
      const photos = await loadMyAlbumPhotos();
      openAlbumPicker(photos, (url) => {
        const ta = document.getElementById('edContent');
        ta.value = ta.value + (ta.value && !ta.value.endsWith('\n') ? '\n' : '') + '![图片](' + url + ')\n';
        ta.dispatchEvent(new Event('input'));
      });
    } catch (e) { alert('加载相册失败，请重试'); }
  };
})();

// 封面：从相册选图
(function () {
  const btn = document.getElementById('edCoverAlbumPick');
  if (!btn) return;
  btn.onclick = async () => {
    const me = await (await fetch('/api/me')).json();
    if (!me.loggedIn) { location.href = '/login.html'; return; }
    try {
      const photos = await loadMyAlbumPhotos();
      openAlbumPicker(photos, (url) => {
        currentCover = url;
        const pv = document.getElementById('edCoverPreview');
        const msg = document.getElementById('edCoverMsg');
        if (pv) { pv.src = url; pv.style.display = 'block'; }
        if (msg) msg.textContent = '已从相册选择封面';
      });
    } catch (e) { alert('加载相册失败，请重试'); }
  };
})();