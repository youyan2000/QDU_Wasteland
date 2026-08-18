# QDU Wasteland · Design Contract（设计契约）

> 本文件是美学的**唯一事实来源**（design-md 方法论）。改任何样式前先读这里。
> 上位文档：`AESTHETIC-MASTERPLAN.md`（总蓝图）· `ANTI-AI-SLOP-GUIDE.md`（去AI味）· `VISUAL-REVIEW.md`（现状评审）。

## 审美主张（Aesthetic Bar）
> **"一所当代大学的数字档案：克制、有编辑感、有人味。"**
四支柱：克制 / 编辑感 / 校园记忆 / 当代。

## 设计 Token（全站唯一来源，禁止硬编码）
### 颜色
| Token | 值 | 用途 |
|-------|-----|------|
| `--magenta` | `#ff2e88` | 全站唯一强调色（锁定） |
| `--magenta-deep` | `#c2185b` | hover/强调深 |
| `--magenta-soft` | `#ffd1e5` | 点缀背景 |
| `--sunrise` | `#ff8a5c` | 渐变第二色（hero 用） |
| `--ink` | `#1a1a2e` | 深墨文字（近黑，禁纯黑） |
| `--paper` | `#faf6f2` | 纸感米白底（禁纯白） |
| `--muted` | `#8a7f87` | 次级文字 |
| `--surface` | 白 | 卡片面（暗黑主题下被覆盖） |
| `--surface-2` | 浅米 | 嵌套底（info-item/代码块） |
| `--error` | `#d32f2f` | 错误（全站统一，禁散落） |
| `--success` | `#1a9e5c` | 成功 |
| `--warn` | `#b26a00` | 警告 |
| `--border` | `rgba(26,26,46,.08-.18)` | 边框 |
| 分类马卡龙 | 低饱和 8 色 | 学院/模块区分（替代蓝紫徽标） |

### 圆角（三档，禁散落 6 种）
- `--radius-sm: 8px`（小控件/分页）
- `--radius-md: 12px`（卡片/面板）
- `--radius-lg: 16px`（大容器/modal）

### 间距（base 8）
`4/8/12/16/24/32/48`；卡片 padding 统一 1.2-1.4rem。

### 字体
- 正文/UI：无衬线 `Aptos / Microsoft YaHei`（可读性）
- 编辑感标题（关于学校/hero）：强字重无衬线或中文宋体系衬线
- 数字/数据：等宽或 `tabular-nums`

### 阴影
柔和、**染背景色**（非纯黑）；`box-shadow` 色带背景 hue。

### 动效
只 `transform/opacity` · `<300ms` · `prefers-reduced-motion` 必支持 · hover 统一微移+阴影。

## 组件契约（common.css 一处定义）
`.btn/.btn-primary/.btn-ghost/.kicker/.empty/.pager/.badge/.modal/.field/.input` 全站统一，禁止各模块内联/重复定义。

## 多主题
默认（米白）/ 暗黑 / 赛博朋克 / GitHub；`--surface/--surface-2/--border` 必须被各主题覆盖，保证暗色可读。

## 去 AI 味红线（摘要）
1 强调色 · 禁纯黑纯白 · 禁霓虹辉光 · 禁三卡平铺模板 · 禁 emoji 当 UI · 空态/加载/错误/选中态必须设计 · 文案去破折号堆砌/去空动词 · 无假数字/版本号。
