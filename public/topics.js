// topics.js — 公开专题浏览页：列出专题，点进看聚合文章（自包含样式，类名以 topic- 开头）
const viewEl = document.getElementById('topicView');
function escape(s) { return (s == null ? '' : String(s)).replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m])); }

// ?id=N 表示查看某个专题详情；否则列表
const idParam = new URLSearchParams(location.search).get('id');

async function loadTopics() {
  try {
    const r = await fetch('/api/topics');
    const d = await r.json();
    const tops = d.topics || [];
    if (tops.length === 0) {
      viewEl.innerHTML = '<div class="topic-empty">还没有专题，站长可在后台创建</div>';
      return;
    }
    viewEl.innerHTML = `<div class="topic-grid">` + tops.map(t => `
      <a class="topic-card" href="/topics.html?id=${t.id}">
        <div class="topic-title">${ICONS.bookmark(16)} ${escape(t.title)}</div>
        <div class="topic-desc">${escape(t.description) || '暂无简介'}</div>
        <div class="topic-meta">${t.articleCount} 篇文章</div>
      </a>`).join('') + `</div>`;
  } catch (e) { viewEl.innerHTML = '<div class="topic-empty">加载失败</div>'; }
}

async function loadTopicDetail(id) {
  try {
    const r = await fetch('/api/topics/' + id);
    if (!r.ok) { viewEl.innerHTML = '<div class="topic-empty">专题不存在</div>'; return; }
    const d = await r.json();
    const t = d.topic, arts = d.articles || [];
    let html = `<div class="topic-detail-head">
        <div class="td-title">${ICONS.bookmark(16)} ${escape(t.title)}</div>
        <p class="td-desc">${escape(t.description) || ''}</p>
        <span class="topic-meta">共 ${arts.length} 篇文章</span>
        <div style="margin-top:8px"><a class="topics-back" href="/topics.html">← 返回专题列表</a></div>
      </div>`;
    if (arts.length === 0) {
      html += '<div class="topic-empty">这个专题还没有文章</div>';
    } else {
      html += `<div class="topic-article-list">` + arts.map(a => `
        <a class="topic-article" href="/article.html?id=${a.id}">
          <div class="ta-top">
            <span class="ta-cat">${escape(a.category)}</span>
            <span class="ta-time">${escape(a.createdAt)}</span>
          </div>
          <div class="ta-title">${escape(a.title)}</div>
          <div class="ta-author">${ICONS.edit(13)} ${escape(a.author)}</div>
        </a>`).join('') + `</div>`;
    }
    viewEl.innerHTML = html;
  } catch (e) { viewEl.innerHTML = '<div class="topic-empty">加载失败</div>'; }
}

if (idParam) loadTopicDetail(idParam); else loadTopics();
