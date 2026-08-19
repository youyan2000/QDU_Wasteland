// course.js — 课程详情页
const wrap = document.getElementById('detailWrap');
const code = new URLSearchParams(location.search).get('code');

async function load() {
  if (!code) { wrap.innerHTML = '<div class="empty">缺少课程编号</div>'; return; }
  try {
    const r = await fetch(`/api/courses/${encodeURIComponent(code)}`);
    if (r.status === 404) { wrap.innerHTML = '<div class="empty">课程不存在</div>'; return; }
    const c = await r.json();
    render(c);
    // 设置"上传本课资料"按钮带课程参数
    const up = document.getElementById('uploadThis');
    if (up) { up.href = '/upload.html?course=' + encodeURIComponent(c.code); }
  } catch (e) { wrap.innerHTML = '<div class="empty">加载失败</div>'; }
}

function render(c) {
  // 基本信息
  // 基本信息
  const infos = [
    ['课程编号', c.code], ['课程名称', c.name],
    ['开课院系', c.college],
    ['课程标签', c.tag], ['考核方式', c.exam],
    ['课程性质', c.nature], ['课程大类', c.bigType],
    ['课程属性', c.attribute], ['学分', c.credit],
    ['通选类别', c.general], ['开课学期', c.terms.join(' / ')]
  ].filter(x => x[1]);

  // 上课专业：专业课列出实际专业（长列表截断显示 + 悬浮提示完整列表）
  const majorsList = (c.majors && c.majors.length) ? c.majors : [];
  const majorsText = majorsList.length === 0 ? '—'
    : majorsList.length === 1 && majorsList[0] === '全校' ? '全校'
    : majorsList.slice(0, 6).join(' · ') + (majorsList.length > 6 ? ` 等${majorsList.length}个专业` : '');
  const majorsFull = majorsList.join(' · ');

  let html = `
    <div class="courses-head">
      <span class="kicker">COURSE · ${escape(c.code)}</span>
      <h1 class="courses-h1">${escape(c.name)}</h1>
    </div>
    <table class="info-table">
  `;
  // 转成两列式信息表
  let i = 0;
  while (i < infos.length) {
    html += '<tr>';
    for (let k = 0; k < 2 && i < infos.length; k++, i++) {
      html += `<td class="info-key">${escape(infos[i][0])}</td><td class="info-val">${escape(infos[i][1])}</td>`;
      if (!(k===0 && i>=infos.length)) html += '';
    }
    html += '</tr>';
  }
  html += `</table>`;
  // 上课专业（独立整块，长内容可换行，不打乱上方表格）
  html += `<div class="major-block"><span class="major-label">上课专业</span><span class="major-vals" title="${escape(majorsFull)}">${escape(majorsText)}</span></div>`;

  // 课程资料区（放前面，优先看到）
  html += `<h2 class="detail-h2">${ICONS.folder(18)} 课程资料</h2>`;
  html += `<div id="filesArea"><div class="empty">加载中…</div></div>`;

  // 课程评价区（放前面）
  html += `<h2 class="detail-h2">${ICONS.chat(18)} 课程评价</h2>`;
  html += `<div id="reviewsArea"><div class="empty">加载中…</div></div>`;

  // 开课班次（铺开、沉底显示，不挤压上方内容）
  html += `<h2 class="detail-h2">${ICONS.calendar(18)} 开课班次 (${c.offerings.length})</h2>`;
  if (c.offerings.length === 0) {
    html += '<div class="empty">暂无班次记录</div>';
  } else {
    // 按学期从新到旧排序（2026春 在前）
    const offSorted = c.offerings.slice().sort((a, b) => termScore(b.term) - termScore(a.term));
    html += `<div class="offering-grid">`;
    offSorted.slice(0, 60).forEach(o => {
      html += `<div class="offering-card">
        <div class="oc-head">
          <span class="off-term">${escape(o.term||'')}</span>
          <span class="oc-teacher">${escape(o.teacher||'')}</span>
        </div>
        <div class="oc-class">${escape(o.classGroup||'')}</div>
        ${o.time?`<div class="oc-time">${ICONS.clock(14)} ${escape(o.time)}</div>`:''}
        ${o.location?`<div class="oc-loc">${ICONS.mapPin(14)} ${escape(o.location)}</div>`:''}
      </div>`;
    });
    html += `</div>`;
  }

  wrap.innerHTML = html;

  // 异步加载资料和评价
  loadFilesArea();
  loadReviewsArea();
}

// ------ 课程资料区 ------
async function loadFilesArea() {
  const area = document.getElementById('filesArea');
  try {
    const r = await fetch(`/api/files?code=${encodeURIComponent(code)}`);
    const d = await r.json();
    const files = d.files || [];
    if (files.length === 0) {
      area.innerHTML = '<div class="empty">还没有资料，快来上传第一份吧</div>';
    } else {
      area.innerHTML = '<div class="file-list">' + files.map(f => `
        <div class="file-item">
          <a href="/viewer.html?id=${f.id}&name=${encodeURIComponent(f.fileName)}" class="file-link">${escape(f.fileName)}</a>
          ${f.title ? `<span class="file-title">${escape(f.title)}</span>` : ''}
          <span class="file-meta">${fmtSize(f.size)} · ${escape(f.createdAt)}</span>
          <a href="${f.downloadUrl}" class="file-dl" title="下载">${ICONS.download(15)}</a>
        </div>`).join('') + '</div>';
    }
  } catch (e) { area.innerHTML = '<div class="empty">资料加载失败</div>'; }
}

// ------ 课程评价区 ------
let myRating = 0;
async function loadReviewsArea() {
  const area = document.getElementById('reviewsArea');
  try {
    const r = await fetch(`/api/reviews?code=${encodeURIComponent(code)}`);
    const d = await r.json();
    area.innerHTML = reviewFormHtml() + '<div class="review-list">' + (d.reviews&&d.reviews.length ? d.reviews.map(rev => `
      <div class="review-item">
        <span class="review-stars">${'★'.repeat(rev.rating)}${'☆'.repeat(5-rev.rating)}</span>
        <span class="review-author">${escape(rev.author)}</span>
        <span class="review-time">${escape(rev.createdAt)}</span>
        <button class="rep-btn-mini" data-report-type="review" data-report-id="${rev.id}" data-report-label="${escape(String(rev.content).slice(0,30))}">举报</button>
        <p class="review-content">${escape(rev.content)}</p>
      </div>`).join('') : '<div class="empty">还没有评价，来说说你的感受吧</div>') + '</div>';
    bindReviewForm();
  } catch (e) { area.innerHTML = '<div class="empty">评价加载失败</div>'; }
}

function reviewFormHtml() {
  return `
    <div class="review-form">
      <div class="rating-row">
        评分：
        ${[1,2,3,4,5].map(n => `<button type="button" class="star-btn" data-n="${n}">★</button>`).join('')}
        <button type="button" id="reviewClear" style="background:none;border:none;color:var(--muted)">清除</button>
      </div>
      <textarea id="reviewContent" rows="2" placeholder="写下对这门课和老师的评价…"></textarea>
      <div style="display:flex;align-items:center;gap:12px;margin-top:8px">
        <label style="display:flex;align-items:center;gap:4px;font-size:.85rem"><input type="checkbox" id="reviewAnon" checked> 匿名</label>
        <button id="reviewSubmit">发布评价</button>
        <span id="reviewMsg"></span>
      </div>
    </div>`;
}

function bindReviewForm() {
  const submit = document.getElementById('reviewSubmit');
  if (!submit) return;
  document.querySelectorAll('.star-btn').forEach(b => {
    b.addEventListener('click', () => {
      myRating = +b.dataset.n;
      document.querySelectorAll('.star-btn').forEach((s,i) => s.style.color = i < myRating ? '#f5a623' : '#ddd');
    });
  });
  document.getElementById('reviewClear').addEventListener('click', () => {
    myRating = 0;
    document.querySelectorAll('.star-btn').forEach(s => s.style.color = '#ddd');
  });
  submit.addEventListener('click', async () => {
    const msg = document.getElementById('reviewMsg');
    const content = document.getElementById('reviewContent').value.trim();
    if (!content) { msg.textContent = '请填写评价内容'; return; }
    const res = await fetch('/api/reviews/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ courseCode: code, rating: myRating, content, anonymous: document.getElementById('reviewAnon').checked })
    });
    const d = await res.json();
    if (!res.ok) {
      msg.textContent = d.error;
      if (d.error === '请先登录') msg.innerHTML = '请先 <a href="/login.html" style="color:var(--magenta)">登录</a>';
      return;
    }
    msg.innerHTML = ICONS.check(14)+' 已发布';
    loadReviewsArea();
  });
}

// 文件大小格式化
function fmtSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1048576) return (bytes/1024).toFixed(1) + ' KB';
  return (bytes/1048576).toFixed(1) + ' MB';
}

function escape(s) { return (s==null?'':String(s)).replace(/[&<>"']/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m])); }

load();

// termScore 学期字典序排序（2026春 > 2023秋）
function termScore(term) {
  var year = parseInt((term||'').match(/\d+/), 10) || 0;
  var score = year * 10;
  if (term.indexOf('秋') >= 0) score += 2;
  else if (term.indexOf('夏') >= 0) score += 1;
  return score;
}
