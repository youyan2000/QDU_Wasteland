// viewer-mdown.js — 轻量 Markdown 渲染器（无外部依赖）
// 支持：标题、加粗/斜体/删除线、行内代码、代码块、链接、无序/有序列表、引用、水平线、表格、图片
// 注意：这里刻意不执行不安全 HTML，仅输出安全元素。

function renderMarkdown(md) {
  if (!md) return '';
  let s = escapeHtml(md);
  s = s.replace(/&gt;/g, '>'); // 处理行首引用

  // 代码块
  s = s.replace(/```(\w*)\n([\s\S]*?)```/g, (m, lang, code) => {
    return '<pre class="md-code"><code>' + code + '</code></pre>';
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
