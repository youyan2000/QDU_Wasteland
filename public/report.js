// report.js — 前台举报组件 (V8)
// 提供全局 reportOpen(type, id) 弹出举报浮层；需要 style.css 的 modal 样式（或本文件自带）
(function () {
  // 注入举报浮层样式（若 style.css 无 modal 时兜底）
  if (!document.getElementById('reportCss')) {
    const s = document.createElement('style');
    s.id = 'reportCss';
    s.textContent = `
      .rep-modal { position:fixed; inset:0; background:rgba(0,0,0,.45); display:flex; align-items:center; justify-content:center; z-index:400; }
      .rep-box { background:#fff; border-radius:16px; padding:1.3rem 1.5rem; width:min(420px,90vw); }
      .rep-box h4 { margin:0 0 .9rem; font-size:1.05rem; }
      .rep-reasons { display:flex; flex-wrap:wrap; gap:8px; margin-bottom:.9rem; }
      .rep-reason { padding:.4rem .9rem; border:1px solid rgba(26,26,46,.18); border-radius:999px; background:#fff; cursor:pointer; font-size:.85rem; }
      .rep-reason.active { background:var(--magenta,#ff2e88); border-color:var(--magenta,#ff2e88); color:#fff; }
      .rep-box textarea { width:100%; box-sizing:border-box; padding:.6rem .8rem; border:1px solid rgba(26,26,46,.18); border-radius:9px; font-family:inherit; font-size:.9rem; resize:vertical; }
      .rep-box .rep-actions { display:flex; justify-content:flex-end; gap:8px; margin-top:.9rem; }
      .rep-box .rep-err { color:#d32f2f; font-size:.82rem; min-height:1em; margin-top:.4rem; }
      .rep-btn-ghost { padding:.5rem 1.2rem; border:1px solid rgba(26,26,46,.18); border-radius:9px; background:#fff; cursor:pointer; }
      .rep-btn-submit { padding:.5rem 1.4rem; border:none; border-radius:9px; background:linear-gradient(90deg,#ff2e88,#ff8a65); color:#fff; font-weight:700; cursor:pointer; }
      .rep-btn-mini { background:none; border:none; color:var(--muted,#888); font-size:.75rem; cursor:pointer; text-decoration:underline; padding:0; }
      .rep-btn-mini:hover { color:#d32f2f; }
    `;
    document.head.appendChild(s);
  }

  let overlay = null;
  function closeOverlay() { if (overlay) { overlay.remove(); overlay = null; } }

  window.reportOpen = function (type, id, label) {
    if (!id) return;
    const reasons = ['垃圾广告', '人身攻击', '色情低俗', '违法违禁', '不实信息', '其他'];
    overlay = document.createElement('div');
    overlay.className = 'rep-modal';
    let selected = '';
    overlay.innerHTML = `
      <div class="rep-box">
        <h4>举报${label ? '「' + label + '」' : ''}</h4>
        <div class="rep-reasons">
          ${reasons.map(r => `<button type="button" class="rep-reason" data-r="${r}">${r}</button>`).join('')}
        </div>
        <textarea id="repDetail" rows="3" placeholder="补充说明（可选）…"></textarea>
        <div class="rep-err" id="repErr"></div>
        <div class="rep-actions">
          <button type="button" class="rep-btn-ghost" id="repCancel">取消</button>
          <button type="button" class="rep-btn-submit" id="repSubmit">提交举报</button>
        </div>
      </div>`;
    document.body.appendChild(overlay);

    overlay.querySelectorAll('.rep-reason').forEach(b => b.onclick = () => {
      overlay.querySelectorAll('.rep-reason').forEach(x => x.classList.remove('active'));
      b.classList.add('active');
      selected = b.dataset.r;
    });
    overlay.querySelector('#repCancel').onclick = closeOverlay;
    overlay.onclick = e => { if (e.target === overlay) closeOverlay(); };
    overlay.querySelector('#repSubmit').onclick = async () => {
      const errEl = overlay.querySelector('#repErr');
      const detail = overlay.querySelector('#repDetail').value.trim();
      const reason = selected ? selected + (detail ? '：' + detail : '') : (detail || '');
      if (!reason) { errEl.textContent = '请选择举报原因或填写说明'; return; }
      const me = await (await fetch('/api/me')).json();
      if (!me.loggedIn) { errEl.textContent = '请先登录'; return; }
      const res = await fetch('/api/report', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ targetType: type, targetId: String(id), reason })
      });
      const d = await res.json();
      if (!res.ok) { errEl.textContent = d.error || '提交失败'; return; }
      closeOverlay();
      alert(d.message || '举报已提交，管理员会尽快处理');
    };
  };

  // 全局委托：任何带 data-report-type / data-report-id 的元素被点击即触发举报
  document.addEventListener('click', (e) => {
    const el = e.target.closest('[data-report-type]');
    if (el) {
      e.preventDefault();
      e.stopPropagation();
      window.reportOpen(el.dataset.reportType, el.dataset.reportId, el.dataset.reportLabel || '');
    }
  });
})();