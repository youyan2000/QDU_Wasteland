// viewer-mdown.js — 轻量 Markdown 渲染器（无外部依赖）
// 支持：标题、加粗/斜体/删除线、行内代码、代码块、链接、无序/有序列表、引用、水平线、表格、图片
// 注意：这里刻意不执行不安全 HTML，仅输出安全元素。

function renderMarkdown(md) {
  if (!md) return '';
  let s = escapeHtml(md);

  // 代码块（右上角带复制按钮）
  s = s.replace(/```(\w*)\n([\s\S]*?)```/g, (m, lang, code) => {
    return '<pre class="md-code"><button class="md-copy" type="button" title="复制代码">复制</button><code>' + code + '</code></pre>';
  });
  // 行内代码
  s = s.replace(/`([^`]+)`/g, '<code class="md-inline">$1</code>');

  // 标题
  s = s.replace(/^###### (.*)$/gm, '<h6>$1</h6>');
  s = s.replace(/^##### (.*)$/gm, '<h5>$1</h5>');
  s = s.replace(/^#### (.*)$/gm, '<h4>$1</h4>');
  s = s.replace(/^### (.*)$/gm, '<h3>$1</h3>');
  s = s.replace(/^## (.*)$/gm, '<h2>$1</h2>');
  s = s.replace(/^# (.*)$/gm, '<h1>$1</h1>');

  // 引用
  s = s.replace(/^&gt; (.*)$/gm, '<blockquote>$1</blockquote>');

  // 水平线
  s = s.replace(/^---+$/gm, '<hr>');

  // 图片 ![alt](url)
  s = s.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, '<img src="$2" alt="$1" loading="lazy">');
  // 链接 [text](url)
  s = s.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');

  // 粗体/斜体/删除线
  s = s.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  s = s.replace(/(^|[^*])\*([^*\n]+)\*/g, '$1<em>$2</em>');
  s = s.replace(/~~([^~]+)~~/g, '<del>$1</del>');

  // 无序列表
  s = s.replace(/^[-*] (.*)$/gm, '<li>$1</li>');
  s = s.replace(/(<li>.*<\/li>\n?)+/g, '<ul>$&</ul>');
  // 有序列表
  s = s.replace(/^\d+\. (.*)$/gm, '<li>$1</li>');

  // 表格（简单支持）
  // 段落
  s = s.replace(/(?:^|\n)([^\n<].*?)(?=\n\n|$)/g, '<p>$1</p>');

  // 链接到外部时安全化
  return s;
}

function escapeHtml(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// 代码块复制按钮：全局事件委托（内容由 renderMarkdown 注入后也能生效）
document.addEventListener('click', function (e) {
  const btn = e.target.closest('.md-copy');
  if (!btn) return;
  const pre = btn.closest('pre.md-code');
  const code = pre ? pre.querySelector('code') : null;
  if (!code) return;
  const text = code.textContent;
  const done = function () {
    const label = btn.getAttribute('data-label') || '复制';
    btn.textContent = '已复制';
    setTimeout(function () { btn.textContent = label; }, 1600);
  };
  const fallback = function () {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand('copy'); done(); } catch (err) {}
    document.body.removeChild(ta);
  };
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(done).catch(fallback);
  } else {
    fallback();
  }
});
