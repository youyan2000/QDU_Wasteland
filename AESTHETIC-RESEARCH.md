# QDU Wasteland · 美学设计研究（token 系统施工图）

> 状态：研究阶段，本次只产出方案，不实现代码。
> 依据：MASTER-PLAN 阶段 I / AESTHETIC-MASTERPLAN / 全站 CSS 现状摸底。

## 一、审美基准（已定）
> **"一所当代大学的'数字档案'：克制、有编辑感、有人味。"**
> 四支柱：克制 / 编辑感 / 校园记忆 / 当代。判据：改动后回问"符合克制+编辑感吗"。

## 二、现状摸底（本次实测）
### 2.1 硬编码色 40+ 种（无语义色）
- 品红系已定（style.css :root）。搜蓝 #3b82f6(3)、紫 #8b5cf6(3)、错误红 #d32f2f(4)、成功绿(散)、警告橙 #f5a623、渐变 #667eea/#764ba2(auth 深紫,最割裂)。#fff 109 处应进 token。

### 2.2 圆角实测 12 种（比蓝图说的 6 种更多）
- 999(19)/10(21)/14(12)/9(6)/6(7)/4(2)/18/2/7/8(23)/12(13)/16(7)
- 收敛为三档 + pill：sm8/md12/lg16/pill999。

### 2.3 间距：未系统化 -> base8(4/8/12/16/24/32/48)。
### 2.4 字体：正文已是 Aptos/雅黑，缺衬线装饰栈与等宽备选。
### 2.5 组件：各模块重复定义 .btn/.pager/.badge -> 收进 common.css。

## 三、Token 系统设计（common.css 蓝图）
### 3.1 色彩（在 :root 补齐，只增不改主值）
主色/中性已有；新增：
--surface:#fff; --border:rgba(26,26,46,.1); --border-strong:rgba(26,26,46,.18);
语义：--error:#c0392b/--error-soft:#fdecea; --success:#1a7f37/--success-soft:#e6f4ea; --warning:#b45309/--warning-soft:#fef3c7;
分类(学院马卡龙8色,低饱和): --cat-1:#5b8def; --cat-2:#8b7ec8; --cat-3:#2ec4b6; --cat-4:#f2a65a; --cat-5:#e2708a; --cat-6:#7fb069; --cat-7:#5cb3d9; --cat-8:#c49a5c;

### 3.2 三档令牌
圆角 sm8/md12/lg16/pill999; 间距 4/8/12/16/24/32/48(space-1..7);
阴影 sm/md/lg(染背景禁纯黑); 字体 sans/serif/mono; 动效 transform/opacity only <300ms + reduced-motion。

### 3.3 唯一强调色纪律
只允许品红一个强调色。搜索徽标蓝紫->移除改品红/分类色；auth 深紫渐变->纸感米白+品红（P0）。

## 四、迁移映射表
### 4.1 圆角
999->pill; 10/9/14->md(12); 8/7/6->sm(8); 16/18->lg(16); 4/2->sm。
### 4.2 颜色
各类白底->--surface/--paper; #d32f2f->--error; #fff3f8->--error-soft; #1a7f37->--success; #f5a623->--warning; #3b82f6->--cat-1/--sea; #8b5cf6/#667eea/#764ba2->--cat-2/移除; #666/#8a8a8a->--muted。
### 4.3 组件
.btn 系列、.pager、.badge/.pill、.empty、.modal 全部收进 common.css 统一。

## 五、执行路径
主线：1 建 common.css 挂全站 -> 2 P0 五硬伤 -> 3 试点课程页。
支线：语义色替换 / 圆角收敛 / 搜索蓝紫 / auth 深紫。
验收：真实浏览器截图对比 + ANTI-AI-SLOP 自检。

## 六、结论
不是推倒重来，是"立体系+收敛"。缺的是一套 token、全站对它的依赖、一个编辑感样板页。
下一步：先实现 common.css（token 地基）挂全站，再 P0 + 试点课程页。
