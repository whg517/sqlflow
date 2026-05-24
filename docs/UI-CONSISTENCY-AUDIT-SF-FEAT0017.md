# SQLFlow UI 一致性评审报告

> **评审编号**：SF-FEAT0017  
> **评审人**：苏晴 ✨  
> **日期**：2026-05-24  
> **参照规范**：`docs/UI-DESIGN-SPEC.md` v1.0  
> **代码基准**：commit `ecfb679`（含 SF-FEAT0016 间距修复）

---

## 评审概览

| 维度 | P0 | P1 | P2 | P3 | 总计 |
|------|----|----|----|----|------|
| 颜色一致性 | 0 | 2 | 2 | 1 | 5 |
| 间距一致性 | 0 | 2 | 2 | 0 | 4 |
| 排版一致性 | 0 | 1 | 1 | 0 | 2 |
| 组件一致性 | 1 | 3 | 3 | 2 | 9 |
| 布局一致性 | 0 | 1 | 1 | 0 | 2 |
| 交互一致性 | 0 | 0 | 2 | 1 | 3 |
| 响应式行为 | 0 | 0 | 0 | 1 | 1 |
| **合计** | **1** | **9** | **11** | **5** | **26** |

---

## P0 — 阻塞上线

### P0-01 | Badge 组件圆角全局错误
- **页面/组件**：全站 — `components/ui/badge.tsx`
- **问题描述**：Badge 组件默认使用 `rounded-sm`（4px），所有页面的 Badge 均继承此值
- **规范要求**（§1.5 + §4.7）：Badge 应使用 `rounded-full`（9999px）圆角
- **实际状态**：`rounded-sm` — 所有操作类型 Badge、角色 Badge、状态 Badge 均为直角矩形
- **影响范围**：审计日志、工单、权限、用户管理、设置等所有使用 Badge 的页面
- **修复建议**：修改 `badge.tsx` 基础样式，将 `rounded-sm` 改为 `rounded-full`

---

## P1 — 本周修复

### P1-01 | 角色 Badge 配色不一致
- **页面/组件**：`pages/Users/index.tsx`（第 265 行），`api/user.ts`（第 75-79 行）
- **问题描述**：Users 页面的角色 Badge 使用 `ROLE_BADGE_CLASS` 常量，配色与规范不符
  - admin：实际 `bg-red-500/20 text-red-400`，规范要求 `bg-accent-primary/20 text-accent-primary`
  - dba：实际 `bg-blue-500/20 text-blue-400`，规范要求 `bg-violet-500/20 text-violet-400`
  - developer：实际 `bg-green-500/20 text-green-400`，规范要求 `bg-blue-500/20 text-blue-400`
- **规范要求**（§3.7）：角色 Badge 配色定义
- **实际状态**：三色均不一致。Permissions 页面 admin 角色已正确使用 token，但 dba/developer 未使用规范配色
- **修复建议**：更新 `api/user.ts` 的 `ROLE_BADGE_CLASS` 常量，对齐 §3.7 规范

### P1-02 | 筛选栏控件尺寸与规范不符
- **页面/组件**：`pages/Audit/index.tsx`（第 466-494 行）、`pages/Ticket/index.tsx`（第 199-262 行）
- **问题描述**：SF-FEAT0016 将工具栏升级为 h-8，但规范 §3.5/§3.6 筛选栏要求 `h-7`
  - Audit 页：SelectTrigger 实际 `h-8 w-[120px]/w-[120px]/w-[132px]`，规范 `h-7 w-28/w-28/w-32`
  - Ticket 页：SelectTrigger 实际 `h-8 w-36/w-32`，规范 `h-7 w-32/w-28`
  - Audit 日期：实际 `w-[130px]`，规范 `w-[124px]`
  - Ticket 搜索：实际 `w-52`，规范 `w-48`
- **规范要求**（§3.5/§3.6）：筛选栏属于紧凑区域，使用 `h-7` 小尺寸
- **实际状态**：SF-FEAT0016 统一升级为 h-8，但筛选栏应保持 h-7（紧凑控件），与工具栏（h-8，主操作区）区分层级
- **修复建议**：筛选栏回退为 `h-7` + `text-xs`，主工具栏保持 `h-8` + `text-sm`

### P1-03 | Table 组件基础样式与规范偏差
- **页面/组件**：`components/ui/table.tsx`
- **问题描述**：
  - TableHead 高度：实际 `h-10`，规范 `h-9`（36px）
  - TableHead 内边距：实际 `px-2`，规范 `px-4`
  - TableCell 内边距：实际 `p-2`（8px），规范 `px-4 py-2.5`（16px/10px）
  - 容器：组件缺少 `rounded-lg border border-default bg-surface` 包裹
  - hover 态：实际 `hover:bg-muted/50`，规范 `hover:bg-elevated`
- **规范要求**（§4.3）：表格容器、表头、行间距、hover 态统一定义
- **实际状态**：shadcn/ui 默认样式，未按规范定制。各页面自行在 className 中覆盖
- **修复建议**：修改 Table 基础组件，默认样式对齐规范；清理各页面冗余覆盖

### P1-04 | Select 组件 compact 尺寸缺失
- **页面/组件**：`components/ui/select.tsx`
- **问题描述**：SelectTrigger 的 `size` prop 有 `sm`（h-8）和 `default`（h-9），但缺少 `compact`（h-7）
  - 规范 §4.2/§4.6 要求 Input/Select 有 default（h-9）和 compact（h-7）两种尺寸
  - 筛选栏场景需要 h-7 紧凑模式，但组件无此选项，导致各页面手动写 `h-7` className
- **规范要求**（§4.2）：Input/Select 应支持 default（h-9）/ compact（h-7）
- **实际状态**：有 `sm`（h-8）但无 `compact`（h-7），且 h-8 不在规范定义中
- **修复建议**：新增 `compact` size（h-7），或重命名 `sm` → `compact` 并调整高度

### P1-05 | Card 组件圆角与规范不符
- **页面/组件**：`components/ui/card.tsx`
- **问题描述**：Card 默认 `rounded-xl`（12px），规范 §3.2 要求 `rounded-lg`（8px）
- **规范要求**（§1.5 + §3.2）：Card 使用 `rounded-lg`
- **实际状态**：`rounded-xl`。Dashboard 卡片视觉上偏圆，与其他面板（Table/Dialog 的 rounded-lg）不协调
- **修复建议**：Card 基础改为 `rounded-lg`，登录卡片单独用 `rounded-xl`（§3.1 明确要求）

### P1-06 | Settings 子导航宽度不符
- **页面/组件**：`pages/Settings/index.tsx`（第 540 行）
- **问题描述**：Settings 子导航容器 `w-48`（192px），规范 §3.9 要求 `w-44`（176px）
- **规范要求**（§3.9）：`w-44 border-r border-default bg-surface p-3`
- **实际状态**：`w-48`（192px），比规范宽 16px
- **修复建议**：改为 `w-44`

### P1-07 | gray 色残留
- **页面/组件**：`pages/Settings/index.tsx`（第 77/81 行）、`pages/Users/index.tsx`（第 265/302 行）
- **问题描述**：disabled 状态和 fallback 角色使用 `bg-gray-500/20 text-gray-400`
- **规范要求**：应使用 `bg-[var(--bg-elevated)]` / `text-[var(--text-muted)]` 或定义专用 disabled token
- **实际状态**：直接引用 Tailwind gray 色阶，未走 CSS token 体系
- **修复建议**：替换为 `bg-[var(--bg-elevated)]/50 text-[var(--text-muted)]`

### P1-08 | Sheet 默认宽度过窄
- **页面/组件**：`components/ui/sheet.tsx`（第 65 行）
- **问题描述**：Sheet 右侧滑出默认 `w-3/4 sm:max-w-sm`（384px），规范 §3.5 工单提交抽屉要求 480px
- **规范要求**（§4.5）：工单详情 `w-[60%] max-w-[720px]`，工单提交 `w-[480px]`
- **实际状态**：默认 max-w-sm（384px），TicketDetailDrawer 已用 className 覆盖为正确值，但基础组件默认值不合规
- **修复建议**：Sheet 默认 max-w 改为 `sm:max-w-md`（448px），或使用时始终显式指定宽度

### P1-09 | Table hover 态使用 muted 而非 elevated
- **页面/组件**：`components/ui/table.tsx`（第 60 行）
- **问题描述**：TableRow hover 使用 `hover:bg-muted/50`，应为 `hover:bg-[var(--bg-elevated)]`
- **规范要求**（§4.3）：数据行 `hover:bg-elevated`
- **实际状态**：`hover:bg-muted/50`，shadcn/ui 默认样式
- **修复建议**：改为 `hover:bg-[var(--bg-elevated)]`

---

## P2 — 排期修复

### P2-01 | 子控件仍为 h-7 + text-xs
- **页面/组件**：全站约 41 处 `h-7`，246 处 `text-xs` 残留
- **问题描述**：SF-FEAT0016 升级了主工具栏，但子组件/内联控件（StatusBar 按钮、AIReviewCard 按钮、分页按钮、ResultTable 控件、MongoEditor 小按钮）仍为 h-7
- **规范要求**：部分 h-7 是合理的（compact 模式），但需逐个确认是否符合场景
- **修复建议**：P3 收尾阶段（SF-FEAT0015）统一扫描，区分合理 compact（筛选栏）和遗漏升级（子工具栏按钮）

### P2-02 | TableHead 字号不一致
- **页面/组件**：各页面 TableHead
- **问题描述**：规范要求表头 `text-xs text-secondary font-medium`，但 Table 组件基础样式为 `text-foreground font-medium`，各页面需手动覆盖
- **规范要求**（§4.3）：表头文字 `text-xs text-secondary font-medium h-9`
- **修复建议**：修改 Table 基础组件的 TableHead 默认样式

### P2-03 | 分页控件未使用 Pagination 组件
- **页面/组件**：`pages/Audit/index.tsx`（第 681-705 行）、`pages/Ticket/index.tsx`
- **问题描述**：分页使用内联 `<` `>` Button 而非项目已有的 `Pagination` 组件
- **规范要求**：P0 阶段已补充 Pagination 组件，应复用
- **修复建议**：统一使用 Pagination 组件

### P2-04 | 工具栏间距 px-4 vs 规范 px-3
- **页面/组件**：`pages/Query/index.tsx`（第 267 行）、`pages/Query/components/MongoEditor.tsx`（第 460 行）
- **问题描述**：SF-FEAT0016 将工具栏 px 从 3 升为 4，但规范 §3.4 要求 `px-3`
- **规范要求**（§3.4）：工具栏容器 `px-3 py-2`
- **实际状态**：`px-4 py-2.5`，比规范略宽
- **修复建议**：确认设计意图，如需更宽松则更新规范；否则回退为 px-3

### P2-05 | 审计展开行 grid-cols 偏差
- **页面/组件**：`pages/Audit/index.tsx`（第 122 行）
- **问题描述**：展开行内容使用 `grid-cols-2` + 响应式 `lg:grid-cols-4`，规范 §3.6 要求始终 `grid-cols-4 gap-3`
- **规范要求**（§3.6）：`grid grid-cols-4 gap-3`
- **实际状态**：响应式处理更合理，但与规范不一致
- **修复建议**：更新规范允许响应式列数

### P2-06 | Dialog 默认宽度偏差
- **页面/组件**：`components/ui/dialog.tsx`
- **问题描述**：Dialog 默认 `sm:max-w-lg`（512px），规范 §4.4 要求标准 `sm:max-w-md`（448px）/ 宽 `sm:max-w-lg`
- **规范要求**（§4.4）：标准弹窗 `sm:max-w-md`，宽弹窗 `sm:max-w-lg`
- **实际状态**：默认 `sm:max-w-lg`，所有标准弹窗偏宽
- **修复建议**：默认改为 `sm:max-w-md`，宽弹窗显式指定

### P2-07 | Button default variant 使用 bg-primary 而非 token
- **页面/组件**：`components/ui/button.tsx`
- **问题描述**：default variant 使用 `bg-primary`，对应 shadcn 映射的 `--color-primary`。虽然通过附录 A 映射到 `var(--accent-primary)`，但其他 variant（outline/ghost）直接使用 `var(--border-default)` 等原始 token
- **规范要求**：保持 token 引用方式一致
- **修复建议**：统一为显式 `var(--accent-primary)` / `var(--accent-hover)` 引用

### P2-08 | Dashboard 卡片 gap-5 vs 规范 gap-4
- **页面/组件**：`pages/Dashboard/index.tsx`
- **问题描述**：SF-FEAT0016 升级为 `gap-5`（20px），规范 §3.2 要求 `grid-cols-2 gap-4`（16px）
- **规范要求**（§3.2）：`grid grid-cols-2 gap-4`
- **实际状态**：`gap-5` + `CardContent gap-5 py-5`，视觉更宽松
- **修复建议**：如果认可当前间距，更新规范为 gap-5

### P2-09 | Card 默认 gap-6 和 py-6 与规范不符
- **页面/组件**：`components/ui/card.tsx`
- **问题描述**：Card 组件默认 `gap-6 py-6`，规范未指定 Card 默认 gap/padding，但实际效果导致 Dashboard 卡片有过多内边距
- **规范要求**（§3.2）：Dashboard 卡片无显式 padding 定义，通过 CardContent 控制
- **修复建议**：Card 默认 `gap-4 py-4`，或在页面级别覆盖

### P2-10 | Audit 搜索框图标位置偏移
- **页面/组件**：`pages/Audit/index.tsx`（搜索输入框）
- **问题描述**：Search 图标 `left-2.5`，规范要求 `left-3`（§4.2 带图标 Input）
- **规范要求**（§4.2）：左侧图标 `absolute left-3` + `pl-8`
- **实际状态**：`left-2.5` + `pl-7`（SF-FEAT0016 调整前遗留）
- **修复建议**：对齐为 `left-3` + `pl-8`

### P2-11 | Skeleton 加载缺少统一规范
- **页面/组件**：Dashboard、Users 等页面的 Skeleton 加载
- **问题描述**：各页面自行实现 Skeleton 布局，使用 `animate-pulse` + 手写 div，无统一加载骨架屏组件
- **规范要求**（§4.13）：骨架屏使用 `animate-pulse` + `bg-elevated rounded`
- **实际状态**：基本符合但无统一标准
- **修复建议**：后续迭代统一骨架屏模式

---

## P3 — 记录即可

### P3-01 | 功能色未收归 CSS token
- **页面/组件**：全站
- **问题描述**：功能色（emerald/amber/red/violet/sky/blue）使用 Tailwind 原子类，未定义为 CSS token
- **规范要求**：规范本身也给出了 Tailwind class 参考（如 `bg-emerald-500/15`），当前不算违反
- **建议**：长期优化——如需主题化功能色（如品牌定制），应统一收归 token

### P3-02 | Audit 工具栏 SelectTrigger 缺少 bg-[var(--bg-elevated)]
- **页面/组件**：`pages/Audit/index.tsx`
- **问题描述**：SelectTrigger 使用 `bg-[var(--bg-elevated)]` 但 Select 基础组件使用 `bg-transparent`，筛选栏的 Select 需手动覆盖背景
- **建议**：在 Select 基础组件中增加 `bg-[var(--bg-elevated)]` 默认值

### P3-03 | Login 页面副标题间距
- **页面/组件**：`pages/Login/index.tsx`
- **问题描述**：品牌区 `mb-2` + 副标题 `mb-6`，规范未明确定义间距但视觉上品牌区与副标题偏紧
- **建议**：品牌区 `mb-3`

### P3-04 | Tooltip 延迟未配置
- **页面/组件**：`components/ui/tooltip.tsx`（未审查，推测）
- **问题描述**：规范 §4.14 要求 Tooltip 延迟 300ms，需确认实际配置
- **建议**：检查并确保 `delayDuration={300}`

### P3-05 | 响应式断点行为未验证
- **页面/组件**：全站
- **问题描述**：规范 §6.2 定义了 lg（1024-1279px）侧边栏自动收起行为，已在 Layout.tsx 中实现，但各页面内部的响应式布局（表格水平滚动等）未在本次代码审查中逐个验证
- **建议**：P3 收尾阶段做实际浏览器测试

---

## 按修复优先级排序的建议

### 第一批：组件基础修复（预计 8h，影响全局）
| 编号 | 修复项 | 预计工时 |
|------|--------|----------|
| P0-01 | Badge rounded-sm → rounded-full | 0.5h |
| P1-03 | Table 组件基础样式对齐规范 | 2h |
| P1-04 | Select 新增 compact size | 1h |
| P1-05 | Card rounded-xl → rounded-lg | 0.5h |
| P1-09 | Table hover bg-muted → bg-elevated | 0.5h |
| P2-02 | TableHead 字号对齐 | 0.5h |
| P2-06 | Dialog 默认宽度 sm:max-w-md | 0.5h |
| P2-07 | Button variant token 统一 | 1h |
| P2-09 | Card 默认 gap/py 调整 | 1h |

### 第二批：页面级修复（预计 6h）
| 编号 | 修复项 | 预计工时 |
|------|--------|----------|
| P1-01 | 角色 Badge 配色对齐 | 1h |
| P1-02 | 筛选栏回退 h-7 + 调整宽度 | 2h |
| P1-06 | Settings 子导航 w-48 → w-44 | 0.5h |
| P1-07 | gray 色清理 | 0.5h |
| P1-08 | Sheet 默认宽度调整 | 0.5h |
| P2-04 | 工具栏间距确认 | 0.5h |
| P2-08 | Dashboard gap 规范确认 | 0.5h |
| P2-10 | Audit 搜索图标位置 | 0.5h |

### 第三批：收尾优化（预计 4h，P3 阶段）
| 编号 | 修复项 | 预计工时 |
|------|--------|----------|
| P2-01 | 子控件 h-7 逐个审核 | 2h |
| P2-03 | 分页统一使用 Pagination | 1h |
| P2-05 | 审计展开行 grid 规范更新 | 0.5h |
| P2-11 | 骨架屏统一模式 | 0.5h |
| P3-* | P3 类改进项 | 按需 |

---

## 总结

SQLFlow 前端在 SF-FEAT0016 之后，间距和排版有显著改善。核心问题集中在 **组件基础样式未按设计规范定制**（Badge 圆角、Table 样式、Card 圆角、Select 尺寸），导致各页面需要手动覆盖 className。

建议 Marcus 根据本报告拆分为 2-3 个独立修复子需求：
1. **SF-FEAT0018**：组件基础对齐（P0 + P1 组件级，第一批，8h）
2. **SF-FEAT0019**：页面级修复（P1 页面级 + P2，第二批，6h）
3. 收尾优化归入 SF-FEAT0015（P3 阶段）

---

> **变更记录**
> 
> | 日期 | 版本 | 变更内容 |
> |------|------|----------|
> | 2026-05-24 | v1.0 | 初始版本，全站 UI 一致性评审 |
