# QDU Wasteland · 服务器部署教学文档（DEPLOY-GUIDE）

> 本教程带你把 QDU Wasteland 从"本地代码"部署到"公网服务器"，任何人能访问。
> 全程手把手，每步有"为什么"和"怎么验证"。跟着顺序做即可。

---

## 〇、部署全景图（先看这张图，心里有数）

```
买服务器（境外 VPS，免备案）        ← 第 1 章
  ↓
连上服务器（SSH）                  ← 第 2 章
  ↓
装环境（MySQL / Nginx / ClamAV）   ← 第 3 章
  ↓
配 MySQL（建库 + 建账号）           ← 第 4 章
  ↓
配防火墙（只开 22/80/443）          ← 第 5 章
  ↓
配 Nginx（反向代理 80→3000）        ← 第 6 章
  ↓
传程序 + 传课表数据                 ← 第 7 章
  ↓
启动 Go 程序（systemd 守护）        ← 第 8 章
  ↓
验证：http://你的IP 能打开          ← 第 9 章
  ↓
（后续）HTTPS / 域名 / MySQL迁移 / ClamAV / 备份 / 压测  ← 第 10 章
```

---

## 第 1 章 · 购买境外 VPS（免备案）

### 1.1 为什么用境外服务器
- **免备案**：国内服务器必须 ICP 备案（要身份证实名 + 审核 1-2 周）；境外服务器不用。
- **便宜**：约 ¥40-80/月，符合项目预算（≤100 元/月）。

### 1.2 推荐厂商与配置
| 厂商 | 官网 | 价格 | 特点 |
|------|------|------|------|
| **Vultr**（本教程用） | https://www.vultr.com | $10/月 ≈ ¥72 | 东京/新加坡节点近、按小时计费 |

**选套餐（关键）**：选 **vc2-1c-2gb（1核2G / 55GB / 2TB流量 / $10月）**
- 不要选 1GB 内存档：MySQL(约500MB) + Go程序 + Nginx + ClamAV 加起来超 700MB，1GB 会卡死。
- 2GB 内存跑日常 200 人 / 峰值 2000 人足够；Go 是编译型语言，单核也能扛。

### 1.3 购买步骤
1. 注册 Vultr（支持支付宝/微信充值），充值 $10。
2. **Deploy New Server**：
   - Location → **Tokyo（东京）或 Singapore（新加坡）**
   - Server Type → **Ubuntu 22.04 LTS**
   - Plan → **vc2-1c-2gb（$10/月）**
   - Hostname → `qdu-wasteland`
   - Deploy Now
3. 等 1-2 分钟，服务器列表出现后记录三样东西：
   - **IP 地址**（如 207.148.106.155）
   - **用户名**：root
   - **密码**（实例详情页可看/重置）

> ⚠️ 密码可能是自动生成的一串，务必保存好；实例 ID 不是密码。

---

## 第 2 章 · 连接服务器（SSH）

### 2.1 Windows 自带 SSH（推荐）
1. 按 `Win+R` → 输入 `cmd` → 回车
2. 输入：`ssh root@你的服务器IP`
3. 首次连接问 `Are you sure... (yes/no)?` → 输入 `yes` 回车
4. 输密码（**粘贴**，屏幕不显示字符是正常的）→ 回车
5. 看到 `root@qdu-wasteland:~#` 即连接成功

> 备用：PuTTY（https://www.putty.org），Host 填 IP → Open → 输 root/密码。

### 2.2 连接易断怎么办
- 网络波动或空闲超时会断开（`client_loop: send disconnect`），**重新 ssh 连接即可**，服务器没坏。
- 长时间操作时偶尔按回车保持活跃，或用 PuTTY（更稳）。

---

## 第 3 章 · 安装环境（MySQL / Nginx / ClamAV / 工具）

在服务器终端**逐条**执行：

```bash
# 1) 更新软件源
apt update

# 2) 一次性安装全部组件（跑几分钟，耐心等）
apt install -y mysql-server nginx clamav clamav-daemon ufw git curl
```

**验证**（可选）：
```bash
systemctl status mysql    # MySQL 应 active (running)
systemctl status nginx    # Nginx 应 active (running)
```

---

## 第 4 章 · 配置 MySQL（建库 + 建账号）

```bash
# 进入 MySQL（刚装完 root 免密）
mysql
```

在 `mysql>` 提示符下**逐条**执行（复制粘贴整段也可以，每条是独立语句）：

```sql
ALTER USER 'root'@'localhost' IDENTIFIED WITH mysql_native_password BY '你的root密码';
CREATE DATABASE IF NOT EXISTS qdu_wasteland CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'qdu'@'localhost' IDENTIFIED BY '你的应用密码';
GRANT ALL PRIVILEGES ON qdu_wasteland.* TO 'qdu'@'localhost';
FLUSH PRIVILEGES;
exit;
```

每条应返回 `Query OK`。完成后回到 `root@qdu-wasteland:~#`。

> ⚠️ **密码安全**：把上面两个密码换成自己的强密码，并**记下来**（第 8 章配置要用）。不要写进任何会提交 git 的文件。

---

## 第 5 章 · 配置防火墙（UFW，只开 22/80/443）

```bash
ufw allow 22/tcp      # SSH（先放行，防止把自己关门外）
ufw allow 80/tcp      # HTTP 网页
ufw allow 443/tcp     # HTTPS（后续用）
ufw enable            # 问 y/n 时输入 y
ufw status            # 确认 active + 三个端口 ALLOW
```

---

## 第 6 章 · 配置 Nginx（反向代理 80 → 3000）

**作用**：用户访问 80 端口，Nginx 把请求转给 Go 程序（监听 3000），以后还能加 HTTPS。

```bash
# 1) 打开编辑器（nano 若没装：apt install -y nano）
nano /etc/nginx/sites-available/qdu-wasteland
```

**粘贴**（nano 里右键=粘贴）：
```nginx
server {
    listen 80;
    server_name _;

    client_max_body_size 200m;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
    }
}
```

保存退出：`Ctrl+O` → 回车 → `Ctrl+X`

```bash
# 2) 启用配置 + 删默认 + 测试 + 重载
ln -s /etc/nginx/sites-available/qdu-wasteland /etc/nginx/sites-enabled/
rm /etc/nginx/sites-enabled/default
nginx -t                              # 必须显示 test is successful
systemctl reload nginx                # 使配置生效
systemctl status nginx                # active (running)
```

---

## 第 7 章 · 传程序 + 传课表数据（在你的本地电脑执行）

### 7.1 本地编译 Linux 版（PowerShell）

```powershell
cd F:\My_Projects\AI_projects\qdu-wasteland
$env:GOOS="linux"; $env:GOARCH="amd64"
go build -o qdu-wasteland-linux .
$env:GOOS=""; $env:GOARCH=""
```
生成 `qdu-wasteland-linux`（约 16MB）。

### 7.2 用 SCP 传到服务器（本地 CMD/PowerShell）

```powershell
# 传程序到 /root/
scp F:\My_Projects\AI_projects\qdu-wasteland\qdu-wasteland-linux root@你的IP:/root/

# 传课表 CSV 目录（8 份课表，可整个目录打包）
scp -r F:\My_Projects\AI_projects\_csv root@你的IP:/root/csv_data/

# ⚠️ 必须传前端文件夹 public/（否则网站 404！程序运行时 ./public/ 提供静态页面）
scp -r F:\My_Projects\AI_projects\qdu-wasteland\public root@你的IP:/root/
```

> SCP = 跨网络复制文件，语法：`scp 本地路径 root@IP:服务器路径`。输密码后等待 100% 完成。
>
> ⚠️ **三个都要传**：程序 + 课表 + **public 前端**。漏传 public 会导致首页 404（程序本身能跑，`/api/*` 正常，但页面全部打不开）。验证：服务器上 `ls /root/public/` 应看到 index.html 等前端文件。

---

## 第 8 章 · 启动 Go 程序（systemd 守护，开机自启）

### 8.1 创建 systemd 服务（服务器上）

```bash
nano /etc/systemd/system/qdu-wasteland.service
```

粘贴（**换成你自己的密码**）：
```ini
[Unit]
Description=QDU Wasteland
After=network.target mysql.service

[Service]
WorkingDirectory=/root/qdu-wasteland
ExecStart=/root/qdu-wasteland/qdu-wasteland-linux
Restart=always
Environment=CSV_DIR=/root/csv_data
Environment=PORT=:3000
Environment=ADMIN_EMAIL=admin@你的域名或邮箱
Environment=ADMIN_PASSWORD=你的站长密码
# 邮件（可选，不配则日志模式）
# Environment=MAIL_HOST=smtp.qq.com
# Environment=MAIL_PORT=465
# Environment=MAIL_USER=xxx@qq.com
# Environment=MAIL_PASS=授权码
# Environment=MAIL_FROM=xxx@qq.com
# AI 审查（可选）
# Environment=AI_API_URL=https://api.deepseek.com/v1/chat/completions
# Environment=AI_API_KEY=你的key
# Environment=AI_MODEL=deepseek-chat

[Install]
WantedBy=multi-user.target
```

保存退出（Ctrl+O → 回车 → Ctrl+X）。

### 8.2 放置程序并启动

```bash
# 建工作目录并把程序放进去
mkdir -p /root/qdu-wasteland
mv /root/qdu-wasteland-linux /root/qdu-wasteland/
chmod +x /root/qdu-wasteland/qdu-wasteland-linux

# 注册服务并启动
systemctl daemon-reload
systemctl enable qdu-wasteland      # 开机自启
systemctl start qdu-wasteland
systemctl status qdu-wasteland      # active (running) 即成功
```

---

## 第 9 章 · 验证上线

1. 浏览器打开：`http://你的服务器IP`
2. 看到网站首页 = ✅ 部署成功！
3. 用 admin 账号登录后台测试。

> ⚠️ 首次启动 Go 程序会加载 8 份课表（约 25 秒），期间页面可能打不开，属正常。

---

## 第 10 章 · 后续上线步骤（对应 MASTER-PLAN 阶段 H）

| 步骤 | 内容 | 说明 |
|------|------|------|
| **H0** | 上线前安全核查 | 对所有用户上传内容百分百安全检查（代码已就绪，逐项核对） |
| **H1** | Docker 化 | 可选：把 Go + Nginx 打进容器（当前 systemd 方式已可用，Docker 为规范化） |
| **H2** | HTTPS/域名 | 买域名 + Let's Encrypt 免费证书（`certbot`）或 Cloudflare |
| **H3** | SQLite → MySQL | 本项目当前开发用 SQLite，生产建议迁 MySQL（代码支持，需迁移脚本） |
| **H4** | 启用 ClamAV | 设置 `CLAMAV_CMD=/usr/bin/clamscan` 环境变量，病毒扫描上线 |
| **H5** | 数据备份 | 见下方"第 10.5 章 · 备份体系（H5 完整配置）" |
| **H6** | 压测 2000 | 用压测工具验证日常200/峰值2000，调优 Nginx/Go |
| **H7** | 境外 VPS + CDN | 已在做；文件下载可走 jsDelivr/Cloudflare 分流带宽 |
| **H8** | 运维文档 | 本文即运维文档的一部分 |

---

## 第 10.5 章 · 备份体系（H5 完整配置）

> 三方向备份：
> - **方向①**：Web 源码 → GitHub（public）—— 从开发机手动 `git push`
> - **方向②**：课程资料 → GitHub `QDU_course_resource`（public）—— 服务器定时（自动）
> - **方向③**：服务器完整备份 → 电脑（private）—— 电脑定时拉取（自动）

### 10.5.1 方向② · 课程资料备份（服务器 → GitHub）

**服务器上配置**（一次性）：

```bash
# 1. 把脚本放到服务器
scp deploy/course-backup.sh root@服务器IP:/root/qdu-wasteland/deploy/
chmod +x /root/qdu-wasteland/deploy/course-backup.sh

# 2. 安装 sqlite3（脚本读数据库用）
apt install -y sqlite3

# 3. 创建 GitHub Token（只授权 QDU_course_resource 仓库）：
#    GitHub → Settings → Developer settings → Personal access tokens → Tokens(classic)
#    → Generate → 勾选 repo → 生成后复制

# 4. 手动测试一次
QW_GITHUB_TOKEN=你的token /root/qdu-wasteland/deploy/course-backup.sh

# 5. 定时（每天凌晨 4 点）：
crontab -e
# 加一行：
0 4 * * * QW_GITHUB_TOKEN=你的token /root/qdu-wasteland/deploy/course-backup.sh >> /var/log/qdu-course-backup.log 2>&1
```

> ⚠️ 注意：GitHub 单文件 ≤100MB，脚本会自动跳过超大文件（由方向③兜底）。

### 10.5.2 方向③ · 服务器完整备份 → 电脑（private）

**服务器端**（已就绪，backup.go + deploy/backup.sh）：

```bash
# 配置定时（每天凌晨 3 点触发服务端备份接口）
crontab -e
# 加一行：
0 3 * * * QW_ADMIN_EMAIL=备份账号 QW_ADMIN_PASSWORD=密码 /root/qdu-wasteland/deploy/backup.sh >> /var/log/qdu-backup.log 2>&1
# 产物在 /root/qdu-wasteland/backups/（qdu-时间戳.db + uploads-时间戳.zip，轮转保留14份）
```

**电脑端**（本仓库 pull-backup.ps1）：

```powershell
# 1. 配置环境变量
$env:QW_SSH_HOST = 'root@207.148.106.155'          # 服务器
$env:QW_LOCAL_BACKUP_DIR = 'D:\QDU-backups'        # 电脑备份目录
$env:QW_BACKUP_GIT_REPO = 'https://github.com/youyan2000/qdu-server-backup.git'  # 私有仓库（可选）
$env:QW_BACKUP_GIT_TOKEN = '你的token'

# 2. 手动拉一次
powershell -ExecutionPolicy Bypass -File pull-backup.ps1

# 3. 定时（Windows 计划任务，每天凌晨 5 点）
schtasks /Create /SC DAILY /ST 05:00 /TN QW-PullBackup /TR "powershell -ExecutionPolicy Bypass -File F:\My_Projects\AI_projects\qdu-wasteland\pull-backup.ps1" /F
```

> ⚠️ 方向③备份含**用户数据**（数据库），推送的 GitHub 仓库**必须 Private**，绝不能 public。

### 10.5.3 恢复方法（万一服务器挂了）

1. 方向③：从电脑 `D:\QDU-backups\` 取最新 `qdu-时间戳.db` 和 `uploads-时间戳.zip`
2. 上传到新服务器对应目录（`qdu-auth.db` + `uploads/`）
3. 重启服务即恢复
4. 若只丢课程资料：从方向②的 GitHub 仓库拉回文件 + 清单

---

## 第 10·B 章 · 域名 + Cloudflare CDN + HTTPS（H2 实操版，2026-08 验证通过）

> 目标：让 `你的域名` 访问网站 + 免费 CDN 加速 + 免费 HTTPS + 防护。
> 全程**不需要备案**（境外 VPS + Cloudflare 免费版）。

### 10B.1 买域名（推荐：阿里云/腾讯云，支付宝付）

| 后缀 | 首年 | 续费 | Cloudflare 注册支持 |
|------|------|------|---------------------|
| `.top` | ¥14 | ¥39/年 | ❌ 不支持在 Cloudflare 注册（但可托管） |
| `.site` | ~¥10 | ~¥40 | ✅ 支持 |
| `.com` | ¥85 | ¥95 | ✅ 支持 |

**实操要点**：
- 选 `.top` 最省钱（¥14/年），**在阿里云买**（支付宝付，几乎不会失败）。
- ⚠️ **Cloudflare 注册商不支持 `.top`**（"cannot be registered"），但**托管 `.top` 完全没问题**——托管和注册是两回事。
- ⚠️ 别在 Cloudflare 用 PayPal 买域名，**支付常失败**（"An unexpected error..."），还可能产生欠费账单。要买就在 Cloudflare 用信用卡，或干脆阿里云买。
- 买完要**实名认证**（个人身份证，几分钟到几小时）。

### 10B.2 注册 Cloudflare（免费）

- https://dash.cloudflare.com/sign-up → 邮箱注册 → 验证。
- **免费版即可**，CDN/HTTPS/防护都有。

### 10B.3 把域名 DNS 托管给 Cloudflare（改 NS）

1. Cloudflare → **Add a site** → 输入域名 → 选 **Free** 计划。
2. Cloudflare 会分配**两个 Name Server**（形如 `xxx.ns.cloudflare.com` / `yyy.ns.cloudflare.com`）——**复制**。
3. 去阿里云域名控制台（https://dc.console.aliyun.com/）→ 你的域名 → **修改 DNS 服务器**：
   - 删掉阿里云的 `dns1.hichina.com` / `dns2.hichina.com`
   - 填 Cloudflare 的两个 NS（**不要带结尾点号**，如 `jocelyn.ns.cloudflare.com`）
   - 保存
4. 等 1-2 小时生效（实测可以很快）。验证：
   ```
   nslookup -type=NS 你的域名
   ```
   显示 Cloudflare 的 NS = 成功。

### 10B.4 Cloudflare 添加 A 记录

DNS 页面 → **Add record**：
| Type | Name | 内容 | 代理 |
|------|------|------|------|
| A | `@` | 服务器 IP（如 207.148.106.155） | 先灰云(DNS only) |
| A | `www` | 同上 | 先灰云 |

> 先灰云（DNS only）→ 等网站通了 → 再点成橙云（代理）启用加速。分步验证好排查。

### 10B.5 配 HTTPS（certbot 免费证书）

服务器上：
```bash
apt install -y certbot python3-certbot-nginx

# 让 Nginx 认新域名（把 server_name _; 改成你的域名）
sed -i 's/server_name _;/server_name 你的域名 www.你的域名;/' /etc/nginx/sites-available/qdu-wasteland
systemctl reload nginx

# 一键签发证书 + 自动配置 HTTPS + 强制跳转
certbot --nginx -d 你的域名 -d www.你的域名
```
- 邮箱填你的；同意条款 Y；分享邮箱 N；HTTP→HTTPS 重定向选 2。

### 10B.6 验证

- 浏览器打开 `https://你的域名` → 有小锁 🔒 = 成功
- Cloudflare 里把 A 记录点成**橙云（代理）** → CDN 加速 + 防护生效

### ⚠️ 踩坑记录（本次实操真实遇到）
1. **.top 不能在 Cloudflare 注册** → 阿里云买 + Cloudflare 托管，绕开。
2. **Cloudflare PayPal 支付失败** → 换信用卡，或阿里云买域名。
3. **密码含 `#` 写进 systemd .service** → `#` 变注释，密码被截断！**改用 EnvironmentFile（.env 文件）**，密码原样生效。
4. **漏传 public/ 前端文件夹** → 网站 404（程序在跑但没页面）。程序 + CSV + **public** 三样都要传。
5. **Nginx 502** = 后端没起来（等 3-4 分钟课表加载完）；**404** = 静态文件缺失（public 没传）。

---

## 附录 A · 常用运维命令

```bash
# 查看服务状态
systemctl status qdu-wasteland nginx mysql

# 重启服务
systemctl restart qdu-wasteland
systemctl reload nginx

# 看日志
journalctl -u qdu-wasteland -f        # Go 程序日志（实时）
tail -f /var/log/nginx/error.log      # Nginx 错误日志
tail -f /var/log/mysql/error.log      # MySQL 日志

# 更新网站程序（重新部署）
#  1. 本地重新编译 + scp 上传
#  2. systemctl restart qdu-wasteland

# 备份数据库
mysqldump -u qdu -p qdu_wasteland > /root/backup_$(date +%F).sql
```

## 附录 B · 本次实际操作记录（2026-08）

- 服务器：Vultr Tokyo，Ubuntu 22.04，IP `207.148.106.155`
- 已装：MySQL 8 / Nginx / ClamAV / UFW / git / curl
- 已配：MySQL 库 `qdu_wasteland` + 账号 `qdu`；UFW 放行 22/80/443；Nginx 反向代理 80→3000
- **已完成部署**：程序（systemd 守护 + 开机自启）· 课表 8 份 · public 前端 · 管理员账号
- **域名**：`qdwasteland.top`（阿里云购买 ¥14/年，实名已过）→ NS 已托管 Cloudflare（jocelyn/valentin.ns.cloudflare.com）
- **Cloudflare**：Free 计划，A 记录 @/www → 207.148.106.155（灰云，待开橙云加速）
- **待做**：certbot 配 HTTPS → 开橙云加速 → H0/H3/H6/H7 余项

> ⚠️ 密码请勿写进本文件；如已误写，用环境变量/密钥管理替代，并重置密码。
