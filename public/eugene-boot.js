// eugene-boot.js — 后台顶栏的管理员昵称与退出（原内联脚本，CSP 禁止内联后外置）。
(function () {
  fetch('/api/me')
    .then(function (r) { return r.json(); })
    .then(function (d) {
      var who = document.getElementById('eugeneWho');
      if (d.loggedIn && d.user && who) {
        who.textContent = (d.user.nickname || d.user.email) + ' · 管理员';
      }
    })
    .catch(function () {});

  var logout = document.getElementById('eugeneLogout');
  if (logout) {
    logout.onclick = async function () {
      await fetch('/api/logout', { method: 'POST' });
      location.href = '/login.html';
    };
  }
})();
