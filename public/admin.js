// admin.js — 后台管理界面逻辑
// 校验登录 + 管理员权限；渲染 数据总览 / 用户管理 / 举报中心 / 资料管理 / 评价管理 / 敏感词审查

const app = document.getElementById('adminApp');

// 当前激活的管理页签（操作后原地刷新时保持所在页签，不再跳回初始状态）
let currentTab = 'reports';

// 操作完成后原地刷新数据并保留当前页签（替代 location.reload）
async function reloadAdmin() {
  await init();
}

// ---- 全局错误捕获（诊断用）：任何运行时错误都会显示在页面顶部，便于定位隐藏 bug ----
window.addEventListener('error', function (ev) {
  try {
    const bar = document.createElement('div');
    bar.style.cssText = 'position:fixed;top:0;left:0;right:0;z-index:99999;background:#c0392b;color:#fff;padding:10px 16px;font:12px/1.6 monospace;white-space:pre-wrap;word-break:break-all';
    bar.textContent = '[JS错误] ' + (ev.message || '') + '\n' + (ev.filename ? ev.filename.split('/').pop() + ':' + ev.lineno : '');
    document.body.appendChild(bar);
  } catch (e) {}
});
window.addEventListener('unhandledrejection', function (ev) {
  try {
    const bar = document.createElement('div');
    bar.style.cssText = 'position:fixed;top:34px;left:0;right:0;z-index:99999;background:#e67e22;color:#fff;padding:10px 16px;font:12px/1.6 monospace;white-space:pre-wrap;word-break:break-all';
    bar.textContent = '[Promise错误] ' + (ev.reason && ev.reason.message ? ev.reason.message : String(ev.reason));
    document.body.appendChild(bar);
  } catch (e) {}
});

async function fetchJSON(url, opts) {
  const r = await fetch(url, opts || {});
  return r.json();
}

async function init() {
  const me = await fetchJSON('/api/me');
  if (!me.loggedIn) {
    app.innerHTML = `<div class="admin-deny">
      <p style="font-size:1.4rem;font-weight:700;margin-bottom:.6rem">需要登录</p>
      <p style="margin-bottom:1rem;color:var(--muted)">进入管理后台前请先登录站长账号</p>
      <a href="/login.html" class="btn btn-primary">去登录</a>
    </div>`;
    return;
  }
  if (!me.user || me.user.isAdmin !== 1) {
    app.innerHTML = `<div class="admin-deny">
      <p style="font-size:1.4rem;font-weight:700">403 · 无权限</p>
      <p style="color:var(--muted);margin-top:.6rem">你不是管理员，无法进入后台</p>
      <p style="margin-top:1.2rem"><a href="/">返回首页</a></p>
    </div>`;
    return;
  }

  const [stat, reviewRes, topicCand] = await Promise.all([
    fetchJSON('/api/admin/dashboard'),
    fetchJSON('/api/admin/review'),
    fetchJSON('/api/admin/topic-articles')
  ]);

  render(stat, reviewRes.items || [], topicCand.articles || []);
}

function render(stat, reviewItems, topicCand) {
  const statCards = [
    ['待处理举报', stat.reportPending, 'reports'],
    ['用户', stat.userCount, 'users'],
    ['帖子', stat.postCount, 'posts'],
    ['评价', stat.reviewCount, 'reviews'],
    ['资料', stat.fileCount, 'files']
  ].map(([label, num, tab]) =>
    `<button type="button" class="stat-card stat-btn" data-tab="${tab}"><div class="stat-num">${num}</div><div class="stat-label">${label}</div></button>`
  ).join('');

  const typeLabel = { post: '帖子', article: '文章', file: '资料', review: '评价' };
  const reviewQueueRows = reviewItems.length === 0
    ? `<tr><td colspan="7" class="empty">审核队列为空</td></tr>`
    : reviewItems.map(rv => `
      <tr>
        <td>#${escapeHtml(rv.reviewId)}</td>
        <td><span class="pill ${rv.status === '已下架' ? 'banned' : 'pending'}">${escapeHtml(typeLabel[rv.type] || rv.type)}</span></td>
        <td>${escapeHtml(rv.ref || '—')}</td>
        <td style="max-width:260px">${escapeHtml(rv.preview)}</td>
        <td>${escapeHtml(rv.author || '—')}</td>
        <td><span class="pill ${rv.status === '已下架' ? 'banned' : 'pending'}">${escapeHtml(rv.status)}</span></td>
        <td style="white-space:nowrap">
          <button class="btn-mini" data-review-act data-type="${rv.type}" data-id="${rv.id}" data-act="approve">通过</button>
          ${rv.status === '已下架'
            ? `<button class="btn-mini" data-review-act data-type="${rv.type}" data-id="${rv.id}" data-act="restore">恢复</button>`
            : `<button class="btn-mini danger" data-review-act data-type="${rv.type}" data-id="${rv.id}" data-act="remove">下架</button>`}
        </td>
      </tr>
    `).join('');

  const articleStatusPill = (st) => {
    const cls = st === '待审' ? 'pending' : (st === '已驳回' || st === '已下架') ? 'banned' : 'user';
    return `<span class="pill ${cls}">${escapeHtml(st)}</span>`;
  };
    app.innerHTML = `
    <div class="admin-title">管理后台</div>
    <div class="admin-sub">站长专属 · 数据总览</div>

    <div class="panel">
      <h2>数据总览</h2>
      <p style="color:var(--muted);font-size:.88rem;margin:-4px 0 12px">点方块或下方标签进入对应管理</p>
      <div class="stat-grid">${statCards}</div>
      <div class="stat-grid" style="margin-top:12px">
        <button type="button" class="stat-card stat-btn" data-stat="courses"><div class="stat-num">${stat.courseCount}</div><div class="stat-label">收录课程</div></button>
        <button type="button" class="stat-card stat-btn" data-stat="offerings"><div class="stat-num">${stat.offeringCount}</div><div class="stat-label">开课班次</div></button>
        <button type="button" class="stat-card stat-btn" data-stat="colleges"><div class="stat-num">${stat.collegeCount}</div><div class="stat-label">覆盖学院</div></button>
      </div>
    </div>

    <div class="admin-tabs">
      <button class="admin-tab" data-tab="users">用户</button>
      <button class="admin-tab" data-tab="reports">举报</button>
      <button class="admin-tab" data-tab="files">资料</button>
      <button class="admin-tab" data-tab="reviews">评价</button>
      <button class="admin-tab" data-tab="posts">帖子</button>
      <button class="admin-tab" data-tab="articles">文章</button>
      <button class="admin-tab" data-tab="review">复核</button>
      <button class="admin-tab" data-tab="censor">敏感词</button>
      <button class="admin-tab" data-tab="announcement">公告</button>
      <button class="admin-tab" data-tab="topics">专题</button>
      <button class="admin-tab" data-tab="courses">课程</button>
      <button class="admin-tab" data-tab="audit">审计</button>
      <button class="admin-tab" data-tab="maintenance">维护</button>
      <button class="admin-tab" data-tab="opinions">意见</button>
      <button class="admin-tab" data-tab="aireview">AI审查</button>
      <button class="admin-tab" data-tab="aireviewauto">AI自动审查</button>
    </div>

    <div class="admin-pane" data-pane="users">
      <div class="pane-title">用户管理</div>
      <div style="display:flex;gap:8px;margin-bottom:12px">
        <input id="userSearch" type="text" placeholder="搜索邮箱 / 昵称 / 学院 / 专业…" style="flex:1;min-width:240px;padding:8px 10px;border:1px solid #ddd;border-radius:6px">
        <button class="btn-mini" id="userSearchBtn">搜索</button>
      </div>
      <table>
        <thead><tr><th>ID</th><th>邮箱</th><th>昵称</th><th>学院</th><th>专业</th><th>角色</th><th>操作</th></tr></thead>
        <tbody id="userListBody"><tr><td colspan="7" class="empty">加载中…</td></tr></tbody>
      </table>
      <div id="userPager" style="display:flex;justify-content:center;align-items:center;margin:12px 0"></div>
    </div>

    <div class="admin-pane" data-pane="reports">
      <div class="pane-title">举报中心</div>
      <div style="display:flex;gap:8px;margin-bottom:12px">
        <input id="reportSearch" type="text" placeholder="搜索被举报内容 / 被举报人…" style="flex:1;min-width:240px;padding:8px 10px;border:1px solid #ddd;border-radius:6px">
        <button class="btn-mini" id="reportSearchBtn">搜索</button>
      </div>
      <table>
        <thead><tr><th>ID</th><th>类型</th><th>举报对象</th><th>被举报人</th><th>原因</th><th>状态</th><th>操作</th></tr></thead>
        <tbody id="reportListBody"><tr><td colspan="7" class="empty">加载中…</td></tr></tbody>
      </table>
      <div id="reportPager" style="display:flex;justify-content:center;align-items:center;margin:12px 0"></div>
    </div>

    <div class="admin-pane" data-pane="files">
      <div class="pane-title">资料管理</div>
      <div style="display:flex;gap:8px;margin-bottom:12px">
        <input id="fileSearch" type="text" placeholder="搜索文件名 / 课程 / 上传者…" style="flex:1;min-width:240px;padding:8px 10px;border:1px solid #ddd;border-radius:6px">
        <button class="btn-mini" id="fileSearchBtn">搜索</button>
      </div>
      <table>
        <thead><tr><th>ID</th><th>文件</th><th>课程</th><th>分类</th><th>前台</th><th>真实作者</th><th>状态</th><th>操作</th></tr></thead>
        <tbody id="fileListBody"><tr><td colspan="8" class="empty">加载中…</td></tr></tbody>
      </table>
      <div id="filePager" style="display:flex;justify-content:center;align-items:center;margin:12px 0"></div>
    </div>

    <div class="admin-pane" data-pane="reviews">
      <div class="pane-title">评价管理</div>
      <div style="display:flex;gap:8px;margin-bottom:12px">
        <input id="reviewSearch" type="text" placeholder="搜索课程 / 内容 / 作者…" style="flex:1;min-width:240px;padding:8px 10px;border:1px solid #ddd;border-radius:6px">
        <button class="btn-mini" id="reviewSearchBtn">搜索</button>
      </div>
      <table>
        <thead><tr><th>ID</th><th>课程</th><th>评分</th><th>内容</th><th>前台</th><th>真实作者</th><th>状态</th><th>操作</th></tr></thead>
        <tbody id="reviewListBody"><tr><td colspan="8" class="empty">加载中…</td></tr></tbody>
      </table>
      <div id="reviewPager" style="display:flex;justify-content:center;align-items:center;margin:12px 0"></div>
    </div>

    <div class="admin-pane" data-pane="articles">
      <div class="pane-title">文章管理</div>
      <div style="display:flex;gap:8px;margin-bottom:12px">
        <input id="articleSearch" type="text" placeholder="搜索标题 / 内容 / 作者…" style="flex:1;min-width:240px;padding:8px 10px;border:1px solid #ddd;border-radius:6px">
        <button class="btn-mini" id="articleSearchBtn">搜索</button>
      </div>
      <table>
        <thead><tr><th>ID</th><th>标题</th><th>分类</th><th>前台</th><th>真实作者</th><th>理由</th><th>状态</th><th>操作</th></tr></thead>
        <tbody id="articleListBody"><tr><td colspan="8" class="empty">加载中…</td></tr></tbody>
      </table>
      <div id="articlePager" style="display:flex;justify-content:center;align-items:center;margin:12px 0"></div>
    </div>

    <div class="admin-pane" data-pane="posts">
      <div class="pane-title">帖子管理</div>
      <p style="color:var(--muted);font-size:.88rem;margin:8px 0 12px">论坛全部帖子（含草稿/已下架），可下架/恢复；点查看跳到前台帖子页。</p>
      <div style="display:flex;gap:8px;margin-bottom:12px">
        <input id="postSearch" type="text" placeholder="搜索标题 / 内容 / 板块 / 作者…" style="flex:1;min-width:240px;padding:8px 10px;border:1px solid #ddd;border-radius:6px">
        <button class="btn-mini" id="postSearchBtn">搜索</button>
      </div>
      <table>
        <thead><tr><th>ID</th><th>板块</th><th>标题</th><th>内容</th><th>前台</th><th>真实作者</th><th>状态</th><th>时间</th><th>操作</th></tr></thead>
        <tbody id="postListBody"><tr><td colspan="9" class="empty">加载中…</td></tr></tbody>
      </table>
      <div id="postPager" style="display:flex;justify-content:center;align-items:center;margin:12px 0"></div>
    </div>

    <div class="admin-pane" data-pane="review">
      <div class="pane-title">复核（待复核 / 已下架内容）</div>
      <p style="color:var(--muted);font-size:.88rem;margin:8px 0 12px">这里是<b>被系统敏感词拦截或已被下架的内容</b>（帖子/文章/资料/评价）。逐条决定：通过（公开）→ 恢复正常展示；下架 → 保持隐藏；恢复 → 重新上架。与上方各内容管理页的区别：这里只列出需要你处理的问题内容。</p>
      <table>
        <thead><tr><th>序号</th><th>类型</th><th>定位</th><th>内容</th><th>作者</th><th>状态</th><th>操作</th></tr></thead>
        <tbody>${reviewQueueRows}</tbody>
      </table>
    </div>

    <div class="admin-pane" data-pane="announcement">
      <div class="pane-title">站长公告</div>
      <p style="color:var(--muted);font-size:.88rem;margin:8px 0 12px">发布后全站顶部显示公告条。同一时间只展示最新一条，旧公告标记为"已替换"（仍保留，可单独删除）。</p>
      <div style="display:flex;gap:8px;margin-bottom:14px">
        <input id="annInput" type="text" placeholder="输入公告内容…" style="flex:1;min-width:260px;padding:8px 10px;border:1px solid #ddd;border-radius:6px">
        <button class="btn-mini" id="annPublishBtn">发布</button>
        <button class="btn-mini danger" id="annClearBtn">清空全部</button>
      </div>
      <table>
        <thead><tr><th>ID</th><th>内容</th><th>发布时间</th><th>状态</th><th>操作</th></tr></thead>
        <tbody id="annListBody"><tr><td colspan="5" class="empty">加载中…</td></tr></tbody>
      </table>
      <div id="annPager" style="display:flex;justify-content:center;align-items:center;margin:12px 0"></div>
    </div>

    <div class="admin-pane" data-pane="censor">
      <div class="pane-title">敏感词审查</div>
      <p style="color:var(--muted);font-size:.88rem;margin:8px 0 12px">发布内容（发帖/评论/评价/上传标题与描述）命中下列词即被自动拦截，不落库。</p>
      <div style="display:flex;gap:8px;margin-bottom:12px;flex-wrap:wrap">
        <input id="censorInput" type="text" placeholder="输入要拦截的词，多个用英文逗号分隔" style="flex:1;min-width:260px;padding:8px 10px;border:1px solid #ddd;border-radius:6px">
        <button class="btn-mini" id="censorAddBtn">＋ 添加</button>
      </div>
      <table>
        <thead><tr><th>ID</th><th>敏感词</th><th>添加时间</th><th>操作</th></tr></thead>
        <tbody id="censorListBody"><tr><td colspan="4" class="empty">加载中…</td></tr></tbody>
      </table>
      <div id="censorPager" style="display:flex;justify-content:center;align-items:center;margin:12px 0"></div>
    </div>

    <div class="admin-pane" data-pane="topics">
      <div class="pane-title">专题管理</div>
      <div class="panel">
        <h2>新建专题</h2>
        <input id="topicTitle" placeholder="专题标题" style="padding:8px 10px;border:1px solid #ddd;border-radius:8px;margin-right:8px;width:200px">
        <input id="topicDesc" placeholder="专题简介（可选）" style="padding:8px 10px;border:1px solid #ddd;border-radius:8px;margin-right:8px;width:280px">
        <button class="btn-mini" id="topicCreateBtn" style="background:var(--magenta);color:#fff">创建</button>
      </div>
      <div class="panel">
        <h2>已有专题</h2>
        <table>
          <thead><tr><th>ID</th><th>标题</th><th>文章数</th><th>操作</th></tr></thead>
          <tbody id="topicListBody"><tr><td colspan="4" class="empty">加载中…</td></tr></tbody>
        </table>
        <div id="topicPager" style="display:flex;justify-content:center;align-items:center;margin:12px 0"></div>
      </div>
      <div class="panel" id="topicManagePanel" style="display:none">
        <h2 id="topicManageTitle">管理专题文章</h2>
        <select id="topicArticleSelect" style="padding:8px;border:1px solid #ddd;border-radius:8px;margin-right:8px;min-width:240px">
          <option value="">选择要加入的文章…</option>
          ${topicCand.map(a => `<option value="${a.id}">${escapeHtml(a.title)}</option>`).join('')}
        </select>
        <button class="btn-mini" id="topicAddArticleBtn" style="background:var(--magenta);color:#fff">加入</button>
        <table style="margin-top:12px"><thead><tr><th>ID</th><th>标题</th><th>操作</th></tr></thead>
          <tbody id="topicDetailBody"><tr><td colspan="3" class="empty">选择专题以查看文章</td></tr></tbody>
        </table>
      </div>
    </div>

    <div class="admin-pane" data-pane="courses">
      <div class="pane-title">课程数据中心</div>
      <div style="display:flex;gap:8px;margin:0 0 12px;flex-wrap:wrap">
        <button class="btn-mini" data-cview="courses" style="background:var(--magenta);color:#fff">课程</button>
        <button class="btn-mini" data-cview="offerings">班次</button>
        <button class="btn-mini" data-cview="colleges">学院</button>
      </div>
      <div id="courseView-courses">
        <p style="color:var(--muted);font-size:.88rem;margin:0 0 10px">搜索并修正课程字段（纠错 xlsx 导入误差），保存后全站实时生效并持久化。</p>
        <div style="display:flex;gap:8px;margin-bottom:12px;flex-wrap:wrap">
          <input id="courseSearch" type="text" placeholder="输入课程名/编号…" style="flex:1;min-width:240px;padding:8px 10px;border:1px solid #ddd;border-radius:6px">
          <button class="btn-mini" id="courseSearchBtn">搜索</button>
        </div>
        <table>
          <thead><tr><th>课程名</th><th>编号</th><th>学院</th><th>学分</th><th>考核</th><th>标记</th><th>操作</th></tr></thead>
          <tbody id="courseListBody"><tr><td colspan="7" class="empty">输入关键词搜索课程</td></tr></tbody>
        </table>
      <div id="coursePager" style="display:flex;justify-content:center;align-items:center;margin:12px 0"></div>
      <div id="courseEditOverlay" style="display:none;position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,.45);z-index:8000;align-items:center;justify-content:center"></div>
      <div id="courseEditPanel" style="display:none;position:fixed;top:50%;left:50%;transform:translate(-50%,-50%);z-index:8001;width:min(640px,92vw);max-height:85vh;overflow:auto;background:#fff;border-radius:14px;box-shadow:0 12px 48px rgba(0,0,0,.25);padding:20px">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px">
          <h2 id="courseEditTitle" style="margin:0;font-size:1.05rem">编辑课程</h2>
          <button class="btn-mini" id="ceCloseBtn" style="font-size:1rem;line-height:1">${ICONS.close(14)}</button>
        </div>
        <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(150px,1fr));gap:10px">
          <label>课程名称<input id="ceName" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box"></label>
          <label>课程编号<input id="ceCode" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box"></label>
          <label>学分<input id="ceCredit" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box"></label>
          <label>考核方式<input id="ceExam" placeholder="考试/考查" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box"></label>
          <label>课程标签<input id="ceTag" placeholder="专业课/公共必修课…" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box"></label>
          <label>课程大类<input id="ceBig" placeholder="普通课/通选课…" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box"></label>
          <label>课程性质<input id="ceNature" placeholder="课程/实践环节…" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box"></label>
          <label>课程属性<input id="ceAttr" placeholder="必修/限选/任选" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box"></label>
          <label>通选类别<input id="ceGen" placeholder="通选课类别" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box"></label>
          <label>开课院系<input id="ceCollege" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box"></label>
        </div>
        <div style="margin-top:12px;display:flex;gap:8px">
          <button class="btn-mini" id="ceSaveBtn" style="background:var(--magenta);color:#fff">保存纠错</button>
          <button class="btn-mini danger" id="ceResetBtn">还原为原始</button>
          <button class="btn-mini" id="ceCancelBtn">取消</button>
        </div>
      </div>
      </div>
      <div id="courseView-offerings" style="display:none">
        <p style="color:var(--muted);font-size:.88rem;margin:0 0 10px">开课班次列表（按课程编号/老师/班级/学院搜索），供核对排课数据。</p>
        <div style="display:flex;gap:8px;margin-bottom:12px;flex-wrap:wrap">
          <input id="offeringSearch" type="text" placeholder="输入课程编号/老师/班级…" style="flex:1;min-width:240px;padding:8px 10px;border:1px solid #ddd;border-radius:6px">
          <button class="btn-mini" id="offeringSearchBtn">搜索</button>
        </div>
        <div style="max-height:480px;overflow:auto;border:1px solid #eee;border-radius:8px">
        <table>
          <thead><tr><th>编号</th><th>学期</th><th>老师</th><th>班级</th><th>时间</th><th>地点</th><th>操作</th></tr></thead>
          <tbody id="offeringListBody"><tr><td colspan="7" class="empty">输入关键词搜索班次</td></tr></tbody>
        </table>
        </div>
        <div id="offeringPager" style="display:flex;justify-content:center;align-items:center;margin:12px 0"></div>
      </div>
      <div id="offeringEditOverlay" style="display:none;position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,.45);z-index:8000;align-items:center;justify-content:center"></div>
      <div id="offeringEditPanel" style="display:none;position:fixed;top:50%;left:50%;transform:translate(-50%,-50%);z-index:8001;width:min(520px,92vw);background:#fff;border-radius:14px;box-shadow:0 12px 48px rgba(0,0,0,.25);padding:20px">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px">
          <h2 id="offeringEditTitle" style="margin:0;font-size:1.05rem">编辑班次</h2>
          <button class="btn-mini" id="offeringCloseBtn" style="font-size:1rem;line-height:1">${ICONS.close(14)}</button>
        </div>
        <input id="oeCode" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box;margin-bottom:8px" placeholder="课程编号" readonly>
        <input id="oeTerm" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box;margin-bottom:8px" placeholder="学期" readonly>
        <input id="oeClass" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box;margin-bottom:8px" placeholder="班级" readonly>
        <label style="font-size:.85rem;color:var(--muted)">老师</label>
        <input id="oeTeacher" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box;margin-bottom:8px">
        <label style="font-size:.85rem;color:var(--muted)">时间</label>
        <textarea id="oeTime" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box;margin-bottom:8px;min-height:60px"></textarea>
        <label style="font-size:.85rem;color:var(--muted)">地点</label>
        <input id="oeLocation" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box;margin-bottom:12px">
        <div style="display:flex;gap:8px">
          <button class="btn-mini" id="oeSaveBtn" style="background:var(--magenta);color:#fff">保存</button>
          <button class="btn-mini" id="oeCancelBtn">取消</button>
        </div>
      </div>
      <div id="courseView-colleges" style="display:none">
        <p style="color:var(--muted);font-size:.88rem;margin:0 0 10px">全部开课院系（覆盖学院），附各院系课程数。</p>
        <div style="max-height:480px;overflow:auto;border:1px solid #eee;border-radius:8px">
        <table>
          <thead><tr><th>学院(展示名)</th><th>课表名</th><th>学科类别</th><th>课程数</th><th>专业</th><th>操作</th></tr></thead>
          <tbody id="collegeListBody"><tr><td colspan="5" class="empty">加载中…</td></tr></tbody>
        </table>
        </div>
      </div>
      <div id="collegeEditOverlay" style="display:none;position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,.45);z-index:8000;align-items:center;justify-content:center"></div>
      <div id="collegeEditPanel" style="display:none;position:fixed;top:50%;left:50%;transform:translate(-50%,-50%);z-index:8001;width:min(520px,92vw);background:#fff;border-radius:14px;box-shadow:0 12px 48px rgba(0,0,0,.25);padding:20px">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px">
          <h2 id="collegeEditTitle" style="margin:0;font-size:1.05rem">编辑学院</h2>
          <button class="btn-mini" id="collegeCloseBtn" style="font-size:1rem;line-height:1">${ICONS.close(14)}</button>
        </div>
        <input id="ceColName" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box;margin-bottom:8px" placeholder="学院本名（课表原始名）" readonly>
        <label style="font-size:.85rem;color:var(--muted)">在本网站的展示名</label>
        <input id="ceColDisplay" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box;margin-bottom:8px" placeholder="例如：计科院 / 计算机学院">
        <label style="font-size:.85rem;color:var(--muted)">学科类别</label>
        <input id="ceColCategory" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box;margin-bottom:8px" placeholder="理工类 / 医学类 / 人文社科类 / 艺术类 / 中外合作办学">
        <label style="font-size:.85rem;color:var(--muted)">学校课表里的名字</label>
        <input id="ceColSchedule" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box;margin-bottom:8px" placeholder="课表里的准确名称">
        <label style="font-size:.85rem;color:var(--muted)">该学院下的专业及缩写（每行一个：专业名[缩写]，用换行分隔）</label>
        <textarea id="ceColMajors" style="width:100%;padding:6px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box;margin-bottom:12px;min-height:80px" placeholder="计算机科学与技术[计科]&#10;软件工程[软工]"></textarea>
        <div style="display:flex;gap:8px">
          <button class="btn-mini" id="collegeSaveBtn" style="background:var(--magenta);color:#fff">保存</button>
          <button class="btn-mini" id="collegeCancelBtn">取消</button>
        </div>
      </div>
      <div class="panel" style="margin-top:16px">
        <h2>导入课表 CSV</h2>
        <p style="color:var(--muted);font-size:.85rem;margin:6px 0 10px">上传新学期的课程总表 CSV（表头需含"通知单号"），保存到 CSV 目录并立即重载全站课表。</p>
        <input type="file" id="csvFileInput" accept=".csv" style="margin-right:8px">
        <button class="btn-mini" id="csvImportBtn" style="background:var(--magenta);color:#fff">导入并重载</button>
        <span id="csvMsg" style="margin-left:8px;font-size:.85rem"></span>
      </div>
    </div>

    <div class="admin-pane" data-pane="audit">
      <div class="pane-title">审计日志</div>
      <p style="color:var(--muted);font-size:.88rem;margin:0 0 10px">后台关键写操作记录（设管理员/封禁/举报处理/编辑课程/班次/学院/维护等）。</p>
      <div style="max-height:500px;overflow:auto;border:1px solid #eee;border-radius:8px">
        <table>
          <thead><tr><th>时间</th><th>操作者</th><th>动作</th><th>详情</th></tr></thead>
          <tbody id="auditListBody"><tr><td colspan="4" class="empty">加载中…</td></tr></tbody>
        </table>
      </div>
      <div id="auditPager" style="display:flex;justify-content:center;align-items:center;margin:12px 0"></div>
    </div>

    <div class="admin-pane" data-pane="maintenance">
      <div class="pane-title">站点维护模式</div>
      <p style="color:var(--muted);font-size:.88rem;margin:0 0 12px">开启后普通访问者看到"维护中"页面，管理员和 API 不受影响。</p>
      <div style="display:flex;align-items:center;gap:12px;margin-bottom:12px">
        <label style="font-size:.92rem">当前状态：
          <strong id="maintStatus">未知</strong></label>
        <button class="btn-mini" id="maintBtn" style="background:var(--magenta);color:#fff"></button>
      </div>
      <div>
        <label style="font-size:.85rem;color:var(--muted)">维护提示语</label>
        <input id="maintMsg" type="text" placeholder="站点维护中，请稍后再来" style="width:100%;padding:8px 10px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box;margin:6px 0 12px">
      </div>
      <div style="margin-top:18px;border-top:1px solid #eee;padding-top:12px">
        <label style="font-size:.95rem;font-weight:600">数据备份（H5）</label>
        <p style="color:var(--muted);font-size:.85rem;margin:4px 0 8px">一键生成数据库一致性快照 + 上传目录压缩包，保留最近 14 份自动轮转。定时任务可调用 /api/admin/backup。</p>
        <button class="btn-mini" id="backupBtn" style="background:var(--magenta);color:#fff">立即备份</button>
        <span id="backupMsg" style="margin-left:8px;font-size:.85rem;color:var(--muted)"></span>
      </div>
    </div>

    <div class="admin-pane" data-pane="aireview">
      <div class="pane-title">AI 内容审查</div>
      <p style="color:var(--muted);font-size:.88rem;margin:0 0 10px">把文本交给 AI 审查是否违规（需在服务器配置 AI_API_KEY）。可粘贴帖子/评论/评价/文章内容，或点击"AI审查"从审核队列带内容过来。</p>
      <div style="margin-bottom:10px">
        <label style="font-size:.85rem;color:var(--muted)">待审查文本（最多 3000 字）</label>
        <textarea id="aiReviewInput" rows="6" placeholder="粘贴要审查的内容…" style="width:100%;padding:10px;border:1px solid #ddd;border-radius:8px;box-sizing:border-box;margin-top:6px;font-family:inherit"></textarea>
      </div>
      <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap">
        <button class="btn-mini" id="aiReviewBtn" style="background:var(--magenta);color:#fff;padding:.5rem 1.4rem">开始 AI 审查</button>
        <span id="aiReviewMsg" style="font-size:.85rem;color:var(--muted)"></span>
      </div>
      <div id="aiReviewResult" style="margin-top:14px;display:none;padding:14px;border:1px solid #eee;border-radius:8px;background:#fafbfc;font-size:.9rem;line-height:1.7"></div>
    </div>

    <div class="admin-pane" data-pane="aireviewauto">
      <div class="pane-title">AI 自动审查（两级策略）</div>
      <p style="color:var(--muted);font-size:.88rem;margin:0 0 10px">拉取最近的帖子/评论/评价/文章批量审查。<b>明显恶意</b>（反复出现/空洞/恶俗/色情/辱骂/诈骗）<b>自动下架</b>；<b>模糊内容</b>列出来供你人工复核。需配置 AI_API_KEY（DeepSeek 等 OpenAI 兼容接口）。</p>
      <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap">
        <button class="btn-mini" id="aiAutoBtn" style="background:var(--magenta);color:#fff;padding:.5rem 1.4rem">开始自动审查</button>
        <span id="aiAutoMsg" style="font-size:.85rem;color:var(--muted)"></span>
      </div>
      <div id="aiAutoResult" style="margin-top:14px;font-size:.88rem;line-height:1.7"></div>
    </div>

    <div class="admin-pane" data-pane="opinions">
      <div class="pane-title">意见箱</div>
      <p style="color:var(--muted);font-size:.88rem;margin:0 0 10px">用户提交的网站意见/建议。</p>
      <div style="max-height:500px;overflow:auto;border:1px solid #eee;border-radius:8px">
        <table>
          <thead><tr><th>时间</th><th>来自</th><th>内容</th><th>状态</th><th>操作</th></tr></thead>
          <tbody id="opinionListBody"><tr><td colspan="5" class="empty">加载中…</td></tr></tbody>
        </table>
      </div>
      <div id="opinionPager" style="display:flex;justify-content:center;align-items:center;margin:12px 0"></div>
    </div>
  `;

  // Tab 切换：点哪个标签，底下就只显示哪一块
  function showAdminTab(tab) {
    currentTab = tab;
    document.querySelectorAll('.admin-pane').forEach(p => { p.style.display = p.dataset.pane === tab ? '' : 'none'; });
    document.querySelectorAll('.admin-tab').forEach(t => { t.classList.toggle('active', t.dataset.tab === tab); });
    document.querySelectorAll('.stat-btn').forEach(b => { b.classList.toggle('active', b.dataset.tab === tab); });
    // 切到课程数据中心时：保持当前视图（默认课程），加载对应列表
    if (tab === 'courses') {
      showCourseView(courseViewCurrent || 'courses');
    }
    // 审计 / 维护 / 意见 / AI 审查 tab：懒加载（带 try/catch 防止渲染中断）
    try { if (tab === 'users') loadAdminUsers(); } catch (e) { console.error('users', e); }
    try { if (tab === 'reports') loadAdminReports(); } catch (e) { console.error('reports', e); }
    try { if (tab === 'files') loadAdminFiles(); } catch (e) { console.error('files', e); }
    try { if (tab === 'reviews') loadAdminReviews(); } catch (e) { console.error('reviews', e); }
    try { if (tab === 'articles') loadAdminArticles(); } catch (e) { console.error('articles', e); }
    try { if (tab === 'posts') loadAdminPosts(); } catch (e) { console.error('posts', e); }
    try { if (tab === 'audit') loadAudit(); } catch (e) { console.error('audit', e); }
    try { if (tab === 'maintenance') loadMaintenance(); } catch (e) { console.error('maintenance', e); }
    try { if (tab === 'opinions') loadOpinions(); } catch (e) { console.error('opinions', e); }
    try { if (tab === 'announcement') loadAnnouncements(); } catch (e) { console.error('announcement', e); }
    try { if (tab === 'censor') loadCensor(); } catch (e) { console.error('censor', e); }
    try { if (tab === 'topics') loadAdminTopics(); } catch (e) { console.error('topics', e); }
    try { if (tab === 'aireview') bindAiReview(); } catch (e) { console.error('aireview', e); }
    try { if (tab === 'aireviewauto') bindAiAutoReview(); } catch (e) { console.error('aireviewauto', e); }
  }
  window.showAdminTab = showAdminTab; // 暴露全局，供外部(课程数据中心等)点击调用
  document.querySelectorAll('.admin-tab').forEach(t => t.addEventListener('click', () => showAdminTab(t.dataset.tab)));
  // 统计卡：普通卡(data-tab)切 tab；课程数据卡(data-stat)进课程数据中心对应视图
  document.querySelectorAll('.stat-btn').forEach(b => {
    if (b.dataset.stat) {
      b.addEventListener('click', (e) => {
        e.stopPropagation();
        showAdminTab('courses');
        const target = b.dataset.stat;
        showCourseView(target === 'offerings' ? 'offerings' : (target === 'colleges' ? 'colleges' : 'courses'));
      });
    } else {
      b.addEventListener('click', () => showAdminTab(b.dataset.tab));
    }
  });
  showAdminTab(currentTab);
}

async function addCensorWords(input) {
  const words = (input.value || '').split(/[,，]/).map(w => w.trim()).filter(Boolean);
  if (words.length === 0) { alert('请输入要拦截的词'); return; }
  for (const w of words) {
    await fetchJSON('/api/admin/censor', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ word: w })
    });
  }
  input.value = '';
  reloadAdmin();
}

document.addEventListener('click', async (e) => {
  const roleBtn = e.target.closest('[data-role-toggle]');
  if (roleBtn) {
    const id = roleBtn.dataset.id;
    const now = Number(roleBtn.dataset.now);
    const next = now === 1 ? 0 : 1;
    await fetchJSON(`/api/admin/users/${id}/admin`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ isAdmin: next })
    });
    loadAdminUsers();
    return;
  }
  const banBtn = e.target.closest('[data-user-ban]');
  if (banBtn) {
    const id = banBtn.dataset.id;
    const cur = Number(banBtn.dataset.banned);
    const next = cur === 1 ? 0 : 1;
    if (next === 1 && !confirm('确定封禁该用户？其内容将不再显示，且无法登录。')) return;
    await fetchJSON(`/api/admin/userban/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ banned: next })
    });
    loadAdminUsers();
    return;
  }
  const muteBtn = e.target.closest('[data-user-mute]');
  if (muteBtn) {
    const id = muteBtn.dataset.id;
    const cur = Number(muteBtn.dataset.muted);
    const next = cur === 1 ? 0 : 1;
    if (next === 1 && !confirm('确定禁言该用户？其仍可登录浏览，但不能发帖/评论/上传。')) return;
    await fetchJSON(`/api/admin/userban/mute/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ muted: next })
    });
    loadAdminUsers();
    return;
  }
  const fileBtn = e.target.closest('[data-file-status]');
  if (fileBtn) {
    const id = fileBtn.dataset.id;
    const cur = fileBtn.dataset.status;
    const next = cur === '已下架' ? '正常' : '已下架';
    await fetchJSON(`/api/admin/files/${id}/status`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status: next })
    });
    loadAdminFiles();
    return;
  }
  const repBtn = e.target.closest('[data-report-resolve]');
  if (repBtn) {
    const id = repBtn.dataset.id;
    const act = repBtn.dataset.act;
    await fetchJSON(`/api/admin/reports/${id}/resolve`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: act })
    });
    loadAdminReports();
    refreshStats();
    return;
  }
  const reviewBtn = e.target.closest('[data-review-status]');
  if (reviewBtn) {
    const id = reviewBtn.dataset.id;
    const cur = reviewBtn.dataset.status;
    const next = cur === '已下架' ? '正常' : '已下架';
    await fetchJSON(`/api/admin/reviews/${id}/status`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status: next })
    });
    loadAdminReviews();
    return;
  }
  const artRev = e.target.closest('[data-article-review]');
  if (artRev) {
    const id = artRev.dataset.id;
    const act = artRev.dataset.act;
    if (act === 'approve') {
      await fetchJSON(`/api/admin/articles/${id}/status`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: '正常' })
      });
    } else if (act === 'reject') {
      const reason = prompt('请输入驳回理由（会展示给作者）：');
      if (reason === null) return;
      await fetchJSON(`/api/admin/articles/${id}/status`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: '已驳回', rejectReason: reason.trim() })
      });
    }
    loadAdminArticles();
    return;
  }
  const articleBtn = e.target.closest('[data-article-status]');
  if (articleBtn) {
    const id = articleBtn.dataset.id;
    const cur = articleBtn.dataset.status;
    const next = cur === '已下架' ? '正常' : '已下架';
    await fetchJSON(`/api/admin/articles/${id}/status`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status: next })
    });
    loadAdminArticles();
    return;
  }
  const revBtn = e.target.closest('[data-review-act]');
  if (revBtn) {
    const { type, id, act } = revBtn.dataset;
    if (act === 'remove' && !confirm('确定下架该内容？将不再公开展示。')) return;
    await fetchJSON(`/api/admin/review/${type}/${id}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: act })
    });
    reloadAdmin();
    return;
  }
  const censorDel = e.target.closest('[data-censor-del]');
  if (censorDel) {
    const id = censorDel.dataset.id;
    await fetchJSON(`/api/admin/censor/${id}`, { method: 'DELETE' });
    reloadAdmin();
    return;
  }
  const annPub = e.target.closest('#annPublishBtn');
  if (annPub) {
    const input = document.getElementById('annInput');
    const content = input.value.trim();
    if (!content) { alert('请输入公告内容'); return; }
    await fetchJSON('/api/admin/announcement', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ content }) });
    input.value = '';
    loadAnnouncements();
    return;
  }
  const annClear = e.target.closest('#annClearBtn');
  if (annClear) {
    if (!confirm('确定清空所有公告？')) return;
    await fetchJSON('/api/admin/announcement', { method:'DELETE' });
    loadAnnouncements();
    return;
  }
  const annDel = e.target.closest('[data-ann-del]');
  if (annDel) {
    if (!confirm('确定删除该条公告？')) return;
    await fetchJSON('/api/admin/announcement/' + annDel.dataset.id, { method:'DELETE' });
    loadAnnouncements();
    return;
  }
  const annSt = e.target.closest('[data-ann-status]');
  if (annSt) {
    const next = annSt.dataset.status === '展示中' ? '已下架' : '展示中';
    const r = await fetch('/api/admin/announcement/' + annSt.dataset.id + '/status', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status: next })
    });
    const d = await r.json();
    if (!r.ok) { alert(d.error || '操作失败'); return; }
    loadAnnouncements();
    return;
  }
  const censorAdd = e.target.closest('#censorAddBtn');
  if (censorAdd) {
    addCensorWords(document.getElementById('censorInput'));
  }
});

document.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && e.target && e.target.id === 'censorInput') {
    e.preventDefault();
    addCensorWords(e.target);
  }
});

function escapeHtml(s) {
  return (s == null ? '' : String(s)).replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m]));
}

init();


// 帖子下架/恢复 + 分页
document.addEventListener('click', async (e) => {
  const postPg = e.target.closest('[data-post-page]');
  if (postPg) {
    const p2 = Number(postPg.dataset.postPage);
    if (p2 >= 1) loadAdminPosts(p2);
    return;
  }
  const postSt = e.target.closest('[data-post-status]');
  if (postSt) {
    const cur = postSt.dataset.status;
    const next = cur === '正常' ? '已下架' : '正常';
    if (next === '已下架' && !confirm('确定下架该帖子？前台将不再显示。')) return;
    const r = await fetch('/api/admin/posts/' + postSt.dataset.id + '/status', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status: next })
    });
    const d = await r.json();
    if (!r.ok) { alert(d.error || '操作失败'); return; }
    loadAdminPosts(postPage);
    return;
  }
});

// ---- F4 专题管理交互 ----
document.addEventListener('click', async (e) => {
  const createBtn = e.target.closest('#topicCreateBtn');
  if (createBtn) {
    const title = document.getElementById('topicTitle').value.trim();
    const desc = document.getElementById('topicDesc').value.trim();
    if (!title) { alert('请输入专题标题'); return; }
    const body = JSON.stringify({ title, description: desc });
    const r = await fetchJSON('/api/admin/topics', { method: 'POST', headers: {'Content-Type':'application/json'}, body });
    reloadAdmin();
    return;
  }
  const delBtn = e.target.closest('[data-topic-del]');
  if (delBtn) {
    if (!confirm('确定删除该专题？')) return;
    await fetch('/api/admin/topics/' + delBtn.dataset.id + '/', { method: 'DELETE' });
    reloadAdmin();
    return;
  }
});
document.addEventListener('click', async (e) => {
  const manageBtn = e.target.closest('[data-topic-articles]');
  if (manageBtn) {
    const id = manageBtn.dataset.id;
    const panel = document.getElementById('topicManagePanel');
    panel.style.display = '';
    const r = await fetchJSON('/api/topics/' + id);
    document.getElementById('topicManageTitle').textContent = '管理专题 #' + id + ' · ' + (r.topic && r.topic.title || '');
    panel.dataset.topicId = id;
    const arts = r.articles || [];
    document.getElementById('topicDetailBody').innerHTML = arts.length === 0
      ? '<tr><td colspan="3" class="empty">该专题还没有文章</td></tr>'
      : arts.map(a => '<tr><td>#'+a.id+'</td><td>'+escapeHtml(a.title)+'</td><td><button class="btn-mini danger" data-topic-rm data-id="'+a.id+'">移除</button></td></tr>').join('');
    return;
  }
  const rmBtn = e.target.closest('[data-topic-rm]');
  if (rmBtn) {
    const panel = document.getElementById('topicManagePanel');
    const tid = panel.dataset.topicId;
    const body = JSON.stringify({ articleId: Number(rmBtn.dataset.id), action: 'remove' });
    await fetchJSON('/api/admin/topics/' + tid + '/articles', { method: 'POST', headers: {'Content-Type':'application/json'}, body });
    document.querySelector('[data-topic-articles][data-id="'+tid+'"]').click();
    return;
  }
});
document.addEventListener('click', async (e) => {
  const addBtn = e.target.closest('#topicAddArticleBtn');
  if (addBtn) {
    const sel = document.getElementById('topicArticleSelect');
    const aid = Number(sel.value);
    const panel = document.getElementById('topicManagePanel');
    const tid = panel.dataset.topicId;
    if (!aid || !tid) { alert('请先选择专题和文章'); return; }
    const body = JSON.stringify({ articleId: aid, action: 'add' });
    await fetchJSON('/api/admin/topics/' + tid + '/articles', { method: 'POST', headers: {'Content-Type':'application/json'}, body });
    document.querySelector('[data-topic-articles][data-id="'+tid+'"]').click();
    return;
  }
});


// ---- F5 课程管理交互 ----
let courseEditName = '';
let coursePage = 1;
const COURSE_PAGE_SIZE = 50;

async function loadAdminCourses(q, page) {
  if (page !== undefined) coursePage = page;
  if (coursePage < 1) coursePage = 1;
  const params = new URLSearchParams();
  if (q) params.set('q', q);
  params.set('page', coursePage);
  params.set('pageSize', COURSE_PAGE_SIZE);
  const d = await fetchJSON('/api/admin/courses?' + params.toString());
  const rows = (d.courses || []).map(c => `
    <tr>
      <td>${escapeHtml(c.name)}</td>
      <td>${escapeHtml(c.code)}</td>
      <td>${escapeHtml(c.college)}</td>
      <td>${escapeHtml(c.credit)}</td>
      <td>${escapeHtml(c.examType)}</td>
      <td>${c.overridden ? '<span class="pill pending">已纠错</span>' : ''}</td>
      <td><button class="btn-mini" data-course-edit data-name="${encodeURIComponent(c.name)}">编辑</button></td>
    </tr>`).join('');
  document.getElementById('courseListBody').innerHTML = rows.length
    ? rows
    : '<tr><td colspan="7" class="empty">未找到课程</td></tr>';
  // 分页信息
  const total = d.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / COURSE_PAGE_SIZE));
  const pgEl = document.getElementById('coursePager');
  if (pgEl) {
    pgEl.innerHTML = total === 0
      ? ''
      : `<button class="btn-mini" data-course-page="${coursePage - 1}" ${coursePage <= 1 ? 'disabled' : ''}>← 上一页</button>
         <span style="margin:0 10px;color:var(--muted);font-size:.85rem">第 ${coursePage} / ${totalPages} 页 · 共 ${total} 门</span>
         <button class="btn-mini" data-course-page="${coursePage + 1}" ${coursePage >= totalPages ? 'disabled' : ''}>下一页 →</button>`;
  }
}

function closeCourseEdit() {
  document.getElementById('courseEditPanel').style.display = 'none';
  document.getElementById('courseEditOverlay').style.display = 'none';
}
function openCourseEdit(name) {
  courseEditName = name;
  const panel = document.getElementById('courseEditPanel');
  panel.style.display = 'block';
  document.getElementById('courseEditOverlay').style.display = 'flex';
  document.getElementById('courseEditTitle').textContent = '编辑课程 · ' + name;
  // 从列表当前数据填表单（简化：拉取课程详情）
  fetchJSON('/api/admin/courses/' + encodeURIComponent(name)).then(d => {
    const c = d.course || {};
    document.getElementById('ceName').value = c.name || '';
    document.getElementById('ceCode').value = c.code || '';
    document.getElementById('ceCredit').value = c.credit || '';
    document.getElementById('ceExam').value = c.examType || '';
    document.getElementById('ceTag').value = c.tag || '';
    document.getElementById('ceBig').value = c.bigType || '';
    document.getElementById('ceNature').value = c.nature || '';
    document.getElementById('ceAttr').value = c.attribute || '';
    document.getElementById('ceGen').value = c.generalCat || '';
    document.getElementById('ceCollege').value = c.college || '';
  });
}

document.addEventListener('click', async (e) => {
  const pgBtn = e.target.closest('[data-course-page]');
  if (pgBtn) {
    const p = Number(pgBtn.dataset.coursePage);
    if (p >= 1) loadAdminCourses(document.getElementById('courseSearch').value.trim(), p);
    return;
  }
  const editBtn = e.target.closest('[data-course-edit]');
  if (editBtn) { openCourseEdit(decodeURIComponent(editBtn.dataset.name)); return; }
  const searchBtn = e.target.closest('#courseSearchBtn');
  if (searchBtn) { coursePage = 1; loadAdminCourses(document.getElementById('courseSearch').value.trim(), 1); return; }
  const saveBtn = e.target.closest('#ceSaveBtn');
  if (saveBtn) {
    const body = JSON.stringify({
      name: document.getElementById('ceName').value.trim(),
      code: document.getElementById('ceCode').value.trim(),
      credit: document.getElementById('ceCredit').value.trim(),
      examType: document.getElementById('ceExam').value.trim(),
      tag: document.getElementById('ceTag').value.trim(),
      bigType: document.getElementById('ceBig').value.trim(),
      nature: document.getElementById('ceNature').value.trim(),
      attribute: document.getElementById('ceAttr').value.trim(),
      generalCat: document.getElementById('ceGen').value.trim(),
      college: document.getElementById('ceCollege').value.trim()
    });
    const r = await fetchJSON('/api/admin/courses/' + encodeURIComponent(courseEditName) + '/override', {
      method: 'POST', headers: {'Content-Type':'application/json'}, body
    });
    if (r.ok) { closeCourseEdit(); loadAdminCourses(document.getElementById('courseSearch').value.trim()); }
    else alert(r.error || '保存失败');
    return;
  }
  const resetBtn = e.target.closest('#ceResetBtn');
  if (resetBtn) {
    if (!confirm('确定还原该课程为原始数据？')) return;
    const body = JSON.stringify({ code:'', credit:'', examType:'', tag:'', bigType:'', nature:'', attribute:'', generalCat:'', college:'' });
    const r = await fetchJSON('/api/admin/courses/' + encodeURIComponent(courseEditName) + '/override', {
      method: 'POST', headers: {'Content-Type':'application/json'}, body
    });
    if (r.ok) { closeCourseEdit(); loadAdminCourses(document.getElementById('courseSearch').value.trim()); }
    return;
  }
  const cancelBtn = e.target.closest('#ceCancelBtn');
  if (cancelBtn) { closeCourseEdit(); return; }
  const closeBtn = e.target.closest('#ceCloseBtn');
  if (closeBtn) { closeCourseEdit(); return; }
  // 点击遮罩关闭
  if (e.target.id === 'courseEditOverlay') { closeCourseEdit(); return; }
});

document.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && e.target && e.target.id === 'courseSearch') {
    e.preventDefault();
    coursePage = 1;
    loadAdminCourses(e.target.value.trim(), 1);
  }
});

document.addEventListener('click', async (e) => {
  const impBtn = e.target.closest('#csvImportBtn');
  if (impBtn) {
    const file = document.getElementById('csvFileInput').files[0];
    if (!file) { document.getElementById('csvMsg').textContent = '请先选择 csv 文件'; return; }
    const fd = new FormData();
    fd.append('file', file);
    const msgEl = document.getElementById('csvMsg');
    msgEl.textContent = '导入中…';
    try {
      const res = await fetch('/api/admin/courses/import', { method: 'POST', body: fd });
      const d = await res.json();
      msgEl.innerHTML = d.ok ? (ICONS.check(14)+' 导入成功，当前共 ' + d.courses + ' 门课程') : (ICONS.x(14)+' ' + (d.error || '失败'));
    } catch (err) { msgEl.innerHTML = ICONS.x(14)+' ' + err.message; }
    return;
  }
});


// ---- 课程数据中心：视图切换（课程/班次/学院）----
let courseViewCurrent = 'courses';
function showCourseView(v) {
  courseViewCurrent = v;
  ['courses','offerings','colleges'].forEach(x => {
    const el = document.getElementById('courseView-' + x);
    if (el) el.style.display = (x === v) ? '' : 'none';
  });
  document.querySelectorAll('[data-cview]').forEach(b => {
    const on = b.dataset.cview === v;
    b.style.background = on ? 'var(--magenta)' : '';
    b.style.color = on ? '#fff' : '';
  });
  if (v === 'courses') loadAdminCourses(document.getElementById('courseSearch').value.trim());
  if (v === 'offerings') loadAdminOfferings(document.getElementById('offeringSearch').value.trim());
  if (v === 'colleges') loadAdminColleges();
}

let offeringPage = 1;
const OFFERING_PAGE_SIZE = 100;
async function loadAdminOfferings(q, page) {
  if (page !== undefined) offeringPage = page;
  if (offeringPage < 1) offeringPage = 1;
  const params = new URLSearchParams();
  if (q) params.set('q', q);
  params.set('page', offeringPage);
  params.set('pageSize', OFFERING_PAGE_SIZE);
  const d = await fetchJSON('/api/admin/offerings?' + params.toString());
  const rows = (d.offerings || []).map(o => `
    <tr>
      <td>${escapeHtml(o.courseCode)}</td>
      <td>${escapeHtml(o.term)}</td>
      <td>${escapeHtml(o.teacher)}</td>
      <td>${escapeHtml(o.classGroup)}</td>
      <td title="${escapeHtml(o.time)}">${escapeHtml((o.time||'').slice(0,30))}</td>
      <td>${escapeHtml(o.location)}</td>
      <td style="white-space:nowrap">
        <button class="btn-mini" data-offer-edit
          data-code="${escapeHtml(o.courseCode)}" data-term="${escapeHtml(o.term)}"
          data-class="${escapeHtml(o.classGroup)}" data-teacher="${escapeHtml(o.teacher)}"
          data-time="${escapeHtml(o.time)}" data-location="${escapeHtml(o.location)}">编辑</button>
        <button class="btn-mini danger" data-offer-del
          data-code="${escapeHtml(o.courseCode)}" data-term="${escapeHtml(o.term)}"
          data-class="${escapeHtml(o.classGroup)}">删除</button>
      </td>
    </tr>`).join('');
  document.getElementById('offeringListBody').innerHTML = rows.length
    ? rows
    : '<tr><td colspan="6" class="empty">未找到班次</td></tr>';
  const total = d.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / OFFERING_PAGE_SIZE));
  const pgEl = document.getElementById('offeringPager');
  if (pgEl) {
    pgEl.innerHTML = total === 0
      ? ''
      : `<button class="btn-mini" data-offer-page="${offeringPage - 1}" ${offeringPage <= 1 ? 'disabled' : ''}>← 上一页</button>
         <span style="margin:0 10px;color:var(--muted);font-size:.85rem">第 ${offeringPage} / ${totalPages} 页 · 共 ${total} 条</span>
         <button class="btn-mini" data-offer-page="${offeringPage + 1}" ${offeringPage >= totalPages ? 'disabled' : ''}>下一页 →</button>`;
  }
}

async function loadAdminColleges() {
  const d = await fetchJSON('/api/admin/colleges');
  const rows = (d.colleges || []).map(c => `
    <tr>
      <td>${escapeHtml(c.displayName || c.name)}</td>
      <td>${escapeHtml(c.scheduleName || '')}</td>
      <td>${escapeHtml(c.category || '')}</td>
      <td>${c.courseCount}</td>
      <td style="max-width:180px;font-size:.8rem;color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis" title="${escapeHtml(c.majors || '')}">${escapeHtml(c.majors || '')}</td>
      <td style="white-space:nowrap">
        <button class="btn-mini" data-college-edit data-name="${encodeURIComponent(c.name)}" data-display="${encodeURIComponent(c.displayName || '')}" data-schedule="${encodeURIComponent(c.scheduleName || '')}" data-category="${encodeURIComponent(c.category || '')}" data-majors="${encodeURIComponent(c.majors || '')}">编辑</button>
        <button class="btn-mini" data-college-courses data-name="${encodeURIComponent(c.name)}">看课程</button>
      </td>
    </tr>`).join('');
  document.getElementById('collegeListBody').innerHTML = rows.length
    ? rows
    : '<tr><td colspan="3" class="empty">暂无学院</td></tr>';
}

document.addEventListener('click', async (e) => {
  const offPg = e.target.closest('[data-offer-page]');
  if (offPg) {
    const p = Number(offPg.dataset.offerPage);
    if (p >= 1) loadAdminOfferings(document.getElementById('offeringSearch').value.trim(), p);
    return;
  }
  const viewBtn = e.target.closest('[data-cview]');
  if (viewBtn) { showCourseView(viewBtn.dataset.cview); return; }
  const statBtn = e.target.closest('[data-stat]');
  if (statBtn) {
    showAdminTab('courses');
    showCourseView(statBtn.dataset.stat === 'offerings' ? 'offerings' : (statBtn.dataset.stat === 'colleges' ? 'colleges' : 'courses'));
    return;
  }
  const offSearch = e.target.closest('#offeringSearchBtn');
  if (offSearch) { loadAdminOfferings(document.getElementById('offeringSearch').value.trim()); return; }
  const colBtn = e.target.closest('[data-college-courses]');
  if (colBtn) {
    showCourseView('courses');
    document.getElementById('courseSearch').value = decodeURIComponent(colBtn.dataset.name);
    loadAdminCourses(decodeURIComponent(colBtn.dataset.name));
    return;
  }
});
document.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && e.target && e.target.id === 'offeringSearch') {
    e.preventDefault();
    loadAdminOfferings(e.target.value.trim());
  }
});


// ---- 班次编辑/删除交互 ----
function openOfferingEdit(btn) {
  document.getElementById('oeCode').value = btn.dataset.code;
  document.getElementById('oeTerm').value = btn.dataset.term;
  document.getElementById('oeClass').value = btn.dataset.class;
  document.getElementById('oeTeacher').value = btn.dataset.teacher || '';
  document.getElementById('oeTime').value = btn.dataset.time || '';
  document.getElementById('oeLocation').value = btn.dataset.location || '';
  document.getElementById('offeringEditPanel').style.display = 'block';
  document.getElementById('offeringEditOverlay').style.display = 'flex';
}
function closeOfferingEdit() {
  document.getElementById('offeringEditPanel').style.display = 'none';
  document.getElementById('offeringEditOverlay').style.display = 'none';
}
document.addEventListener('click', async (e) => {
  const editBtn = e.target.closest('[data-offer-edit]');
  if (editBtn) { openOfferingEdit(editBtn); return; }
  const delBtn = e.target.closest('[data-offer-del]');
  if (delBtn) {
    if (!confirm('确定删除该班次？删除后不再显示（持久化）。')) return;
    await fetchJSON('/api/admin/offerings/delete', {
      method: 'POST', headers: {'Content-Type':'application/json'},
      body: JSON.stringify({ code: delBtn.dataset.code, term: delBtn.dataset.term, classGroup: delBtn.dataset.class })
    });
    loadAdminOfferings(document.getElementById('offeringSearch').value.trim(), offeringPage);
    return;
  }
  const saveBtn = e.target.closest('#oeSaveBtn');
  if (saveBtn) {
    const body = JSON.stringify({
      code: document.getElementById('oeCode').value.trim(),
      term: document.getElementById('oeTerm').value.trim(),
      classGroup: document.getElementById('oeClass').value.trim(),
      teacher: document.getElementById('oeTeacher').value.trim(),
      time: document.getElementById('oeTime').value.trim(),
      location: document.getElementById('oeLocation').value.trim()
    });
    await fetchJSON('/api/admin/offerings/edit', { method: 'POST', headers: {'Content-Type':'application/json'}, body });
    closeOfferingEdit();
    loadAdminOfferings(document.getElementById('offeringSearch').value.trim(), offeringPage);
    return;
  }
  const cancelBtn = e.target.closest('#oeCancelBtn');
  if (cancelBtn) { closeOfferingEdit(); return; }
  const closeBtn = e.target.closest('#offeringCloseBtn');
  if (closeBtn) { closeOfferingEdit(); return; }
  if (e.target.id === 'offeringEditOverlay') { closeOfferingEdit(); return; }
});


// ---- 学院编辑交互 ----
function openCollegeEdit(btn) {
  document.getElementById('ceColName').value = decodeURIComponent(btn.dataset.name);
  document.getElementById('ceColDisplay').value = btn.dataset.display ? decodeURIComponent(btn.dataset.display) : '';
  document.getElementById('ceColSchedule').value = btn.dataset.schedule ? decodeURIComponent(btn.dataset.schedule) : '';
  document.getElementById('ceColCategory').value = btn.dataset.category ? decodeURIComponent(btn.dataset.category) : '';
  // 专业：按行长格式显示；存储时用逗号分隔
  document.getElementById('ceColMajors').value = (btn.dataset.majors ? decodeURIComponent(btn.dataset.majors) : '').split(',').filter(Boolean).join('\n');
  document.getElementById('collegeEditTitle').textContent = '编辑学院 · ' + decodeURIComponent(btn.dataset.name);
  document.getElementById('collegeEditPanel').style.display = 'block';
  document.getElementById('collegeEditOverlay').style.display = 'flex';
}
function closeCollegeEdit() {
  document.getElementById('collegeEditPanel').style.display = 'none';
  document.getElementById('collegeEditOverlay').style.display = 'none';
}
document.addEventListener('click', async (e) => {
  const editBtn = e.target.closest('[data-college-edit]');
  if (editBtn) { openCollegeEdit(editBtn); return; }
  const saveBtn = e.target.closest('#collegeSaveBtn');
  if (saveBtn) {
    const body = JSON.stringify({
      name: document.getElementById('ceColName').value.trim(),
      displayName: document.getElementById('ceColDisplay').value.trim(),
      scheduleName: document.getElementById('ceColSchedule').value.trim(),
      category: document.getElementById('ceColCategory').value.trim(),
      majors: document.getElementById('ceColMajors').value.split(/\n|\r\n/).map(x => x.trim()).filter(Boolean).join(',')
    });
    try {
      await fetchJSON('/api/admin/college', { method: 'POST', headers: {'Content-Type':'application/json'}, body });
      closeCollegeEdit();
      loadAdminColleges();
    } catch (err) { alert('保存失败: ' + err.message); }
    return;
  }
  const cancelBtn = e.target.closest('#collegeCancelBtn');
  if (cancelBtn) { closeCollegeEdit(); return; }
  const closeBtn = e.target.closest('#collegeCloseBtn');
  if (closeBtn) { closeCollegeEdit(); return; }
  if (e.target.id === 'collegeEditOverlay') { closeCollegeEdit(); return; }
});


// ---- 公告列表加载 ----
let annPage = 1;
const ANN_PAGE_SIZE = 30;
async function loadAnnouncements(page) {
  const body = document.getElementById('annListBody');
  if (!body) return;
  if (page !== undefined) annPage = page;
  if (annPage < 1) annPage = 1;
  try {
    const d = await fetchJSON('/api/admin/announcement?page=' + annPage + '&pageSize=' + ANN_PAGE_SIZE);
    const list = d.announcements || [];
    body.innerHTML = list.length === 0
      ? '<tr><td colspan="5" class="empty">还没有发布过公告</td></tr>'
      : list.map(function (a) {
          const st = a.status || '展示中';
          const pill = st === '展示中' ? 'admin' : 'banned';
          const label = st === '展示中' ? '&#10004; 展示中' : '已下架';
          return '<tr>' +
            '<td>#' + escapeHtml(a.id) + '</td>' +
            '<td style="max-width:360px">' + escapeHtml(a.content) + '</td>' +
            '<td style="white-space:nowrap">' + escapeHtml(a.createdAt) + '</td>' +
            '<td><span class="pill ' + pill + '">' + label + '</span></td>' +
            '<td style="white-space:nowrap">' +
              (st === '展示中' ? '<button class="btn-mini danger" data-ann-status data-id="' + a.id + '" data-status="展示中">下架</button> ' : '<button class="btn-mini" data-ann-status data-id="' + a.id + '" data-status="已下架">恢复</button> ') +
              '<button class="btn-mini danger" data-ann-del data-id="' + a.id + '">删除</button>' +
            '</td></tr>';
        }).join('');
    const total = d.total || 0;
    pagerHtml('annPager', annPage, Math.max(1, Math.ceil(total / ANN_PAGE_SIZE)), total);
  } catch (e) { body.innerHTML = '<tr><td colspan="5" class="empty">加载失败</td></tr>'; }
}
// ---- 举报/资料/评价/文章/帖子 列表加载（含搜索）----
let curReportQ = '', curFileQ = '', curReviewQ = '', curArticleQ = '', curPostQ = '';
let postPage = 1, reportPage = 1, filePage = 1, reviewPage = 1, articlePage = 1, censorPage = 1, topicPage = 1;
const POST_PAGE_SIZE = 50;
const LIST_PAGE_SIZE = 30;
const CENSOR_PAGE_SIZE = 30;
const TOPIC_PAGE_SIZE = 20;

// 通用分页控件渲染（按函数名生成 data-page-fn/data-page 按钮）
function pagerHtml(pgId, cur, totalPages, total) {
  const el = document.getElementById(pgId);
  if (!el) return;
  const fnMap = { reportPager: 'loadAdminReports', filePager: 'loadAdminFiles', reviewPager: 'loadAdminReviews', articlePager: 'loadAdminArticles', censorPager: 'loadCensor', topicPager: 'loadAdminTopics', annPager: 'loadAnnouncements' };
  el.innerHTML = '<button class="btn-mini" data-page-fn="' + (fnMap[pgId] || '') + '" data-page="' + (cur - 1) + '" ' + (cur <= 1 ? 'disabled' : '') + '>← 上一页</button>' +
    '<span style="margin:0 10px;color:var(--muted);font-size:.85rem">第 ' + cur + ' / ' + totalPages + ' 页 · 共 ' + total + ' 条</span>' +
    '<button class="btn-mini" data-page-fn="' + (fnMap[pgId] || '') + '" data-page="' + (cur + 1) + '" ' + (cur >= totalPages ? 'disabled' : '') + '>下一页 →</button>';
}
// 数据总览统计刷新（举报处理等操作后更新数字）
async function refreshStats() {
  try {
    const stat = await fetchJSON('/api/admin/dashboard');
    const map = { reports: stat.reportPending, users: stat.userCount, posts: stat.postCount, reviews: stat.reviewCount, files: stat.fileCount };
    document.querySelectorAll('.stat-btn[data-tab]').forEach(function (b) {
      const num = b.querySelector('.stat-num');
      if (num && map[b.dataset.tab] !== undefined) num.textContent = map[b.dataset.tab];
    });
    document.querySelectorAll('.stat-btn[data-stat]').forEach(function (b) {
      const num = b.querySelector('.stat-num');
      if (!num) return;
      const v = b.dataset.stat === 'courses' ? stat.courseCount : b.dataset.stat === 'offerings' ? stat.offeringCount : b.dataset.stat === 'colleges' ? stat.collegeCount : null;
      if (v !== null) num.textContent = v;
    });
  } catch (e) {}
}

// 分页按钮委托（通用）
document.addEventListener('click', function (e) {
  const b = e.target.closest('[data-page-fn]');
  if (!b) return;
  const pnum = Number(b.dataset.page);
  if (pnum < 1) return;
  const fn = b.dataset.pageFn;
  if (fn === 'loadAdminPosts') loadAdminPosts(pnum);
  else if (fn === 'loadAnnouncements') loadAnnouncements(pnum);
  else if (fn === 'loadCensor') loadCensor(pnum);
  else if (fn === 'loadAdminTopics') loadAdminTopics(pnum);
  else if (fn === 'loadAdminReports') loadAdminReports(pnum);
  else if (fn === 'loadAdminFiles') loadAdminFiles(pnum);
  else if (fn === 'loadAdminReviews') loadAdminReviews(pnum);
  else if (fn === 'loadAdminArticles') loadAdminArticles(pnum);
});

function reportViewUrl(r) {
  const id = String(r.targetId || '');
  switch (r.targetType) {
    case 'post': return '/post.html?id=' + id;
    case 'article': return '/article.html?id=' + id;
    case 'file': return '/viewer.html?id=' + id;
    case 'review': return '/course.html?code=' + encodeURIComponent(r.targetPreview || '');
    case 'comment': return r.commentPostId ? '/post.html?id=' + r.commentPostId : null;
  }
  return null;
}

function authorLink(uid, name) {
  return uid ? '<a href="/user.html?id=' + uid + '" target="_blank" style="color:var(--magenta);text-decoration:none;font-weight:600">' + escapeHtml(name) + ' ↗</a>' : escapeHtml(name || '—');
}

async function loadAdminReports(page) {
  const body = document.getElementById('reportListBody');
  if (!body) return;
  if (page !== undefined) reportPage = page;
  if (reportPage < 1) reportPage = 1;
  const q = (document.getElementById('reportSearch') || {}).value || '';
  curReportQ = q;
  try {
    const d = await fetchJSON('/api/admin/reports?q=' + encodeURIComponent(q) + '&page=' + reportPage + '&pageSize=' + LIST_PAGE_SIZE);
    const list = d.reports || [];
    const typeLabel = { post: '帖子', article: '文章', file: '资料', review: '评价', comment: '评论' };
    body.innerHTML = list.length === 0
      ? '<tr><td colspan="7" class="empty">暂无举报</td></tr>'
      : list.map(function (r) {
          const url = reportViewUrl(r);
          return '<tr>' +
            '<td>#' + escapeHtml(r.id) + '</td>' +
            '<td><span class="pill ' + (r.status === '待处理' ? 'pending' : 'user') + '">' + escapeHtml(typeLabel[r.targetType] || r.targetType) + '</span></td>' +
            '<td style="max-width:240px">' + (url ? '<a class="btn-mini" href="' + url + '" target="_blank">查看</a> ' : '') + escapeHtml((r.targetPreview || '#' + r.targetId).slice(0, 40)) + '</td>' +
            '<td>' + authorLink(r.targetAuthorId, r.targetAuthor || '—') + '</td>' +
            '<td style="max-width:160px">' + escapeHtml(r.reason) + '</td>' +
            '<td><span class="pill ' + (r.status === '待处理' ? 'pending' : 'user') + '">' + escapeHtml(r.status) + '</span></td>' +
            '<td style="white-space:nowrap">' +
              '<button class="btn-mini danger" data-report-resolve data-id="' + r.id + '" data-act="remove">下架</button> ' +
              '<button class="btn-mini" data-report-resolve data-id="' + r.id + '" data-act="ignore">忽略</button>' +
            '</td></tr>';
        }).join('');
    const total = d.total || 0;
    pagerHtml('reportPager', reportPage, Math.max(1, Math.ceil(total / LIST_PAGE_SIZE)), total);
  } catch (e) { body.innerHTML = '<tr><td colspan="7" class="empty">加载失败</td></tr>'; }
}

async function loadAdminFiles(page) {
  const body = document.getElementById('fileListBody');
  if (!body) return;
  if (page !== undefined) filePage = page;
  if (filePage < 1) filePage = 1;
  const q = (document.getElementById('fileSearch') || {}).value || '';
  curFileQ = q;
  try {
    const d = await fetchJSON('/api/admin/files?q=' + encodeURIComponent(q) + '&page=' + filePage + '&pageSize=' + LIST_PAGE_SIZE);
    const list = d.files || [];
    body.innerHTML = list.length === 0
      ? '<tr><td colspan="8" class="empty">暂无上传资料</td></tr>'
      : list.map(function (f) {
          return '<tr>' +
            '<td>#' + escapeHtml(f.id) + '</td>' +
            '<td><a href="/viewer.html?id=' + f.id + '&name=' + encodeURIComponent(f.fileName) + '" target="_blank" style="color:var(--magenta);text-decoration:none">' + escapeHtml(f.fileName) + '</a></td>' +
            '<td>' + escapeHtml(f.courseCode) + '</td>' +
            '<td>' + escapeHtml(f.category || '—') + '</td>' +
            '<td style="color:var(--muted);font-weight:600">' + (f.anonymous ? '匿名' : '实名') + '</td>' +
            '<td>' + authorLink(f.uploaderId, f.realAuthor) + '</td>' +
            '<td><span class="pill ' + (f.status === '已下架' ? 'pending' : 'user') + '">' + escapeHtml(f.status) + '</span></td>' +
            '<td style="white-space:nowrap"><button class="btn-mini ' + (f.status === '已下架' ? '' : 'danger') + '" data-file-status data-id="' + f.id + '" data-status="' + f.status + '">' + (f.status === '已下架' ? '恢复' : '下架') + '</button></td>' +
          '</tr>';
        }).join('');
    const total = d.total || 0;
    pagerHtml('filePager', filePage, Math.max(1, Math.ceil(total / LIST_PAGE_SIZE)), total);
  } catch (e) { body.innerHTML = '<tr><td colspan="8" class="empty">加载失败</td></tr>'; }
}

async function loadAdminReviews(page) {
  const body = document.getElementById('reviewListBody');
  if (!body) return;
  if (page !== undefined) reviewPage = page;
  if (reviewPage < 1) reviewPage = 1;
  const q = (document.getElementById('reviewSearch') || {}).value || '';
  curReviewQ = q;
  try {
    const d = await fetchJSON('/api/admin/reviews?q=' + encodeURIComponent(q) + '&page=' + reviewPage + '&pageSize=' + LIST_PAGE_SIZE);
    const list = d.reviews || [];
    body.innerHTML = list.length === 0
      ? '<tr><td colspan="8" class="empty">暂无课程评价</td></tr>'
      : list.map(function (re) {
          const stars = '★'.repeat(Math.max(0, Math.min(5, re.rating || 0))) + '☆'.repeat(5 - Math.max(0, Math.min(5, re.rating || 0)));
          return '<tr>' +
            '<td>#' + escapeHtml(re.id) + '</td>' +
            '<td><a href="/course.html?code=' + encodeURIComponent(re.courseCode) + '" target="_blank" style="color:var(--magenta);text-decoration:none">' + escapeHtml(re.courseCode) + '</a></td>' +
            '<td>' + stars + '</td>' +
            '<td style="max-width:280px">' + escapeHtml(re.content) + '</td>' +
            '<td style="color:var(--muted);font-weight:600">' + (re.anonymous ? '匿名' : '实名') + '</td>' +
            '<td>' + authorLink(re.userId, re.realAuthor) + '</td>' +
            '<td><span class="pill ' + (re.status === '已下架' ? 'pending' : 'user') + '">' + escapeHtml(re.status) + '</span></td>' +
            '<td style="white-space:nowrap"><button class="btn-mini ' + (re.status === '已下架' ? '' : 'danger') + '" data-review-status data-id="' + re.id + '" data-status="' + re.status + '">' + (re.status === '已下架' ? '恢复' : '下架') + '</button></td>' +
          '</tr>';
        }).join('');
    const total = d.total || 0;
    pagerHtml('reviewPager', reviewPage, Math.max(1, Math.ceil(total / LIST_PAGE_SIZE)), total);
  } catch (e) { body.innerHTML = '<tr><td colspan="8" class="empty">加载失败</td></tr>'; }
}

async function loadAdminArticles(page) {
  const body = document.getElementById('articleListBody');
  if (!body) return;
  if (page !== undefined) articlePage = page;
  if (articlePage < 1) articlePage = 1;
  const q = (document.getElementById('articleSearch') || {}).value || '';
  curArticleQ = q;
  try {
    const d = await fetchJSON('/api/admin/articles?q=' + encodeURIComponent(q) + '&page=' + articlePage + '&pageSize=' + LIST_PAGE_SIZE);
    const list = d.articles || [];
    const pill = function (st) {
      const cls = st === '待审' ? 'pending' : (st === '已驳回' || st === '已下架') ? 'banned' : 'user';
      return '<span class="pill ' + cls + '">' + escapeHtml(st) + '</span>';
    };
    body.innerHTML = list.length === 0
      ? '<tr><td colspan="8" class="empty">暂无文章</td></tr>'
      : list.map(function (art) {
          return '<tr>' +
            '<td>#' + escapeHtml(art.id) + '</td>' +
            '<td style="max-width:200px">' + escapeHtml(art.title) + '</td>' +
            '<td>' + escapeHtml(art.category) + '</td>' +
            '<td style="color:var(--muted);font-weight:600">' + (art.anonymous ? '匿名' : '实名') + '</td>' +
            '<td>' + authorLink(art.userId, art.realAuthor) + '</td>' +
            '<td style="max-width:140px">' + (art.rejectReason ? '<span class="pill banned" title="' + escapeHtml(art.rejectReason) + '">ⓘ 驳回理由</span>' : '') + '</td>' +
            '<td>' + pill(art.status) + '</td>' +
            '<td style="white-space:nowrap">' +
              '<button class="btn-mini" data-view-article data-id="' + art.id + '">查看</button> ' +
              (art.status !== '正常' ? '<button class="btn-mini" data-article-review data-id="' + art.id + '" data-act="approve">通过</button> ' : '') +
              (art.status === '待审' ? '<button class="btn-mini danger" data-article-review data-id="' + art.id + '" data-act="reject">驳回</button> ' : '') +
              (art.status === '正常' ? '<button class="btn-mini danger" data-article-status data-id="' + art.id + '" data-status="正常">下架</button>' : (art.status === '已下架' ? '<button class="btn-mini" data-article-status data-id="' + art.id + '" data-status="已下架">恢复</button>' : '')) +
            '</td></tr>';
        }).join('');
    const total = d.total || 0;
    pagerHtml('articlePager', articlePage, Math.max(1, Math.ceil(total / LIST_PAGE_SIZE)), total);
  } catch (e) { body.innerHTML = '<tr><td colspan="8" class="empty">加载失败</td></tr>'; }
}

async function loadAdminPosts(page) {
  const body = document.getElementById('postListBody');
  if (!body) return;
  if (page !== undefined) postPage = page;
  if (postPage < 1) postPage = 1;
  const q = (document.getElementById('postSearch') || {}).value || '';
  curPostQ = q;
  const params = new URLSearchParams();
  if (q) params.set('q', q);
  params.set('page', postPage);
  params.set('pageSize', POST_PAGE_SIZE);
  try {
    const d = await fetchJSON('/api/admin/posts?' + params.toString());
    const list = d.posts || [];
    body.innerHTML = list.length === 0
      ? '<tr><td colspan="9" class="empty">暂无帖子</td></tr>'
      : list.map(function (pt) {
          return '<tr>' +
            '<td>#' + escapeHtml(pt.id) + '</td>' +
            '<td>' + escapeHtml(pt.forum) + '</td>' +
            '<td style="max-width:180px">' + escapeHtml(pt.title) + '</td>' +
            '<td style="max-width:200px;color:var(--muted)">' + escapeHtml((pt.content || '').slice(0, 20)) + '</td>' +
            '<td style="color:var(--muted);font-weight:600">' + (pt.anonymous ? '匿名' : '实名') + '</td>' +
            '<td>' + authorLink(pt.userId, pt.realAuthor) + '</td>' +
            '<td><span class="pill ' + (pt.status === '正常' ? 'user' : (pt.status === 'draft' ? 'pending' : 'banned')) + '">' + escapeHtml(pt.status === '正常' ? '正常' : pt.status === 'draft' ? '草稿' : '已下架') + '</span></td>' +
            '<td style="white-space:nowrap">' + escapeHtml(pt.createdAt) + '</td>' +
            '<td style="white-space:nowrap">' +
              '<a class="btn-mini" href="/post.html?id=' + pt.id + '" target="_blank" style="text-decoration:none">查看</a> ' +
              (pt.status === '正常' ? '<button class="btn-mini danger" data-post-status data-id="' + pt.id + '" data-status="正常">下架</button>' : (pt.status === '已下架' ? '<button class="btn-mini" data-post-status data-id="' + pt.id + '" data-status="已下架">恢复</button>' : '')) +
            '</td></tr>';
        }).join('');
    const total = d.total || 0;
    const totalPages = Math.max(1, Math.ceil(total / POST_PAGE_SIZE));
    const pg = document.getElementById('postPager');
    if (pg) pg.innerHTML = '<button class="btn-mini" data-post-page="' + (postPage - 1) + '" ' + (postPage <= 1 ? 'disabled' : '') + '>← 上一页</button>' +
      '<span style="margin:0 10px;color:var(--muted);font-size:.85rem">第 ' + postPage + ' / ' + totalPages + ' 页 · 共 ' + total + ' 条</span>' +
      '<button class="btn-mini" data-post-page="' + (postPage + 1) + '" ' + (postPage >= totalPages ? 'disabled' : '') + '>下一页 →</button>';
  } catch (e) { body.innerHTML = '<tr><td colspan="9" class="empty">加载失败</td></tr>'; }
}

// 搜索按钮/回车 统一绑定（面板重渲染后依然有效：事件委托）
document.addEventListener('click', function (e) {
  const map = {
    userSearchBtn: function () { userPage = 1; loadAdminUsers(); },
    reportSearchBtn: function () { reportPage = 1; loadAdminReports(1); },
    fileSearchBtn: function () { filePage = 1; loadAdminFiles(1); },
    reviewSearchBtn: function () { reviewPage = 1; loadAdminReviews(1); },
    articleSearchBtn: function () { articlePage = 1; loadAdminArticles(1); },
    postSearchBtn: function () { postPage = 1; loadAdminPosts(1); }
  };
  if (e.target.id && map[e.target.id]) { map[e.target.id](); }
});
document.addEventListener('keydown', function (e) {
  if (e.key !== 'Enter') return;
  const ids = ['userSearch', 'reportSearch', 'fileSearch', 'reviewSearch', 'articleSearch', 'postSearch'];
  if (e.target && e.target.id && ids.indexOf(e.target.id) >= 0) {
    e.preventDefault();
    const map = { userSearch: 'loadAdminUsers', reportSearch: 'loadAdminReports', fileSearch: 'loadAdminFiles', reviewSearch: 'loadAdminReviews', articleSearch: 'loadAdminArticles', postSearch: 'loadAdminPosts' };
    if (e.target.id === 'postSearch') { postPage = 1; loadAdminPosts(1); }
    else if (e.target.id === 'userSearch') { userPage = 1; loadAdminUsers(); }
    else { window[map[e.target.id]](1); }
  }
});

// ---- 敏感词/专题 懒加载（分页）----
async function loadCensor(page) {
  const body = document.getElementById('censorListBody');
  if (!body) return;
  if (page !== undefined) censorPage = page;
  if (censorPage < 1) censorPage = 1;
  try {
    const d = await fetchJSON('/api/admin/censor?page=' + censorPage + '&pageSize=' + CENSOR_PAGE_SIZE);
    const words = d.words || [];
    body.innerHTML = words.length === 0
      ? '<tr><td colspan="4" class="empty">暂无敏感词，发布内容不设拦截</td></tr>'
      : words.map(function (cw) {
          return '<tr><td>#' + escapeHtml(cw.id) + '</td><td style="font-weight:600;color:var(--magenta)">' + escapeHtml(cw.word) + '</td><td>' + escapeHtml(cw.createdAt || '—') + '</td><td><button class="btn-mini danger" data-censor-del data-id="' + cw.id + '">删除</button></td></tr>';
        }).join('');
    const total = d.total || 0;
    pagerHtml('censorPager', censorPage, Math.max(1, Math.ceil(total / CENSOR_PAGE_SIZE)), total);
  } catch (e) { body.innerHTML = '<tr><td colspan="4" class="empty">加载失败</td></tr>'; }
}
async function loadAdminTopics(page) {
  const body = document.getElementById('topicListBody');
  if (!body) return;
  if (page !== undefined) topicPage = page;
  if (topicPage < 1) topicPage = 1;
  try {
    const d = await fetchJSON('/api/topics?page=' + topicPage + '&pageSize=' + TOPIC_PAGE_SIZE);
    const topics = d.topics || [];
    body.innerHTML = topics.length === 0
      ? '<tr><td colspan="4" class="empty">暂无专题</td></tr>'
      : topics.map(function (tp) {
          return '<tr><td>#' + tp.id + '</td><td>' + escapeHtml(tp.title) + '</td><td>' + tp.articleCount + '</td><td><button class="btn-mini" data-topic-articles data-id="' + tp.id + '">管理文章</button> <button class="btn-mini danger" data-topic-del data-id="' + tp.id + '">删除</button></td></tr>';
        }).join('');
    const total = d.total || 0;
    pagerHtml('topicPager', topicPage, Math.max(1, Math.ceil(total / TOPIC_PAGE_SIZE)), total);
  } catch (e) { body.innerHTML = '<tr><td colspan="4" class="empty">加载失败</td></tr>'; }
}

// ---- 审计日志加载 ----
let auditPage = 1;
let userPage = 1;
async function loadAdminUsers() {
  const body = document.getElementById('userListBody');
  if (!body) return;
  try {
    const q = (document.getElementById('userSearch') || {}).value || '';
    const d = await fetchJSON('/api/admin/users?page=' + userPage + '&pageSize=50&q=' + encodeURIComponent(q));
    const users = d.users || [];
    const rows = users.map(u => `
      <tr>
        <td>${escapeHtml(u.id)}</td>
        <td>${escapeHtml(u.email)}</td>
        <td>${escapeHtml(u.nickname || '—')}</td>
        <td>${escapeHtml(u.college || '—')}</td>
        <td>${escapeHtml(u.major || '—')}</td>
        <td><span class="pill ${u.banned ? 'banned' : (u.isAdmin ? 'admin' : 'user')}">${u.banned ? '已封禁' : (u.muted ? '已禁言' : (u.isAdmin ? '管理员' : '用户'))}</span></td>
        <td style="white-space:nowrap">
          <button class="btn-mini" data-role-toggle data-id="${u.id}" data-now="${u.isAdmin}">${u.isAdmin ? '取消管理' : '设管理员'}</button>
          <button class="btn-mini ${u.banned ? '' : 'danger'}" data-user-ban data-id="${u.id}" data-banned="${u.banned ? 1 : 0}">${u.banned ? '解封' : '封禁'}</button>
          <button class="btn-mini ${u.muted ? '' : 'danger'}" data-user-mute data-id="${u.id}" data-muted="${u.muted ? 1 : 0}">${u.muted ? '解除禁言' : '禁言'}</button>
        </td>
      </tr>`).join('');
    body.innerHTML = rows.length ? rows : '<tr><td colspan="7" class="empty">暂无用户</td></tr>';
    const total = d.total || 0;
    const totalPages = Math.max(1, Math.ceil(total / 50));
    const pg = document.getElementById('userPager');
    if (pg) pg.innerHTML = `<button class="btn-mini" data-user-page="${userPage-1}" ${userPage<=1?'disabled':''}>← 上一页</button>
      <span style="margin:0 10px;color:var(--muted);font-size:.85rem">第 ${userPage} / ${totalPages} 页 · 共 ${total} 人</span>
      <button class="btn-mini" data-user-page="${userPage+1}" ${userPage>=totalPages?'disabled':''}>下一页 →</button>`;
  } catch (e) { body.innerHTML = '<tr><td colspan="7" class="empty">加载失败</td></tr>'; }
}
// 后台文章预览弹层（不跳转，退出留在当前列表）
async function viewArticlePreview(id) {
  try {
    const d = await (await fetch('/api/articles/' + id)).json();
    const a = d.article;
    if (!a) { alert('文章不存在'); return; }
    const overlay = document.createElement('div');
    overlay.className = 'modal';
    overlay.style.display = 'flex';
    overlay.innerHTML = '<div class="modal-box" style="width:min(720px,94vw);max-height:86vh;overflow:auto">' +
      '<div class="modal-head"><h3>' + escapeHtml(a.title) + '</h3>' +
      '<button class="modal-close" id="avClose">' + ICONS.close(14) + '</button></div>' +
      '<div style="color:var(--muted);font-size:.82rem;margin-bottom:10px">' + escapeHtml(a.category) + ' · ' + escapeHtml(a.author) + ' · ' + escapeHtml(a.createdAt) + ' · 阅读 ' + (a.views||0) + '</div>' +
      '<div class="md-view" style="line-height:1.8;font-size:.95rem">' + (window.renderMarkdown ? renderMarkdown(a.content||'') : '<pre style="white-space:pre-wrap">' + escapeHtml(a.content||'') + '</pre>') + '</div>' +
      '<div style="margin-top:14px;display:flex;gap:10px">' +
      '<a class="btn-mini" href="/article.html?id=' + id + '" target="_blank" style="text-decoration:none">在新标签打开</a>' +
      '</div></div>';
    document.body.appendChild(overlay);
    overlay.querySelector('#avClose').onclick = () => overlay.remove();
    overlay.addEventListener('click', function (e) { if (e.target === overlay) overlay.remove(); });
  } catch (e) { alert('加载失败'); }
}
async function loadAudit() {
  const d = await fetchJSON('/api/admin/audit?page=' + auditPage + '&pageSize=50');
  const rows = (d.logs || []).map(l => `
    <tr>
      <td style="white-space:nowrap">${escapeHtml(l.createdAt)}</td>
      <td>${escapeHtml(l.opName || l.operator)}</td>
      <td>${escapeHtml(l.action)}</td>
      <td>${escapeHtml(l.detail)}</td>
    </tr>`).join('');
  document.getElementById('auditListBody').innerHTML = rows.length ? rows : '<tr><td colspan="4" class="empty">暂无审计日志</td></tr>';
  const total = d.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / 50));
  const pgEl = document.getElementById('auditPager');
  if (pgEl) pgEl.innerHTML = `<button class="btn-mini" data-audit-page="${auditPage-1}" ${auditPage<=1?'disabled':''}>← 上一页</button>
    <span style="margin:0 10px;color:var(--muted);font-size:.85rem">第 ${auditPage} / ${totalPages} 页 · 共 ${total} 条</span>
    <button class="btn-mini" data-audit-page="${auditPage+1}" ${auditPage>=totalPages?'disabled':''}>下一页 →</button>`;
}

// ---- 维护模式 ----
async function loadMaintenance() {
  const d = await fetchJSON('/api/admin/maintenance');
  const on = (d.enabled === true || d.enabled === 1);
  document.getElementById('maintStatus').textContent = on ? '维护中' : '运行中';
  document.getElementById('maintStatus').style.color = on ? '#c0392b' : '#2e7d32';
  document.getElementById('maintBtn').textContent = on ? '关闭维护' : '开启维护';
  document.getElementById('maintBtn').dataset.state = on ? 'off' : 'on';
  document.getElementById('maintMsg').value = d.msg || '';
}

// 在 showAdminTab 切换时，若切到 audit/maintenance 则加载（通过 showAdminTab 的 tab 检测）
document.addEventListener('click', async (e) => {
  // showAdminTab 的点击已由 .admin-tab 绑定，这里补 audit 分页按钮
  const auditPg = e.target.closest('[data-audit-page]');
  if (auditPg) {
    const p = Number(auditPg.dataset.auditPage);
    if (p >= 1) { auditPage = p; loadAudit(); }
    return;
  }
  const maintBtn = e.target.closest('#maintBtn');
  if (maintBtn) {
    const turnOn = maintBtn.dataset.state === 'on';
    const msg = document.getElementById('maintMsg').value.trim();
    if (turnOn && !confirm('确定开启维护模式？普通用户将无法访问本站。')) return;
    try {
      const r = await fetch('/api/admin/maintenance', {
        method: 'POST', headers: {'Content-Type':'application/json'},
        body: JSON.stringify({ enabled: turnOn ? 1 : 0, msg })
      });
      const d = await r.json();
      if (!d.ok) { alert('切换维护模式失败，请稍后再试'); return; }
      loadMaintenance();
    } catch (err) { alert('切换维护模式失败，请稍后再试'); }
    return;
  }
  const backupBtn = e.target.closest('#backupBtn');
  if (backupBtn) {
    const msg = document.getElementById('backupMsg');
    if (!msg) return;
    msg.textContent = '备份中…';
    backupBtn.disabled = true;
    try {
      const r = await fetch('/api/admin/backup', { method: 'POST' });
      const d = await r.json();
      if (!r.ok) { msg.textContent = d.error || '备份失败'; return; }
      msg.innerHTML = ICONS.check(14) + ' 备份完成：' + (d.size / 1024 / 1024).toFixed(1) + ' MB → ' + d.db + (d.pruned ? '（已清理旧备份 ' + d.pruned + ' 份）' : '');
    } catch (e) { msg.textContent = '网络错误'; }
    finally { backupBtn.disabled = false; }
  }
});


// ---- 意见箱 ----
let opinionPage = 1;
async function loadOpinions() {
  const d = await fetchJSON('/api/admin/opinions?page=' + opinionPage + '&pageSize=30');
  const rows = (d.opinions || []).map(o => `
    <tr>
      <td style="white-space:nowrap">${escapeHtml(o.createdAt)}</td>
      <td>${o.anonymous ? '匿名' : escapeHtml(o.userName)}</td>
      <td style="max-width:320px">${escapeHtml(o.content)}</td>
      <td><span class="pill ${o.status === '待处理' ? 'pending' : 'user'}">${escapeHtml(o.status)}</span></td>
      <td style="white-space:nowrap">
        ${o.status === '待处理' ? `<button class="btn-mini" data-opinion-done data-id="${o.id}">标记已处理</button>` : ''}
        <button class="btn-mini danger" data-opinion-del data-id="${o.id}">删除</button>
      </td>
    </tr>`).join('');
  document.getElementById('opinionListBody').innerHTML = rows.length ? rows : '<tr><td colspan="5" class="empty">暂无意见</td></tr>';
  const total = d.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / 30));
  document.getElementById('opinionPager').innerHTML = `<button class="btn-mini" data-opinion-page="${opinionPage-1}" ${opinionPage<=1?'disabled':''}>← 上一页</button>
    <span style="margin:0 10px;color:var(--muted);font-size:.85rem">第 ${opinionPage} / ${totalPages} 页 · 共 ${total} 条</span>
    <button class="btn-mini" data-opinion-page="${opinionPage+1}" ${opinionPage>=totalPages?'disabled':''}>下一页 →</button>`;
}
document.addEventListener('click', async (e) => {
  const upg = e.target.closest('[data-user-page]');
  if (upg) { const p = Number(upg.dataset.userPage); if (p >= 1) { userPage = p; loadAdminUsers(); } return; }
  const va = e.target.closest('[data-view-article]');
  if (va) { viewArticlePreview(va.dataset.id); return; }
  const pg = e.target.closest('[data-opinion-page]');
  if (pg) { const p = Number(pg.dataset.opinionPage); if (p >= 1) { opinionPage = p; loadOpinions(); } return; }
  const done = e.target.closest('[data-opinion-done]');
  if (done) {
    await fetchJSON('/api/admin/opinions/' + done.dataset.id + '/status', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ status: '已处理' }) });
    loadOpinions(); return;
  }
  const del = e.target.closest('[data-opinion-del]');
  if (del) {
    if (!confirm('确定删除该意见？')) return;
    await fetchJSON('/api/admin/opinions/' + del.dataset.id + '/status', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ status: '已删除' }) });
    loadOpinions(); return;
  }
});

// AI 内容审查
let aiReviewBound = false;
function bindAiReview() {
  const btn = document.getElementById('aiReviewBtn');
  if (!btn || aiReviewBound) return;
  aiReviewBound = true;
  btn.onclick = async () => {
    const input = document.getElementById('aiReviewInput');
    const msg = document.getElementById('aiReviewMsg');
    const result = document.getElementById('aiReviewResult');
    const content = (input.value || '').trim();
    if (!content) { msg.textContent = '请先粘贴要审查的内容'; return; }
    msg.textContent = 'AI 审查中…';
    btn.disabled = true;
    try {
      const r = await fetch('/api/admin/ai-review', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ content, type: 'manual' })
      });
      const d = await r.json();
      if (!r.ok) { msg.textContent = d.error || '审查失败'; result.style.display = 'none'; return; }
      msg.textContent = '';
      if (d.verdict) {
        const v = d.verdict;
        const okColor = v.ok ? '#1a9e5c' : '#d32f2f';
        const okText = v.ok ? (ICONS.check(14) + ' 通过') : (ICONS.ban(14) + ' 疑似违规');
        result.style.display = 'block';
        result.style.borderColor = v.ok ? '#cde8d8' : '#f0c9c9';
        result.innerHTML = '<div style="font-size:1rem;font-weight:700;color:' + okColor + ';margin-bottom:6px">' + okText + '</div>' +
          '<div><strong>命中类别：</strong>' + (v.categories && v.categories.length ? escapeHtml(v.categories.join('、')) : '无') + '</div>' +
          '<div><strong>理由：</strong>' + escapeHtml(v.reason || '—') + '</div>' +
          '<div><strong>建议：</strong>' + escapeHtml(v.suggestion || '—') + '</div>';
      } else if (d.raw) {
        result.style.display = 'block';
        result.innerHTML = '<div><strong>AI 原始输出：</strong><pre style="white-space:pre-wrap;margin-top:6px">' + escapeHtml(d.raw) + '</pre></div>';
      }
    } catch (e) { msg.textContent = '网络错误'; }
    finally { btn.disabled = false; }
  };
}
// AI 自动批量审查（两级策略）
let aiAutoBound = false;
function bindAiAutoReview() {
  const btn = document.getElementById('aiAutoBtn');
  if (!btn || aiAutoBound) return;
  aiAutoBound = true;
  btn.onclick = async () => {
    const msg = document.getElementById('aiAutoMsg');
    const result = document.getElementById('aiAutoResult');
    if (!confirm('开始 AI 自动审查？明显恶意的内容会被自动下架，模糊内容列出供复核。')) return;
    msg.textContent = 'AI 审查中（每批约 10 秒，请稍候）…';
    btn.disabled = true;
    try {
      const r = await fetch('/api/admin/ai-review/auto', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ limit: 30 })
      });
      const d = await r.json();
      if (!r.ok) { msg.textContent = d.error || '审查失败'; return; }
      msg.textContent = '';
      const removed = d.removed || [];
      const borderline = d.borderline || [];
      let html = '<div style="margin-bottom:10px;font-weight:700">处理 ' + d.processed + ' 条：自动下架 <span style="color:#d32f2f">' + removed.length + '</span> 条，模糊待复核 <span style="color:#e67e22">' + borderline.length + '</span> 条</div>';
      if (removed.length) {
        html += '<div style="margin-bottom:8px"><b>已自动下架：</b></div>';
        html += removed.map(v => '<div style="padding:6px 8px;border-left:3px solid #d32f2f;background:#fdf2f2;border-radius:4px;margin-bottom:4px">[' + escapeHtml(v.kind) + ' #' + v.id + '] ' + escapeHtml(v.content) + ' — ' + escapeHtml(v.reason || '') + '</div>').join('');
      }
      if (borderline.length) {
        html += '<div style="margin:10px 0 8px"><b>模糊内容，需人工复核：</b></div>';
        html += borderline.map(v => '<div style="padding:6px 8px;border-left:3px solid #e67e22;background:#fdf6ec;border-radius:4px;margin-bottom:4px">[' + escapeHtml(v.kind) + ' #' + v.id + '] ' + escapeHtml(v.content) + ' — ' + escapeHtml(v.reason || '') + '</div>').join('');
      }
      if (!removed.length && !borderline.length) html += '<div style="color:#1a9e5c">全部内容正常</div>';
      result.innerHTML = html;
    } catch (e) { msg.textContent = '网络错误'; }
    finally { btn.disabled = false; }
  };
}