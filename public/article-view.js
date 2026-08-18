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
  wrap.innerHTML = `
    <header class="read-head">
      <span class="cat-chip active" style="cursor:default">${escape(a.category)}</span>
      <h1 class="read-title">${escape(a.title)}</h1>
      <div class="read-meta">
        <span class="read-author">${ICONS.user(14)} ${a.userId ? `<a class="author-link" href="/user.html?id=${a.userId}">${escape(a.author)}</a>` : escape(a.author)}</span>
        <span>${escape(a.createdAt)}</span>
        <button class="rep-btn-mini" data-report-type="article" data-report-id="${a.id}" data-report-label="${escape(a.title)}">举报</button>
      </div>
    </header>
    <article class="read-body md-view">${renderMarkdown(a.content || '')}</article>
    <footer class="read-foot">
      <a class="btn btn-ghost" href="/articles.html">← 返回文章列表</a>
    </footer>`;
}

function escape(s) { return (s == null ? '' : String(s)).replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m])); }

load();