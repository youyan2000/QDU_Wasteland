// icon-boot.js — 把页面里的图标占位替换成内联 SVG。
// 放在外部文件里执行，避免内联脚本被 CSP（script-src 'self'）拦截。
(function () {
  function sizeFor(el) {
    if (el.dataset.size) return Number(el.dataset.size);
    if (el.classList.contains('cat-ic')) return 14;
    return 18;
  }

  function paint() {
    var nodes = document.querySelectorAll('.icon-fill[data-icon], .cat-ic[data-icon]');
    nodes.forEach(function (el) {
      var f = window.ICONS && window.ICONS[el.dataset.icon];
      if (!f) return;
      el.innerHTML = f(sizeFor(el));
    });
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', paint);
  else paint();
})();
