// themeboot.js — 全站主题引导：尽早应用已保存的主题、主题色与导航位置。
// 由所有页面在 <head> 中同步引入，避免首屏先亮后暗；
// nav.js 在渲染设置面板时复用这里的 window.QWTheme。

(function () {
  'use strict';

  var THEMES = ['dark', 'cyber', 'github'];
  // 任意主题下都可以用自定义主题色；"跟随主题"则各自回到内置配色：
  // 默认=品红、暗黑=暗红、赛博=荧光绿、简约=黑白。
  var COLOR_THEMES = ['', 'dark', 'cyber', 'github'];

  function clamp(v) { return Math.max(0, Math.min(255, Math.round(v))); }

  function parseHex(hex) {
    if (!/^#[0-9a-fA-F]{6}$/.test(hex)) return null;
    var n = parseInt(hex.slice(1), 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  }

  function toHex(rgb) {
    return '#' + rgb.map(function (v) {
      return clamp(v).toString(16).padStart(2, '0');
    }).join('');
  }

  function mix(a, b, weight) {
    return [
      a[0] + (b[0] - a[0]) * weight,
      a[1] + (b[1] - a[1]) * weight,
      a[2] + (b[2] - a[2]) * weight,
    ];
  }

  function ensureThemesCss() {
    if (document.getElementById('themesCss')) return;
    var link = document.createElement('link');
    link.id = 'themesCss';
    link.rel = 'stylesheet';
    link.href = '/themes.css';
    document.head.appendChild(link);
  }

  function read() {
    try {
      return {
        theme: localStorage.getItem('qwTheme') || '',
        color: localStorage.getItem('qwColor') || '',
        nav: localStorage.getItem('qwNav') || '',
      };
    } catch (_error) {
      return { theme: '', color: '', nav: '' };
    }
  }

  function save(patch) {
    try {
      Object.keys(patch).forEach(function (key) {
        var value = patch[key];
        if (value === '' || value == null) localStorage.removeItem(key);
        else localStorage.setItem(key, value);
      });
    } catch (_error) {}
  }

  // 把用户选的一个色相展开成完整调色板：主色 / 深色 / 浅色底 / 渐变副色。
  function palette(hex) {
    var base = parseHex(hex);
    if (!base) return null;
    var white = [255, 255, 255];
    return {
      base: toHex(base),
      deep: toHex(mix(base, [0, 0, 0], 0.26)),
      soft: 'rgba(' + base.map(clamp).join(', ') + ', 0.16)',
      // 渐变副色只做同色相明暗变化，避免"红配橙"那种两块不搭的颜色。
      gradient: toHex(mix(base, white, 0.28)),
    };
  }

  function applyTheme(theme) {
    if (theme) document.documentElement.setAttribute('data-theme', theme);
    else document.documentElement.removeAttribute('data-theme');
  }

  function apply() {
    var s = read();
    var preview = '';
    try {
      preview = new URLSearchParams(location.search).get('theme') || '';
    } catch (_error) {}

    var theme = s.theme;
    if (/^(dark|cyber|github|default)$/.test(preview)) theme = preview === 'default' ? '' : preview;

    ensureThemesCss();
    if (theme) document.body.setAttribute('data-theme', theme);
    else document.body.removeAttribute('data-theme');
    if (s.nav === 'left') document.body.setAttribute('data-nav', 'left');
    else document.body.removeAttribute('data-nav');

    var root = document.body.style;
    root.removeProperty('--magenta');
    root.removeProperty('--magenta-deep');
    root.removeProperty('--magenta-soft');
    root.removeProperty('--sunrise');

    if (s.color && COLOR_THEMES.indexOf(theme) >= 0) {
      var p = palette(s.color);
      if (p) {
        root.setProperty('--magenta', p.base);
        root.setProperty('--magenta-deep', p.deep);
        root.setProperty('--magenta-soft', p.soft);
        root.setProperty('--sunrise', p.gradient);
        // 各页面把这些"派生变量"写在 :root 里，var() 会在 :root 处就固定成默认色，
        // 所以还要在 body 上重新赋值，才能让工具页等地方一起换色。
        root.setProperty('--grad-magenta', 'linear-gradient(90deg, ' + p.base + ', ' + p.gradient + ')');
        root.setProperty('--tool-accent', p.base);
        root.setProperty('--tool-accent-soft', p.soft);
        root.setProperty('--tool-accent-text', p.deep);
        root.setProperty('--theme-accent', p.base);
        root.setProperty('--theme-accent-strong', p.deep);
        // 工具页的按钮用的是独立变量，默认值写死成玫红，需要一起覆盖。
        root.setProperty('--tool-action-bg', p.base);
        root.setProperty('--tool-action-hover', p.deep);
        root.setProperty('--tool-action-fg', '#fff');
      }
      document.documentElement.setAttribute('data-accent', 'custom');
    } else {
      root.removeProperty('--grad-magenta');
      root.removeProperty('--tool-accent');
      root.removeProperty('--tool-accent-soft');
      root.removeProperty('--tool-accent-text');
      root.removeProperty('--theme-accent');
      root.removeProperty('--theme-accent-strong');
      root.removeProperty('--tool-action-bg');
      root.removeProperty('--tool-action-hover');
      root.removeProperty('--tool-action-fg');
      document.documentElement.removeAttribute('data-accent');
    }
  }

  window.QWTheme = {
    themes: THEMES,
    colorThemes: COLOR_THEMES,
    apply: apply,
    read: read,
    save: save,
    palette: palette,
    supportsColor: function (theme) {
      return COLOR_THEMES.indexOf(theme || '') >= 0;
    },
  };

  ensureThemesCss();
  if (document.body) apply();
  else document.addEventListener('DOMContentLoaded', apply);
})();
