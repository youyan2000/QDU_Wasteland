// upload.js — 上传资料页
// 未登录直接跳登录（点上传资料应先去登录）
(async function() {
  try {
    const r = await fetch('/api/me');
    const d = await r.json();
    if (!d.loggedIn) { location.href = '/login.html'; return; }
    init();
  } catch (e) { location.href = '/login.html'; }
})();

let selectedCourse = null; // from ?course=
let selectedFile = null;
const queryCourse = new URLSearchParams(location.search).get('course');

function init() {
  const lockArea = document.getElementById('lockArea');
  const pickerArea = document.getElementById('pickerArea');
  if (queryCourse) {
    // 从课程页进入：锁定课程
    lockArea.style.display = 'block';
    const sel = document.getElementById('lockName');
    sel.textContent = queryCourse;
    fetch('/api/courses/' + encodeURIComponent(queryCourse))
      .then(r=>r.json())
      .then(c=>{ if(c && c.name) sel.textContent = c.name + '（' + c.code + '）'; })
      .catch(()=>{});
    selectedCourse = { code: queryCourse, name: queryCourse };
  } else {
    // 独立进入：显示课程选择
    pickerArea.style.display = 'block';
    initPicker();
  }
  initFile();
  initSubmit();
}

// ---------- 课程搜索选择（独立进入时） ----------
function initPicker() {
  const search = document.getElementById('uSearch');
  const suggest = document.getElementById('uSuggest');
  let deb;
  search.addEventListener('input', () => { clearTimeout(deb); deb = setTimeout(doSearch, 300); });
  async function doSearch() {
    const q = search.value.trim();
    if (!q) { suggest.style.display='none'; return; }
    try {
      const r = await fetch('/api/courses?q=' + encodeURIComponent(q) + '&_=' + Date.now());
      const d = await r.json();
      suggest.innerHTML = '';
      if (!d.courses || !d.courses.length) suggest.innerHTML = '<div class="empty">没有匹配课程</div>';
      else d.courses.slice(0,10).forEach(c => {
        const div = document.createElement('div');
        div.textContent = c.name + '（' + c.code + '）';
        div.onclick = () => {
          selectedCourse = { code: c.code, name: c.name };
          document.getElementById('uSel').innerHTML = `<span class="selected-course">${ICONS.check(14)} ${c.name}</span>`;
          search.value = c.name;
          suggest.style.display='none';
          document.getElementById('pickerErr').style.display='none';
        };
        suggest.appendChild(div);
      });
      suggest.style.display = 'block';
    } catch(e){}
  }
  document.addEventListener('click', e => { if (!e.target.closest('.course-picker')) suggest.style.display='none'; });
}

// ---------- 文件拖拽 ----------
function initFile() {
  const dz = document.getElementById('dropZone');
  const uf = document.getElementById('uFile');
  dz.addEventListener('click', () => uf.click());
  dz.addEventListener('dragover', e => { e.preventDefault(); dz.classList.add('dragover'); });
  dz.addEventListener('dragleave', () => dz.classList.remove('dragover'));
  dz.addEventListener('drop', e => { e.preventDefault(); dz.classList.remove('dragover'); if(e.dataTransfer.files.length) setFile(e.dataTransfer.files[0]); });
  uf.addEventListener('change', e => { if(e.target.files.length) setFile(e.target.files[0]); });
}
function fmtSize(b){ if(b<1024)return b+' B'; if(b<1048576)return (b/1024).toFixed(1)+' KB'; return (b/1048576).toFixed(1)+' MB'; }
function setFile(f){
  if (f.size > 200*1024*1024) { document.getElementById('uErr').textContent='文件超过 200MB 限制'; return; }
  selectedFile = f;
  const chip = document.getElementById('uFileChip');
  chip.style.display = 'flex';
  chip.innerHTML = `<span>${ICONS.file(14)} ${f.name}</span><span class="fc-size">${fmtSize(f.size)}</span><button class="fc-remove" type="button">${ICONS.close(12)}</button>`;
  chip.querySelector('.fc-remove').onclick = () => { selectedFile=null; chip.style.display='none'; document.getElementById('uFile').value=''; };
  document.getElementById('uErr').textContent='';
}

// ---------- 提交 ----------
function initSubmit() {
  document.getElementById('uploadForm').addEventListener('submit', async e => {
    e.preventDefault();
    const err = document.getElementById('uErr');
    err.textContent = '';
    if (!selectedCourse) { err.textContent = '请先选择课程'; return; }
    if (!selectedFile) { err.textContent = '请添加文件'; return; }

    const fd = new FormData();
    fd.append('course_code', selectedCourse.code);
    fd.append('category', document.getElementById('uCategory').value);
    fd.append('title', document.getElementById('uTitle').value.trim());
    fd.append('semester', document.getElementById('uSemester').value);
    fd.append('description', document.getElementById('uDesc').value.trim());
    fd.append('anonymous', '1');
    fd.append('file', selectedFile);

    const btn = document.getElementById('uSubmit');
    btn.disabled = true; btn.textContent = '上传中…';
    // 进度条（大文件时可见）
    let progWrap = document.getElementById('uProg');
    if (!progWrap) {
      progWrap = document.createElement('div');
      progWrap.id = 'uProg';
      progWrap.style.cssText = 'margin-top:10px;display:none';
      btn.parentElement.appendChild(progWrap);
    }
    progWrap.innerHTML = '<div style="display:flex;justify-content:space-between;font-size:.85rem;margin-bottom:4px"><span>上传进度</span><span class="uProgPct">0%</span></div><div style="height:6px;background:var(--line,#eee);border-radius:3px;overflow:hidden"><div class="uProgBar" style="height:100%;width:0%;background:var(--accent,#4a90e2);transition:width .2s"></div></div>';
    progWrap.style.display = 'block';
    const pctEl = progWrap.querySelector('.uProgPct');
    const barEl = progWrap.querySelector('.uProgBar');

    try {
      const done = await new Promise((resolve) => {
        const xhr = new XMLHttpRequest();
        xhr.open('POST', '/api/upload');
        xhr.upload.onprogress = (ev) => {
          if (ev.lengthComputable) {
            const pct = Math.round(ev.loaded / ev.total * 100);
            pctEl.textContent = pct + '%';
            barEl.style.width = pct + '%';
          }
        };
        xhr.onload = () => resolve({ ok: xhr.status >= 200 && xhr.status < 300, body: xhr.responseText });
        xhr.onerror = () => resolve({ ok: false, body: '' });
        xhr.send(fd);
      });
      const d = done.body ? JSON.parse(done.body) : {};
      if (!done.ok) { err.textContent = d.error || '上传失败'; progWrap.style.display='none'; }
      else {
        barEl.style.width = '100%'; pctEl.textContent = '100%';
        btn.innerHTML = ICONS.check(14)+' 已提交';
        setTimeout(()=>location.href='/course.html?code='+encodeURIComponent(selectedCourse.code), 900);
      }
    } catch(e) {
      err.textContent = '网络好像开小差了，请检查连接后重试';
      progWrap.style.display='none';
    }
    btn.disabled = false; btn.textContent = '提交审核';
  });
}