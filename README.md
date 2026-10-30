# QDU Wasteland 【青岛大学课程资料共享与学习社区】

> https://www.qdwasteland.top/
> 一个集「课程资料 / 同学社区 / 学校信息 / 个人中心」于一体的校园平台。
> 技术栈：**Go 后端（标准库 net/http）+ 原生 JS 前端（无框架） + SQLite**。

---

## ✨ 功能总览

| 模块 | 前台 | 后台（站长） |
|------|------|------|
| 📚 课程资料 | 全校课表浏览 / 多维检索 / 课程详情 / 匿名上传(≤200MB) / 在线预览 | 查真实作者 / 下架恢复资料 |
| 💬 同学社区 | 四广场论坛 / 发帖评论点赞 / 分页 / 举报 / 草稿箱 | 举报处理 / 删帖 |
| 🏫 学校信息 | 分类文章 / 发布 / Markdown 渲染 / 点赞 / 评论 | 文章审核 / 下架 |
| 👤 个人中心 | 我的帖子/评价/资料/文章/草稿聚合 / 通知 / 私信 / 公开主页 / 设置 | 设管理员 / 用户管理 |

**交叉能力**：全站搜索 · 敏感词自动审查 · AI 内容审查(两级) · 邮箱验证(斐波那契) · 找回密码 · 通知/私信 · 站长公告 · 稿件管理(仿B站) · 自定义主题/导航/拉黑 · 头像上传 · 相册照片 · 论坛发图 · 课程分类 · 禁言 · 等级积分 · 举报审核闭环 · 意见箱 · 响应式

---

## 📁 目录结构

```
Wasteland/
├── cmd/wasteland/        主程序（package main，全部站点业务代码）
│   ├── main.go           入口：建库 → 路由 → 维护模式 → securityHeaders → 静态文件
│   ├── auth*.go          用户与会话        course.go / api.go     课程与检索
│   ├── content*.go       资料与评价        api_forum.go           论坛
│   ├── article*.go       文章              admin*.go              后台
│   └── security_*.go     安全中间件与护栏
├── cmd/devseed|seedhistory|toolsdemo|migrate-digests/   本地工具
├── internal/
│   ├── tools/            工具框架 + 5 个工具（nextjlc / history / digest / hot / collector）
│   ├── githubdigest/     开源日报抓取
│   ├── hotdigest/        热点日报抓取
│   └── digestdb/         日报快照入库（表 <prefix>_issues）
├── public/               站点前端（原生 JS，无构建）
├── deploy/               部署与运维脚本（含 nginx.conf.example）
├── docs/                 全部文档（ARCHITECTURE / SECURITY-FIXES / 部署 / 运维）
├── classes/ + _csv/      课表原始表与转换后的 CSV（数据，gitignore）
├── course-files/         课程资料清单（本体不入库）
├── backups/ dist/ .cache/ tool-data/ VPN/   本地运行时数据（全部 gitignore）
└── go.mod                module wasteland（全仓唯一）
```

构建：`go build -o dist/wasteland ./cmd/wasteland`
---

## 🚀 如何运行

```bash
# 1. 启动脚本（含编译、自动处理误报白名单）
powershell -ExecutionPolicy Bypass -File start-server.ps1

# 或手动
set CSV_DIR=F:\My_Projects\Wasteland\_csv
set ADMIN_EMAIL=admin@qdu.edu.cn
set ADMIN_PASSWORD=你的密码
go build -o qdu-wasteland.exe .
qdu-wasteland.exe
```

打开 **http://localhost:3000**

**管理员账号**：首次启动通过 `ADMIN_EMAIL` / `ADMIN_PASSWORD` 播种；若未设置则自动生成随机密码并打印到日志（见 `../docs/SECURITY.md`）。

---

## 🔐 环境变量

| 变量 | 作用 |
|------|------|
| `CSV_DIR` | 课表 csv 目录（默认 `../_csv`） |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | 首启播种站长（幂等） |
| `MAIL_HOST`/`MAIL_PORT`/`MAIL_USER`/`MAIL_PASS`/`MAIL_FROM` | SMTP 邮件；未配置时验证码/链接打印到 server.log（日志模式） |
| `CLAMAV_CMD` | ClamAV clamscan 路径（设置后启用病毒扫描） |

---

## 🌐 API 概览（REST）

- 认证：`/api/register /login /logout /me /me/activity`
- 课程：`/api/courses(/search/detail) /tags /stats /search`
- 资料评价：`/api/upload /files(/preview /download) /reviews(/add)`
- 论坛：`/api/forums /forum/posts(分页) /forum/post(/create /update /delete) /forum/comment /forum/like`
- 文章：`/api/articles(/create /update /delete)`
- 社交：`/api/notifications(/read) /messages(/send /read) /message/eligible /announcement`
- 用户：`/api/user/{id}`（公开主页）
- 邮箱：`/api/email/(verify /verify/confirm /reset /reset/confirm)`
- 举报：`/api/report`
- 后台（管理员）：`/api/admin/{dashboard,users,reports,files,reviews,articles,censor,announcement}...`
