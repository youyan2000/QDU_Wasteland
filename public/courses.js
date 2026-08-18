// courses.js — 课程检索页逻辑（J4 高级检索）
const PAGE_SIZE = 24;
let currentTag = '';
let currentQ = '';
let currentCat = '';
let currentTeacher = '';
let currentCollege = '';
let currentMajor = '';
let currentTime = '';
let currentSem = '';
let currentExam = '';
let currentCreditMin = '';
let currentCreditMax = '';
let currentSort = '';
let currentPage = 0;

const CATS = ['专业课','公共必修课','公共选修课','通选课','体育课','实验课','实践','美育课'];

const qInput = document.getElementById('qInput');
const subText = document.getElementById('subText');
const tagRow = document.getElementById('tagRow');
const listMeta = document.getElementById('listMeta');
const courseList = document.getElementById('courseList');
const pager = document.getElementById('pager');

// 高级检索折叠
document.getElementById('filterToggle').onclick = () => {
  const b = document.getElementById('filterBody');
  const open = b.style.display !== 'block';
  b.style.display = open ? 'block' : 'none';
  document.getElementById('filterToggle').querySelector('.caret').textContent = open ? '▴' : '▾';
};
document.getElementById('filterBody').style.display = 'none';

// 加载类别筛选
function loadCats() {
  const row = document.getElementById('catOpts');
  CATS.forEach(cat => {
    const chip = document.createElement('button');
    chip.className = 'cat-opt';
    chip.textContent = cat;
    chip.onclick = () => { currentCat = (currentCat === cat ? '' : cat); renderCats(); load(); };
    row.appendChild(chip);
  });
}
function renderCats() {
  [...document.querySelectorAll('.cat-opt')].forEach(ch => ch.classList.toggle('active', ch.textContent === currentCat));
}

// 加载标签
async function loadTags() {
  try {
    const r = await fetch('/api/tags');
    const d = await r.json();
    d.tags.forEach(t => {
      const chip = document.createElement('button');
      chip.className = 'tag-chip';
      chip.textContent = t;
      chip.onclick = () => { currentTag = (currentTag === t ? '' : t); renderChips(); load(); };
      tagRow.appendChild(chip);
    });
  } catch (e) { /* 忽略 */ }
}

function renderChips() {
  [...tagRow.children].forEach(ch => ch.classList.toggle('active', ch.textContent === currentTag));
}

// 查询 + 渲染
async function load() {
  const q = (currentQ || qInput.value).trim();
  let url = `/api/courses?`;
  if (q) url += `q=${encodeURIComponent(q)}&`;
  if (currentTag) url += `tag=${encodeURIComponent(currentTag)}&`;
  if (currentCat) url += `cat=${encodeURIComponent(currentCat)}&`;
  if (currentTeacher) url += `teacher=${encodeURIComponent(currentTeacher)}&`;
  if (currentCollege) url += `college=${encodeURIComponent(currentCollege)}&`;
  if (currentMajor) url += `major=${encodeURIComponent(currentMajor)}&`;
  if (currentTime) url += `time=${encodeURIComponent(currentTime)}&`;
  if (currentSem) url += `semester=${encodeURIComponent(currentSem)}&`;
  if (currentExam) url += `exam=${encodeURIComponent(currentExam)}&`;
  if (currentCreditMin) url += `creditMin=${encodeURIComponent(currentCreditMin)}&`;
  if (currentCreditMax) url += `creditMax=${encodeURIComponent(currentCreditMax)}&`;
  if (currentSort) url += `sort=${encodeURIComponent(currentSort)}&`;
  url += `_=${Date.now()}`;

  const r = await fetch(url);
  const d = await r.json();
  const total = d.total;
  subText.textContent = `从课程档案中找到 ${total} 门课`;
  const sortLabel = ({name:'名称',credit:'学分',college:'院系'}[currentSort] || '名称');
  listMeta.textContent = `共 ${total} 门 — 按${sortLabel}排序`;

  // 本地分页
  const items = d.courses;
  const pages = Math.ceil(items.length / PAGE_SIZE) || 1;
  if (currentPage >= pages) currentPage = pages - 1;
  const slice = items.slice(currentPage * PAGE_SIZE, (currentPage + 1) * PAGE_SIZE);

  courseList.innerHTML = '';
  if (slice.length === 0) {
    courseList.innerHTML = '<div class="empty"><span class="empty-title">没有找到匹配的课程</span><span class="empty-hint">换个关键词或放宽筛选条件再试试</span></div>';
    pager.innerHTML = '';
    return;
  }
  slice.forEach(c => {
    const card = document.createElement('a');
    card.className = 'course-card';
    card.href = `/course.html?code=${encodeURIComponent(c.code)}`;
    const college = c.college || '未知院系';
    card.innerHTML = `
      <div class="course-name">${escapeHtml(c.name)}</div>
      <div class="course-college">${escapeHtml(college)}</div>
      <div class="course-meta">
        ${c.tag ? `<span class="badge">${escapeHtml(c.tag)}</span>` : ''}
        ${c.exam ? `<span class="badge gray">${escapeHtml(c.exam)}</span>` : ''}
        ${c.nature ? `<span class="badge gray">${escapeHtml(c.nature)}</span>` : ''}
        ${c.credit ? `<span class="badge gray">${escapeHtml(c.credit)}学分</span>` : ''}
        <span class="badge gray">${c.code}</span>
      </div>
    `;
    courseList.appendChild(card);
  });

  // 分页：首页/上一页/页码窗口/下一页/末页 + 跳转输入框
  pager.innerHTML = '';
  if (pages > 1) {
    pager.appendChild(pageBtn('‹', 0, currentPage === 0 || pages === 0));
    pager.appendChild(pageBtn('‹‹', Math.max(0, currentPage - 1), currentPage === 0));
    // 页码窗口：当前页 ±2（含省略）
    const start = Math.max(0, currentPage - 2);
    const end = Math.min(pages - 1, currentPage + 2);
    if (start > 0) pager.appendChild(ellipsisBtn());
    for (let i = start; i <= end; i++) pager.appendChild(pageBtn(String(i + 1), i, i === currentPage));
    if (end < pages - 1) pager.appendChild(ellipsisBtn());
    pager.appendChild(pageBtn('›', Math.min(pages - 1, currentPage + 1), currentPage === pages - 1));
    pager.appendChild(pageBtn('››', pages - 1, currentPage === pages - 1));
    // 跳转输入框
    const input = document.createElement('input');
    input.type = 'number';
    input.min = 1;
    input.max = pages;
    input.value = String(currentPage + 1);
    input.className = 'page-input';
    input.title = `共 ${pages} 页`;
    input.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        let v = parseInt(input.value, 10);
        if (isNaN(v)) return;
        if (v < 1) v = 1; if (v > pages) v = pages;
        currentPage = v - 1; load();
      }
    });
    const go = document.createElement('button');
    go.textContent = '跳转';
    go.onclick = () => { let v = parseInt(input.value, 10); if (isNaN(v)) return; if (v < 1) v = 1; if (v > pages) v = pages; currentPage = v - 1; load(); };
    pager.appendChild(input);
    pager.appendChild(go);
    const total = document.createElement('span');
    total.className = 'page-total';
    total.textContent = `共 ${pages} 页`;
    pager.appendChild(total);
  }
}

function pageBtn(label, page, active) {
  const b = document.createElement('button');
  b.textContent = label;
  b.className = active ? 'active' : '';
  b.onclick = () => { currentPage = page; load(); };
  return b;
}
function ellipsisBtn() { const s = document.createElement('span'); s.textContent = '…'; s.className = 'page-ellipsis'; return s; }
function escapeHtml(s) {
  return (s||'').replace(/[&<>"']/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]));
}

// 事件
document.getElementById('searchBtn').onclick = () => { currentQ = qInput.value.trim(); currentPage = 0; load(); };
qInput.addEventListener('keydown', e => { if (e.key === 'Enter') { currentQ = qInput.value.trim(); currentPage = 0; load(); } });

// 多维筛选输入（失焦或回车生效）
const FILTER_TARGETS = {
  'collegeInput': 'currentCollege',
  'teacherInput': 'currentTeacher',
  'majorInput': 'currentMajor',
  'timeSelect': 'currentTime',
  'semSelect': 'currentSem',
  'examSelect': 'currentExam',
  'sortSelect': 'currentSort',
  'creditMin': 'currentCreditMin',
  'creditMax': 'currentCreditMax',
};
function bindFilter(id, key) {
  const el = document.getElementById(id);
  if (!el) return;
  const target = FILTER_TARGETS[id] || ('current' + cap(key));
  const apply = () => {
    currentPage = 0;
    window[target] = el.value.trim();
    load();
  };
  el.addEventListener('keydown', e => { if (e.key === 'Enter') apply(); });
  el.addEventListener('change', apply);
}
function cap(s) { return s.charAt(0).toUpperCase() + s.slice(1); }
bindFilter('collegeInput', 'college');
bindFilter('teacherInput', 'teacher');
bindFilter('majorInput', 'major');
bindFilter('timeSelect', 'time');
bindFilter('semSelect', 'sem');
bindFilter('examSelect', 'exam');
bindFilter('sortSelect', 'sort');
bindFilter('creditMin', 'creditMin');
bindFilter('creditMax', 'creditMax');

// 学期下拉（J4）
async function loadSemesters() {
  try {
    const d = await (await fetch('/api/semesters')).json();
    const sel = document.getElementById('semSelect');
    (d.semesters || []).forEach(s => {
      const o = document.createElement('option');
      o.value = s; o.textContent = s;
      sel.appendChild(o);
    });
  } catch (e) {}
}

// 清除筛选
document.getElementById('clearFilters').onclick = () => {
  currentCat=''; currentTeacher=''; currentCollege=''; currentMajor=''; currentTime=''; currentTag=''; currentQ='';
  currentSem=''; currentExam=''; currentCreditMin=''; currentCreditMax=''; currentSort='';
  qInput.value='';
  document.getElementById('collegeInput').value='';
  document.getElementById('teacherInput').value='';
  document.getElementById('majorInput').value='';
  document.getElementById('timeSelect').value='';
  document.getElementById('semSelect').value='';
  document.getElementById('examSelect').value='';
  document.getElementById('sortSelect').value='';
  document.getElementById('creditMin').value='';
  document.getElementById('creditMax').value='';
  renderCats(); renderChips();
  currentPage = 0; load();
};

loadSemesters();
loadTags();
loadCats();
courseList.innerHTML = '<div class="skeleton-list"><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div></div>';
load();
