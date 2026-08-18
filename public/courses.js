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
let currentSort = '';
let currentPage = 0;

// 新分类体系（必修/选修两级，v2：实验课入必修、体育课仅公共体育）
const CAT_GROUPS = [
  { name: '必修', children: ['通识教育必修课', '大类专业必修课', '专业基础课', '专业核心课', '集中实践', '实验课'] },
  { name: '选修', children: ['通识教育选修课/核心课', '通识教育选修课/普通课', '通识教育选修课/美育课', '体育课'] }
];

const qInput = document.getElementById('qInput');
const subText = document.getElementById('subText');
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

// 加载类别筛选（必修/选修 两级）
function loadCats() {
  const row = document.getElementById('catOpts');
  row.innerHTML = '';
  CAT_GROUPS.forEach(g => {
    const groupEl = document.createElement('div');
    groupEl.className = 'cat-group';
    const gBtn = document.createElement('button');
    gBtn.className = 'cat-opt cat-group-btn';
    gBtn.textContent = g.name;
    gBtn.onclick = () => { currentCat = (currentCat === g.name ? '' : g.name); renderCats(); load(); };
    groupEl.appendChild(gBtn);
    const subRow = document.createElement('div');
    subRow.className = 'cat-subrow';
    g.children.forEach(sub => {
      const full = g.name + '/' + sub;
      const chip = document.createElement('button');
      chip.className = 'cat-opt cat-sub';
      chip.dataset.full = full;
      chip.textContent = sub.split('/').pop();
      chip.onclick = () => { currentCat = (currentCat === full ? '' : full); renderCats(); load(); };
      subRow.appendChild(chip);
    });
    groupEl.appendChild(subRow);
    row.appendChild(groupEl);
  });
}
function renderCats() {
  [...document.querySelectorAll('.cat-opt')].forEach(ch => {
    const full = ch.dataset.full || ch.textContent;
    ch.classList.toggle('active', currentCat === full);
  });
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
  if (currentSort) url += `sort=${encodeURIComponent(currentSort)}&`;
  url += `_=${Date.now()}`;

  const r = await fetch(url);
  const d = await r.json();
  const total = d.total;
  subText.textContent = `从课程资料中找到 ${total} 门课`;
  const sortLabel = ({name:'名称',files:'资料数量',hot:'热度',credit:'学分',college:'院系'}[currentSort] || '名称');
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
};
function bindFilter(id, key) {
  const el = document.getElementById(id);
  if (!el) return;
  const apply = () => {
    currentPage = 0;
    const v = el.value.trim();
    // 直接给模块级变量赋值（不能用 window[target]，let 变量不在 window 上）
    switch (id) {
      case 'collegeInput': currentCollege = v; break;
      case 'teacherInput': currentTeacher = v; break;
      case 'majorInput': currentMajor = v; break;
      case 'timeSelect': currentTime = v; break;
      case 'semSelect': currentSem = v; break;
      case 'examSelect': currentExam = v; break;
      case 'sortSelect': currentSort = v; break;
      default: window['current' + cap(key)] = v;
    }
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
  currentCat=''; currentTeacher=''; currentCollege=''; currentMajor=''; currentTime=''; currentQ='';
  currentSem=''; currentExam=''; currentSort='';
  qInput.value='';
  document.getElementById('collegeInput').value='';
  document.getElementById('teacherInput').value='';
  document.getElementById('majorInput').value='';
  document.getElementById('timeSelect').value='';
  document.getElementById('semSelect').value='';
  document.getElementById('examSelect').value='';
  document.getElementById('sortSelect').value='';
  renderCats();
  currentPage = 0; load();
};

loadSemesters();
loadCats();
courseList.innerHTML = '<div class="skeleton-list"><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div><div class="skeleton-card"></div></div>';
load();
