# Dark Mode Design Token 清单

> 生成日期：2026-06-15
> 关联需求：SF-FEAT0042 深色主题视觉完善

## Token 体系

所有颜色使用 CSS 自定义属性（CSS Variables），定义于 `src/index.css`。

### 背景色（Background）

| Token | 深色值 | 浅色值 | 用途 |
|-------|--------|--------|------|
| `--bg-base` | `#09090b` (zinc-950) | `#fafafa` | 页面背景 |
| `--bg-surface` | `#18181b` (zinc-900) | `#ffffff` | 面板、卡片、头部 |
| `--bg-elevated` | `#27272a` (zinc-800) | `#f4f4f5` | 输入框、弹出层 |
| `--bg-hover` | `#3f3f46` (zinc-700) | `#e4e4e7` | 悬浮状态 |
| `--bg-sidebar` | `#09090b` (zinc-950) | `#ffffff` | 侧边栏 |

### 边框色（Border）

| Token | 深色值 | 浅色值 | 用途 |
|-------|--------|--------|------|
| `--border-default` | `rgba(255,255,255,0.12)` | `#e4e4e7` | 默认边框 |
| `--border-subtle` | `rgba(255,255,255,0.08)` | `#f4f4f5` | 微弱分隔 |
| `--border-hover` | `rgba(255,255,255,0.20)` | `#d4d4d8` | 悬浮边框 |

### 文字色（Text）

| Token | 深色值 | 浅色值 | 用途 |
|-------|--------|--------|------|
| `--text-primary` | `#fafafa` (zinc-50) | `#09090b` | 主文字 |
| `--text-secondary` | `#a1a1aa` (zinc-400) | `#52525b` | 辅助文字 |
| `--text-tertiary` | `#71717a` (zinc-500) | `#71717a` | 三级文字 |
| `--text-muted` | `#52525b` (zinc-600) | `#a1a1aa` | 禁用/弱化文字 |

### 强调色（Accent）

| Token | 深色值 | 浅色值 | 用途 |
|-------|--------|--------|------|
| `--accent-primary` | `#ea580c` (orange-600) | `#ea580c` | 主强调色（按钮、链接） |
| `--accent-hover` | `#c2410c` (orange-700) | `#c2410c` | 强调悬浮 |
| `--accent-muted` | `rgba(234,88,12,0.10)` | `rgba(234,88,12,0.10)` | 弱化强调背景 |
| `--accent-secondary` | `#8b5cf6` | `#7c3aed` | 辅助强调 |

### 功能色（Functional）

| Token | 值 | 用途 |
|-------|-----|------|
| `--success` | `#10b981` | 成功/通过 |
| `--warning` | `#f59e0b` | 警告 |
| `--danger` | `#ef4444` | 危险/拒绝/删除 |
| `--info` | `#3b82f6` | 信息 |

### 表格

| Token | 深色值 | 浅色值 | 用途 |
|-------|--------|--------|------|
| `--table-row-alt` | `rgba(255,255,255,0.03)` | `rgba(0,0,0,0.03)` | 斑马纹 |
| `--table-row-hover` | `rgba(255,255,255,0.06)` | `rgba(0,0,0,0.06)` | 行悬浮 |

### 阴影

| Token | 深色值 | 浅色值 | 用途 |
|-------|--------|--------|------|
| `--shadow-sm` | `0 1px 2px rgba(0,0,0,0.3)` | `0 1px 2px rgba(0,0,0,0.04)` | 微弱阴影 |
| `--shadow-md` | `0 2px 8px rgba(0,0,0,0.4)` | `0 2px 6px rgba(0,0,0,0.06)` | 中等阴影 |
| `--shadow-lg` | `0 4px 16px rgba(0,0,0,0.5)` | `0 4px 12px rgba(0,0,0,0.08)` | 大阴影 |
| `--shadow-xl` | `0 8px 24px rgba(0,0,0,0.6)` | `0 8px 24px rgba(0,0,0,0.10)` | 超大阴影 |

## 使用规则

1. **所有颜色必须使用 CSS 变量**，禁止硬编码 hex/rgb
2. **按钮**：通过 Button 组件 variant 使用，danger 使用 `--danger` token
3. **CodeMirror 编辑器**：使用 `var(--bg-surface)`、`var(--text-primary)` 等变量
4. **表格边框/分隔线**：使用 `var(--border-default)` 或 `var(--border-subtle)`
5. **主题切换**：通过 `data-theme="light"` 属性切换，所有 token 自动适配

## 本次修复记录

| 文件 | 问题 | 修复 |
|------|------|------|
| ResultTable.tsx | `bg-black/10` 列分隔线深色不可见 | → `bg-[var(--border-default)]` |
| button.tsx | `bg-red-600` 硬编码 destructive | → `bg-[var(--danger)]` |
| AIReviewCard.tsx | `bg-emerald-600`/`bg-red-600` 硬编码 | → `bg-[var(--success)]`/`bg-[var(--danger)]` |
| TicketDetailDrawer.tsx | `bg-emerald-600` 硬编码 | → `bg-[var(--success)]` |
| HistoryPanel.tsx | `bg-red-600` 硬编码 | → `bg-[var(--danger)]` |
| Users/index.tsx | `bg-red-600` 硬编码 | → `bg-[var(--danger)]` |
| MaskRulesTab.tsx | `bg-red-600` 硬编码 | → `bg-[var(--danger)]` |
| Settings/index.tsx | `bg-red-600` 硬编码 | → `bg-[var(--danger)]` |
| SLATab.tsx | `bg-red-600` 硬编码 | → `bg-[var(--danger)]` |
| Permissions/index.tsx | `bg-red-600` 硬编码 | → `bg-[var(--danger)]` |
| Reports/index.tsx | `text-gray-400`/`bg-gray-500` 硬编码 | → `text-[var(--text-tertiary)]`/`bg-[var(--text-tertiary)]/10` |
| PermRequestPage/index.tsx | `text-gray-400`/`bg-gray-500` 硬编码 | → `text-[var(--text-tertiary)]`/`bg-[var(--text-tertiary)]/10` |
