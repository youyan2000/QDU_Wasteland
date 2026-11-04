// site.js — 只标记当前标签页；页面内容由 app.js 统一渲染。
(() => {
  const path = location.pathname;
  const route = path.split('/').filter(Boolean).at(-1) || 'site';
  const aliases = { 'index.html': 'site' };
  const current = aliases[route] || route;
  document.querySelectorAll('[data-history-tab]').forEach(link => {
    if (link.dataset.historyTab === current) link.setAttribute('aria-current', 'page');
  });
})();
