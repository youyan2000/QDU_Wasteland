// article-view.js — 独立文章阅读页
const wrap = document.getElementById('readWrap');
const id = new URLSearchParams(location.search).get('id');

async function load() {
  if (!id) { wrap.innerHTML = '<div class="empty">缺少文章ID</div>'; return; }
  try {
    const r = await fetch('/api/articles/' + encodeURIComponent(id));
    if (r.status === 404) { wrap.innerHTML = '<div class="empty">文章不存在或已下架</div>'; return; }
    const d = await r.json();
    render(d.article);
  } catch (e) { wrap.innerHTML = '<div class="empty">加载失败</div>'; }
}

function render(a) {
  document.title = a.title + ' · 文章 · QDU Wasteland';
  var coverHtml = a.cover ? '<div class="read-cover"><img src="' + escape(a.cover) + '" alt="封面" loading="lazy"></div>' : '';
  wrap.innerHTML = [
    '<header class="read-head">',
    '  <span class="cat-chip active" style="cursor:default">' + escape(a.category) + '</span>',
    '  <h1 class="read-title">' + escape(a.title) + '</h1>',
    '  <div class="read-meta">',
    '    <span class="read-author">' + ICONS.user(14) + ' ' + (a.userId ? '<a class="author-link" href="/user.html?id=' + a.userId + '">' + escape(a.author) + '</a>' : escape(a.author)) + '</span>',
    '    <span>' + escape(a.createdAt) + '</span>',
    '    <span class="read-views">' + ICONS.eye(14) + ' ' + (a.views || 0) + ' 阅读</span>',
    '    <button class="rep-btn-mini" data-report-type="article" data-report-id="' + a.id + '" data-report-label="' + escape(a.title) + '">举报</button>',
    '    <button id="artShareBtn" style="margin-left:auto;display:inline-flex;align-items:center;gap:4px;padding:.35rem .9rem;border:1px solid var(--border-strong);border-radius:999px;background:var(--surface);color:var(--ink);cursor:pointer;font-size:.8rem">' + ICONS.download(15) + ' 分享</button>',
    '  </div>',
    '</header>',
    coverHtml,
    '<article class="read-body md-view">' + renderMarkdown(a.content || '') + '</article>',
    '<footer class="read-foot"></footer>'
  ].join('');
  // 分享：复制链接
  var sb = document.getElementById('artShareBtn');
  if (sb) sb.onclick = function () {
    var url = location.href;
    var done = function () { sb.innerHTML = ICONS.check(15) + ' 已复制'; setTimeout(function () { sb.innerHTML = ICONS.download(15) + ' 分享'; }, 1500); };
    var fb = function () { var ta = document.createElement('textarea'); ta.value = url; ta.style.position='fixed'; ta.style.opacity='0'; document.body.appendChild(ta); ta.select(); try{document.execCommand('copy');}catch(e){} document.body.removeChild(ta); };
    if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(url).then(done).catch(function(){ fb(); done(); });
    else { fb(); done(); }
  };
  attachSaveToAlbum(wrap);
}

function escape(s) { return (s == null ? '' : String(s)).replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m])); }

load();

function attachSaveToAlbum(container) {
  if (!container) return;
  const imgs = container.querySelectorAll('.read-body img, .md-view img, .post-content img');
  imgs.forEach(img => {
    if (img.closest('.save-album-wrap')) return;
    const wrap = document.createElement('span');
    wrap.className = 'save-album-wrap';
    wrap.style.cssText = 'position:relative;display:inline-block';
    img.parentNode.insertBefore(wrap, img);
    wrap.appendChild(img);
    const btn = document.createElement('button');
    btn.textContent = '存相册';
    btn.style.cssText = 'position:absolute;top:6px;right:6px;background:rgba(0,0,0,.6);color:#fff;border:none;border-radius:6px;padding:3px 10px;font-size:.75rem;cursor:pointer;opacity:0;transition:opacity .15s;z-index:5';
    wrap.appendChild(btn);
    wrap.addEventListener('mouseenter', () => { btn.style.opacity = '1'; });
    wrap.addEventListener('mouseleave', () => { btn.style.opacity = '0'; });
    btn.onclick = async () => {
      const me = await (await fetch('/api/me')).json();
      if (!me.loggedIn) { location.href = '/login.html'; return; }
      btn.textContent = '保存中…';
      try {
        const r = await fetch('/api/albums/save-image', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ url: img.getAttribute('src') })
        });
        const d = await r.json();
        if (!r.ok) { alert(d.error || '保存失败'); btn.textContent = '存相册'; return; }
        btn.textContent = 'OK 已保存';
        setTimeout(() => { btn.textContent = '存相册'; }, 1500);
      } catch (e) { alert('网络错误'); btn.textContent = '存相册'; }
    };
  });
}