// home.js — 首页右上角登录态（原内联脚本，CSP 禁止内联后外置）。
(function () {
  var root = document.getElementById('homeAuth');
  if (!root) return;
  fetch('/api/me')
    .then(function (response) { return response.json(); })
    .then(function (data) {
      if (data.loggedIn && data.user) {
        var name = data.user.nickname || data.user.email || '我的主页';
        var link = document.createElement('a');
        link.href = '/me.html';
        link.className = 'home-auth-user';
        link.textContent = name;
        root.replaceChildren(link);
        return;
      }
      var login = document.createElement('a');
      login.href = '/login.html';
      login.className = 'home-auth-login';
      login.textContent = '登录';
      var register = document.createElement('a');
      register.href = '/login.html#register';
      register.className = 'home-auth-register';
      register.textContent = '注册';
      root.replaceChildren(login, register);
    })
    .catch(function () {});
})();
