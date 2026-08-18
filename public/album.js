// album.js — 相册详情页：展示照片 + 上传 + 删除 + 灯箱
const albumId = parseInt(new URLSearchParams(location.search).get('id') || '0', 10);
const grid = document.getElementById('photoGrid');
const titleEl = document.getElementById('albumTitle');
const metaEl = document.getElementById('albumMeta');
const uploadBtn = document.getElementById('uploadPhotoBtn');
const lightbox = document.getElementById('lightbox');
const lightboxImg = document.getElementById('lightboxImg');

let isOwner = false;

async function load() {
  if (!albumId) { grid.innerHTML = '<div class="empty-album">缺少相册 ID</div>'; return; }
  try {
    const me = await (await fetch('/api/me')).json();
    const d = await (await fetch('/api/albums/items?album_id=' + albumId)).json();
    const items = d.items || [];
    const albs = await (await fetch('/api/albums?mine=1')).json();
    isOwner = (albs.albums || []).some(a => a.id === albumId);
    if (me.loggedIn && isOwner) uploadBtn.style.display = 'inline-block';
    renderItems(items);
  } catch (e) {
    grid.innerHTML = '<div class="empty-album">相册不存在或未公开</div>';
  }
}

function renderItems(items) {
  const photos = items.filter(i => i.targetType === 'photo');
  const others = items.filter(i => i.targetType !== 'photo');
  if (photos.length === 0 && others.length === 0) {
    grid.innerHTML = '<div class="empty-album">这个相册还是空的<br>主人可以点右上角"上传照片"</div>';
    return;
  }
  grid.innerHTML = '';
  photos.forEach(p => {
    const div = document.createElement('div');
    div.className = 'photo-item';
    div.innerHTML = '<img src="' + p.photoUrl + '" alt="" loading="lazy">' +
      (isOwner ? '<button class="photo-del" data-del-photo="' + p.id + '">删除</button>' : '');
    div.querySelector('img').onclick = () => openLightbox(p.photoUrl);
    const del = div.querySelector('[data-del-photo]');
    if (del) del.onclick = (e) => { e.stopPropagation(); delPhoto(p.id); };
    grid.appendChild(div);
  });
  if (others.length) {
    const listBox = document.createElement('div');
    listBox.className = 'act-list';
    listBox.style.marginTop = '16px';
    listBox.innerHTML = others.map(o => {
      let href = '#', label = o.targetType + ' #' + o.targetId;
      if (o.targetType === 'post') { href = '/post.html?id=' + o.targetId; label = '帖子 #' + o.targetId; }
      else if (o.targetType === 'article') { href = '/article.html?id=' + o.targetId; label = '文章 #' + o.targetId; }
      else if (o.targetType === 'file') { href = '/viewer.html?id=' + o.targetId; label = '资料 #' + o.targetId; }
      else if (o.targetType === 'course') { href = '/course.html?code=' + encodeURIComponent(o.targetId); label = '课程'; }
      return '<div class="act-item"><span class="act-badge badge-article">藏</span>' +
        '<a class="act-main" href="' + href + '" target="_blank">' + escape(label) + '</a>' +
        '<span class="act-sub">' + escape(o.targetType) + ' · ' + escape(o.createdAt) + '</span></div>';
    }).join('');
    grid.appendChild(listBox);
  }
}

function openLightbox(src) {
  lightboxImg.src = src;
  lightbox.style.display = 'flex';
}
lightbox.onclick = () => { lightbox.style.display = 'none'; lightboxImg.src = ''; };

uploadBtn.onclick = () => {
  const input = document.createElement('input');
  input.type = 'file';
  input.accept = 'image/jpeg,image/png,image/gif,image/webp';
  input.onchange = async () => {
    const f = input.files && input.files[0];
    if (!f) return;
    if (f.size > 10 * 1024 * 1024) { alert('照片不能超过 10MB'); return; }
    const fd = new FormData();
    fd.append('file', f);
    fd.append('albumId', String(albumId));
    uploadBtn.disabled = true; uploadBtn.textContent = '上传中…';
    try {
      const r = await fetch('/api/albums/upload-photo', { method: 'POST', body: fd });
      const d = await r.json();
      if (!r.ok) { alert(d.error || '上传失败'); return; }
      load();
    } catch (e) { alert('网络错误'); }
    finally { uploadBtn.disabled = false; uploadBtn.textContent = '＋ 上传照片'; }
  };
  input.click();
};

async function delPhoto(itemId) {
  if (!confirm('确定从相册删除这张照片？')) return;
  const r = await fetch('/api/albums/remove-item', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ itemId })
  });
  const d = await r.json();
  if (!r.ok) { alert(d.error || '删除失败'); return; }
  load();
}

function escape(s) { return (s == null ? '' : String(s)).replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m])); }

load();
