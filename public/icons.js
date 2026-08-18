// icons.js — 全站统一 SVG 图标库 (V14-I1.6)
// 规范：来自 Tabler Icons (MIT) 标准路径；stroke-width 统一 1.8；fill none；currentColor
// 用法：document.getElementById('x').innerHTML = ICONS.heart(16)
// 或模板里直接 ICONS.xxx(size, className)
window.ICONS = (function () {
  const stroke = 1.8;
  function svg(paths, size, cls) {
    const s = size || 18;
    const c = cls || '';
    return `<svg xmlns="http://www.w3.org/2000/svg" width="${s}" height="${s}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="${stroke}" stroke-linecap="round" stroke-linejoin="round" class="${c}">${paths}</svg>`;
  }
  return {
    // 关闭
    close: (s, c) => svg('<path d="M18 6 6 18"/><path d="m6 6 12 12"/>', s, c),
    // 成功对勾
    check: (s, c) => svg('<path d="M5 12l5 5 9-11"/>', s, c),
    // 错误叉
    x: (s, c) => svg('<path d="M18 6 6 18"/><path d="m6 6 12 12"/>', s, c),
    // 点赞（实心/空心）
    thumb: (s, c, filled) => filled
      ? svg('<path d="M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.28a2 2 0 0 0 2-1.7l1.38-9a2 2 0 0 0-2-2.3zM7 22H4a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2h3"/>', s, c)
      : svg('<path d="M7 11v8a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1h3Zm0 0 4-6a3 3 0 0 1 3 3v3h4a2 2 0 0 1 2 2l-1.5 6a2 2 0 0 1-2 1.5H7" fill="none"/>', s, c),
    // 评论气泡
    chat: (s, c) => svg('<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>', s, c),
    // 铃铛（通知）
    bell: (s, c) => svg('<path d="M10 5a2 2 0 1 1 4 0 7 7 0 0 1 4 6v3a4 4 0 0 0 2 3H4a4 4 0 0 0 2-3v-3a7 7 0 0 1 4-6"/><path d="M9 17v1a3 3 0 0 0 6 0v-1"/>', s, c),
    // 意见/邮件
    mail: (s, c) => svg('<rect width="18" height="14" x="3" y="5" rx="2"/><path d="m3 7 9 6 9-6"/>', s, c),
    // 用户/头像
    user: (s, c) => svg('<circle cx="12" cy="8" r="4"/><path d="M5 21v-1a7 7 0 0 1 14 0v1"/>', s, c),
    // 公告/喇叭
    megaphone: (s, c) => svg('<path d="m3 11 18-5v12L3 14v-3z"/><path d="M11.6 16.8a3 3 0 1 1-5.8-1.6"/>', s, c),
    // 设置齿轮
    settings: (s, c) => svg('<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>', s, c),
    // 眼睛（密码可见）
    eye: (s, c) => svg('<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/>', s, c),
    // 编辑笔
    edit: (s, c) => svg('<path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z"/>', s, c),
    // 锁（隐私）
    lock: (s, c) => svg('<rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>', s, c),
    // 回形针（上传）
    paperclip: (s, c) => svg('<path d="m21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l8.57-8.57A4 4 0 1 1 18 8.84l-8.59 8.57a2 2 0 0 1-2.83-2.83l8.49-8.48"/>', s, c),
    // 文件
    file: (s, c) => svg('<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/>', s, c),
    // 文件夹（资料）
    folder: (s, c) => svg('<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>', s, c),
    // 日历（班次）
    calendar: (s, c) => svg('<path d="M8 2v4"/><path d="M16 2v4"/><rect width="18" height="18" x="3" y="4" rx="2"/><path d="M3 10h18"/>', s, c),
    // 时钟（时间）
    clock: (s, c) => svg('<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 3"/>', s, c),
    // 定位（地点）
    mapPin: (s, c) => svg('<path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z"/><circle cx="12" cy="10" r="3"/>', s, c),
    // 学校/建筑（首页模块）
    school: (s, c) => svg('<path d="M22 10 12 5 2 10l10 5 10-5Z"/><path d="M6 12v5c0 1.7 2.7 3 6 3s6-1.3 6-3v-5"/><path d="M22 10v6"/>', s, c),
    // 书本（课程）
    book: (s, c) => svg('<path d="M4 19.5v-15A2.5 2.5 0 0 1 6.5 2H20v20H6.5a2.5 2.5 0 0 1 0-5H20"/>', s, c),
    // 消息/论坛
    messages: (s, c) => svg('<path d="M21 14l-3-3h-8a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v9Z"/><path d="M7 8H5a2 2 0 0 0-2 2v9l3-3h8a2 2 0 0 0 2-2v-1"/>', s, c),
    // 奖杯（竞赛分类）
    trophy: (s, c) => svg('<path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6"/><path d="M18 9h1.5a2.5 2.5 0 0 0 0-5H18"/><path d="M4 22h16"/><path d="M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20.24 7 22"/><path d="M14 14.66V17c0 .55.47.98.97 1.21C16.15 18.75 17 20.24 17 22"/><path d="M18 2H6v7a6 6 0 0 0 12 0V2Z"/>', s, c),
    // 叶子（生活分类）
    leaf: (s, c) => svg('<path d="M11 20A7 7 0 0 1 9.8 6.1C15.5 5 17 4.48 19 2c1 2 2 4.18 2 8 0 5.5-4.78 10-10 10Z"/><path d="M2 21c0-3 1.85-5.36 5.08-6C9.5 14.52 12 13 13 12"/>', s, c),
    // 面具/社团
    mask: (s, c) => svg('<path d="M8.5 2.5 5 4v7c0 3.5 2 6.5 5.5 8l1.5.75L13.5 19c3.5-1.5 5.5-4.5 5.5-8V4l-3.5-1.5Z"/><circle cx="9" cy="10" r="1.5"/><circle cx="15" cy="10" r="1.5"/>', s, c),
    // 方块/其他分类
    squares: (s, c) => svg('<rect width="7" height="7" x="3" y="3" rx="1"/><rect width="7" height="7" x="14" y="3" rx="1"/><rect width="7" height="7" x="3" y="14" rx="1"/><rect width="7" height="7" x="14" y="14" rx="1"/>', s, c),
    // 专题/书签
    bookmark: (s, c) => svg('<path d="m19 21-7-4-7 4V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v16z"/>', s, c),
    // 首页
    home: (s, c) => svg('<path d="m3 10 9-7 9 7v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/><path d="M9 22V12h6v10"/>', s, c),
    // 照片
    photo: (s, c) => svg('<rect width="18" height="18" x="3" y="3" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.09-3.09a2 2 0 0 0-2.82 0L6 21"/>', s, c),
    // 下载（Tabler download）
    download: (s, c) => svg('<path d="M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2"/><path d="M7 11l5 5 5-5"/><path d="M12 4v12"/>', s, c)
  };
})();