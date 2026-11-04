// forum-boot.js — 论坛页的验证码加载（原内联脚本，CSP 禁止内联后外置）。
window.loadPfCaptcha = async function (img) {
  try {
    const d = await (await fetch('/api/captcha')).json();
    if (!d.id) return;
    const idInput = document.getElementById('pfCaptchaId');
    if (idInput) idInput.value = d.id;
    if (img) img.src = '/api/captcha/' + d.id + '.svg';
  } catch (_error) {}
};

document.addEventListener('DOMContentLoaded', function () {
  const img = document.getElementById('pfCapImg');
  if (img) {
    img.style.cursor = 'pointer';
    img.title = '点击换一张';
    img.addEventListener('click', function () { window.loadPfCaptcha(img); });
  }
  document.querySelectorAll('[data-pf-captcha-refresh]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      window.loadPfCaptcha(document.getElementById('pfCapImg'));
    });
  });
});
