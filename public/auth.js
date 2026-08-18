// loadCaptcha：给指定前缀(l/r)加载图形验证码
window.loadCaptcha = async function (pre, img) {
  try {
    const d = await (await fetch("/api/captcha")).json();
    if (!d.id) return;
    document.getElementById(pre + "CaptchaId").value = d.id;
    if (img) img.src = "/api/captcha/" + d.id + ".svg";
  } catch (e) {}
};
document.addEventListener("DOMContentLoaded", () => {
  loadCaptcha("l", document.getElementById("lCapImg"));
  loadCaptcha("r", document.getElementById("rCapImg"));
});
window.addEventListener("load", function () {
  if (document.getElementById("lCapImg") && !document.getElementById("lCapImg").src) {
    loadCaptcha("l", document.getElementById("lCapImg"));
  }
});
// auth.js — 登录/注册页逻辑
const tabLogin = document.getElementById('tabLogin');
const tabRegister = document.getElementById('tabRegister');
const loginForm = document.getElementById('loginForm');
const registerForm = document.getElementById('registerForm');

function switchTab(which) {
  tabLogin.classList.toggle('active', which === 'login');
  tabRegister.classList.toggle('active', which === 'register');
  loginForm.classList.toggle('hidden', which !== 'login');
  registerForm.classList.toggle('hidden', which !== 'register');
}
tabLogin.onclick = () => switchTab('login');
tabRegister.onclick = () => switchTab('register');

// 若已登录直接跳个人主页
async function checkLogin() {
  try {
    const r = await fetch('/api/me');
    const d = await r.json();
    if (d.loggedIn) location.href = '/me.html';
  } catch (e) {}
}
checkLogin();

// 密码显示切换（保留光标位置，避免 input type 切换导致的跳动/焦点丢失）
document.querySelectorAll('.pw-toggle').forEach(btn => {
  btn.onclick = () => {
    const inp = document.getElementById(btn.dataset.target);
    if (!inp) return;
    const pos = inp.selectionStart;   // 记录光标位置
    const show = inp.type === 'password';
    inp.type = show ? 'text' : 'password';
    // 切回后恢复光标位置
    if (pos != null) { inp.setSelectionRange(pos, pos); }
    inp.focus();
    // 眼睛图标切换（如果有）
    const icon = btn.querySelector('.icon-fill');
    if (icon && icon.dataset.icon) {
      icon.dataset.icon = show ? 'eye-off' : 'eye';
      const f = ICONS[icon.dataset.icon];
      if (f) icon.innerHTML = f(18);
    }
  };
});

// ------ 学院/专业下拉（J1/J2：从 /api/org 加载统一清单）-------
const collegeSel = document.getElementById('rCollege');
const majorSel = document.getElementById('rMajor');
let setupData = { colleges: [], majors: {} };

fetch('/api/org')
  .then(r => r.json())
  .then(d => {
    setupData = d;
    collegeSel.innerHTML = '<option value="">请选择学院</option>';
    (d.colleges || []).forEach(c => {
      const o = document.createElement('option');
      o.value = c; o.textContent = c;
      collegeSel.appendChild(o);
    });
  })
  .catch(() => {
    // 回退：旧 setup.json
    fetch('/setup.json').then(r => r.json()).then(d => {
      setupData = d;
      collegeSel.innerHTML = '<option value="">请选择学院</option>';
      (d.colleges || []).forEach(c => {
        const o = document.createElement('option');
        o.value = c; o.textContent = c;
        collegeSel.appendChild(o);
      });
    }).catch(() => {});
  });

collegeSel.addEventListener('change', () => {
  const college = collegeSel.value;
  majorSel.innerHTML = '<option value="">请选择专业</option>';
  const list = (setupData.majors && setupData.majors[college]) || [];
  if (!college) { majorSel.innerHTML = '<option value="">请先选择学院</option>'; return; }
  if (list.length === 0) {
    const o = document.createElement('option');
    o.value = '其他'; o.textContent = '其他';
    majorSel.appendChild(o);
    return;
  }
  list.forEach(m => {
    const o = document.createElement('option');
    o.value = m; o.textContent = m;
    majorSel.appendChild(o);
  });
});

// 登录
loginForm.addEventListener('submit', async e => {
  e.preventDefault();
  const errEl = document.getElementById('loginErr');
  errEl.textContent = '';
  const submit = document.getElementById('loginSubmit');
  submit.disabled = true; submit.textContent = '登录中…';
  try {
    const res = await fetch('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        account: lEmail.value.trim(),
        password: lPassword.value,
        captcha: document.getElementById("lCaptcha").value.trim(),
        captchaId: document.getElementById("lCaptchaId").value
      })
    });
    const d = await res.json();
    if (!res.ok) throw new Error(d.error || '登录失败');
    location.href = '/me.html';
  } catch (err) {
    errEl.textContent = err.message;
  } finally {
    submit.disabled = false; submit.textContent = '登录';
  }
});

// 注册
registerForm.addEventListener('submit', async e => {
  e.preventDefault();
  const errEl = document.getElementById('registerErr');
  errEl.textContent = '';
  const submit = document.getElementById('registerSubmit');
  submit.disabled = true; submit.textContent = '注册中…';
  try {
    if (!collegeSel.value) throw new Error('请选择学院');
    if (!majorSel.value) throw new Error('请选择专业');
    const res = await fetch('/api/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: rEmail.value.trim(),
        username: rUsername.value.trim(),
        password: rPassword.value,
        nickname: rUsername.value.trim(), // 昵称=用户名（合并）
        college: collegeSel.value,
        major: majorSel.value,
        captcha: document.getElementById("rCaptcha").value.trim(),
        captchaId: document.getElementById("rCaptchaId").value
      })
    });
    const d = await res.json();
    if (!res.ok) throw new Error(d.error || '注册失败');
    location.href = '/me.html';
  } catch (err) {
    errEl.textContent = err.message;
  } finally {
    submit.disabled = false; submit.textContent = '注册并登录';
  }
});
