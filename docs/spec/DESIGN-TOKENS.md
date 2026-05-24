# SQLFlow UI 设计规范文档

> **需求编号**：SF-FEAT0013  
> **设计语言**：shadcn/ui + Tailwind CSS v4  
> **设计风格参考**：Vercel Dashboard、Linear、Supabase Dashboard  
> **作者**：苏晴 ✨  
> **日期**：2025-07-27  
> **版本**：v1.0

---

## 目录

1. [设计系统定义](#1-设计系统定义)
2. [全局布局设计](#2-全局布局设计)
3. [各页面设计稿](#3-各页面设计稿)
4. [通用组件规范](#4-通用组件规范)
5. [动效规范](#5-动效规范)
6. [响应式策略](#6-响应式策略)
7. [无障碍设计](#7-无障碍设计)

---

## 1. 设计系统定义

### 1.1 设计理念

SQLFlow 的视觉设计遵循以下原则：

- **专业克制**：面向 DBA 和开发者的工具型产品，界面应传递专业感和信任感
- **信息密度**：合理利用屏幕空间，不过度留白，但保持视觉呼吸感
- **深色优先**：以深色模式为默认主题，契合开发者使用习惯
- **操作高效**：减少视觉噪音，让用户专注于核心任务（写 SQL、审批工单）

### 1.2 配色方案

#### 深色主题（默认）

| Token | 用途 | 色值 | 预览 |
|-------|------|------|------|
| `--bg-base` | 页面底色 | `#0a0a0f` | 近纯黑，带微蓝底色 |
| `--bg-surface` | 卡片/面板底色 | `#111118` | 深灰，与 base 形成 1 级层次 |
| `--bg-elevated` | 浮层/输入框底色 | `#1a1a24` | 中灰，与 surface 形成 2 级层次 |
| `--bg-hover` | 悬停态底色 | `#22222e` | 交互反馈 |
| `--bg-sidebar` | 侧边栏底色 | `#08080d` | 比主底色更深，明确分区 |

| Token | 用途 | 色值 |
|-------|------|------|
| `--border-default` | 默认边框 | `#27272f` |
| `--border-subtle` | 轻边框（分隔线） | `#1e1e28` |
| `--border-hover` | 悬停边框 | `#3a3a4a` |

| Token | 用途 | 色值 |
|-------|------|------|
| `--text-primary` | 主文本 | `#ededf0` |
| `--text-secondary` | 次要文本 | `#9b9ba8` |
| `--text-tertiary` | 辅助文本 | `#6e6e80` |
| `--text-muted` | 占位/禁用文本 | `#4e4e60` |

| Token | 用途 | 色值 |
|-------|------|------|
| `--accent-primary` | 品牌主色（CTA） | `#f97316` (Orange 500) |
| `--accent-hover` | 品牌主色悬停 | `#fb923c` (Orange 400) |
| `--accent-muted` | 品牌主色浅底 | `#f97316 / 12%` |

**功能色**：

| 语义 | 色值 | 用途 |
|------|------|------|
| `--success` | `#10b981` | 成功、低风险、连接正常 |
| `--warning` | `#f59e0b` | 警告、中风险 |
| `--danger` | `#ef4444` | 错误、高风险、删除 |
| `--info` | `#3b82f6` | 信息提示、链接 |

**风险等级色**（工单与 AI 评审核心）：

| 等级 | 色值 | 标签样式 |
|------|------|----------|
| 低风险 | `#10b981` | `bg-emerald-500/15 text-emerald-400` |
| 中风险 | `#f59e0b` | `bg-amber-500/15 text-amber-400` |
| 高风险 | `#ef4444` | `bg-red-500/15 text-red-400` |
| 待评审 | `#8b5cf6` | `bg-violet-500/15 text-violet-400` |
| 已执行 | `#3b82f6` | `bg-blue-500/15 text-blue-400` |

**数据库类型标识色**：

| 类型 | 色值 |
|------|------|
| MySQL | `#3b82f6` (蓝色) |
| MongoDB | `#22c55e` (绿色) |
| PostgreSQL | `#6366f1` (靛蓝) |
| Redis | `#dc2626` (红色) |

#### 浅色主题

| Token | 色值 |
|-------|------|
| `--bg-base` | `#fafafa` |
| `--bg-surface` | `#ffffff` |
| `--bg-elevated` | `#f4f4f5` |
| `--bg-hover` | `#e4e4e7` |
| `--bg-sidebar` | `#ffffff` |
| `--border-default` | `#e4e4e7` |
| `--border-subtle` | `#f4f4f5` |
| `--text-primary` | `#09090b` |
| `--text-secondary` | `#52525b` |
| `--text-tertiary` | `#71717a` |
| `--text-muted` | `#a1a1aa` |
| `--accent-primary` | `#ea580c` (Orange 600) |
| `--accent-hover` | `#c2410c` (Orange 700) |
| `--accent-muted` | `#ea580c / 10%` |

### 1.3 字体

| 用途 | 字体 | 备选 | 字号 | 字重 |
|------|------|------|------|------|
| 正文 | Inter | system-ui, -apple-system | 14px | 400 |
| 标题 H1 | Inter | — | 20px | 600 (semibold) |
| 标题 H2 | Inter | — | 16px | 600 |
| 标题 H3 | Inter | — | 14px | 600 |
| 小字/辅助 | Inter | — | 12px | 400 |
| 极小标注 | Inter | — | 10px | 500 |
| 代码 | JetBrains Mono | Fira Code, Consolas | 13px | 400 |
| 代码小字 | JetBrains Mono | — | 12px | 400 |

**Tailwind CSS 变量**：

```css
--font-sans: 'Inter', system-ui, -apple-system, sans-serif;
--font-mono: 'JetBrains Mono', 'Fira Code', 'Consolas', monospace;
```

### 1.4 间距系统

基于 4px 网格，使用 Tailwind 的间距单位：

| Token | 值 | Tailwind | 典型用途 |
|-------|------|----------|----------|
| `space-0.5` | 2px | `gap-0.5` | 图标与文字间隙 |
| `space-1` | 4px | `gap-1` | 紧凑元素间隙 |
| `space-1.5` | 6px | `gap-1.5` | 表单项内部间距 |
| `space-2` | 8px | `gap-2` | 通用元素间距 |
| `space-3` | 12px | `gap-3` | 表单组间距 |
| `space-4` | 16px | `gap-4` | 卡片内边距、表单组间距 |
| `space-5` | 20px | `gap-5` | 区块间距 |
| `space-6` | 24px | `gap-6` | 页面区块大间距 |
| `space-8` | 32px | `gap-8` | 页面大留白 |

**关键布局间距**：

- 侧边栏宽度：展开 220px，收起 56px
- 顶栏高度：52px
- 页面内容内边距：24px (`p-6`)
- 卡片内边距：16px~24px
- 表格行高：40px (`h-10`)
- 表头行高：36px (`h-9`)

### 1.5 圆角

| Token | 值 | Tailwind | 用途 |
|-------|------|----------|------|
| `--radius-sm` | 4px | `rounded-sm` | Badge、小标签 |
| `--radius-md` | 6px | `rounded-md` | Button、Input、Select |
| `--radius-lg` | 8px | `rounded-lg` | Card、Dialog、下拉面板 |
| `--radius-xl` | 12px | `rounded-xl` | 登录卡片 |
| `--radius-full` | 9999px | `rounded-full` | Avatar、圆点指示器 |

### 1.6 阴影

深色模式下阴影需要更高透明度才可见：

| Token | 值 | 用途 |
|-------|------|------|
| `--shadow-sm` | `0 1px 2px rgba(0,0,0,0.4)` | 按钮悬停 |
| `--shadow-md` | `0 4px 8px rgba(0,0,0,0.5)` | 卡片 |
| `--shadow-lg` | `0 10px 25px rgba(0,0,0,0.6)` | Dialog、Dropdown |
| `--shadow-xl` | `0 20px 40px rgba(0,0,0,0.7)` | Sheet (抽屉) |

浅色模式下将透明度降低至 0.05~0.15。

### 1.7 图标

- **图标库**：Lucide React
- **默认尺寸**：18px（导航图标）、16px（操作图标）、14px（内联图标）、12px（微小标注）
- **线宽**：1.5px（Lucide 默认）
- **颜色**：跟随文本色或使用功能色，不使用填充图标

---

## 2. 全局布局设计

### 2.1 整体结构

```
┌──────────────────────────────────────────────────┐
│                    Browser                        │
│  ┌─────────┬─────────────────────────────────┐   │
│  │         │         顶栏 (52px)              │   │
│  │         │  ┌─────────────────┬──────────┐  │   │
│  │  侧边栏  │  │  ⌘K 搜索框      │  Avatar  │  │   │
│  │         │  └─────────────────┴──────────┘  │   │
│  │ 220px   ├─────────────────────────────────┤   │
│  │ (收起    │                                  │   │
│  │  56px)  │         主内容区域                │   │
│  │         │         (overflow-auto)           │   │
│  │         │                                  │   │
│  │  Brand  │                                  │   │
│  │  Nav    │                                  │   │
│  │  Items  │                                  │   │
│  │         │                                  │   │
│  │ ─────── │                                  │   │
│  │  Collapse│                                  │   │
│  └─────────┴─────────────────────────────────┘   │
└──────────────────────────────────────────────────┘
```

### 2.2 侧边栏导航

**品牌区**：
- 高度 56px，底部 1px border-subtle 分隔
- 展开：`Database` 图标 (24px) + "SQLFlow" 文字 (text-lg font-bold accent-primary)
- 收起：仅 `Database` 图标，居中

**导航项**：

| 图标 | 标签 | 路由 |
|------|------|------|
| `LayoutDashboard` | 概览 | `/` |
| `Database` | 查询 | `/query` |
| `FileText` | 工单 | `/tickets` |
| `ShieldCheck` | 权限 | `/permissions` |
| `ScrollText` | 审计 | `/audit` |
| `User` | 用户管理 | `/users`（仅 admin） |
| `Settings` | 设置 | `/settings/*` |

**导航项交互**：
- 默认态：`text-secondary`
- 悬停态：`bg-elevated` + `text-primary`
- 激活态：`bg-accent-muted (accent-primary/10%)` + `text-accent-primary` + `font-medium`
- 收起态：仅显示图标，悬停时右侧 Tooltip 显示标签名

**设置子菜单**：
- 点击 Settings 展开/收起子菜单（带 ChevronDown 旋转动画）
- 子项缩进 `ml-4`，左侧 `border-l border-subtle`
- 子项包括：数据源管理、脱敏规则、AI 配置

**底部折叠按钮**：
- 顶部 `border-t border-subtle` 分隔
- 图标：展开态显示 `PanelLeftClose`，收起态显示 `PanelLeft`
- 状态持久化到 `localStorage('sidebar-collapsed')`

### 2.3 顶栏

- 高度 52px，底部 `border-b border-default`
- 背景 `bg-surface`

**左侧**：命令面板触发器
- 样式：`border border-default bg-elevated rounded-md`，`h-8`
- 内容：`Search` 图标 + "搜索..." 文本 + `<kbd>⌘K</kbd>` 快捷键提示
- 悬停：`border-accent-primary text-secondary`

**右侧**：用户头像下拉
- `Avatar` (sm) + `ChevronDown` 箭头
- 点击弹出 `DropdownMenu`（右对齐）
- 菜单项：
  - 用户名标签（`DropdownMenuLabel` + `User` 图标）
  - 分隔线
  - 修改密码（`KeyRound` 图标）
  - 主题切换（`Sun/Moon` 图标，动态显示）
  - 分隔线
  - 退出登录（`LogOut` 图标，`variant="destructive"`）

### 2.4 命令面板 (Command Palette)

- 使用 `cmdk` 组件（shadcn/ui 的 Command）
- 快捷键：`⌘K` / `Ctrl+K`
- 视觉：居中浮层，`shadow-xl`，宽度 520px
- 功能：搜索页面、工单、跳转快捷操作
- 分组：导航、最近工单、快捷操作

---

## 3. 各页面设计稿

### 3.1 登录/注册页

**布局**：全屏居中卡片

```
┌──────────────────────────────────────┐
│           bg-base (#0a0a0f)          │
│                                      │
│     ┌──────────────────────────┐     │
│     │   🔶 SQLFlow             │     │
│     │   SQL 审批管理平台        │     │
│     │                          │     │
│     │   ┌──────────────────┐   │     │
│     │   │  👤 用户名        │   │     │
│     │   └──────────────────┘   │     │
│     │   ┌──────────────────┐   │     │
│     │   │  🔒 密码          │   │     │
│     │   └──────────────────┘   │     │
│     │                          │     │
│     │   ┌──────────────────┐   │     │
│     │   │     登 录         │   │     │
│     │   └──────────────────┘   │     │
│     │                          │     │
│     │   © 2026 SQLFlow         │     │
│     └──────────────────────────┘     │
│                                      │
└──────────────────────────────────────┘
```

**详细规范**：

| 元素 | 规范 |
|------|------|
| 卡片 | `w-[400px] rounded-xl border border-default bg-surface shadow-xl` |
| 品牌区 | `Database` 图标 (28px) + "SQLFlow" (text-2xl font-bold accent-primary) |
| 副标题 | "SQL 审批管理平台" (text-sm text-secondary) |
| 输入框 | `Input` — `h-10 bg-elevated border-default`，placeholder 灰色 |
| 错误信息 | 输入框下方 `text-xs text-danger`，带 `mt-1` |
| 服务器错误 | 顶部红色横幅 `bg-red-500/10 text-red-400 rounded-md px-3 py-2` |
| 登录按钮 | `Button` — `w-full bg-accent-primary text-white hover:bg-accent-hover h-10` |
| 加载态 | 按钮内 `Loader2` 旋转 + "登录中..." 文字 |
| 版权 | `text-xs text-muted text-center mt-6` |

**交互**：
- 输入框失焦触发验证，错误信息渐显
- 登录成功跳转 `/query`
- Enter 提交
- 输入框 `onFocus` 时 `border-accent-primary`

### 3.2 概览页 (Dashboard)

**布局**：居中内容区，最大宽度 960px

```
┌─────────────────────────────────────────┐
│  概览                                    │
│                                          │
│  ┌──────────┐  ┌──────────┐             │
│  │ 📋 5     │  │ 🗄️ 128   │             │
│  │ 待审批工单 │  │ 近7天查询  │             │
│  └──────────┘  └──────────┘             │
│  ┌──────────┐  ┌──────────┐             │
│  │ 🖥️ 4     │  │ 👥 12    │             │
│  │ 活跃数据源 │  │ 系统用户数 │             │
│  └──────────┘  └──────────┘             │
│  ┌──────────┐                           │
│  │ ⚠️ 3     │                           │
│  │ 敏感表    │                           │
│  └──────────┘                           │
└─────────────────────────────────────────┘
```

**统计卡片**：

| 元素 | 规范 |
|------|------|
| 容器 | `grid grid-cols-2 gap-4` |
| 卡片 | `Card rounded-lg hover:shadow-md transition-shadow cursor-pointer` |
| 图标 | 40x40 圆角方块 `rounded-lg` + 各自颜色底色 |
| 数值 | `text-2xl font-bold text-primary` |
| 标签 | `text-sm text-secondary` |
| 可点击卡片 | 点击跳转对应页面 |

**图标配色**：

| 统计项 | 图标 | 颜色 |
|--------|------|------|
| 待审批工单 | `FileText` | `text-blue-500 bg-blue-500/10` |
| 近 7 天查询 | `Database` | `text-green-500 bg-green-500/10` |
| 活跃数据源 | `Server` | `text-purple-500 bg-purple-500/10` |
| 系统用户数 | `Users` | `text-orange-500 bg-orange-500/10` |
| 敏感表 | `ShieldAlert` | `text-red-500 bg-red-500/10` |

### 3.3 数据源管理

> 位于 设置 → 数据源

**页面结构**：

```
┌─────────────────────────────────────────┐
│  数据源配置            [+ 添加数据源]     │
│                                          │
│  ┌────────────────────────────────────┐  │
│  │ Table                              │  │
│  │ 名称 | 类型 | 地址 | 数据库 | 敏感表 │  │
│  │       | 状态 | 操作                │  │
│  ├────────────────────────────────────┤  │
│  │ prod-mysql  MySQL  10.0.0.1:3306  │  │
│  │             mydb    3   正常       │  │
│  │             [编辑] [测试] [禁用]    │  │
│  └────────────────────────────────────┘  │
└─────────────────────────────────────────┘
```

**表格规范**：

| 元素 | 规范 |
|------|------|
| 容器 | `rounded-lg border border-default bg-surface` |
| 表头 | `bg-surface text-secondary text-xs font-medium` |
| 数据行 | `border-default hover:bg-elevated` |
| 类型 Badge | MySQL: `bg-blue-500/20 text-blue-400`，MongoDB: `bg-green-500/20 text-green-400` |
| 状态 Badge | 正常: `bg-emerald-500/20 text-emerald-400`，异常: `bg-red-500/20 text-red-400`，已禁用: `bg-gray-500/20 text-gray-400` |
| 敏感表计数 | 有数据时: `bg-red-500/15 text-red-400` + ShieldAlert 图标；无数据: 灰色 "0" |
| 操作按钮 | `Button variant="ghost" size="sm" h-7` — 编辑/测试/禁用 |

**添加/编辑弹窗** (`Dialog`)：

- 宽度 `sm:max-w-lg`
- 表单 2 列布局：名称+类型、主机+端口、用户名+密码
- 默认数据库和最大连接数各占一行
- 类型 `Select` 联动端口默认值 (MySQL→3306, MongoDB→27017)
- 编辑时密码字段 placeholder "留空不修改"
- 底部：取消 (outline) + 保存 (accent-primary)

**测试连接**：
- 点击测试按钮，按钮内显示 `Loader2` 旋转
- 成功/失败通过 Toast 通知

**禁用确认** (`AlertDialog`)：
- 标题 "确认禁用数据源"
- 描述 "确定要禁用数据源「{name}」吗？禁用后相关查询将不可用。"
- 取消 + 确认禁用 (`bg-red-600`)

### 3.4 SQL 查询编辑器

**页面结构**：

```
┌─────────────────────────────────────────┐
│ 工具栏: [数据源▼] [数据库] [MongoDB?]    │
│                                 [历史]   │
├─────────────────────────────────────────┤
│ 查询标签页: [Tab1 ×] [+]               │
├─────────────────────────────────────────┤
│                                          │
│  ┌──────────────────────────────────┐    │
│  │     SQL 编辑器 (CodeMirror)      │    │
│  │     上半区 (可拖拽调整)           │    │
│  └──────────────────────────────────┘    │
│  ─ ─ ─ ─ 拖拽分割线 ─ ─ ─ ─ ─ ─ ─      │
│  ┌──────────────────────────────────┐    │
│  │     AI 评审卡片 (按需显示)       │    │
│  └──────────────────────────────────┘    │
│  ┌──────────────────────────────────┐    │
│  │     结果表格 + 分页              │    │
│  │     下半区                       │    │
│  └──────────────────────────────────┘    │
├─────────────────────────────────────────┤
│ 状态栏: 执行中... | 128 行 | 12ms       │
└─────────────────────────────────────────┘
```

**工具栏**：

| 元素 | 规范 |
|------|------|
| 容器 | `border-b border-default bg-surface px-3 py-2` |
| 数据源 Select | `h-7 w-48 text-xs`，项前带数据库类型色点 |
| 数据库输入 | `h-7 w-36 text-xs input bg-elevated` |
| MongoDB 标识 | `rounded bg-green-500/20 px-1.5 py-0.5 text-[10px] font-medium text-green-400` |
| 历史按钮 | `Button variant="ghost" size="sm" h-7` + `Clock` 图标 |

**查询标签页**：

| 元素 | 规范 |
|------|------|
| 容器 | `border-b border-default bg-surface` |
| 标签 | `h-8 px-3 text-xs` — 激活: `bg-base text-primary border-b-2 border-accent-primary`；非激活: `text-secondary hover:text-primary` |
| 关闭按钮 | `×` 图标 12px，`text-muted hover:text-danger` |
| 新建按钮 | `+` 图标 |

**SQL 编辑器 (CodeMirror)**：

- 主题：深色自定义主题，与整体配色一致
- 字体：JetBrains Mono, 13px
- 行号：`text-muted`
- 光标色：`accent-primary`
- 选中高亮：`accent-primary / 20%`
- SQL 语法高亮：关键字 `#c084fc` (紫色)、字符串 `#34d399` (绿色)、数字 `#fbbf24` (黄色)、注释 `#6b7280` (灰色)
- 自动补全：schema 表名/列名补全，下拉面板使用 shadcn/ui Command 组件风格
- 快捷键：`Ctrl+Enter` / `⌘+Enter` 执行

**AI 评审卡片**：

评审状态流转：
1. `idle` → 不显示
2. `reviewing` → 显示流式输出卡片
3. `reviewing` 完成 → 显示评审结果

| 状态 | 卡片样式 |
|------|----------|
| 评审中 | `border border-violet-500/30 bg-violet-500/5 rounded-lg p-3`，流式文字逐字显示 |
| 低风险 (execute) | `border border-emerald-500/30 bg-emerald-500/5`，绿色调，显示 "✓ 安全 — 自动执行中..."，1 秒后自动执行 |
| 中风险 (confirm) | `border border-amber-500/30 bg-amber-500/5`，黄色调，显示评审摘要 + [确认执行] [提交工单] 按钮 |
| 高风险 (reject) | `border border-red-500/30 bg-red-500/5`，红色调，显示评审摘要 + [提交工单] 按钮 |
| 错误 | `border border-red-500/30 bg-red-500/5`，显示错误信息 + [关闭] |

**结果表格**：

| 元素 | 规范 |
|------|------|
| 表头 | `sticky top-0 bg-surface text-xs text-secondary font-medium` |
| 数据行 | `hover:bg-elevated` |
| 脱敏列 | 单元格内显示 `****`，列头带 `EyeOff` 图标 + 橙色 tooltip "该字段已脱敏" |
| 空结果 | 居中提示 "查询结果为空" |
| 超长文本 | `truncate max-w-[200px]`，悬停 tooltip 显示完整内容 |

**状态栏**：

| 元素 | 规范 |
|------|------|
| 容器 | `h-7 border-t border-default bg-surface px-3` |
| 左侧 | 执行状态：执行中 `Loader2 spin text-accent-primary` / 完成 `text-success` / 失败 `text-danger` |
| 右侧 | 行数、耗时 (ms)、[导出 CSV] 按钮 |
| 导出按钮 | `text-xs text-muted hover:text-primary` |

**可拖拽分割线**：

- 高度 4px，`cursor-row-resize`
- 悬停 `bg-accent-primary / 30%`
- 默认上下比例 50%:50%
- 最小上下高度 120px

**工单提交抽屉** (`Sheet`)：

- 从右侧滑出，宽度 480px
- 内容：SQL 预览（只读）、变更原因输入框
- 底部：取消 + [提交工单] 按钮

### 3.5 工单管理

**页面结构**：

```
┌─────────────────────────────────────────┐
│  变更工单              [+ 提交新工单]     │
├─────────────────────────────────────────┤
│  [全部][待审批][已通过][已拒绝][已取消][已执行] │
├─────────────────────────────────────────┤
│  [我提交的][待我审批] │ [数据源▼][AI风险▼] │ 🔍搜索│
├─────────────────────────────────────────┤
│  #1  ALTER TABLE ADD...  my_db          │
│      中风险  待审批  2025-07-27 14:30    │
│  #2  UPDATE users SET...  prod_db       │
│      高风险  已拒绝  2025-07-26 10:15    │
├─────────────────────────────────────────┤
│  共 28 条，第 1/2 页            [<] [>]  │
└─────────────────────────────────────────┘
```

**页面头部**：

| 元素 | 规范 |
|------|------|
| 标题 | "变更工单" (text-base font-semibold) |
| 新建按钮 | `Button size="sm"` — accent-primary，`Plus` 图标 + "提交新工单" |

**状态 Tabs**：

| 元素 | 规范 |
|------|------|
| 样式 | `TabsList variant="line"` — 下划线式标签 |
| 标签 | 全部 / 待审批 / 已通过 / 已拒绝 / 已取消 / 已执行 |
| 激活态 | 底部 2px accent-primary 线 + text-primary |

**筛选栏**：

| 元素 | 规范 |
|------|------|
| 快捷筛选 | "我提交的" / "待我审批" — `Button variant="ghost" size="sm" h-7`，激活时 accent-primary |
| 分隔线 | `h-4 w-px bg-default` |
| 数据源 Select | `h-7 w-32 text-xs` |
| AI 风险 Select | `h-7 w-28 text-xs`，选项：全部/低风险/中风险/高风险 |
| 搜索 | `Input h-7 w-48 ml-auto`，左带 Search 图标 |

**工单列表表格**：

| 列 | 宽度 | 样式 |
|----|------|------|
| ID | 64px | `text-xs font-medium accent-primary`，如 `#12` |
| SQL 摘要 | flex | `text-xs max-w-[300px] truncate`，悬停 tooltip 显示完整 SQL |
| 数据库 | 96px | `text-xs text-secondary` |
| AI 风险 | 96px | 圆角标签 + 色点指示器 + 风险文本 |
| 状态 | 96px | Badge（对应状态色） |
| 提交时间 | 112px | `text-xs text-muted` |

**行交互**：
- `cursor-pointer`
- 悬停 `bg-elevated`
- 点击打开右侧 Detail Drawer

**分页**：

| 元素 | 规范 |
|------|------|
| 容器 | `border-t border-default bg-surface px-6 py-2` |
| 信息 | "共 {total} 条，第 {page}/{totalPages} 页" (text-xs text-muted) |
| 按钮 | `<` / `>` — `Button variant="ghost" size="sm" h-7 w-7 p-0` |

**工单详情抽屉** (`Sheet`)：

- 从右侧滑出，宽度 60%，最大 720px
- 内容结构（从上到下）：

```
SheetHeader: "工单 #{id}"

状态 + 风险等级 Badges (横排)
提交人 | 提交时间 | 数据库类型 > 数据库名 (text-xs text-secondary)
────────────────────────────
SQL 内容:
┌ pre 代码块 (max-h-48 overflow-auto) ─┐
│ ALTER TABLE users ADD COLUMN ...     │
│                          [Copy]      │
└──────────────────────────────────────┘

AI 评审: (如有)
┌ 圆角面板 ──────────────────────────────┐
│ 摘要文本                               │
│ • 建议 1                               │
│ • 建议 2                               │
│ 影响分析: ...                          │
└────────────────────────────────────────┘

变更原因: (如有)
审批记录: (如有)
────────────────────────────
评论/讨论区
  [评论输入框]
  [评论列表...]
────────────────────────────
SheetFooter:
  [✓ 通过]  [✕ 拒绝]  [▶ 执行]  [⊘ 取消工单]
```

**详情抽屉底部按钮**：

| 按钮 | 样式 | 条件 |
|------|------|------|
| 通过 | `bg-emerald-600 text-white` | DBA + 待审批 |
| 拒绝 | `outline border-red-500/50 text-red-400` | DBA + 待审批 |
| 执行 | `bg-accent-primary text-white` | 提交者/DBA + 已通过 |
| 取消工单 | `variant="ghost" text-muted` | 提交者/DBA + 未完成状态 |

**提交新工单页**：

- 独立页面 (`/tickets/new`)
- 顶部：返回箭头 + "提交新工单" 标题
- 表单居中，最大宽度 680px
- 字段：数据源 Select、数据库名 Input、SQL Textarea (monospace, min-h-[200px])、变更原因 Textarea (min-h-[100px])
- 底部：取消 + 提交工单（accent-primary + Send 图标）

### 3.6 审计日志

**页面结构**：

```
┌─────────────────────────────────────────┐
│  审计日志                      [导出CSV] │
├─────────────────────────────────────────┤
│  [用户▼] [操作▼] [数据源▼]              │
│  [开始日期] ~ [结束日期]    🔍搜索SQL/表名│
├─────────────────────────────────────────┤
│  ▶  14:30  admin  SELECT  my_db         │
│     SELECT * FROM users WHERE ...       │
│  ▶  14:28  dev1   UPDATE  prod_db       │
│     UPDATE orders SET status...         │
│  ▼  14:25  admin  EXPORT  report_db     │
│    ┌ 展开详情 ─────────────────────────┐ │
│    │ 完整 SQL: (pre 块，可复制)         │ │
│    │ 执行耗时: 45ms                     │ │
│    │ 影响行数: 0                        │ │
│    │ 返回行数: 128                      │ │
│    │ IP: 192.168.1.100                  │ │
│    │ 关联工单: #12 待审批               │ │
│    │ 脱敏字段: [phone] [email]          │ │
│    └──────────────────────────────────┘ │
├─────────────────────────────────────────┤
│  共 156 条，第 1/4 页          [<] [>]   │
└─────────────────────────────────────────┘
```

**筛选栏**：

| 筛选项 | 组件 | 规格 |
|--------|------|------|
| 用户 | `Select h-7 w-28` | 全部用户 + 用户列表 |
| 操作类型 | `Select h-7 w-28` | 全部 / SELECT / INSERT / UPDATE / DELETE / DDL / EXPORT |
| 数据源 | `Select h-7 w-32` | 全部 + 数据源列表 |
| 日期范围 | 两个 `Input type="date" h-7 w-[124px]`，中间 `~` |
| 搜索 | `Input h-7 w-48 ml-auto`，带 Search 图标 |

**表格**：

| 列 | 宽度 | 说明 |
|----|------|------|
| 展开 | 32px | `ChevronRight` / `ChevronDown` 图标 |
| 时间 | 140px | 相对时间，如 "2 分钟前"，tooltip 显示完整时间 |
| 用户 | 96px | 用户名 |
| 操作 | 96px | Badge — SELECT: `bg-blue-500/15 text-blue-400`，UPDATE: `bg-amber-500/15 text-amber-400` 等 |
| 数据库 | 112px | 数据库名 |
| SQL 摘要 | flex | `truncate max-w-[400px]`，tooltip 完整 SQL |

**展开行**：

- 背景 `bg-elevated/30`
- 展开动画 `audit-row-expand`：300ms ease-out，从 opacity:0 + max-height:0 到完整高度
- 内容网格 `grid grid-cols-4 gap-3`

展开行内容：

| 区块 | 规范 |
|------|------|
| 完整 SQL | `pre` 块，`max-h-[200px] overflow-auto rounded-md border bg-base p-3 font-mono text-xs`，右上角 [复制] 按钮 |
| 执行耗时 | 如果 > 1000ms 显示秒，否则显示 ms |
| 影响行数 / 返回行数 | 千分位格式 |
| IP 地址 | — |
| 数据库类型 | Badge `bg-sky-500/15 text-sky-400` |
| 关联工单 | 搜索匹配，显示工单 ID + 摘要 + 状态 Badge，可点击跳转 |
| 脱敏字段 | Badge 列表 `bg-amber-500/15 text-amber-400` |
| 错误信息 | `text-red-400`（仅失败时显示） |

**导出 CSV**：
- `Button variant="outline"` + `Download` 图标
- 导出时按钮内 `Loader2` 旋转
- 最多导出 10000 条
- CSV 带 BOM 头，UTF-8

### 3.7 用户管理 + RBAC 权限配置

> 权限管理页面 (`/permissions`) 使用 Tabs 切换三个子面板

**页面结构**：

```
┌─────────────────────────────────────────┐
│  权限管理           [角色管理][权限策略][用户管理] │
├─────────────────────────────────────────┤
│  共 12 个用户                   [+ 新建用户] │
│                                          │
│  🔍 搜索用户名...                         │
│                                          │
│  ┌────────────────────────────────────┐  │
│  │ 用户名    角色        创建时间  状态  │  │
│  │ admin    [管理员]    07/01    活跃  │  │
│  │          [编辑][重置密码][禁用]      │  │
│  │ dev1     [开发人员]  07/15    活跃  │  │
│  │          [编辑][重置密码][禁用]      │  │
│  └────────────────────────────────────┘  │
│  第 1/1 页                    [<] [>]    │
└─────────────────────────────────────────┘
```

**角色 Badge 配色**：

| 角色 | Badge 样式 |
|------|-----------|
| admin (管理员) | `bg-accent-primary/20 text-accent-primary` |
| dba (DBA) | `bg-violet-500/20 text-violet-400` |
| developer (开发人员) | `bg-blue-500/20 text-blue-400` |

**操作按钮**：

| 操作 | 图标 | 样式 | 条件 |
|------|------|------|------|
| 编辑 | `Pencil` | ghost, text-secondary | 非 admin |
| 重置密码 | `KeyRound` | ghost, text-secondary | 所有 |
| 禁用 | `Ban` | ghost, hover:text-red-400 | 非 admin 且未禁用 |

**新建用户弹窗** (`Dialog`)：

- 字段：用户名、密码、角色 (Select)
- 用户名规则：3-32 字符，字母数字下划线
- 密码规则：8-128 字符，必须含字母和数字
- 底部：取消 + 创建 (accent-primary)

**编辑用户弹窗**：
- 字段：用户名、角色
- 同上验证规则

**重置密码弹窗** (`AlertDialog`)：
- 显示目标用户名
- 密码输入框 + 验证
- 底部：取消 + 确认重置

**禁用确认弹窗** (`AlertDialog`)：
- "确定要禁用用户「{username}」吗？禁用后该用户将无法登录。"
- 确认按钮 `bg-red-600`

**角色管理 / 权限策略**：
- 当前显示 "功能开发中..." 占位
- 后续设计待需求明确后补充

### 3.8 数据脱敏规则管理

> 位于 设置 → 脱敏规则，内含两个子 Tab

**子 Tab 切换**：

| 元素 | 规范 |
|------|------|
| 容器 | `rounded-lg bg-elevated p-1 w-fit` |
| 按钮 | `rounded-md px-3 py-1.5 text-sm` |
| 激活态 | `bg-accent-primary text-white` |
| 非激活态 | `text-secondary hover:text-primary` |

**子 Tab 1：敏感表标记**

| 元素 | 规范 |
|------|------|
| 标题 | "敏感表标记" (text-lg font-semibold) |
| 新建按钮 | "标记敏感表" + Plus 图标 |
| 数据源筛选 | Select w-48 |
| 表名 | 带 ShieldAlert 图标 + 红色底色标签 `bg-red-500/15` |
| 敏感等级 Badge | low: `bg-emerald-500/20`，medium: `bg-amber-500/20`，high: `bg-red-500/20` |
| 操作 | 取消标记 (Trash2 + hover:text-red-400) |

**标记弹窗** (`Dialog`)：

- 数据源 Select → 联动表名下拉（通过 API 获取表列表）
- 敏感等级 Select：低 / 中 / 高
- 底部：取消 + 确认标记

**子 Tab 2：字段脱敏规则**

| 元素 | 规范 |
|------|------|
| 标题 | "字段脱敏规则" (text-lg font-semibold) |
| 新建按钮 | "添加脱敏规则" + Plus 图标 |
| 脱敏类型 Badge | `bg-violet-500/20 text-violet-400` |
| 自定义正则 | `font-mono text-xs text-muted truncate max-w-[200px]` |

**添加/编辑弹窗**：

- 数据源 Select → 联动表名下拉
- 字段名 Input
- 脱敏类型 Select：手机号 / 身份证 / 邮箱 / 姓名 / 银行卡 / 自定义
- 自定义类型时额外显示：正则表达式 Input + 替换模板 Input
- 底部：取消 + 保存

### 3.9 系统设置

**页面布局**：左侧子导航 + 右侧内容

```
┌────────────┬─────────────────────────────┐
│  设置       │  (右侧内容区)                │
│             │                              │
│  🖥️ 数据源  │  AI 评审配置                  │
│  🛡️ 脱敏规则│                              │
│  🤖 AI 配置 │  [✓ AI 评审已启用 — gpt-4o]  │
│             │                              │
│             │  服务商: [OpenAI ▼]           │
│             │  模型:   [GPT-4o ▼]          │
│             │  地址:   [https://...]        │
│             │  Key:    [••••••]            │
│             │  超时:   [10s ▼]             │
│             │                              │
│             │  [保存 AI 配置]               │
│             │                              │
│             │  ──────────────────────       │
│             │                              │
│             │  钉钉通知配置                  │
│             │  ...                         │
└────────────┴─────────────────────────────┘
```

**左侧子导航**：

| 元素 | 规范 |
|------|------|
| 容器 | `w-44 border-r border-default bg-surface p-3` |
| 标题 | "设置" (text-xl font-semibold mb-4 px-3) |
| 导航项 | `flex items-center gap-2 rounded-md px-3 py-2 text-sm` |
| 激活态 | `bg-accent-muted text-accent-primary` |
| 非激活态 | `text-secondary hover:bg-elevated` |

**AI 配置 Tab**：

- 状态指示器：`rounded-lg border p-4`
  - 已启用：`CheckCircle2 emerald` + "AI 评审已启用 — 模型: xxx"
  - 未启用：`XCircle muted` + "AI 评审未启用 — 请配置 API Key"
- 表单：服务商 Select、模型 Select（联动）、API 地址 Input、API Key Input (password)、超时 Select
- 服务商选项：OpenAI / 智谱 GLM / Azure OpenAI / 自定义
- 底部：保存按钮

**钉钉通知配置**：
- 同上状态指示器风格
- Webhook URL Input + 签名密钥 Input
- 底部：保存 + 发送测试消息

---

## 4. 通用组件规范

### 4.1 Button

| 变体 | 用途 | 样式 |
|------|------|------|
| `default` | 主操作 | `bg-accent-primary text-white hover:bg-accent-hover` |
| `destructive` | 危险操作 | `bg-red-600 text-white hover:bg-red-700` |
| `outline` | 次要操作 | `border border-default bg-transparent text-primary hover:bg-elevated` |
| `secondary` | 替代操作 | `bg-elevated text-primary hover:bg-hover` |
| `ghost` | 最轻操作 | `text-secondary hover:bg-elevated hover:text-primary` |
| `link` | 链接按钮 | `text-accent-primary underline-offset-4 hover:underline` |

| 尺寸 | 高度 | 内边距 | 字号 |
|------|------|--------|------|
| `sm` | 28px (h-7) | `px-3` | 12px |
| `default` | 36px (h-9) | `px-4` | 14px |
| `lg` | 44px (h-11) | `px-6` | 14px |
| `icon` | 36px (h-9) | `p-0` | — |

**状态**：
- `disabled`: `opacity-50 cursor-not-allowed`
- `loading`: 内部显示 `Loader2` 旋转 + 按钮文字变为 "xx中..."
- 所有按钮 `transition-colors duration-150`

### 4.2 Input

| 属性 | 值 |
|------|------|
| 高度 | `h-9` (default) / `h-7` (compact) |
| 背景 | `bg-elevated` |
| 边框 | `border border-default` |
| 圆角 | `rounded-md` |
| 字号 | `text-sm` (default) / `text-xs` (compact) |
| 文字色 | `text-primary` |
| Placeholder | `text-muted` |
| Focus | `ring-1 ring-accent-primary border-accent-primary` |
| Error | `border-red-500` |
| Disabled | `opacity-50 cursor-not-allowed` |

**带图标的 Input**：
- 左侧图标：`absolute left-3 top-1/2 -translate-y-1/2` + `pl-8`
- 右侧图标：`absolute right-3 top-1/2 -translate-y-1/2` + `pr-8`

### 4.3 Table

| 元素 | 规范 |
|------|------|
| 容器 | `rounded-lg border border-default bg-surface overflow-hidden` |
| 表头行 | `bg-surface hover:bg-transparent` |
| 表头文字 | `text-xs text-secondary font-medium h-9` |
| 数据行 | `border-t border-default hover:bg-elevated transition-colors` |
| 数据文字 | `text-sm text-primary` |
| 单元格内边距 | `px-4 py-2.5` |
| 空状态 | `h-24 text-center text-muted` |
| 加载状态 | `h-24 text-center text-muted` + Loader2 |

### 4.4 Dialog (弹窗)

| 属性 | 值 |
|------|------|
| 宽度 | `sm:max-w-md` (标准) / `sm:max-w-lg` (宽) |
| 圆角 | `rounded-lg` |
| 边框 | `border border-default` |
| 背景 | `bg-surface` |
| 遮罩 | `bg-black/60` (深色) / `bg-black/40` (浅色) |
| 进入动画 | `fade-in + scale-in` (from 95% to 100%, 150ms) |
| 退出动画 | `fade-out` (100ms) |
| Header 底部 | 不需要，用间距分隔 |
| Footer | 右对齐按钮组，`gap-2` |

### 4.5 Sheet (侧边抽屉)

| 属性 | 值 |
|------|------|
| 方向 | 右侧滑入 (`side="right"`) |
| 宽度 | 工单详情: `w-[60%] max-w-[720px]`；其他: `w-[400px]` |
| 背景 | `bg-surface` |
| 边框 | `border-l border-default` |
| Header | `px-6 pt-6 pb-0` |
| Content | `ScrollArea` + `px-6 py-4` |
| Footer | `border-t border-default px-6 py-3` |
| 进入动画 | `slide-in-from-right` (200ms ease-out) |

### 4.6 Select (下拉选择)

| 属性 | 值 |
|------|------|
| 触发器 | 同 Input 样式 |
| 下拉面板 | `bg-surface border border-default rounded-lg shadow-lg` |
| 选项高度 | `h-8` |
| 选中项 | `bg-accent-muted text-accent-primary` |
| 悬停项 | `bg-elevated` |
| 搜索 | 支持键盘导航 |

### 4.7 Badge (标签)

| 变体 | 样式 |
|------|------|
| 默认 | `bg-elevated text-secondary rounded-sm px-2 py-0.5 text-[10px] font-medium` |
| 成功 | `bg-emerald-500/20 text-emerald-400` |
| 警告 | `bg-amber-500/20 text-amber-400` |
| 危险 | `bg-red-500/20 text-red-400` |
| 信息 | `bg-blue-500/20 text-blue-400` |
| 紫色 | `bg-violet-500/20 text-violet-400` |

### 4.8 Toast 通知

- 使用 `sonner` 组件
- 位置：右上角 (`position="top-right"`)
- 启用 `richColors`
- 类型：`toast.success` (绿色边框) / `toast.error` (红色边框) / `toast.info` (蓝色) / `toast.warning` (黄色)
- 自动关闭：4 秒
- 可手动关闭

### 4.9 AlertDialog (确认弹窗)

| 属性 | 值 |
|------|------|
| 宽度 | `max-w-md` |
| 样式 | 同 Dialog |
| 标题 | `text-lg font-semibold` |
| 描述 | `text-sm text-secondary` |
| 底部 | 左取消 (outline) + 右确认 (default/destructive) |
| 确认按钮 | 危险操作 `bg-red-600`；普通操作 `bg-accent-primary` |

### 4.10 Tabs (标签页)

**下划线式** (`variant="line"`，用于工单状态切换)：

| 元素 | 规范 |
|------|------|
| 容器 | `h-9 border-b border-default` |
| 标签 | `px-3 h-full text-xs` |
| 激活态 | `text-primary border-b-2 border-accent-primary` |
| 非激活态 | `text-secondary hover:text-primary` |

**填充式** (用于权限管理 Tab 切换)：

| 元素 | 规范 |
|------|------|
| 容器 | `rounded-lg bg-elevated p-1` |
| 标签 | `rounded-md px-3 py-1.5 text-sm` |
| 激活态 | `bg-surface text-primary shadow-sm` |
| 非激活态 | `text-secondary hover:text-primary` |

### 4.11 Search Bar (搜索栏)

- `Input` + 左侧 `Search` 图标
- 右侧可附加快捷键提示 `<kbd>`
- 高度 `h-7` (compact) / `h-9` (default)
- Enter 触发搜索
- 搜索图标 14px，颜色 `text-muted`

### 4.12 Empty State (空状态)

| 属性 | 值 |
|------|------|
| 容器 | `flex flex-col items-center justify-center` |
| 最小高度 | `h-32` |
| 图标 | 40px，`text-muted`（可选） |
| 文字 | `text-sm text-muted` |
| 操作按钮 | 可选，`mt-3` |

### 4.13 Loading State (加载状态)

**全页加载**：
- 居中 `Loader2` 旋转，`h-6 w-6 text-muted`

**行内加载**：
- `Loader2` 旋转，`h-4 w-4 text-muted` + "加载中..." 文字

**按钮加载**：
- `Loader2` 旋转替换图标位置，文字变为 "xx中..."
- `animate-spin`

**骨架屏** (可选，未来迭代)：
- 使用 `animate-pulse` + `bg-elevated rounded`

### 4.14 Tooltip

| 属性 | 值 |
|------|------|
| 背景 | `bg-popover` |
| 文字 | `text-xs text-popover-foreground` |
| 圆角 | `rounded-md` |
| 内边距 | `px-2.5 py-1` |
| 延迟 | 300ms |
| 最大宽度 | 320px（可覆盖） |
| 箭头 | 有 |

### 4.15 Separator (分隔线)

- 水平：`h-px w-full bg-default`
- 垂直：`w-px h-4 bg-default`

---

## 5. 动效规范

### 5.1 过渡时间

| 类型 | 时长 | 缓动 | 用途 |
|------|------|------|------|
| 即时 | 100ms | ease | 按钮悬停色变、边框色变 |
| 快速 | 150ms | ease-out | 弹窗出现/消失、下拉展开 |
| 标准 | 200ms | ease-out | Sheet 滑入、侧边栏折叠 |
| 慢速 | 300ms | ease-out | 审计行展开、页面内容渐显 |

### 5.2 关键动画

| 名称 | 定义 | 用途 |
|------|------|------|
| 侧边栏折叠 | `width 200ms ease-out` | 展开/收起侧边栏 |
| 侧边栏子菜单 | `chevron rotate 200ms + max-height 200ms` | Settings 子菜单展开 |
| 弹窗出现 | `opacity 0→1 + scale 95%→100% 150ms` | Dialog/AlertDialog |
| 抽屉滑入 | `translateX 100%→0 200ms ease-out` | Sheet |
| 审计行展开 | `opacity 0→1 + max-height 0→800px 300ms ease-out` | 审计日志展开 |
| 按钮点击 | `scale 100%→98% 50ms` (可选) | 按钮点击反馈 |
| Toast 滑入 | sonner 内置 | Toast 通知 |

### 5.3 减少动效

- 尊重 `prefers-reduced-motion`
- 在用户设置中提供 "减少动效" 开关
- 开启后所有动画时长降为 0ms

---

## 6. 响应式策略

### 6.1 断点

| 断点 | 宽度 | 场景 |
|------|------|------|
| `sm` | 640px | 平板竖屏 |
| `md` | 768px | 平板横屏 |
| `lg` | 1024px | 小笔记本 |
| `xl` | 1280px | 标准桌面 |
| `2xl` | 1536px | 大屏桌面 |

### 6.2 布局适配

| 屏幕 | 策略 |
|------|------|
| ≥ 1280px (xl) | 完整双栏布局，侧边栏展开 |
| 1024-1279px (lg) | 侧边栏自动收起为图标模式 |
| 768-1023px (md) | 侧边栏收起，表格列可水平滚动 |
| < 768px | 不做适配（管理后台不需要移动端支持） |

**核心原则**：SQLFlow 是面向桌面端的管理后台，最小支持 1280px 宽度。不做移动端适配。

---

## 7. 无障碍设计

### 7.1 基础要求

- 所有交互元素可通过键盘访问 (Tab 导航)
- Focus 样式：`ring-2 ring-accent-primary ring-offset-2 ring-offset-surface`
- 颜色对比度：文本至少 4.5:1 (WCAG AA)
- 图标按钮必须有 `aria-label` 或 `title`
- 表单 Input 关联 `Label` 或 `aria-label`
- 错误信息使用 `role="alert"` 或 `aria-live="polite"`

### 7.2 语义化 HTML

- 导航使用 `<nav>` + `aria-label="Main navigation"`
- 主内容使用 `<main>`
- 表格使用语义化 `<table>` / `<thead>` / `<tbody>` / `<th scope="col">`
- 弹窗使用 Radix UI 内置的 `aria-dialog` / `role="dialog"`
- Tab 切换使用 `role="tablist"` / `role="tab"` / `role="tabpanel"`

### 7.3 主题切换

- 深色/浅色切换同时更新 `data-theme` 属性
- 切换动画：`transition-colors duration-200`
- 偏好检测：首次访问检测 `prefers-color-scheme`
- 持久化：`localStorage('theme')`

---

## 附录 A：CSS 变量汇总

```css
@theme inline {
  --color-background: var(--bg-surface);
  --color-foreground: var(--text-primary);
  --color-card: var(--bg-surface);
  --color-card-foreground: var(--text-primary);
  --color-popover: var(--bg-surface);
  --color-popover-foreground: var(--text-primary);
  --color-primary: var(--accent-primary);
  --color-primary-foreground: #ffffff;
  --color-secondary: var(--bg-elevated);
  --color-secondary-foreground: var(--text-primary);
  --color-muted: var(--bg-elevated);
  --color-muted-foreground: var(--text-secondary);
  --color-accent: var(--bg-elevated);
  --color-accent-foreground: var(--text-primary);
  --color-destructive: #ef4444;
  --color-destructive-foreground: #ffffff;
  --color-border: var(--border-default);
  --color-input: var(--border-default);
  --color-ring: var(--accent-primary);
  --color-sidebar-background: var(--bg-sidebar);
  --color-sidebar-foreground: var(--text-secondary);
  --color-sidebar-primary: var(--accent-primary);
  --color-sidebar-primary-foreground: #ffffff;
  --color-sidebar-accent: var(--bg-elevated);
  --color-sidebar-accent-foreground: var(--text-primary);
  --color-sidebar-border: var(--border-subtle);
  --color-sidebar-ring: var(--accent-primary);
  --radius-sm: 4px;
  --radius-md: 6px;
  --radius-lg: 8px;
  --radius-xl: 12px;
  --font-sans: 'Inter', system-ui, -apple-system, sans-serif;
  --font-mono: 'JetBrains Mono', 'Fira Code', 'Consolas', monospace;
}
```

## 附录 B：shadcn/ui 组件映射

| 当前使用的组件 | shadcn/ui 对应组件 | 备注 |
|---------------|-------------------|------|
| Button | `Button` | 支持 default/destructive/outline/secondary/ghost/link |
| Input | `Input` | — |
| Select | `Select` + `SelectTrigger` + `SelectContent` + `SelectItem` | Radix UI |
| Table | `Table` + `TableHeader` + `TableBody` + `TableRow` + `TableHead` + `TableCell` | — |
| Dialog | `Dialog` + `DialogContent` + `DialogHeader` + `DialogTitle` + `DialogFooter` | — |
| AlertDialog | `AlertDialog` + series | — |
| Sheet | `Sheet` + `SheetContent` + `SheetHeader` + `SheetTitle` + `SheetFooter` | — |
| Badge | `Badge` | — |
| Tabs | `Tabs` + `TabsList` + `TabsTrigger` + `TabsContent` | — |
| Tooltip | `Tooltip` + `TooltipTrigger` + `TooltipContent` | — |
| DropdownMenu | `DropdownMenu` + series | — |
| Avatar | `Avatar` + `AvatarFallback` | — |
| ScrollArea | `ScrollArea` | — |
| Separator | `Separator` | — |
| Switch | `Switch` | — |
| Label | `Label` | — |
| Textarea | `Textarea` | — |
| Command | `Command` + `CommandInput` + `CommandList` + `CommandItem` | 用于命令面板 |
| Popover | `Popover` + `PopoverTrigger` + `PopoverContent` | — |
| Card | `Card` + `CardContent` + `CardHeader` | — |
| Toast | `sonner` (Toaster) | 非 shadcn/ui 内置，但推荐搭配 |
| — (新增) | `Pagination` | 可选用于分页组件化 |
| — (新增) | `Skeleton` | 可选用于加载骨架屏 |
| — (新增) | `Progress` | 可选用于进度指示 |
| — (新增) | `Collapsible` | 可选用于审计日志展开行 |

## 附录 C：组件尺寸速查

| 组件 | 高度 (default) | 高度 (compact) |
|------|---------------|----------------|
| Button | 36px (h-9) | 28px (h-7) |
| Input | 36px (h-9) | 28px (h-7) |
| Select Trigger | 36px (h-9) | 28px (h-7) |
| Badge | auto (py-0.5) | — |
| Table Header | 36px (h-9) | — |
| Table Row | ~40px | — |
| Top Bar | 52px | — |
| Sidebar Width | 220px (展开) / 56px (收起) | — |

---

> **变更记录**
> 
> | 日期 | 版本 | 变更内容 |
> |------|------|----------|
> | 2025-07-27 | v1.0 | 初始版本，完整 UI 设计规范 |

---

## 1.4.1 间距层级规范（2026-05-24 修订）

> 修复版本：针对实际页面间距拥挤问题，明确四层间距递进规则。

### 问题

现有页面实现未遵循间距递进层级，大量使用 8~12px（space-2 ~ space-3）作为区块级间距，导致视觉上的拥挤感。

### 四层间距递进

| 层级 | 名称 | 间距值 | Tailwind | 典型用途 | 例子 |
|------|------|--------|----------|---------|------|
| L0 | 页面级 | 24px | `p-6` / `gap-6` | 页面容器内边距、页面标题到内容区 | Layout main padding |
| L1 | 区块级 | 16~20px | `mb-5` / `gap-5` / `space-y-5` | 页面标题到卡片、卡片之间 | 标题 → card 容器 |
| L2 | 组件级 | 12~16px | `gap-4` / `px-4 py-3` | card 内部元素、筛选栏、tab 栏 | card 内 header、filters |
| L3 | 元素级 | 4~8px | `gap-2` / `gap-3` | 同行元素、按钮组、badge 间距 | 筛选项之间、操作按钮 |

### 关键规则

1. **禁止跨层跳跃**：L0 → L2（跳过 L1）不允许，必须层层递进
2. **card 内部 padding** 至少 `px-5 py-4`（20px 水平、16px 垂直），紧凑场景最低 `px-4 py-3`
3. **筛选栏垂直 padding** 至少 `py-3`（12px），不允许 `py-2.5` 或更小
4. **Tab 栏到内容区** 的间距至少 `pt-4`（16px），不允许 `pt-3`
5. **页面标题到第一个内容块** 至少 `mb-5`（20px），不允许 `mb-4`
6. **双栏布局（如 Settings）两栏间距** 至少 `ml-6`（24px），不允许 `ml-4`

### 页面容器高度

- **禁止使用 `h-[calc(100%-48px)]`** 等硬编码高度扣减
- 页面应使用 `h-full` + 内部 overflow 控制，让 Layout main 的 padding 自然提供间距
- 如需填满视口：使用 `flex-1 min-h-0` 而非计算高度

### Card 组件 padding 调整

| 使用场景 | 外部间距 | 内部 padding | 说明 |
|---------|---------|-------------|------|
| 页面主容器（包裹表格/列表） | `mb-5` | `p-0`（子组件自带 padding） | Ticket、Audit、Users |
| 内容卡片（独立内容块） | `gap-5` | `px-5 py-4` | Settings 内容区 |
| Dashboard 统计卡片 | `gap-5` | `px-5 py-5` | 概览页 |

