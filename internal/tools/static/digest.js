'use strict';
(function () {
  const button = document.getElementById('digestRefresh');
  if (button) {
    button.addEventListener('click', async () => {
      const original = button.textContent;
      button.disabled = true;
      button.textContent = '抓取中…';
      try {
        const res = await fetch('/api/tools/digest/refresh', { method: 'POST' });
        const data = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error(data.error || '抓取失败');
        location.reload();
      } catch (err) {
        alert(err.message || '抓取失败');
        button.disabled = false;
        button.textContent = original;
      }
    });
  }
  document.querySelectorAll('img[data-avatar]').forEach((img) => {
    img.addEventListener('error', () => {
      const fallback = document.createElement('span');
      fallback.className = 'digest-avatar digest-avatar-empty';
      fallback.setAttribute('aria-hidden', 'true');
      fallback.textContent = '▣';
      img.replaceWith(fallback);
    }, { once: true });
  });
})();
