// articles.js — 关于学校 文章列表页（V14 排版修复：杜绝嵌套 <a> 导致的卡片断裂）
const listEl = document.getElementById('articleList');
let currentCat = '';

async function loadList() {
  listEl.innerHTML = '<div class="skeleton-list"><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div></div>';
  const url = '/api/articles' + (currentCat ? '?cat=' + encodeURIComponent(currentCat) : '');
  try {
    const r = await fetch(url);
    const d = await r.json();
    let arts = d.articles || [];
    if (window.filterBlocked) arts = window.filterBlocked(arts, 'userId'); // 拉黑过滤
    if (arts.length === 0) {
      listEl.innerHTML = '<div class="empty"><span class="empty-title">还没有文章</span><span class="empty-hint">来写下第一篇校园经验吧</span></div>';
      return;
    }
    listEl.innerHTML = arts.map(a => `
      <article class="article-card clickable" data-id="${a.id}" data-href="/article.html?id=${a.id}" style="cursor:pointer">
        ${a.cover
          ? `<div class="ac-cover ac-cover-img"><img src="${escape(a.cover)}" alt="" loading="lazy"></div>`
          : `<div class="ac-cover ${coverClass(a.category)}"><span class="ac-cover-letter">${coverLetter(a.title)}</span></div>`}
        <div class="ac-body">
          <div class="ac-top">
            <span class="cat-tag">${escape(a.category)}</span>
            <span class="ac-time">${escape(a.createdAt)}</span>
          </div>
          <div class="ac-title">${escape(a.title)}</div>
          <div class="ac-excerpt">${escape(a.content)}</div>
          <div class="ac-meta">
            <span class="ac-author">
              ${a.userId && !a.anonymous
                ? `<a class="author-link" href="/user.html?id=${a.userId}">${ICONS.user(13)} ${escape(a.author)}</a>`
                : `<span>${ICONS.user(13)} ${escape(a.author)}</span>`}
            </span>
          </div>
        </div>
      </article>`).join('');
  } catch (e) { listEl.innerHTML = '<div class="empty">加载失败</div>'; }
}

// 分类切换
document.getElementById('catBar').addEventListener('click', e => {
  const chip = e.target.closest('.cat-chip');
  if (!chip) return;
  document.querySelectorAll('.cat-chip').forEach(c => c.classList.remove('active'));
  chip.classList.add('active');
  currentCat = chip.dataset.cat;
  loadList();
});

// 写文章
document.getElementById('newArticleBtn').addEventListener('click', async () => {
  const me = await (await fetch('/api/me')).json();
  if (!me.loggedIn) { location.href = '/login.html'; return; }
  location.href = '/article-editor.html';
});

function escape(s) { return (s == null ? '' : String(s)).replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m])); }

loadList();

// 封面：按分类取不同渐变背景
function coverClass(cat) {
  const c = (cat||'').trim();
  if (c.includes('竞赛')) return 'cover-competition';
  if (c.includes('社团')) return 'cover-club';
  if (c.includes('生活')) return 'cover-life';
  if (c.includes('老师')) return 'cover-teacher';
  return 'cover-default';
}
// 封面首字母（取标题第一个汉字/字母）
function coverLetter(title) {
  const t = (title||'').trim();
  return t ? t.slice(0,1) : '文';
}

// 整卡点击进入文章详情（点击封面/标题/正文都行）
document.getElementById('articleList').addEventListener('click', (e) => {
  const card = e.target.closest('.article-card.clickable');
  if (!card || e.target.closest('a')) return; // 点击了作者链接等 a 则交给默认
  location.href = card.dataset.href;
});
