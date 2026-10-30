'use strict';
(function () {
  const button = document.getElementById('hotRefresh');
  if (!button) return;
  button.addEventListener('click', async () => {
    const original = button.textContent;
    button.disabled = true;
    button.textContent = '抓取中…';
    try {
      const res = await fetch('/api/tools/hot/refresh', { method: 'POST' });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || '抓取失败');
      location.reload();
    } catch (err) {
      alert(err.message || '抓取失败');
      button.disabled = false;
      button.textContent = original;
    }
  });
})();
