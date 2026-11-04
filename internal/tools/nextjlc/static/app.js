// app.js — NeXTJLC 页面逻辑（原先内联在 index.html 里，被 CSP script-src 'self' 拦截，导致拖拽与按钮全部失效）。

(function () {
  var drop = document.getElementById('drop');
  var input = document.getElementById('file');
  var inputList = document.getElementById('inputList');
  var inputEmpty = document.getElementById('inputEmpty');
  var outputList = document.getElementById('outputList');
  var outputEmpty = document.getElementById('outputEmpty');
  var badges = document.getElementById('badges');
  var processBtn = document.getElementById('processBtn');
  var downloadBtn = document.getElementById('downloadBtn');
  var barFill = document.getElementById('barFill');
  var statusEl = document.getElementById('status');
  var errEl = document.getElementById('err');
  var inputCount = document.getElementById('inputCount');
  var outputCount = document.getElementById('outputCount');

  var files = [];       // File 对象（提交给服务端）
  var session = '';
  var outputs = [];     // {originalName, newName}
  var ready = true;
  var inputs = [];      // 服务端识别出的文件清单（zip 会被展开）
  var display = [];     // 左侧列表实际展示的条目 {name, software}
  var inspecting = false;

  drop.onclick = function () { input.click(); };
  input.onchange = function () { addFiles(input.files); };
  ['dragenter', 'dragover'].forEach(function (t) {
    drop.addEventListener(t, function (e) { e.preventDefault(); drop.classList.add('is-over'); });
  });
  ['dragleave', 'dragend'].forEach(function (t) {
    drop.addEventListener(t, function () { drop.classList.remove('is-over'); });
  });
  drop.addEventListener('drop', function (e) {
    e.preventDefault();
    drop.classList.remove('is-over');
    addFiles((e.dataTransfer && e.dataTransfer.files) || []);
  });

  function addFiles(list) {
    for (var i = 0; i < list.length; i++) files.push(list[i]);
    display = files.map(function (f) { return { name: f.name, software: '' }; });
    renderInputs();
    inspect();
  }

  // 拖进来的是 zip 时，先让服务端展开，左侧直接显示里面的 Gerber 文件（与原版一致）。
  // 展开失败就退回显示原始文件名，不阻塞后续处理。
  function inspect() {
    var hasZip = files.some(function (f) { return /\.zip$/i.test(f.name); });
    if (!hasZip || inspecting) return;
    inspecting = true;
    var fd = new FormData();
    files.forEach(function (f) { fd.append('files', f, f.name); });
    fetch('/tool/nextjlc/api/inspect', { method: 'POST', body: fd })
      .then(function (r) { return r.json(); })
      .then(function (d) {
        if (d && d.error) throw new Error(d.error);
        if (d && (d.inputs || []).length) {
          inputs = d.inputs;
          display = d.inputs;
          renderInputs();
          renderBadges();
        }
      })
      .catch(function (e) {
        // 展开失败只提示，不影响用户继续手选文件后点 Process
        if (e && e.message && e.message !== '没有收到文件') errEl.textContent = e.message;
      })
      .then(function () { inspecting = false; });
  }

  function renderInputs() {
    inputList.innerHTML = '';
    inputEmpty.style.display = display.length ? 'none' : 'flex';
    display.forEach(function (f) {
      var li = document.createElement('li');
      var label = esc(f.name);
      if (f.software && f.software !== 'None') label += ' <span class="nw-sw">' + esc(f.software) + '</span>';
      li.innerHTML = '<span>' + label + '</span><button title="复制到导出列表">&#8594;</button>';
      li.querySelector('button').onclick = function () { copyFile(f.name); };
      inputList.appendChild(li);
    });
    if (inputCount) inputCount.textContent = files.length ? '（' + display.length + ' 个文件）' : '';
    processBtn.disabled = files.length === 0 || !ready;
  }

  function renderBadges() {
    var counts = {};
    inputs.forEach(function (i) {
      var sw = i.software || 'None';
      counts[sw] = (counts[sw] || 0) + 1;
    });
    badges.innerHTML = Object.keys(counts).map(function (k) {
      return '<span class="badge">' + esc(k) + ' ×' + counts[k] + '</span>';
    }).join('');
  }

  function renderOutputs() {
    outputList.innerHTML = '';
    outputEmpty.style.display = outputs.length ? 'none' : 'flex';
    outputs.forEach(function (o) {
      var li = document.createElement('li');
      li.innerHTML = '<span>' + esc(o.newName) + '</span><button title="从导出列表移除">&#10005;</button>';
      li.querySelector('button').onclick = function () { removeFile(o.newName); };
      outputList.appendChild(li);
    });
    if (outputCount) outputCount.textContent = outputs.length ? '（' + outputs.length + ' 个）' : '';
    downloadBtn.disabled = outputs.length === 0;
  }

  processBtn.onclick = function () {
    if (!files.length) return;
    errEl.textContent = '';
    ready = false; processBtn.disabled = true;
    barFill.style.width = '10%';
    statusEl.textContent = '正在处理…';
    var fd = new FormData();
    files.forEach(function (f) { fd.append('files', f, f.name); });
    fetch('/tool/nextjlc/api/process', { method: 'POST', body: fd })
      .then(function (r) { return r.json().then(function (d) { return { ok: r.ok, d: d }; }); })
      .then(function (res) {
        ready = true;
        barFill.style.width = '100%';
        if (!res.ok) throw new Error(res.d.error || '处理失败');
        if ((res.d.errors || []).length) {
          errEl.textContent = 'Gerber file validation failed:\n\n' + res.d.errors.join('\n');
          session = ''; outputs = []; renderOutputs();
          statusEl.textContent = '';
          return;
        }
        session = res.d.session;
        inputs = res.d.inputs || [];
        display = inputs;
        renderInputs();
        outputs = res.d.files || [];
        renderBadges();
        renderOutputs();
        statusEl.textContent = '完成：' + outputs.length + ' 个文件 · ' + res.d.layers + ' 层 · 下载名 ' + res.d.zipName;
        if ((res.d.warnings || []).length) console.warn(res.d.warnings.join('\n'));
      })
      .catch(function (e) {
        ready = true; processBtn.disabled = files.length === 0;
        barFill.style.width = '0';
        statusEl.textContent = '';
        errEl.textContent = e.message || String(e);
      });
  };

  function copyFile(name) {
    if (!session) { alert('请先点 Process'); return; }
    post('/tool/nextjlc/api/copy', { session: session, name: name }, function (d) {
      outputs = d.files || []; renderOutputs();
    });
  }

  function removeFile(name) {
    post('/tool/nextjlc/api/remove', { session: session, name: name }, function (d) {
      outputs = d.files || []; renderOutputs();
    });
  }

  downloadBtn.onclick = function () {
    errEl.textContent = '';
    fetch('/tool/nextjlc/api/download', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ session: session })
    }).then(function (r) {
      if (!r.ok) {
        return r.json().catch(function () { return {}; }).then(function (d) {
          throw new Error(d.error || '下载失败');
        });
      }
      return r.blob().then(function (blob) {
        var a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = 'download.zip';
        a.click();
        setTimeout(function () { URL.revokeObjectURL(a.href); }, 4000);
      });
    }).catch(function (e) { errEl.textContent = e.message || String(e); });
  };

  function post(url, body, cb) {
    fetch(url, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    }).then(function (r) { return r.json(); }).then(cb)
      .catch(function (e) { errEl.textContent = e.message || String(e); });
  }

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (m) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m];
    });
  }
})();

