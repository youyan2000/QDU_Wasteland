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
  if (s === t) {
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

  let res;
  if (editId) {
    res = await fetch('/api/articles/update', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: editId, title, category: edCat.value, content, status: mode === 'publish' ? '正常' : 'draft', cover: currentCover })
    });
  } else {
    res = await fetch('/api/articles/create', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title, category: edCat.value, content, anonymous: document.getElementById('edAnon').checked, status: mode === 'publish' ? '' : 'draft', cover: currentCover })
    });
  }
  const d = await res.json();
  if (!res.ok) { errEl.textContent = d.error || '保存失败'; window.scrollTo({top:0,behavior:'smooth'}); return; }
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