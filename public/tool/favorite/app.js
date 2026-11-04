(() => {
  const form = document.getElementById('bookmark-form');
  const list = document.getElementById('bookmark-list');
  const message = document.getElementById('bookmark-message');
  const cancel = document.getElementById('bookmark-cancel');
  const esc = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  const fields = name => form.elements.namedItem(name);

  async function request(path, options = {}) {
    const response = await fetch(path, { ...options, credentials: 'same-origin', headers: { 'Content-Type': 'application/json', ...(options.headers || {}) } });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error || '操作失败');
    return data;
  }
  function reset() { form.reset(); fields('id').value = ''; cancel.hidden = true; }
  function edit(item) { fields('id').value = item.id; fields('title').value = item.title; fields('url').value = item.url; fields('description').value = item.description; cancel.hidden = false; fields('title').focus(); }
  async function load() {
    try {
      const data = await request('/tool/favorite/api/bookmarks', { headers: {} });
      if (!data.items.length) { list.innerHTML = '<p class="bookmark-empty">还没有收藏网址。</p>'; return; }
      list.innerHTML = data.items.map(item => `<article class="bookmark-row" data-id="${item.id}"><h3><a href="${esc(item.url)}" target="_blank" rel="noopener noreferrer">${esc(item.title)}</a></h3><p>${esc(item.description)}</p><small>${esc(item.url)}</small><div class="bookmark-row-actions"><button type="button" data-action="edit">编辑</button><button type="button" data-action="delete">删除</button></div></article>`).join('');
      list.querySelectorAll('[data-action="edit"]').forEach(button => button.addEventListener('click', () => { const id = Number(button.closest('[data-id]').dataset.id); const item = data.items.find(entry => entry.id === id); if (item) edit(item); }));
      list.querySelectorAll('[data-action="delete"]').forEach(button => button.addEventListener('click', async () => { const row = button.closest('[data-id]'); if (!window.confirm('删除这条收藏？')) return; try { await request(`/tool/favorite/api/bookmarks/${row.dataset.id}`, { method: 'DELETE' }); await load(); } catch (error) { message.textContent = error.message; } }));
    } catch (error) { list.innerHTML = `<p class="bookmark-empty">${esc(error.message)}</p>`; }
  }
  form.addEventListener('submit', async event => {
    event.preventDefault();
    const id = fields('id').value;
    const payload = { title: fields('title').value, url: fields('url').value, description: fields('description').value };
    try { await request(id ? `/tool/favorite/api/bookmarks/${id}` : '/tool/favorite/api/bookmarks', { method: id ? 'PUT' : 'POST', body: JSON.stringify(payload) }); message.textContent = '已保存'; reset(); await load(); }
    catch (error) { message.textContent = error.message; }
  });
  cancel.addEventListener('click', reset);
  void load();
})();
