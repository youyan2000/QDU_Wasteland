// me.js — 个人主页逻辑（V10：我的内容聚合）
async function loadMe() {
  const wrap = document.getElementById('meContent');
  try {
    const r = await fetch('/api/me');
    const d = await r.json();
    if (!d.loggedIn) {
      wrap.innerHTML = `
        <div class="not-logged">
          <p style="font-size:1.3rem;font-weight:700;margin-bottom:.6rem">还没有登录</p>
          <p style="color:var(--muted);margin-bottom:1rem">登录后可以管理你的个人主页</p>
          <a href="/login.html">去登录 / 注册</a>
        </div>`;
      return;
    }
    const u = d.user;
    const avatar = (u.nickname && u.nickname[0]) || '?';
    const avatarHtml = u.avatar
      ? `<div class="avatar"><img src="${u.avatar}" alt="头像"></div>`
      : `<div class="avatar">${escapeHtml(avatar)}</div>`;
    const adminLink = (u.isAdmin === 1)
      ? `<a href="/eugene.html" class="me-btn" style="display:inline-block;margin-top:.8rem;text-decoration:none">${ICONS.settings(15)} 管理后台</a>`
      : '';
    wrap.innerHTML = `
      <div class="profile-card">
        <div class="profile-head">
          ${avatarHtml}
          <div style="flex:1">
            <div class="profile-name">${escapeHtml(u.nickname)}</div>
            <div class="profile-email">${escapeHtml(u.email)}</div>
          </div>
          <button id="logoutBtn" class="me-btn">退出登录</button>
        </div>
        <div class="info-grid">
          <div class="info-item"><div class="label">学院</div><div class="value">${escapeHtml(u.college || '—')}</div></div>
          <div class="info-item"><div class="label">专业</div><div class="value">${escapeHtml(u.major || '—')}</div></div>
          <div class="info-item"><div class="label">性别</div><div class="value">${escapeHtml(u.gender || '—')}</div></div>
          <div class="info-item"><div class="label">年龄</div><div class="value">${u.age ? u.age : '—'}</div></div>
          <div class="info-item"><div class="label">生日</div><div class="value">${escapeHtml(u.birthday || '—')}</div></div>
          <div class="info-item"><div class="label">届别</div><div class="value">${escapeHtml(u.grade || '—')}</div></div>
          <div class="info-item"><div class="label">籍贯</div><div class="value">${escapeHtml(u.nativePlace || '—')}</div></div>
          <div class="info-item"><div class="label">微信</div><div class="value">${escapeHtml(u.wechat || '—')}</div></div>
          <div class="info-item"><div class="label">QQ</div><div class="value">${escapeHtml(u.qq || '—')}</div></div>
          <div class="info-item"><div class="label">电话</div><div class="value">${escapeHtml(u.phone || '—')}</div></div>
          <div class="info-item"><div class="label">其他社媒</div><div class="value">${escapeHtml(u.social || '—')}</div></div>
          <div class="info-item"><div class="label">等级</div><div class="value">${u.level ? 'Lv.'+u.level+' · '+escapeHtml(u.levelTitle || '') : '—'}</div></div>
          <div class="info-item"><div class="label">注册时间</div><div class="value">${escapeHtml(u.createdAt || '—')}</div></div>
        </div>
        <div class="bio-line"><span class="bio-label">个性签名</span>${escapeHtml(u.bio || '这个人很懒，什么都没写')}</div>
        <div style="display:flex;gap:10px;margin-top:1rem;flex-wrap:wrap">
          <button id="editProfileBtn" class="me-btn">${ICONS.edit(15)} 编辑资料</button>
          <button id="changePwdBtn" class="me-btn">${ICONS.settings(15)} 修改密码</button>
          <button id="privacyBtn" class="me-btn">${ICONS.lock(15)} 对外展示</button>
          <button id="blockMgrBtn" class="me-btn">${ICONS.ban(15)} 黑名单管理</button>
          ${adminLink}
        </div>
        ${u.emailVerified === 1
          ? `<div class="verify-ok"><span>${ICONS.check(15)} 邮箱已验证</span></div>`
          : `<div class="verify-box">
              <div><strong>邮箱未验证</strong><span class="verify-hint">发布内容达到验证节点时需先验证邮箱</span></div>
              <div class="verify-actions">
                <input id="verifyCode" type="text" placeholder="6位验证码" maxlength="6">
                <button id="verifySend" class="me-btn">发送验证码</button>
                <button id="verifyConfirm" class="me-btn primary">确认</button>
              </div>
              <div id="verifyMsg" class="verify-msg"></div>
            </div>`}
      </div>
      <div id="notifWrap"><div class="me-loading">加载中…</div></div>
      <div id="msgWrap"></div>
      <div id="activityWrap"><div class="me-loading">加载中…</div></div>`;
    loadActivity();
    loadNotifications();
    loadMessages();
    bindVerify();
    bindEditProfile(u);
    bindChangePwd();
    bindPrivacy(u);
    bindBlockManager(u);
  } catch (e) {
    wrap.innerHTML = '<div class="me-loading">加载失败</div>';
  }
}

// D1 通知中心
async function loadNotifications() {
  const wrap = document.getElementById('notifWrap');
  try {
    const r = await fetch('/api/notifications');
    if (r.status === 401) { wrap.innerHTML = ''; return; }
    const d = await r.json();
    const list = d.notifications || [];
    const icon = d.unread ? `<span class="notif-badge">${d.unread}</span>` : '';
    const items = list.length === 0
      ? '<div class="placeholder">暂无通知</div>'
      : list.map(n => `
        <div class="notif-item ${n.isRead ? '' : 'unread'}">
          <span class="notif-text">${escapeHtml(n.actor || '有人')} ${escapeHtml(n.text)}${n.refId ? ` <a href="/post.html?id=${n.refId}">查看</a>` : ''}</span>
          <span class="notif-time">${escapeHtml(n.createdAt)}</span>
        </div>`).join('');
    wrap.innerHTML = `
      <div class="notif-card">
        <div class="notif-head">
          <h2 style="font-size:1.1rem;font-weight:800">通知 ${icon}</h2>
          ${list.length ? '<button class="me-btn mini" id="notifReadBtn">全部已读</button>' : ''}
        </div>
        <div class="notif-list">${items}</div>
      </div>`;
    const rb = document.getElementById('notifReadBtn');
    if (rb) rb.onclick = async () => {
      await fetch('/api/notifications/read', { method: 'POST' });
      loadNotifications();
    };
  } catch (e) { wrap.innerHTML = ''; }
}

// D2 私信收件箱
async function loadMessages() {
  const wrap = document.getElementById('msgWrap');
  try {
    const r = await fetch('/api/messages');
    if (r.status === 401) { wrap.innerHTML = ''; return; }
    const d = await r.json();
    const list = d.messages || [];
    if (list.length === 0) { wrap.innerHTML = ''; return; }
    const items = list.map(m => `
      <div class="notif-item ${m.isRead ? '' : 'unread'}">
        <span class="notif-text"><b>${escapeHtml(m.fromName)}</b>：${escapeHtml(m.content)}</span>
        <span class="notif-time">${escapeHtml(m.createdAt)}</span>
      </div>`).join('');
    wrap.innerHTML = `
      <div class="notif-card">
        <div class="notif-head">
          <h2 style="font-size:1.1rem;font-weight:800">私信 ${d.unread ? `<span class="notif-badge">${d.unread}</span>` : ''}</h2>
          <button class="me-btn mini" id="msgReadBtn">全部已读</button>
        </div>
        <div class="notif-list">${items}</div>
      </div>`;
    document.getElementById('msgReadBtn').onclick = async () => {
      await fetch('/api/messages/read', { method: 'POST' });
      loadMessages();
    };
  } catch (e) { wrap.innerHTML = ''; }
}

async function loadActivity() {
  const wrap = document.getElementById('activityWrap');
  try {
    const r = await fetch('/api/me/activity');
    if (r.status === 401) { wrap.innerHTML = '<div class="placeholder">请先登录</div>'; return; }
    const a = await r.json();
    renderActivity(wrap, a);
  } catch (e) { wrap.innerHTML = '<div class="placeholder">内容加载失败</div>'; }
}

function renderActivity(wrap, a) {
  const cs = a.counts || {};
  const statCards = [
    ['posts','帖子', cs.posts || 0], ['reviews','评价', cs.reviews || 0],
    ['files','资料', cs.files || 0], ['articles','文章', cs.articles || 0], ['drafts','草稿', cs.drafts || 0],
    ['favorites','收藏', cs.favorites || 0], ['albums','相册', cs.albums || 0]
  ].map(([tab, label, num]) =>
    `<button class="stat-card clickable" data-tab="${tab}"><div class="stat-num">${num}</div><div class="stat-label">我的${label}</div></button>`
  ).join('');

  const posts = (a.posts || []).map(p => `
    <div class="act-item">
      <span class="act-badge badge-post">帖</span>
      <a class="act-main" href="/post.html?id=${p.id}" target="_blank">${escapeHtml(p.title)}${p.anonymous ? ' <em title="匿名发布">(匿名)</em>' : ''}</a>
      <span class="act-sub">${escapeHtml(p.forum)} · ${escapeHtml(p.createdAt)}</span>
      <span class="act-stat">${ICONS.thumb(13)} ${p.likes}</span>
      <span class="act-actions">
        <button class="mgmt-btn" data-mgmt-withdraw type="post" data-id="${p.id}">撤回</button>
      </span>
    </div>`).join('');

  const reviews = (a.reviews || []).map(re => `
    <a class="act-item" href="/course.html?code=${encodeURIComponent(re.courseCode)}">
      <span class="act-badge badge-review">评</span>
      <span class="act-main">${'★'.repeat(Math.max(0,Math.min(5,re.rating||0)))} ${escapeHtml(re.content)}</span>
      <span class="act-sub">课程 ${escapeHtml(re.courseCode)} · ${escapeHtml(re.createdAt)}</span>
      <span class="act-stat ${re.status==='已下架'?'down':''}">${re.status==='已下架'?'已下架':''}</span>
    </a>`).join('');

  const files = (a.files || []).map(f => `
    <a class="act-item" href="/viewer.html?id=${f.id}&name=${encodeURIComponent(f.fileName||'')}">
      <span class="act-badge badge-file">资</span>
      <span class="act-main">${escapeHtml(f.title || f.fileName)}</span>
      <span class="act-sub">课程 ${escapeHtml(f.courseCode)} · ${escapeHtml(f.createdAt)}</span>
      <span class="act-stat ${f.status==='已下架'?'down':''}">${f.status==='已下架'?'已下架':''}</span>
    </a>`).join('');

  const postsStat = (art) => {
    if (art.status === '待审') return '<span class="act-stat" style="color:#b26a00">待审中</span>';
    if (art.status === '已驳回') return `<span class="act-stat" style="color:#c0392b" title="${escapeHtml(art.rejectReason || '')}">已驳回${art.rejectReason ? ' ⓘ' : ''}</span>`;
    if (art.status === '已下架') return '<span class="act-stat down">已下架</span>';
    return '';
  };
  const articles = (a.articles || []).map(art => `
    <div class="act-item">
      <span class="act-badge badge-article">文</span>
      <a class="act-main" href="/article.html?id=${art.id}" target="_blank">${escapeHtml(art.title)}</a>
      <span class="act-sub">${escapeHtml(art.category)} · ${escapeHtml(art.createdAt)}</span>
      ${postsStat(art)}
      <span class="act-actions">
        <button class="mgmt-btn" data-mgmt-withdraw type="article" data-id="${art.id}">撤回</button>
      </span>
    </div>`).join('');

  const drafts = (a.drafts || []).map(d => {
    const t = d.type === 'post';
    const href = t ? '/post.html?id=' + d.id : '/article.html?id=' + d.id;
    const badge = t ? 'badge-post' : 'badge-article';
    const tag = t ? '帖' : '文';
    return `<div class="act-item">
      <span class="act-badge ${badge}">${tag}</span>
      <a class="act-main" href="${href}" target="_blank">${escapeHtml(d.title)}</a>
      <span class="act-sub">${escapeHtml(d.category === 'post' ? '帖子草稿' : d.category)} · ${escapeHtml(d.createdAt)}</span>
      <span class="act-actions">
        <button class="mgmt-btn primary" data-draft-edit type="${t ? 'post' : 'article'}" data-id="${d.id}">编辑</button>
        <button class="mgmt-btn primary" data-draft-pub type="${t ? 'post' : 'article'}" data-id="${d.id}">发布</button>
        <button class="mgmt-del" data-draft-del type="${t ? 'post' : 'article'}" data-id="${d.id}">删除</button>
      </span>
    </div>`;
  }).join('');

  const empty = '<div class="placeholder">还没有内容</div>';
  const newDraftBtns = `<div style="display:flex;gap:10px;margin-bottom:12px;flex-wrap:wrap">
      <button class="mgmt-btn primary" id="newPostDraft">＋ 新建帖子草稿</button>
      <button class="mgmt-btn primary" id="newArticleDraft">＋ 新建文章草稿</button>
    </div>`;
  const bodyHtml = {
    posts: `<div class="act-list">${posts || empty}</div>`,
    reviews: `<div class="act-list">${reviews || empty}</div>`,
    files: `<div class="act-list">${files || empty}</div>`,
    articles: `<div class="act-list">${articles || empty}</div>`,
    drafts: `${newDraftBtns}<div class="act-list">${drafts || '<div class="placeholder">还没有草稿</div>'}</div>`,
    favorites: `<div class="act-list" id="favList"><div class="placeholder">加载中…</div></div>`,
    albums: `<div class="act-list" id="albList"><div class="placeholder">加载中…</div></div>`
  };
  wrap.innerHTML = `
    <div class="stat-grid">${statCards}</div>
    <div id="tabBody" class="my-tab-body">${bodyHtml.posts}</div>`;

  // 方块即 Tab：点击方块切换下方内容
  wrap.querySelectorAll('.stat-card.clickable').forEach(c => {
    c.classList.add('active');
    c.onclick = () => {
      wrap.querySelectorAll('.stat-card.clickable').forEach(x => x.classList.remove('active'));
      c.classList.add('active');
      const tab = c.dataset.tab;
      wrap.querySelector('#tabBody').innerHTML = bodyHtml[tab] || bodyHtml.posts;
      if (tab === 'favorites') loadFavoritesTab();
      if (tab === 'albums') loadAlbumsTab();
    };
  });
  // 切到指定 tab 的辅助（供撤回后自动跳草稿）
  window.switchMyTab = function (tab) {
    const c = wrap.querySelector('.stat-card.clickable[data-tab="' + tab + '"]');
    if (c) c.click();
  };
}

// V14 编辑资料弹层（J5：籍贯改省市下拉）
let regionData = { provinces: [], cities: {} };
async function loadRegions() {
  try {
    const r = await (await fetch('/api/regions/provinces')).json();
    regionData.provinces = r.provinces || [];
  } catch (e) {}
}
function fillProvinceSelect() {
  const p = document.getElementById('epProv');
  if (!p) return;
  p.innerHTML = '<option value="">选择省份</option>';
  (regionData.provinces || []).forEach(pr => {
    const o = document.createElement('option');
    o.value = pr; o.textContent = pr;
    p.appendChild(o);
  });
}
async function loadCities(prov) {
  const c = document.getElementById('epCity');
  if (!c) return;
  c.innerHTML = '<option value="">市/区县</option>';
  if (!prov) { c.innerHTML = '<option value="">先选省份</option>'; return; }
  try {
    const r = await (await fetch('/api/regions/cities?province=' + encodeURIComponent(prov))).json();
    (r.cities || []).forEach(ct => {
      const o = document.createElement('option');
      o.value = ct; o.textContent = ct;
      c.appendChild(o);
    });
  } catch (e) {}
}
// 学院/专业下拉（编辑资料用，J2）
async function fillEpCollege() {
  const sel = document.getElementById('epCollege');
  if (!sel) return;
  sel.innerHTML = '<option value="">选择学院</option>';
  try {
    const d = await (await fetch('/api/org')).json();
    (d.colleges || []).forEach(c => {
      const o = document.createElement('option');
      o.value = c; o.textContent = c;
      sel.appendChild(o);
    });
  } catch (e) {}
}
async function fillEpMajor(college) {
  const sel = document.getElementById('epMajor');
  if (!sel) return;
  sel.innerHTML = '<option value="">选择专业</option>';
  if (!college) { sel.innerHTML = '<option value="">先选学院</option>'; return; }
  try {
    const d = await (await fetch('/api/org')).json();
    const list = (d.majors && d.majors[college]) || [];
    list.forEach(m => {
      const o = document.createElement('option');
      o.value = m; o.textContent = m;
      sel.appendChild(o);
    });
  } catch (e) {}
}
// 修改密码（登录状态下）
function bindChangePwd() {
  const btn = document.getElementById('changePwdBtn');
  if (!btn) return;
  const modal = document.getElementById('changePwdModal');
  if (!modal) return;
  btn.onclick = () => {
    document.getElementById('cpOld').value = '';
    document.getElementById('cpNew').value = '';
    document.getElementById('cpNew2').value = '';
    document.getElementById('cpErr').textContent = '';
    modal.style.display = 'flex';
  };
  document.getElementById('cpClose').onclick = () => modal.style.display = 'none';
  modal.addEventListener('click', e => { if (e.target === modal) modal.style.display = 'none'; });
  document.getElementById('cpSave').onclick = async () => {
    const err = document.getElementById('cpErr');
    const oldP = document.getElementById('cpOld').value;
    const newP = document.getElementById('cpNew').value;
    const newP2 = document.getElementById('cpNew2').value;
    if (!oldP) { err.textContent = '请输入旧密码'; return; }
    if (newP.length < 6) { err.textContent = '新密码至少 6 位'; return; }
    if (newP !== newP2) { err.textContent = '两次新密码不一致'; return; }
    try {
      const r = await fetch('/api/me/password', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ oldPassword: oldP, newPassword: newP })
      });
      const d = await r.json();
      if (!r.ok) { err.textContent = d.error || '修改失败'; return; }
      alert('密码已修改，请用新密码重新登录');
      modal.style.display = 'none';
      await fetch('/api/logout', { method: 'POST' });
      location.href = '/login.html';
    } catch (e) { err.textContent = '网络错误'; }
  };
}
function bindEditProfile(u) {
  const btn = document.getElementById('editProfileBtn');
  if (!btn) return;
  const modal = document.getElementById('editProfileModal');
  const provSel = document.getElementById('epProv');
  const citySel = document.getElementById('epCity');
  let curProv = '', curCity = '';
  // 解析已有籍贯 "省 市"
  if (u.nativePlace) {
    const parts = (u.nativePlace || '').trim().split(/\s+/);
    if (parts.length >= 2) { curProv = parts[0]; curCity = parts[1]; }
  }
  btn.onclick = async () => {
    document.querySelectorAll('input[name="epGender"]').forEach(r => { r.checked = r.value === (u.gender || ''); });
    document.getElementById('epNickname').value = u.nickname || '';
    document.getElementById('epAge').value = u.age || '';
    document.getElementById('epBio').value = u.bio || '';
    document.getElementById('epBirthday').value = u.birthday || '';
    document.getElementById('epGrade').value = u.grade || '';
    document.getElementById('epWechat').value = u.wechat || '';
    document.getElementById('epQQ').value = u.qq || '';
    document.getElementById('epPhone').value = u.phone || '';
    document.getElementById('epSocial').value = u.social || '';
    document.getElementById('epErr').textContent = '';
    // 头像预览回填
    const avImg = document.getElementById('epAvatarPreview');
    if (avImg) {
      if (u.avatar) { avImg.src = u.avatar; avImg.style.display = 'block'; }
      else { avImg.style.display = 'none'; }
    }
    const avFile = document.getElementById('epAvatarFile');
    if (avFile) avFile.value = '';
    // 学院/专业回填（J2 联动）
    const epColSel2 = document.getElementById('epCollege');
    const epMajSel2 = document.getElementById('epMajor');
    if (epColSel2 && epMajSel2) {
      await fillEpCollege();
      epColSel2.value = u.college || '';
      await fillEpMajor(epColSel2.value);
      epMajSel2.value = u.major || '';
    }
    // 填省份
    if (regionData.provinces.length === 0) await loadRegions();
    fillProvinceSelect();
    // 选中当前省
    if (curProv) { provSel.value = curProv; }
    // 载入城市并选中当前市
    await loadCities(provSel.value);
    if (curCity) { citySel.value = curCity; }
    modal.style.display = 'flex';
  };
  const epColSel = document.getElementById('epCollege');
  const epMajSel = document.getElementById('epMajor');
  if (epColSel) epColSel.onchange = async () => { await fillEpMajor(epColSel.value); };
  provSel.onchange = async () => {
    citySel.value = '';
    await loadCities(provSel.value);
  };
  // 头像上传（选择文件即上传，成功后预览 + 更新 u.avatar）
  const avFile = document.getElementById('epAvatarFile');
  if (avFile) {
    avFile.onchange = async () => {
      const errEl = document.getElementById('epErr');
      if (!avFile.files || !avFile.files.length) return;
      const fd = new FormData();
      fd.append('file', avFile.files[0]);
      errEl.textContent = '上传中…';
      try {
        const r = await fetch('/api/avatar/upload', { method: 'POST', body: fd });
        const d = await r.json();
        if (!r.ok) { errEl.textContent = d.error || '上传失败'; return; }
        u.avatar = d.avatar;
        const avImg = document.getElementById('epAvatarPreview');
        if (avImg) { avImg.src = d.avatar; avImg.style.display = 'block'; }
        errEl.textContent = '✅ 头像已更新';
      } catch (e) { errEl.textContent = '网络错误，上传失败'; }
    };
  }
  document.getElementById('epClose').onclick = () => modal.style.display = 'none';
  modal.addEventListener('click', e => { if (e.target === modal) modal.style.display = 'none'; });
  document.getElementById('epSave').onclick = async () => {
    const errEl = document.getElementById('epErr');
    const gen = (document.querySelector('input[name="epGender"]:checked') || {}).value || '';
    const ageStr = document.getElementById('epAge').value.trim();
    const prov = provSel.value.trim();
    const city = citySel.value.trim();
    const native = (prov && city) ? (prov + ' ' + city) : '';
    const bio = document.getElementById('epBio').value.trim();
    let age = 0;
    if (ageStr !== '') {
      if (!/^\d{1,3}$/.test(ageStr)) { errEl.textContent = '年龄需为合法的数字'; return; }
      age = parseInt(ageStr, 10);
      if (age < 1 || age > 100) { errEl.textContent = '年龄需在 1~100 之间'; return; }
    }
    if (bio.length > 20) { errEl.textContent = '个性签名最多 20 字'; return; }
    const cleanBio = bio.replace(/[<>"'\/`]/g, '');
    // 昵称校验（2~20 字，可选；空 = 不修改）
    const nickVal = document.getElementById('epNickname').value.trim();
    if (nickVal && (nickVal.length < 2 || nickVal.length > 20)) { errEl.textContent = '昵称需 2~20 个字符'; return; }
    // 联系方式格式校验（非空才校验）
    const qqVal = document.getElementById('epQQ').value.trim();
    const phoneVal = document.getElementById('epPhone').value.trim();
    const gradeVal = document.getElementById('epGrade').value.trim();
    if (qqVal && (!/^\d{5,12}$/.test(qqVal))) { errEl.textContent = 'QQ 号应为 5~12 位数字'; return; }
    if (phoneVal && (!/^\d{7,15}$/.test(phoneVal))) { errEl.textContent = '电话号码应为 7~15 位数字'; return; }
    if (gradeVal && !/^\d{4}级$/.test(gradeVal)) { errEl.textContent = '届别格式应为如 2023级'; return; }
    // 学院/专业：若下拉未加载/未选择（值为空），保留原值，避免误清空
    const epCol = document.getElementById('epCollege');
    const epMaj = document.getElementById('epMajor');
    const collegeVal = (epCol && epCol.value) ? epCol.value : (u.college || '');
    const majorVal = (epMaj && epMaj.value) ? epMaj.value : (u.major || '');
    const r = await fetch('/api/me/profile', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        nickname: nickVal,
        gender: gen, age, birthday: document.getElementById('epBirthday').value.trim(),
        grade: gradeVal,
        nativePlace: native,
        wechat: document.getElementById('epWechat').value.trim(),
        qq: qqVal,
        phone: phoneVal,
        social: document.getElementById('epSocial').value.trim(),
        bio: cleanBio, college: collegeVal, major: majorVal
      })
    });
    const d = await r.json();
    if (!r.ok) { errEl.textContent = d.error || '保存失败'; return; }
    modal.style.display = 'none';
    location.reload();
  };
}

// B2 邮箱验证交互
function bindVerify() {
  const sendBtn = document.getElementById('verifySend');
  const confirmBtn = document.getElementById('verifyConfirm');
  if (!sendBtn || !confirmBtn) return;
  sendBtn.onclick = async () => {
    const msg = document.getElementById('verifyMsg');
    msg.textContent = '发送中…';
    const r = await fetch('/api/email/verify', { method: 'POST' });
    const d = await r.json();
    msg.textContent = d.error || d.message || '已发送';
  };
  confirmBtn.onclick = async () => {
    const msg = document.getElementById('verifyMsg');
    const code = document.getElementById('verifyCode').value.trim();
    if (!code) { msg.textContent = '请输入验证码'; return; }
    const r = await fetch('/api/email/verify/confirm', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code })
    });
    const d = await r.json();
    msg.textContent = d.error || d.message || '验证成功';
    if (d.verified) { setTimeout(() => location.reload(), 800); }
  };
}

// 重新加载稿件列表并切到指定 tab（不整页刷新）
async function reloadActivityShow(tab) {
  await loadActivity();
  if (window.switchMyTab) window.switchMyTab(tab);
}

// 草稿箱：发布/删除（操作后重载并切到草稿箱）
async function draftAction(type, id, act) {
  const url = type === 'post'
    ? (act === 'del' ? '/api/forum/post/delete' : '/api/forum/post/update')
    : (act === 'del' ? '/api/articles/delete' : '/api/articles/update');
  const body = type === 'post'
    ? { id, status: '正常', title: '', content: '' }
    : { id, status: '正常', title: '', category: '', content: '' };
  if (act === 'del') {
    if (!confirm('确定删除这条草稿？将从草稿箱移除。')) return;
  } else {
    if (!confirm('确定发布这条草稿？')) return;
  }
  const r = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  });
  const d = await r.json();
  if (!r.ok) { alert(d.error || '操作失败'); return; }
  await reloadActivityShow('drafts');
}

// 已发布稿件"撤回" → 回草稿箱（撤回后自动切到草稿箱 tab）
async function mgmtWithdraw(type, id) {
  if (!confirm('确定撤回该内容？将收回草稿箱。')) return;
  const url = type === 'post' ? '/api/forum/post/delete' : '/api/articles/delete';
  await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id }) });
  await reloadActivityShow('drafts');
}
// 草稿"编辑" → 新标签打开编辑器/详情（保持本页稿件管理）
function draftEdit(type, id) {
  const url = type === 'post' ? '/post.html?id=' + id : '/article-editor.html?edit=' + id;
  window.open(url, '_blank');
}


// J6 收藏 / 相册 Tab 加载
async function loadFavoritesTab() {
  const box = document.getElementById('favList');
  if (!box) return;
  try {
    const d = await (await fetch('/api/favorites')).json();
    const favs = d.favorites || [];
    if (favs.length === 0) { box.innerHTML = '<div class="placeholder">还没有收藏</div>'; return; }
    const rows = favs.map(f => {
      let href='#', label='';
      if (f.targetType === 'post') { href='/post.html?id='+f.targetId; label='帖子 #'+f.targetId; }
      else if (f.targetType === 'article') { href='/article.html?id='+f.targetId; label='文章 #'+f.targetId; }
      else if (f.targetType === 'file') { href='/viewer.html?id='+f.targetId; label='资料 #'+f.targetId; }
      else if (f.targetType === 'course') { href='/course.html?code='+encodeURIComponent(f.targetId); label='课程'; }
      else { href='#'; label=f.targetId; }
      return `<div class="act-item">
        <span class="act-badge badge-file">藏</span>
        <a class="act-main" href="${href}" target="_blank">${escapeHtml(label)}</a>
        <span class="act-sub">${escapeHtml(f.targetType)} · ${escapeHtml(f.createdAt)}</span>
        <span class="act-actions"><button class="mgmt-btn" data-unfav type="${escapeHtml(f.targetType)}" data-id="${f.targetId}">取消收藏</button></span>
      </div>`;
    }).join('');
    box.innerHTML = `<div class="act-list">${rows}</div>`;
    box.querySelectorAll('[data-unfav]').forEach(b => {
      b.onclick = async () => {
        await fetch('/api/favorite/toggle', { method:'POST', headers:{'Content-Type':'application/json'},
          body: JSON.stringify({ targetType: b.dataset.type, targetId: b.dataset.id }) });
        loadActivity();
        const c = wrap2ActiveTab();
        if (c) c.click();
      };
    });
  } catch(e) { box.innerHTML = '<div class="placeholder">加载失败</div>'; }
}
function wrap2ActiveTab() {
  return document.querySelector('.stat-card.clickable.active[data-tab="favorites"]');
}
async function loadAlbumsTab() {
  const box = document.getElementById('albList');
  if (!box) return;
  try {
    const d = await (await fetch('/api/albums?mine=1')).json();
    const albs = d.albums || [];
    if (albs.length === 0) { box.innerHTML = '<div class="placeholder">还没有相册 <button class="mgmt-btn" id="mkAlbumTop">新建相册</button></div>'; bindMkAlbum(); return; }
    box.innerHTML = `<div class="act-list">${
      albs.map(al => `<div class="act-item">
        <span class="act-badge badge-article">册</span>
        <a class="act-main" href="#" data-album-id="${al.id}">${escapeHtml(al.title)}</a>
        <span class="act-sub">${al.isPrivate ? `${ICONS.lock(13)} 私密` : '公开'} · ${al.itemsCount || 0}项</span>
        <span class="act-actions"><button class="mgmt-btn" data-del-album="${al.id}">删除</button></span>
      </div>`).join('')
    }<button class="mgmt-btn" id="mkAlbumBottom" style="margin-top:10px">+ 新建相册</button></div>`;
    [...box.querySelectorAll('[data-album-id]')].forEach(b => { b.onclick = e => { e.preventDefault(); location.href = '/album.html?id=' + b.dataset.albumId; }; });
    [...box.querySelectorAll('[data-del-album]')].forEach(b => {
      b.onclick = async () => {
        if (!confirm('确定删除该相册？')) return;
        await fetch('/api/albums/delete', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ id: parseInt(b.dataset.delAlbum,10) }) });
        loadActivity(); const a = wrap2ActiveTab(); if (a) a.click();
      };
    });
    const mb = document.getElementById('mkAlbumBottom'); if (mb) mb.onclick = () => bindMkAlbum();
  } catch(e) { box.innerHTML = '<div class="placeholder">加载失败</div>'; }
}
function bindMkAlbum() {
  // 弹层表单：标题 + 描述 + 私密开关（替代 prompt）
  const overlay = document.createElement('div');
  overlay.className = 'modal';
  overlay.style.display = 'flex';
  overlay.innerHTML = `
    <div class="modal-box" style="width:min(420px,92vw)">
      <div class="modal-head">
        <h3>新建相册</h3>
        <button class="modal-close" id="mkAlbClose"><i class="icon-fill" data-icon="close"></i></button>
      </div>
      <div class="pf-row">
        <label>相册名称</label>
        <input id="mkAlbTitle" type="text" placeholder="如：校园风景 / 小猫图鉴" maxlength="30" style="width:100%;padding:.65rem .8rem;border:1px solid var(--border-strong);border-radius:8px;font-size:.95rem;box-sizing:border-box">
      </div>
      <div class="pf-row">
        <label>描述（选填）</label>
        <input id="mkAlbDesc" type="text" placeholder="一句话介绍这个相册" maxlength="60" style="width:100%;padding:.65rem .8rem;border:1px solid var(--border-strong);border-radius:8px;font-size:.95rem;box-sizing:border-box">
      </div>
      <div class="pf-row" style="display:flex;align-items:center;gap:8px">
        <input type="checkbox" id="mkAlbPrivate" style="width:auto">
        <label for="mkAlbPrivate" style="margin:0">私密相册（仅自己可见）</label>
      </div>
      <div class="err" id="mkAlbErr" style="color:#d32f2f;min-height:1em;font-size:.85rem"></div>
      <div class="pf-actions">
        <button id="mkAlbSave" class="btn-submit" style="background:linear-gradient(90deg,var(--magenta),var(--sunrise));border:none;color:#fff;padding:.7rem 1.8rem;border-radius:10px;font-weight:700;cursor:pointer">创建相册</button>
      </div>
    </div>`;
  document.body.appendChild(overlay);
  overlay.querySelector('#mkAlbClose').onclick = () => overlay.remove();
  overlay.addEventListener('click', e => { if (e.target === overlay) overlay.remove(); });
  overlay.querySelector('#mkAlbSave').onclick = async () => {
    const title = overlay.querySelector('#mkAlbTitle').value.trim();
    const desc = overlay.querySelector('#mkAlbDesc').value.trim();
    const isPrivate = overlay.querySelector('#mkAlbPrivate').checked ? 1 : 0;
    const err = overlay.querySelector('#mkAlbErr');
    if (!title) { err.textContent = '请填写相册名称'; return; }
    try {
      const r = await fetch('/api/albums/create', { method:'POST', headers:{'Content-Type':'application/json'},
        body: JSON.stringify({ title, description: desc, isPrivate }) });
      const d = await r.json();
      if (!r.ok) { err.textContent = d.error || '创建失败'; return; }
      overlay.remove();
      loadActivity(); const c = wrap2ActiveTab(); if (c) c.click();
    } catch (e) { err.textContent = '网络错误'; }
  };
}

// 退出按钮在用户数据渲染后存在，用全局代理处理
document.addEventListener('click', function (e) {
  if (e.target && e.target.id === 'newPostDraft') {
    // 新建帖子草稿：跳转帖子创建页（论坛发帖弹层，先进入论坛）
    location.href = '/forum.html?newdraft=1';
    return;
  }
  if (e.target && e.target.id === 'newArticleDraft') {
    // 新建文章草稿：打开文章编辑器，自动存为草稿
    location.href = '/article-editor.html?newdraft=1';
    return;
  }
  const withdrawBtn = e.target.closest('[data-mgmt-withdraw]');
  if (withdrawBtn) { mgmtWithdraw(withdrawBtn.getAttribute('type') || 'post', withdrawBtn.dataset.id); return; }
  const editBtn = e.target.closest('[data-draft-edit]');
  if (editBtn) { draftEdit(editBtn.getAttribute('type') || 'article', editBtn.dataset.id); return; }
  const delBtn = e.target.closest('[data-draft-del]');
  if (delBtn) { draftAction(delBtn.dataset.type || delBtn.getAttribute('type'), delBtn.dataset.id, 'del'); return; }
  const pubBtn = e.target.closest('[data-draft-pub]');
  if (pubBtn) { draftAction(pubBtn.dataset.type || pubBtn.getAttribute('type'), pubBtn.dataset.id, 'pub'); return; }
  if (e.target && e.target.id === 'logoutBtn') {
    (async () => { await fetch('/api/logout', { method: 'POST' }); location.href = '/login.html'; })();
  }
});

function escapeHtml(s) {
  return (s==null?'':String(s)).replace(/[&<>"']/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]));
}

loadMe();


// ---- 隐私设置（G3 + 内容显隐扩展）----
function bindPrivacy(u) {
  const btn = document.getElementById('privacyBtn');
  if (!btn) return;
  const modal = document.getElementById('privacyModal');
  const defaultPriv = { age: 0, gender: 0, college: 1, major: 1, nativePlace: 1, birthday: 0, wechat: 0, qq: 0, phone: 0, social: 0, posts: 1, reviews: 1, articles: 1, favorites: 1, albums: 1 };
  const conflated = Object.assign({}, defaultPriv, u.privacy || {});
  btn.onclick = () => {
    document.getElementById('prvAge').checked = conflated.age === 1;
    document.getElementById('prvGender').checked = conflated.gender === 1;
    document.getElementById('prvCollege').checked = conflated.college === 1;
    document.getElementById('prvMajor').checked = conflated.major === 1;
    document.getElementById('prvNative').checked = conflated.nativePlace === 1;
    document.getElementById('prvBirthday').checked = conflated.birthday === 1;
    document.getElementById('prvWechat').checked = conflated.wechat === 1;
    document.getElementById('prvQQ').checked = conflated.qq === 1;
    document.getElementById('prvPhone').checked = conflated.phone === 1;
    document.getElementById('prvSocial').checked = conflated.social === 1;
    document.getElementById('prvPosts').checked = conflated.posts === 1;
    document.getElementById('prvReviews').checked = conflated.reviews === 1;
    document.getElementById('prvArticles').checked = conflated.articles === 1;
    document.getElementById('prvFavorites').checked = conflated.favorites === 1;
    document.getElementById('prvAlbums').checked = conflated.albums === 1;
    modal.style.display = 'flex';
  };
  document.getElementById('prvClose').onclick = () => modal.style.display = 'none';
  modal.addEventListener('click', e => { if (e.target === modal) modal.style.display = 'none'; });
  document.getElementById('prvSave').onclick = async () => {
    const g = (id) => (document.getElementById(id).checked ? 1 : 0);
    const privacy = {
      age: g('prvAge'), gender: g('prvGender'), college: g('prvCollege'),
      major: g('prvMajor'), nativePlace: g('prvNative'),
      birthday: g('prvBirthday'), wechat: g('prvWechat'), qq: g('prvQQ'),
      phone: g('prvPhone'), social: g('prvSocial'),
      posts: g('prvPosts'), reviews: g('prvReviews'), articles: g('prvArticles'),
      favorites: g('prvFavorites'), albums: g('prvAlbums')
    };
    const r = await fetch('/api/me/profile', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ privacy })
    });
    const d = await r.json();
    if (!r.ok) { alert(d.error || '保存失败'); return; }
    modal.style.display = 'none';
    location.reload();
  };
}


// ---- 黑名单管理（从设置移到我的风采）----
function bindBlockManager() {
  const btn = document.getElementById('blockMgrBtn');
  if (!btn) return;
  btn.onclick = async () => {
    try {
      const d = await (await fetch('/api/block/list')).json();
      const list = d.blocked || [];
      const overlay = document.createElement('div');
      overlay.className = 'modal';
      overlay.style.display = 'flex';
      overlay.innerHTML = '<div class="modal-box" style="width:min(480px,92vw)">' +
        '<div class="modal-head"><h3>黑名单管理</h3><button class="modal-close" id="bmClose"><i class="icon-fill" data-icon="close"></i></button></div>' +
        '<p style="color:var(--muted);font-size:.85rem;margin:0 0 12px">拉黑后，该用户的帖子、文章、评论将在全站对你隐藏。</p>' +
        '<div id="bmList" style="max-height:380px;overflow:auto">' + (list.length ? '' : '<div class="placeholder">黑名单为空</div>') + '</div></div>';
      document.body.appendChild(overlay);
      overlay.querySelector('#bmClose').onclick = () => overlay.remove();
      overlay.addEventListener('click', function (e) { if (e.target === overlay) overlay.remove(); });
      const listBox = overlay.querySelector('#bmList');
      list.forEach(function (b) {
        const row = document.createElement('div');
        row.className = 'act-item';
        row.style.borderBottom = '1px solid var(--border,#eee)';
        row.innerHTML = '<span class="act-badge badge-article">拉黑</span>' +
          '<a class="act-main" href="/user.html?id=' + b.id + '" target="_blank">' + escapeHtml(b.nickname || ('用户#' + b.id)) + '</a>' +
          '<button class="mgmt-del" data-unblock="' + b.id + '">取消拉黑</button>';
        row.querySelector('[data-unblock]').onclick = async function () {
          await fetch('/api/block', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ id: Number(b.id) }) });
          row.remove();
        };
        listBox.appendChild(row);
      });
    } catch (e) { alert('加载黑名单失败'); }
  };
}
