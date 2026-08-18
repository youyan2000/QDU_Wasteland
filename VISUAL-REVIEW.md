# QDU Wasteland · 视觉美学改进评审报告

> 评审对象：`F:\My_Projects\AI_projects\qdu-wasteland\public\`（Go 后端 + 原生 HTML/CSS/JS 前端，共 13 个 CSS + 20 余个 HTML/JS）
> 评审方式：两名资深 UI/UX 视觉评审专家分模块深度审查 + 硬编码色值/一致性量化扫描 + 关键点交叉核对。
> 时间：2026-08-16 · 只评美学与视觉，不含功能 bug（代码级问题另列附录）。

---

## 一、总评

**审美水平：5.5 / 10（偏"能用的工程稿"，未达"完整视觉产品"）**

**优点（底子是好的）**：
- **CSS 变量体系健康**：`--magenta / --ink / --paper / --muted` 定义完整，首页 hero 的品红大字渐变、wash 洗膜、克制动效（transform+shadow 而非炫技）做得很到位，**首页确实达到了我们定的"阔绰且先锋"水准**。
- 导航系统统一（nav.css + nav.js），hover 反馈整体良好，响应式（clamp/grid auto-fill）有意识。
- 主题系统（暗黑/赛博/GitHub/左侧导航/设置面板）覆盖面广，思路完整。

**但普遍问题是**："各模块做到功能可用就停"，**没有把首页那套精致质感带到内页**——出现大量半成品痕迹、样式碎片化、跨模块不一致。

---

## 二、最该先处理的三件"系统性顽疾"

这三件不解决，其余都是治标：

### 🔴 S1 · 圆角体系完全没收敛
全站圆角散落 6+ 种值，毫无章法：
```
8px(pager courses) / 10px(offering/major) / 12px(course-card过滤面板)
14px(filter-panel/article-card) / 16px(modal/read-body) / 999px(pager forum)
```
**建议**：建立三档令牌 →
```css
--radius-sm: 8px; --radius-md: 12px; --radius-lg: 16px;
```
同类组件（如分页器）必须同一圆角。

### 🔴 S2 · CSS 重复定义 + 组件风格分裂
`.kicker`/`.empty`/`.pager`/`.btn`/`.btn-primary` 被各模块重复定义，且同名组件视觉不同：
- 分页器：forum `999px` vs courses `8px`
- 按钮：有的复用 `.btn.btn-primary`，有的写内联 `background:linear-gradient(...)`
**建议**：抽一个 `common.css`，集中管理 `.btn/.btn-primary/.btn-ghost/.kicker/.empty/.pager/.badge` 等，全站统一；内联按钮类全部改成复用类。

### 🔴 S3 · CSS 变量未贯穿 = 颜色碎片化
硬编码色值在各模块大量出现（me.css 26 个、courses.css 22 个、search 徽标等），且**关键语义色没有变量**：
- `--error / --success / --warn` 不存在 → 错误红 #d32f2f、成功后无统一色，散落各处
- 搜索徽标 `#3b82f6`（蓝）/`#8b5cf6`（紫）与品红主色无关
**建议**：在 `:root` 补语义变量，全站硬编码色替换为变量。
```css
--error:#d32f2f; --success:#1a9e5c; --warn-*:#fff5f8/#ff5c9d/#c2185b;
--badge-post:#3b82f6; --badge-article:#8b5cf6;
```

---

## 三、P0 · 立即修复（共 9 项，其中 3 项是功能性/视觉硬伤）

| # | 位置 | 问题 | 建议 |
|---|------|------|------|
| P0-1 | `courses.css` L194-195 | `.course-name span { display:block }` 推翻 L128 的 `margin-left` 行内布局 → 卡片学院名/badge 错乱 | 删除重复定义，或改 `.badge-inline` |
| P0-2 | `auth.css` L3 | 登录页全屏深紫黑渐变 `linear-gradient(160deg,#1a1220...)`，与全站纸感米白完全割裂 | 改 `background: var(--paper)` 或透明玻璃态叠米白 |
| P0-3 | `me.css` L68 | 残缺空 `@keyframes { }` 垃圾语法 + 错误夹带的 `@media` | 删除该行，补完整响应式 |
| P0-4 | `me.css` L81 | `.stat-card.active` 依赖 JS 加 class 但 CSS **无任何 active 样式** → 选中态不可辨识 | 加 `.stat-card.active{background:var(--ink);color:#fff}` |
| P0-5 | `nav.js` L117 | 铃铛面板用 `right = rect.left` 定位 → 面板被推到屏幕外 | 改用 `left = rect.left` + `right:auto` |
| P0-6 | `user.css` L20 | `.u-item:hover transform:translateX(2px)` 右移，右侧导航下可能触发横向滚动 | 改 `translateY(-2px)` 或纯颜色变化 |
| P0-7 | `themes.css` L55(GitHub) | GitHub 主题渐变依赖的 `--sunrise` 之前落空/不一致（现已核对 L57 有 `#1a7f37`，但与 `--magenta:#0969da` 一起用会偏绿） | 确认 GitHub 渐变两色同系，避免橙/绿混入 |
| P0-8 | `report.js` / `user.js` | 硬编码 `#ff2e88,#ff8a65` 渐变 + 成功提示无绿色反馈 | 改用 `var(--magenta)/var(--sunrise)`；加 `--success` 色 |
| P0-9 | `nav.css` L66 | announce-bar 渐变起点 `#fff3f8` 硬编码 | 改 `var(--magenta-soft)` → `var(--paper)` |

> ⚠️ 代码级硬伤（非美学但必修）：`course.html` L27 脚本标签里有 `` `r`n`` 乱码字符 → **JS 完全不执行**（P0）。

---

## 四、P1 · 高优先级改进（跨模块一致性）

### 4.1 Emoji 入侵 → 破坏"克制"美学
标题大量出现 📂💬🗓🏆🌱🎪，与"现代先锋+纸感+克制"定位相悖。
**建议**：换 SVG icon 或 CSS 装饰，保持克制。

### 4.2 课程模块（courses.css / course.html / courses.js）
- `course.html` L16 大段内联 `style="display:flex..."` → 提取为 CSS 类（结构/样式分离）
- 按钮系统分裂：有的 `.btn.btn-primary`，有的内联 → 统一
- `.badge.gray`（L138）`#ececf1` 硬编码 → 用变量
- `.offering-card` 无 hover（vs `.course-card` 有）→ 补
- `.filter-panel` 圆角 14px（应 12px）；`.oc-time/.oc-loc` `#666` → `var(--muted)`
- 星星 `.star-btn` `#ddd/#f5a623` 硬编码 → 定义 `--star-*` 变量
- `upload-box-inline` 虚线 `rgba(255,46,136,.4)` 过重 → `.3` 或实线细边

### 4.3 论坛模块（forum.css）
- 分页 `.pager` 圆角 `999px` → 统一 `8px`（与 courses 一致）
- `.like-btn` 无过渡 → 加 `transition:all .2s`
- 激活态语言不一：".all-chip 用下划线 vs .forum-chip 用渐变" → 统一
- `.modal backdrop-filter:blur(2px)` 性能差 → 移除或加粗
- `.err` class 在 forum.css 未定义 → 补

### 4.4 文章模块（articles.css / article-editor.html）
- `.article-card .ac-title` 重复定义（L38-42 vs L50）→ 删除
- `.cat-tag` 背景 `rgba(255,46,136,.08)` 过淡 → 用 `var(--magenta-soft)`
- 编辑器工具栏按钮无 active → 补 `:active`
- `read-body`(16px) vs `article-card`(14px) 圆角不一 → 统一
- **编辑器（article-editor.html）大量类名（editor-main/editor-head/editor-cat/editor-anon）完全没有样式定义** → 需新建编辑器专属样式
- 原生 checkbox 太粗糙 → 自定义 `appearance:none`

### 4.5 上传模块（upload.css）
- drop-zone 的 dragover 态无醒目动画 → 加 pulse 动效
- `.btn-next/.btn-prev` 无 hover → 补
- `.course-suggest` 阴影过重（比 card 还重）→ 降

### 4.6 导航/主题（nav.css / themes.css）
- `.site-nav.transparent` 是否实际被 nav.js 使用？若未使用或依赖深色背景 → 确认或删除
- 左侧导航模式下内容区（me-main/user-main 等）未 `padding-left:228px` 对齐 → 补
- 设置面板在暗黑/赛博下子元素对比度需验证

### 4.7 后台（admin.html）
- **所有样式内联 `<style>`，与设计体系完全脱节、无法主题化** → 抽 admin.css
- admin-deny 用了 `.btn` 但无定义 → 修正类名 + 居中（flex min-height:60vh）

---

## 五、P2 · 打磨级（低优先）

- viewer.css：`.md-view` Markdown 渲染色值碎片化（#eee/#f3f0ee/#e5e2dd）→ 统一变量；`.plain-view` 字体 `Consolas` → 加备选字体栈
- 搜索页信息密度偏低，`.rs-item` hover 只有 translateX 无阴影 → 补 shadow 对齐其他卡片
- `.nb-empty` 空状态无表情 → 加 🔔图标；`.nb-sec` 无底部分隔
- 表单/下拉无加载态（auth 初始化下拉、按钮 loading 不对称）
- 私信成功无绿色反馈

---

## 六、如果只改 5 个最重要的地方

1. **`courses.css` L194-195** — 修复 `.course-name span{display:block}` 推翻 badge 布局（课程列表可读性）
2. **`course.html` L27** — 修复脚本 `` `r`n`` 乱码 → JS 恢复执行
3. **`auth.css` L3** — 把教程页深紫黑渐变改回纸感米白，消除全网最刺目的割裂
4. **建立 `common.css` + 三项令牌（圆角/语义色/共享组件）** — 一次性收编全站重复与分裂
5. **抽 `admin.css` + 补 `editor` 样式** — 让后台与编辑器脱离"无样式"状态

---

## 七、附录 · 代码级问题（非美学但值得记录）

- `courses.js` L167 `eval()` 动态赋值 → 应改为直接赋值
- `viewer-mdown.js` 简易 Markdown 不支持 GFM 表格/任务列表；行内代码解析有边界漏洞
- 模态框 `display:none` 写在 HTML inline 而非 CSS

---

## 八、结束语

**首页已经达标（可以说漂亮）**，问题集中在内页的「一致性」与「完成度」。核心路径是：**三件系统性顽疾（圆角/重复定义/颜色变量）→ 两个硬伤页面（auth 背景、course 脚本）→ 统一组件库**。这属于"打磨提升"，不是推倒重来——地基是稳的。
