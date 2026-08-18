// search.js — 全站搜索结果页
const resultEl = document.getElementById('searchResult');
const inputEl = document.getElementById('searchInput');

function currentQ() {
  return (new URLSearchParams(location.search).get('q') || '').trim();
}

// 初始化：预填并执行
const q0 = currentQ();
if (q0) { inputEl.value = q0; doSearch(q0); }

document.getElementById('searchForm').addEventListener('submit', e => {
  e.preventDefault();
  const q = inputEl.value.trim();
  if (q) {
    history.replaceState(null, '', '/search.html?q=' + encodeURIComponent(q));
    doSearch(q);
  }
});

async function doSearch(q) {
  resultEl.innerHTML = '<div class="skeleton-list"><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div></div>';
  try {
    const r = await fetch('/api/search?q=' + encodeURIComponent(q));
    if (r.status !== 200) { resultEl.innerHTML = '<div class="empty">搜索失败</div>'; return; }
    render(await r.json());
  } catch (e) { resultEl.innerHTML = '<div class="empty">搜索失败</div>'; }
}

function render(d) {
  const courses = d.courses || [];
  const posts = d.posts || [];
  const articles = d.articles || [];
  const total = courses.length + posts.length + articles.length;
  if (total === 0) {
    resultEl.innerHTML = `<div class="empty"><span class="empty-title">没有找到相关结果</span><span class="empty-hint">没有与「${escapeHtml(d.query)}」相关的内容，换个说法试试</span></div>`;
    return;
  }

  const courseHtml = courses.length ? `
    <section class="rs-sec">
      <h2 class="rs-h2">课程 (${courses.length})</h2>
      ${courses.map(c => `
        <a class="rs-item" href="/course.html?code=${encodeURIComponent(c.code)}">
          <span class="rs-badge badge-course">课</span>
          <span class="rs-title">${escapeHtml(c.name)}</span>
          <span class="rs-sub">${escapeHtml(c.code)} · ${escapeHtml(c.college || '')}</span>
        </a>`).join('')}
    </section>` : '';

  const postHtml = posts.length ? `
    <section class="rs-sec">
      <h2 class="rs-h2">帖子 (${posts.length})</h2>
      ${posts.map(p => `
        <a class="rs-item" href="/post.html?id=${p.id}">
          <span class="rs-badge badge-post">帖</span>
          <span class="rs-title">${escapeHtml(p.title)}</span>
          <span class="rs-sub">${escapeHtml(p.forum)} · ${escapeHtml(p.author)}</span>
        </a>`).join('')}
    </section>` : '';

  const articleHtml = articles.length ? `
    <section class="rs-sec">
      <h2 class="rs-h2">文章 (${articles.length})</h2>
      ${articles.map(a => `
        <a class="rs-item" href="/article.html?id=${a.id}">
          <span class="rs-badge badge-article">文</span>
          <span class="rs-title">${escapeHtml(a.title)}</span>
          <span class="rs-sub">${escapeHtml(a.category)} · ${escapeHtml(a.author)}</span>
        </a>`).join('')}
    </section>` : '';

  resultEl.innerHTML = `
    <div class="rs-total">共 ${total} 条结果</div>
    ${courseHtml}${postHtml}${articleHtml}`;
}

function escapeHtml(s) {
  return (s == null ? '' : String(s)).replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m]));
}