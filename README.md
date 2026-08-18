# QDU Wasteland 【青岛大学课程资料共享与学习社区】

> 一个集「课程资料 / 同学社区 / 学校信息 / 个人中心」于一体的校园平台。
> 技术栈：**Go 后端（标准库 net/http）+ 原生 JS 前端（无框架） + SQLite**。
> 主线规划见 `../docs/MASTER-PLAN.md`（仓库内只保留本 README，设计文档已外移）。

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
qdu-wasteland/
├── main.go / README.md / start-server.ps1
├── auth.go / auth_http.go       # 用户、会话、注册登录 (SQLite)
├── avatar.go                    # 头像上传
├── course.go                    # 课表 CSV 导入 + 课程/班次内存库
├── api.go / search.go           # 课程列表/详情、全站搜索
├── content.go                   # 上传/下载/预览、课程评价
├── api_forum.go / content_update.go  # 论坛、编辑删除
├── article.go / article_cover.go     # 学校信息文章、封面
├── admin.go / censor.go / report.go / virus.go / csrf.go / ratelimit.go / audit.go
├── mail.go / email_verify.go     # 邮件、邮箱验证、找回密码
├── notify.go / message.go        # 通知、私信
├── org.go / regions.go           # 学院专业统一、籍贯
├── favorites.go / topic.go / block.go / ban.go / maintenance.go / opinion.go
├── public/                      # 前端静态文件
└── uploads/ / qdu-auth.db       # 上传文件 / SQLite 库（gitignore）
```

> 设计/规划文档（MASTER-PLAN / DESIGN / AESTHETIC-* / SECURITY 等）在仓库外的 `../docs/`。

---

## 🚀 如何运行

```bash
# 1. 启动脚本（含编译、自动处理误报白名单）
powershell -ExecutionPolicy Bypass -File start-server.ps1

# 或手动
set CSV_DIR=F:\My_Projects\AI_projects\_csv
set ADMIN_EMAIL=admin@qdu.edu.cn
set ADMIN_PASSWORD=你的密码
go build -o qdu-wasteland.exe .
qdu-wasteland.exe
```

打开 **http://localhost:3000**

> ⚠️ **重要**：后端使用 `modernc.org/sqlite`（纯 Go 内嵌转译字节码），可能被 360 等杀软**误报**为 `Backdoor/W64.CobaltStrike`。这不是真的后门（源码零恶意特征 + `go mod verify` 依赖哈希通过）。请把项目目录加入杀软白名单。

**管理员账号**：首次启动通过 `ADMIN_EMAIL` / `ADMIN_PASSWORD` 播种；若未设置则自动生成随机密码并打印到日志（见 `../docs/SECURITY.md`）。

---

## 📚 课程数据（8 份历年课表）

| 学期 | 转换文件 | 来源 |
|------|---------|------|
| 2023春 | `_csv/2023春_utf8.csv` | xlsx 转换 |
| 2023秋 | `_csv/2023秋_utf8.csv` | xlsx 转换 |
| 2024春 | `_csv/2024春_utf8.csv` | xlsx 转换 |
| 2024夏 | `_csv/2024夏_utf8.csv` | xlsx 转换 |
| 2024秋 | `_csv/2024秋_utf8.csv` | xlsx 转换 |
| 2025春 | `_csv/2025spring_utf8.csv` | 原生 |
| 2025秋 | `_csv/2025autumn_utf8.csv` | 原生 |
| 2026春 | `_csv/2026spring_utf8.csv` | 原生 |

**当前规模**：课程 **4462 门** · 开课班次 **38123 条** · 官方学院 **25 个**（按《青岛大学专业统计.md》统一全站；课表另有 50+ 开课机构名仅用于课程显示）。

新课表（.xlsx）导入步骤：用 `_xlsx_tmp/xlsx2csv.py` 转成同构 csv → 放入 `_csv` → 在 `course.go` `loadAll` 注册学期。

---

## 🔐 环境变量

| 变量 | 作用 |
|------|------|
| `CSV_DIR` | 课表 csv 目录（默认 `../_csv`） |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | 首启播种站长（幂等） |
| `MAIL_HOST`/`MAIL_PORT`/`MAIL_USER`/`MAIL_PASS`/`MAIL_FROM` | SMTP 邮件；未配置时验证码/链接打印到 server.log（日志模式） |
| `CLAMAV_CMD` | ClamAV clamscan 路径（设置后启用病毒扫描） |

---

## 🗄️ 数据库表（SQLite qdu-auth.db）

- `users` — id/email/username/密码哈希/nickname/college/major/is_admin/email_verified/created_at
- `files` — 课程资料（含 status: 正常/已下架，匿名机制）
- `reviews` — 课程评价（status: 正常/已下架）
- `forums`/`posts`/`comments`/`post_likes` — 论坛（posts 含 status: 正常/draft 草稿箱）
- `articles` — 学校文章（status: 正常/draft）
- `reports` — 举报
- `censor_words` — 敏感词（可后台增删）
- `email_tokens` — 邮箱验证/重置 token
- `notifications` / `messages` / `announcements` — 通知/私信/公告

---

## 🔒 安全措施（已实现）

- **上传防护**：危险扩展名黑名单(exe/dll/bat/vbs/宏docm等) + 文件头魔数嗅探(伪装PE/ELF/宏拦截) + 可选 ClamAV
- **CSRF**：写方法同源校验(Origin/Referer) + Cookie SameSite=Strict
- **频率限制**：注册/登录/发帖/评论/评价/举报 同IP 1小时 ≤5次
- **敏感词**：发帖/评论/评价/上传/文章/举报内容自动拦截
- **邮箱验证**：发帖第1/2/3/5/8…（斐波那契）次触发；6位验证码30分钟一次性
- **找回密码**：邮箱链接一次性 token
- **产出需登录**：发文章/传资料/评论/发帖/评价/举报需登录；下载/预览公开
- **草稿箱/编辑**：删除=转草稿仅作者+管理员可见；发布=公开

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

---

## 📆 开发进度

- **V0–V13 全部完成**（功能/安全/治理/社交闭环）
- **V14+ 综合改版完成**（课程多维检索、个人中心自定义、稿件管理、头像、AI 审查、相册照片、论坛图片、课程分类、禁言等）
- **阶段 F/G/I/J/K/L/M/N 全部完成**（详见 `MASTER-PLAN.md`，唯一待办为 H 上线部署）

> 历史多份 plan 已合并为 `MASTER-PLAN.md` 唯一主线。