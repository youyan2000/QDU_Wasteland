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
├── deploy/               部署与运维脚本（含 Tunnel/直连 Nginx 安全模板与本机审计）
├── docs/                 全部文档（ARCHITECTURE / SECURITY-FIXES / 部署 / 运维）
├── classes/ + _csv/      课表原始表与转换后的 CSV（数据，gitignore）
├── course-files/         课程资料清单（本体不入库）
├── backups/ dist/ .cache/ tool-data/ VPN/   本地运行时数据（全部 gitignore）
└── go.mod                module wasteland（全仓唯一）
```

构建：`go build -o dist/wasteland ./cmd/wasteland`

生产入口推荐 `Cloudflare Tunnel → loopback Nginx → 应用`，应用默认绑定 `127.0.0.1:3000`。Tunnel 不等于 VPN 或物理隔离；上线前须审阅 [DEPLOY-GUIDE.md](DEPLOY-GUIDE.md) 和 `deploy/` 模板，尤其是真实 IP 信任边界、防火墙、日志轮转及固定 commit 的显式部署批准。模板不会自动改变现有公网服务。
---

## 🚀 如何运行

```bash
# 1. 启动脚本（含编译、自动处理误报白名单）
powershell -ExecutionPolicy Bypass -File start-server.ps1

# 或手动
set CSV_DIR=F:\My_Projects\Wasteland\_csv
set ADMIN_EMAIL=admin@qdu.edu.cn
set ADMIN_PASSWORD=你的密码
go build -o qdu-wasteland.exe ./cmd/wasteland
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
| `MAIL_HOST`/`MAIL_PORT`/`MAIL_USER`/`MAIL_PASS`/`MAIL_FROM` | SMTP 邮件；587 等端口要求 STARTTLS，465 使用隐式 TLS；未配置时正文默认不入日志 |
| `MAIL_ALLOW_PLAINTEXT` | 仅可信本地 SMTP relay 且已确认链路隔离时设置 `1`，允许无 STARTTLS（默认禁止） |
| `AI_API_URL` / `AI_ALLOWED_HOSTS` | AI 接口 HTTPS URL；内建 OpenAI/DeepSeek 主机，其他供应商须用逗号分隔精确配置主机名 |
| `HSTS_INCLUDE_SUBDOMAINS` | 默认只启用当前站点 HSTS；确认所有子域均支持 HTTPS 后才设 `true` |
| `CLAMAV_CMD` | ClamAV clamscan 路径（设置后启用病毒扫描） |

---

## 🧭 浏览足迹与浏览器扩展

浏览器不允许网页直接读取本机完整历史，所以「浏览器历史 / B站历史 / 书签」由配套扩展同步：

1. 登录站点 → `工具 → 浏览足迹 → 数据备份`；
2. 按浏览器下载对应扩展包：
   - Chrome / Edge：`chrome://extensions` 开启开发者模式 →「加载已解压的扩展程序」；
   - Firefox：`about:debugging#/runtime/this-firefox` →「临时载入附加组件」选中 manifest.json
     （正式版 Firefox 只允许临时载入未签名扩展，重启后需再载入一次；Developer Edition / Nightly 可关闭签名校验后永久安装）；
3. 页面上「生成配对码」，把**站点地址 + 配对码**填进扩展设置，点「立即同步」。

之后扩展每 30 分钟自动增量同步，不需要每次手动导入。扩展源码在 `internal/tools/history/extension/`，
服务端接口是 `POST /api/history/sync`（Bearer 设备令牌，令牌只存 SHA-256，可逐设备撤销）。
## 📖 全站单词条（Word Atlas 词库）每页顶部与底部会显示一个随机雅思单词和英文释义（首页整屏封面除外），数据来自
[Word Atlas](https://github.com/designfuchen/word-atlas)（词库内容 CC BY 4.0，署名 "Word Atlas contributors"）。

- **运行时零外部依赖**：词库经 `cmd/wordbankgen` 精简后写入 `internal/wordbank/words.json`，
  编译时嵌入二进制；生产环境不需要本地目录，也不需要联网。
- 前端通过 `GET /api/words/random?n=2` 取词，点击「换一个」可换词。
- 需要更新词库时（手动，不是定时任务）：

```powershell
# 从 GitHub 拉取最新词库并重新生成内置数据
powershell -ExecutionPolicy Bypass -File deploy/sync-wordbank.ps1
# 或使用本地副本
powershell -ExecutionPolicy Bypass -File deploy/sync-wordbank.ps1 -Source "F:\path\to\word-atlas.json"
```

生成后重新编译部署即可生效。词库体积约 900 KB（7298 词），只增加二进制大小，不占页面带宽。

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
