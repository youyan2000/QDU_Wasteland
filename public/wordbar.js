// wordbar.js — 全站顶部/底部随机单词条。
// 词库来自站点内置数据（/api/words/random），不读取任何本地目录。
// 样式全部走主题变量，因此默认/暗黑/赛博/简约四个主题自动适配。
(function () {
  'use strict';

  var mounted = false;

  // 词条样式单独成表并自带兜底变量：即使页面没引 style.css 也能正常显示。
  function ensureCss() {
    if (document.getElementById('wordbarCss')) return;
    var link = document.createElement('link');
    link.id = 'wordbarCss';
    link.rel = 'stylesheet';
    link.href = '/wordbar.css';
    document.head.appendChild(link);
  }

  function el(tag, cls, text) {
    var node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text != null) node.textContent = text;
    return node;
  }

  function fill(slot, word) {
    var inner = slot.querySelector('.wordline-inner');
    if (!inner) return;
    var w = inner.querySelector('.wordline-word');
    var ph = inner.querySelector('.wordline-ph');
    var pos = inner.querySelector('.wordline-pos');
    var def = inner.querySelector('.wordline-def');
    w.textContent = word.w || '';
    ph.textContent = word.ph || '';
    ph.hidden = !word.ph;
    pos.textContent = word.pos || '';
    pos.hidden = !word.pos;
    def.textContent = word.en || '';
    slot.dataset.word = word.w || '';
  }

  function build(kind) {
    var slot = el('aside', 'wordline wordline-' + kind);
    slot.setAttribute('aria-label', kind === 'top' ? '今日单词' : '再记一个单词');
    var inner = el('div', 'wordline-inner');
    inner.appendChild(el('span', 'wordline-kicker', kind === 'top' ? '单词' : '再记一个'));
    inner.appendChild(el('span', 'wordline-word'));
    inner.appendChild(el('span', 'wordline-ph'));
    inner.appendChild(el('span', 'wordline-pos'));
    inner.appendChild(el('span', 'wordline-def'));
    var next = el('button', 'wordline-next', '换一个');
    next.type = 'button';
    inner.appendChild(next);
    slot.appendChild(inner);

    next.addEventListener('click', function () {
      next.disabled = true;
      fetch('/api/words/random?n=1', { cache: 'no-store' })
        .then(function (r) { return r.ok ? r.json() : null; })
        .then(function (data) {
          if (data && data.words && data.words[0]) fill(slot, data.words[0]);
        })
        .catch(function () {})
        .then(function () { next.disabled = false; });
    });
    return slot;
  }

  function mount(words) {
    if (mounted || !words || !words.length) return;
    // 首页是整屏封面，不放词条；只在一级页面以内出现。
    if (document.querySelector('.hero')) return;
    mounted = true;

    var top = build('top');
    var nav = document.querySelector('.site-nav') || document.querySelector('.top-nav');
    if (nav && nav.parentNode) {
      nav.parentNode.insertBefore(top, nav.nextSibling);
    } else {
      document.body.insertBefore(top, document.body.firstChild);
    }

    var bottom = build('bottom');
    if (words[1]) fill(bottom, words[1]);
    var footer = document.querySelector('footer, .footer, .site-footer, .history-site-footer');
    if (footer && footer.parentNode) footer.parentNode.insertBefore(bottom, footer);
    else document.body.appendChild(bottom);

    fill(top, words[0]);
  }

  function boot() {
    ensureCss();
    fetch('/api/words/random?n=2', { cache: 'no-store' })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (data) { if (data) mount(data.words); })
      .catch(function () {});
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();
})();
