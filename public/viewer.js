// viewer.js — 资料预览器
// 支持：markdown/pdf/图片/文本；word等不可预览时提示下载
const params = new URLSearchParams(location.search);
const id = params.get('id');
const name = params.get('name') || '';
const content = document.getElementById('viewerContent');
const nameEl = document.getElementById('viewerName');
const dlEl = document.getElementById('viewerDownload');
const backEl = document.getElementById('viewerBack');

// 返回：优先回上一页；当无历史（如后台新标签页直接打开预览）时回退到后台
function goBack() {
  if (document.referrer && (document.referrer.endsWith('/admin.html') || document.referrer.includes('/admin.html'))) {
    location.href = '/admin.html';
    return;
  }
  if (history.length > 1) { history.back(); return; }
  location.href = '/admin.html';
}
if (backEl) backEl.addEventListener('click', function (e) { e.preventDefault(); goBack(); });

function extOf(n) {
  const i = (n || '').lastIndexOf('.');
  return i >= 0 ? (n.slice(i + 1) || '').toLowerCase() : '';
}

async function load() {
  if (!id) { content.innerHTML = '<div class="empty">文件链接不完整，请返回重试</div>'; return; }
  nameEl.textContent = name;
  dlEl.href = '/api/files/' + id + '/download';

  const ext = extOf(name);
  const previewUrl = '/api/files/' + id + '/preview';

  // 可内嵌预览的类型
  if (ext === 'md' || ext === 'mdx') {
    try {
      const r = await fetch('/api/files/' + id + '/preview');
      const text = await r.text();
      content.innerHTML = '<div class="md-view">' + renderMarkdown(text) + '</div>';
    } catch (e) { content.innerHTML = '<div class="empty">Markdown 加载失败</div>'; }
    return;
  }
  if (ext === 'pdf') {
    content.innerHTML = `<iframe class="viewer-frame" src="${previewUrl}" type="application/pdf"></iframe>`;
    return;
  }
  const imgExts = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'bmp'];
  if (imgExts.includes(ext)) {
    content.innerHTML = `<div class="img-wrap"><img src="${previewUrl}" alt="${escapeHtml(name)}"></div>`;
    return;
  }
  if (ext === 'txt' || ext === 'log') {
    try {
      const r = await fetch(previewUrl);
      const text = await r.text();
      content.innerHTML = '<pre class="plain-view">' + escapeHtml(text) + '</pre>';
    } catch (e) { content.innerHTML = '<div class="empty">文本加载失败</div>'; }
    return;
  }
  if (ext === 'mp4' || ext === 'webm') {
    content.innerHTML = `<video class="viewer-frame" src="${previewUrl}" controls></video>`;
    return;
  }

  // 暂不支持直接预览（word/doc/ppt等）——提示下载用办公软件打开
  content.innerHTML = `
    <div class="empty">
      <p style="font-size:1.2rem;margin-bottom:.6rem">此格式暂不支持在线预览</p>
      <p style="margin-bottom:1.2rem">Word / Excel / PPT 等格式请下载后用办公软件打开</p>
      <a class="btn btn-primary" href="/api/files/${id}/download">${ICONS.download(16)} 下载文件</a>
    </div>`;
}

function escapeHtml(s) {
  return String(s == null ? '' : s).replace(/[&<>"']/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]));
}

load();
