// reset.js — 找回密码页逻辑（原内联脚本，CSP 禁止内联后外置）。
(function () {
  // 密码可见切换（auth.js 已处理 .pw-toggle，这里保留兜底）
  document.querySelectorAll('.pw-toggle').forEach(function (btn) {
    btn.onclick = function () {
      var inp = document.getElementById(btn.dataset.target);
      if (!inp) return;
      inp.type = inp.type === 'password' ? 'text' : 'password';
    };
  });

  var token = new URLSearchParams(location.search).get('token');
  var stepEmail = document.getElementById('stepEmail');
  var stepNew = document.getElementById('stepNew');
  if (token && stepEmail && stepNew) {
    stepEmail.style.display = 'none';
    stepNew.classList.remove('hidden');
  }

  // 步骤1：请求重置邮件
  var sendBtn = document.getElementById('rpSend');
  if (sendBtn) {
    sendBtn.onclick = async function () {
      var errEl = document.getElementById('resetErr');
      errEl.textContent = '';
      var email = document.getElementById('rpEmail').value.trim();
      if (!email) {
        errEl.textContent = '请输入邮箱';
        return;
      }
      var r = await fetch('/api/email/reset', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email }),
      });
      var d = await r.json().catch(function () { return {}; });
      errEl.style.color = r.ok ? '#1a9e5c' : '#d32f2f';
      errEl.textContent = d.error || d.message || (r.ok ? '如果该邮箱已注册，重置链接已发送' : '发送失败');
    };
  }

  // 步骤2：确认重置
  var confirmBtn = document.getElementById('rpConfirm');
  if (confirmBtn) {
    confirmBtn.onclick = async function () {
      var errEl = document.getElementById('resetErr2');
      errEl.textContent = '';
      var password = document.getElementById('rpPassword').value;
      if (password.length < 6) {
        errEl.textContent = '密码至少 6 位';
        return;
      }
      var r = await fetch('/api/email/reset/confirm', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token: token, password: password }),
      });
      var d = await r.json().catch(function () { return {}; });
      if (!r.ok) {
        errEl.textContent = d.error || '重置失败';
        return;
      }
      errEl.style.color = '#1a9e5c';
      errEl.textContent = '密码已重置，3 秒后跳转登录…';
      setTimeout(function () {
        location.href = '/login.html';
      }, 3000);
    };
  }
})();
