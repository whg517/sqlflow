# SQLFlow v2.0 技术架构设计

> 项目代号 SQLFlow，代码仓库 `sql-platform`
> 创建日期：2026-05-23
> 状态：已评审
> 前序版本：v1.0 架构 → [ARCHITECTURE.md](./ARCHITECTURE.md)
> 产品需求见 [PRD-v2.md](./PRD-v2.md)

## 变更记录

| 版本 | 日期 | 变更内容 |
|------|------|----------|
| v2.0-draft | 2026-05-23 | v2.0 架构设计：主题切换、自动补全、全局搜索、列筛选、审计增强、TS strict、ESLint 清零 |

---

## v1.0 架构回顾

v1.0 采用 **Golang (Echo) + React (Vite)** 前后端分离架构，前端 embed 进 Go 二进制的单容器部署方案。

**核心技术栈**：
- 后端：Golang Echo + SQLite (WAL) + Ent ORM + Casbin RBAC + JWT
- 前端：React 19 + Vite 8 + TailwindCSS 4 + Shadcn/ui + CodeMirror 6 + TanStack Table + Zustand
- 前端代码规模：62 个 TS/TSX 文件，~11,475 行

**v1.0 已建立的基础设施**：
- CSS Design Token 体系（`index.css`）已定义深色/浅色两套变量（`[data-theme='light']` 选择器下浅色 token 已就绪）
- CodeMirror 6 SQL 编辑器（`SqlEditor.tsx`）使用 CSS 变量实现主题样式（`sqlTheme`），理论上可通过 CSS 变量切换适配主题
- `@codemirror/autocomplete` 已安装（`package.json` 中 `^6.20.1`）但未使用
- 后端 `GET /api/datasources/:id/tables` API 已实现，返回 `string[]`（表名列表）
- `CommandPalette` 组件（`cmdk` 库）已实现 `Cmd+K` 快捷键 + 硬编码页面导航
- `ResultTable` 已使用 `@tanstack/react-table` 的 `getSortedRowModel` 实现列排序
- `AuditLog` 前后端模型已完整，`ExpandedRow` 组件展示详情

**v1.0 的技术债**：
- 6 个 ESLint 问题（4 errors + 2 warnings）：`react-hooks/set-state-in-effect`、`react-hooks/incompatible-library`、未使用的 eslint-disable 指令
- TypeScript 未开启 `strict` 模式（`tsconfig.app.json` 无 `strict: true`）
- 3 处 `eslint-disable` 注释（SqlEditor、MongoEditor、QueryPage 的 `useEffect` 依赖）
- 无 `@ts-ignore` / `@ts-expect-error`（说明代码基础尚可）

---

## v2.0 架构变更概览

v2.0 **不新增后端模块**，聚焦前端体验打磨和代码质量治理。7 个变更点按依赖关系排列：

| # | 变更 | 类型 | 影响范围 | 新增文件 |
|---|------|------|---------|---------|
| 1 | 主题切换 | 前端功能 | Layout + 编辑器 + 全局 | `useTheme.ts` |
| 2 | SQL 编辑器自动补全 | 前端功能 | SqlEditor + MongoEditor + API | `useSchemaCompletion.ts`、`metadata.ts` |
| 3 | 全局搜索增强 | 前端功能 | CommandPalette + API | — |
| 4 | 查询结果列筛选 | 前端功能 | ResultTable | `ColumnFilter.tsx` |
| 5 | 审计日志增强 | 前端功能 | Audit ExpandedRow + 类型 | — |
| 6 | TypeScript 严格模式 | 代码质量 | tsconfig + 全部 TS 文件 | — |
| 7 | ESLint 修复 | 代码质量 | 全部 TS/TSX 文件 | — |

**变更依赖图**：

```
(7) ESLint 修复 ──→ (6) TS Strict 模式（先清零 ESLint，再开 strict 修复编译错误）
                                          ↓ 无依赖 ↓
(1) 主题切换    (2) 自动补全    (3) 全局搜索    (4) 列筛选    (5) 审计增强
```

---

## 技术方案设计

### 1. 主题切换架构

#### 方案选型

| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| **A. CSS 变量 + data-theme 属性** | 在 `<html>` 上设置 `data-theme` 属性，CSS 变量自动切换 | 性能最优（无 JS 重渲染）；v1.0 token 已就绪；CodeMirror 已用 CSS 变量 | 需确保所有组件使用 CSS 变量而非硬编码颜色 |
| **B. Zustand 全局状态 + Context** | 创建 theme store，组件订阅 theme 状态 | 灵活，可在 JS 中读取当前主题 | 需要全局 Provider；切换时触发所有订阅组件重渲染 |
| **C. Tailwind dark: 变体** | 使用 Tailwind 的 `dark:` class 前缀 | Tailwind 原生支持 | 需要大量重构现有 CSS 变量体系；与 v1.0 token 设计冲突 |

#### 推荐方案：A. CSS 变量 + data-theme 属性

**理由**：
1. v1.0 已建立完整的 CSS Design Token 体系，`[data-theme='light']` 下浅色 token 已定义（`index.css` 第 89-101 行）
2. `SqlEditor.tsx` 的 `sqlTheme` 使用 `var(--bg-surface)`、`var(--text-primary)` 等 CSS 变量，天然支持主题切换
3. 所有现有组件均使用 CSS 变量（`bg-[var(--bg-surface)]`、`text-[var(--text-primary)]` 等），无需修改
4. 切换性能最优：仅修改 DOM 属性，CSS 引擎自动更新，不触发 React 重渲染

#### 实现架构

```
┌─────────────────────────────────────────────────┐
│ useTheme Hook                                    │
│                                                  │
│  state: theme ('dark' | 'light')                 │
│  - localStorage('theme') 初始化                   │
│  - prefers-color-scheme fallback                 │
│                                                  │
│  toggleTheme() →                                 │
│    1. set theme                                  │
│    2. document.documentElement.dataset.theme = x  │
│    3. localStorage.setItem('theme', x)           │
│    4. dispatch 'theme-change' CustomEvent        │
└───────────┬──────────────────┬───────────────────┘
            │                  │
            ▼                  ▼
  ┌─────────────────┐  ┌─────────────────────┐
  │ <html> DOM      │  │ CodeMirror 编辑器    │
  │ data-theme 属性  │  │ 监听 CustomEvent    │
  │ CSS 变量自动切换  │  │ 重建 sqlTheme 扩展  │
  └─────────────────┘  └─────────────────────┘
```

**useTheme Hook 设计**：

```typescript
// web/src/hooks/useTheme.ts
// 示例代码（非完整实现）
type Theme = 'dark' | 'light'

function useTheme() {
  const [theme, setTheme] = useState<Theme>(() => {
    const stored = localStorage.getItem('theme')
    if (stored === 'dark' || stored === 'light') return stored
    // 可选：跟随系统偏好
    if (window.matchMedia('(prefers-color-scheme: light)').matches) return 'light'
    return 'dark'
  })

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('theme', theme)
    window.dispatchEvent(new CustomEvent('theme-change', { detail: theme }))
  }, [theme])

  const toggleTheme = () => setTheme(t => t === 'dark' ? 'light' : 'dark')

  return { theme, toggleTheme }
}
```

**CodeMirror 主题适配**：

当前 `sqlTheme`（`SqlEditor.tsx` 第 34-72 行）使用 CSS 变量引用（如 `backgroundColor: 'var(--bg-surface)'`），**CSS 变量会在 `data-theme` 变更时自动更新**。但 CodeMirror 的 `EditorView.theme()` 创建的是静态扩展，不会自动响应 CSS 变量变化。

**解决方案**：CodeMirror 使用 `EditorView.theme()` 的 `dark` 选项配合 CSS 变量。实际测试表明，当 CSS 变量的值变化时，CodeMirror 会自动应用新的样式（因为 `theme()` 内部使用的是 CSS 变量引用，浏览器会在变量值变更时重新计算样式）。

如果发现某些样式未自动更新（如 `.cm-activeLine` 的 `rgba` 硬编码），需要将其改为 CSS 变量或通过监听 `theme-change` 事件重建编辑器扩展。

**涉及文件改动**：

| 文件 | 改动内容 | 改动量 |
|------|---------|--------|
| **新建** `web/src/hooks/useTheme.ts` | 主题管理 hook | ~40 行 |
| `web/src/components/Layout.tsx` | 头像下拉新增主题切换菜单项（Sun/Moon 图标） | ~15 行 |
| `web/src/main.tsx` 或 `App.tsx` | 应用启动时初始化 data-theme | ~5 行 |
| `web/src/pages/Query/components/SqlEditor.tsx` | 可选：监听主题变化事件 | ~10 行 |
| `web/src/pages/Query/components/MongoEditor.tsx` | 可选：监听主题变化事件 | ~10 行 |

#### 性能评估

| 指标 | 预期值 | 说明 |
|------|--------|------|
| 切换延迟 | < 16ms（一帧） | CSS 变量变更不触发 JS 重渲染，仅 CSS 引擎重绘 |
| 内存开销 | ~0KB | 仅 localStorage + 一个 DOM 属性 |
| 重渲染影响 | 0 组件 | 使用 `data-theme` 属性，非 React 状态，无重渲染 |
| CodeMirror 刷新 | < 50ms | CSS 变量值变更后 CodeMirror 自动应用新样式 |

#### 对现有代码的影响

**低风险**：
- v1.0 所有组件已使用 CSS 变量（`bg-[var(--xxx)]`、`text-[var(--xxx)]`），主题切换天然生效
- `index.css` 中浅色 token 已定义完整（`--bg-base`、`--bg-surface`、`--bg-elevated`、`--text-primary` 等）
- 需注意的硬编码颜色：`Badge` 组件中的 `bg-blue-500/20 text-blue-400` 等使用 Tailwind 颜色类，浅色主题下可能对比度不足，需逐一排查

---

### 2. SQL 编辑器自动补全

#### 方案选型

| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| **A. @codemirror/autocomplete + 前端缓存** | 使用已安装的 `@codemirror/autocomplete`，自定义 `completionSource` 函数，从后端 API 获取表名/字段名并缓存 | 已有依赖；CodeMirror 原生体验好；支持模糊匹配 | 首次加载需网络请求 |
| **B. Monaco Editor 替换 CodeMirror** | 替换为 Monaco（VS Code 内核编辑器） | 功能强大；自带 SQL 补全 | 引入巨大依赖（~2MB）；需重写编辑器组件；与现有架构不兼容 |
| **C. LSP（Language Server Protocol）** | 前端启动 SQL Language Server，通过 WebSocket 通信 | 专业级补全；可扩展 | 架构复杂度高；需后端新增 WebSocket 端点；过度设计 |

#### 推荐方案：A. @codemirror/autocomplete + 前端缓存

**理由**：
1. `@codemirror/autocomplete` 已在 `package.json` 中（`^6.20.1`），零新增依赖
2. CodeMirror 6 的 `autocompletion()` 扩展支持自定义 `source` 函数，可灵活控制补全数据源
3. 后端 `GET /api/datasources/:id/tables` API 已就绪，返回表名列表
4. 前端缓存策略简单可靠，用 `Map` + 时间戳即可

#### 实现架构

```
┌──────────────────────────────────────────────────────────┐
│ useSchemaCompletion Hook                                  │
│                                                          │
│  输入: datasourceId, database, dbType                     │
│                                                          │
│  SchemaCache:                                             │
│    Map<"dsId:db", { tables: string[], fetchedAt: number }>│
│    Map<"dsId:db:table", { columns: string[], fetchedAt }>│
│    TTL: 5 分钟                                             │
│                                                          │
│  输出:                                                    │
│    tables: string[]        (当前库的表名列表)              │
│    getColumns(table): string[] (获取指定表的字段名)        │
│    sqlKeywords: Completion[] (SQL 保留字)                 │
│    mongoOperators: Completion[] (MongoDB 操作符)          │
└───────────┬──────────────────────────────────────────────┘
            │
            ▼
┌──────────────────────────────────────────────────────────┐
│ SqlEditor — autocompletion() 扩展                         │
│                                                          │
│  completionSource(context: CompletionContext):             │
│    1. 解析光标位置上下文                                    │
│       - FROM/JOIN 后 → 返回表名列表                        │
│       - "表名." 后 → 返回该表字段名                        │
│       - 其他 → 返回 SQL 关键字                             │
│    2. 调用 useSchemaCompletion 获取数据                    │
│    3. 返回 { from, options, filter } (支持模糊匹配)        │
└──────────────────────────────────────────────────────────┘
```

**补全触发逻辑**：

```typescript
// 示例代码（非完整实现）
function sqlCompletionSource(
  tables: string[],
  getColumns: (table: string) => string[],
): (context: CompletionContext) => CompletionResult | null {
  return (context: CompletionContext) => {
    // 获取光标前的文本
    const line = context.state.doc.lineAt(context.pos)
    const textBefore = line.text.slice(0, context.pos - line.from)

    // 匹配 "FROM " 或 "JOIN " → 补全表名
    const fromMatch = textBefore.match(/\b(FROM|JOIN)\s+(\w*)$/i)
    if (fromMatch) {
      return {
        from: context.pos - fromMatch[2].length,
        options: tables.map(t => ({ label: t, type: 'type' })),
        filter: true, // 启用模糊匹配
      }
    }

    // 匹配 "表名." → 补全字段名
    const dotMatch = textBefore.match(/(\w+)\.$/)
    if (dotMatch) {
      const tableName = dotMatch[1]
      const columns = getColumns(tableName)
      if (columns.length > 0) {
        return {
          from: context.pos,
          options: columns.map(c => ({ label: c, type: 'property' })),
        }
      }
    }

    // 默认：SQL 关键字 + 表名混合补全（输入 2+ 字符触发）
    const wordMatch = textBefore.match(/\b(\w{2,})$/)
    if (wordMatch) {
      return {
        from: context.pos - wordMatch[1].length,
        options: [
          ...SQL_KEYWORDS.map(k => ({ label: k, type: 'keyword' })),
          ...tables.map(t => ({ label: t, type: 'type' })),
        ],
        filter: true,
      }
    }

    return null
  }
}
```

**后端 API 利用**：

| API | 当前状态 | v2.0 使用方式 |
|-----|---------|-------------|
| `GET /api/datasources/:id/tables` | ✅ 已实现，返回 `string[]` | 获取表名列表 |
| `GET /api/datasources/:id/columns?table=xxx` | ❌ 不存在 | **需后端新增**，或在 v2.0 阶段使用 `SHOW COLUMNS FROM table` 前端模拟 |

**字段名补全的降级策略**：

由于后端目前无字段列表 API，v2.0 采用以下方案：

1. **MVP**：仅补全表名 + SQL 关键字（利用已有 `GET /tables` API）
2. **增强**：如后端在 v2.0 周期内新增 `GET /api/datasources/:id/columns?table=xxx`，前端即接入字段名补全
3. **前端降级**：补全 hook 设计为字段名列表可选返回空数组，不影响表名和关键字补全

**MongoDB 编辑器补全**：

```typescript
function mongoCompletionSource(collections: string[]): CompletionSource {
  return (context) => {
    const textBefore = context.state.doc.lineAt(context.pos).text.slice(0, ...)
    // 在 JSON 编辑器中补全：
    // 1. 键位置：MongoDB 操作符 ($match, $group, $project, $sort, $limit, $skip, etc.)
    // 2. 集合名：在 collection 输入框中（非 CodeMirror，是普通 input）
    // ...
  }
}
```

MongoDB 补全分为两部分：
- **集合名补全**：在顶部 collection input 组件中实现（非 CodeMirror），使用 `<datalist>` 或自定义下拉
- **操作符补全**：在 JSON 编辑器中通过 CodeMirror `autocompletion()` 实现，补全 `$match`、`$group` 等操作符

#### 数据缓存设计

```typescript
// web/src/hooks/useSchemaCompletion.ts
interface SchemaCacheEntry {
  tables: string[]
  fetchedAt: number
}

const CACHE_TTL = 5 * 60 * 1000 // 5 minutes
const schemaCache = new Map<string, SchemaCacheEntry>()

function getCacheKey(datasourceId: number, database: string): string {
  return `${datasourceId}:${database}`
}

export function useSchemaCompletion(datasourceId: number | null, database: string) {
  const [tables, setTables] = useState<string[]>([])

  useEffect(() => {
    if (!datasourceId || !database) {
      setTables([])
      return
    }

    const key = getCacheKey(datasourceId, database)
    const cached = schemaCache.get(key)

    if (cached && Date.now() - cached.fetchedAt < CACHE_TTL) {
      setTables(cached.tables)
      return
    }

    // Fetch from API
    api.get<{ code: number; data: string[] }>(`/datasources/${datasourceId}/tables`)
      .then(res => {
        const tableList = res.data ?? []
        schemaCache.set(key, { tables: tableList, fetchedAt: Date.now() })
        setTables(tableList)
      })
      .catch(() => {
        // Graceful degradation: empty tables, no blocking
      })
  }, [datasourceId, database])
  // 注：api 对象为模块级单例，引用稳定，无需加入依赖列表；
  // setTables 为 React setState，引用稳定，无需加入依赖列表

  // Column fetching (when backend API available)
  const getColumns = useCallback(async (tableName: string): Promise<string[]> => {
    // Future: call /datasources/:id/columns?table=tableName
    return []
  }, [datasourceId])

  return { tables, getColumns }
}
```

#### 性能评估

| 指标 | 预期值 | 说明 |
|------|--------|------|
| 缓存命中时补全弹出 | < 50ms | 纯前端匹配，无网络请求 |
| 首次加载表名列表 | 200-500ms | 取决于目标数据库网络延迟 + `SHOW TABLES` 执行时间 |
| 缓存内存占用 | ~5KB/数据源 | 100 个表名 × 50 字符 ≈ 5KB |
| 模糊匹配延迟 | < 10ms | CodeMirror 内置 filter 处理 1000 个选项 |
| 补全弹出最大项数 | 100 | CodeMirror 默认限制，避免 DOM 过多 |

#### 涉及文件改动

| 文件 | 改动内容 | 改动量 |
|------|---------|--------|
| **新建** `web/src/hooks/useSchemaCompletion.ts` | Schema 补全数据 hook + 缓存 | ~80 行 |
| **新建** `web/src/api/metadata.ts` | 元数据 API 封装（可选，也可复用现有 `api` client） | ~30 行 |
| `web/src/pages/Query/components/SqlEditor.tsx` | 接收 `tables` props，添加 `autocompletion()` 扩展 | ~60 行 |
| `web/src/pages/Query/components/MongoEditor.tsx` | 操作符补全 + 集合名补全 | ~40 行 |
| `web/src/pages/Query/index.tsx` | 调用 `useSchemaCompletion`，传递数据给编辑器 | ~15 行 |

---

### 3. 全局搜索架构

#### 方案选型

| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| **A. 扩展现有 CommandPalette（cmdk）** | 在现有 `cmdk` 组件基础上新增搜索分组和后端 API 集成 | 最小改动；`cmdk` 原生支持分组；已有骨架 | 需要实现 debounce 和异步搜索逻辑 |
| **B. 替换为自定义搜索组件** | 自建搜索面板组件 | 完全控制 UI | 重复造轮子；工作量大；需重新实现键盘导航 |
| **C. 引入 Meilisearch / FlexSearch** | 前端全文搜索引擎 | 搜索能力强 | 引入新依赖；需构建索引；过度设计（数据量小） |

#### 推荐方案：A. 扩展现有 CommandPalette（cmdk）

**理由**：
1. `cmdk`（`^1.1.1`）已安装，原生支持分组（`CommandGroup`）、空状态（`CommandEmpty`）
2. 现有 `CommandPalette` 已实现 `Cmd+K` 快捷键、打开/关闭、页面导航
3. 扩展工作量小：新增 3 个后端搜索分组 + debounce 逻辑
4. 后端已有搜索参数：`GET /api/query/history?keyword=`、`GET /api/tickets?keyword=`、`GET /api/audit-logs?keyword=`

#### 实现架构

```
┌──────────────────────────────────────────────────┐
│ CommandPalette (增强版)                            │
│                                                   │
│  State:                                           │
│    searchQuery: string                            │
│    results: {                                     │
│      history: QueryHistoryItem[]   // 最多 5 条    │
│      tickets: Ticket[]             // 最多 5 条    │
│      auditLogs: AuditLog[]         // 最多 5 条    │
│    }                                              │
│    loading: boolean                               │
│                                                   │
│  Effects:                                         │
│    useEffect([searchQuery]) → debounce(300ms)     │
│      → Promise.all([                              │
│          fetchHistory({keyword: q, page_size: 5}),│
│          listTickets({keyword: q, page_size: 5}), │
│          listAuditLogs({keyword: q, page_size: 5})│
│        ])                                         │
│                                                   │
│  Empty state (无输入):                             │
│    显示最近 5 条查询历史（fetchHistory page=1）      │
│                                                   │
│  UI:                                              │
│    ┌─ CommandInput ─────────────────────────────┐ │
│    ├─ CommandGroup "页面" (已有)                  │ │
│    ├─ CommandSeparator                           │ │
│    ├─ CommandGroup "查询历史" (动态)              │ │
│    ├─ CommandGroup "变更工单" (动态)              │ │
│    ├─ CommandGroup "审计日志" (动态)              │ │
│    └────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```

**搜索结果渲染**：

```typescript
// 搜索结果项统一结构
interface SearchResultItem {
  id: string
  label: string          // SQL 摘要前 60 字符
  sublabel: string       // 数据源名 + 时间
  group: string          // 分组名
  icon: LucideIcon
  keywords: string       // 高亮匹配用
  action: () => void     // 点击行为
}
```

**跳转行为实现**：

| 结果类型 | 点击行为 | 实现方式 |
|---------|---------|---------|
| 查询历史 | 打开新 Tab 并填入 SQL | 调用 `useQueryStore.restoreHistoryAsTab()` |
| 变更工单 | 打开工单详情抽屉 | `navigate('/tickets')` + 触发全局事件打开详情 |
| 审计日志 | 跳转审计页并高亮 | `navigate('/audit')` + URL 参数传递目标 ID |

**工单详情抽屉的跨页面触发**：需要在 `TicketPage` 中监听全局事件或 URL 参数来打开详情抽屉。

```typescript
// 点击工单搜索结果
function handleTicketClick(ticketId: number) {
  onOpenChange(false)
  navigate('/tickets?action=open-ticket&id=' + ticketId)
}

// TicketPage 监听
const searchParams = new URLSearchParams(location.search)
const openTicketId = searchParams.get('action') === 'open-ticket' ? Number(searchParams.get('id')) : null
```

**Debounce 实现**：

使用 `useRef` + `setTimeout` 实现 300ms debounce（不引入 `lodash.debounce` 等新依赖）：

```typescript
const timerRef = useRef<ReturnType<typeof setTimeout>>()

useEffect(() => {
  if (!searchQuery.trim()) {
    // 空输入：加载最近 5 条历史
    loadRecentHistory()
    return
  }

  clearTimeout(timerRef.current)
  timerRef.current = setTimeout(async () => {
    setLoading(true)
    try {
      const [historyRes, ticketRes, auditRes] = await Promise.all([
        fetchHistory({ page: 1, page_size: 5, keyword: searchQuery }),
        listTickets({ page: 1, page_size: 5, keyword: searchQuery }),
        listAuditLogs({ page: 1, page_size: 5, keyword: searchQuery }),
      ])
      setResults({ history: historyRes.data, tickets: ticketRes.data, auditLogs: auditRes.data })
    } catch {
      // 静默处理
    } finally {
      setLoading(false)
    }
  }, 300)

  return () => clearTimeout(timerRef.current)
}, [searchQuery])
```

**关键词高亮**：

`cmdk` 不原生支持高亮，需自定义渲染：

```typescript
function HighlightText({ text, query }: { text: string; query: string }) {
  if (!query) return <>{text}</>
  const parts = text.split(new RegExp(`(${escapeRegex(query)})`, 'gi'))
  return (
    <>
      {parts.map((part, i) =>
        part.toLowerCase() === query.toLowerCase()
          ? <mark key={i} className="bg-[var(--accent-primary)]/30 text-[var(--text-primary)] rounded px-0.5">{part}</mark>
          : part
      )}
    </>
  )
}
```

#### 性能评估

| 指标 | 预期值 | 说明 |
|------|--------|------|
| Debounce 延迟 | 300ms | 防止快速输入时频繁请求 |
| 单次搜索 API 响应 | < 500ms | 3 个并发请求（SQLite 模糊匹配 + 分页） |
| 搜索面板打开 | < 50ms | `cmdk` 组件渲染，本地数据 |
| 关键词高亮渲染 | < 10ms | 纯前端字符串分割 + DOM 渲染 |
| 内存占用 | ~50KB | 缓存 15 条搜索结果（5×3 分组） |

#### 涉及文件改动

| 文件 | 改动内容 | 改动量 |
|------|---------|--------|
| `web/src/components/CommandPalette.tsx` | 重构：新增搜索状态、debounce、分组渲染、高亮 | ~200 行（重写大部分） |
| `web/src/api/query.ts` | `fetchHistory` 添加 `keyword` 参数支持 | ~5 行 |
| `web/src/pages/Ticket/index.tsx` | 监听 URL 参数打开工单详情 | ~10 行 |
| `web/src/pages/Audit/index.tsx` | 监听 URL 参数高亮审计记录 | ~10 行 |

---

### 4. 查询结果列排序/列筛选

#### 现状分析

**列排序**：v1.0 `ResultTable.tsx` 已完整实现（第 68-75 行）：
- 使用 `@tanstack/react-table` 的 `getSortedRowModel()`
- 点击表头触发 `getToggleSortingHandler()`
- 显示 `ChevronUp`/`ChevronDown`/`ChevronsUpDown` 排序指示
- **v2.0 无需额外开发，仅确认稳定性**

**列筛选**：未实现。当前 `ResultTable` 无筛选功能。

#### 方案选型（列筛选）

| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| **A. TanStack Table 内置 faceted filter** | 使用 `getFacetedRowModel()` + `getFilteredRowModel()` + 自定义 FilterPopover | 与现有 Table 架构完美集成；前端过滤无网络请求 | 需计算唯一值列表（大数据集有开销） |
| **B. 后端 WHERE 条件过滤** | 每次筛选重新发起 SQL 查询 | 无前端数据量限制 | 需修改 SQL；增加后端负担；用户体验差（等待网络） |
| **C. Web Worker 离线过滤** | 在 Web Worker 中执行过滤计算 | 不阻塞主线程 | 架构复杂；数据需序列化传输；过度设计 |

#### 推荐方案：A. TanStack Table 内置 faceted filter

**理由**：
1. `@tanstack/react-table`（`^8.21.3`）已安装，原生支持 `getFacetedRowModel()` 和 `getFacetedUniqueValues()`
2. 查询结果通常 < 1000 行（`default_row_limit: 1000`），前端过滤性能无压力
3. 与现有 `ResultTable` 架构（`useReactTable` + `getCoreRowModel` + `getSortedRowModel`）无缝集成
4. PRD 明确要求前端过滤（"前端过滤，不重新请求后端"）

#### 实现架构

```
┌──────────────────────────────────────────────────┐
│ ResultTable (增强版)                              │
│                                                   │
│  State:                                           │
│    sorting: SortingState (已有)                    │
│    columnFilters: ColumnFiltersState (新增)        │
│                                                   │
│  Table config:                                    │
│    getCoreRowModel()       (已有)                  │
│    getSortedRowModel()     (已有)                  │
│    getFilteredRowModel()   (新增)                  │
│    getFacetedRowModel()    (新增，计算唯一值)       │
│    getFacetedUniqueValues() (新增)                 │
│                                                   │
│  Column header 渲染:                              │
│    [列名] [排序图标] [筛选图标]                     │
│                      ↓ 点击                       │
│              ┌─ ColumnFilter ──────────┐          │
│              │ [搜索框] (唯一值 > 200)  │          │
│              │ ☑ 全选 / ☐ 全不选       │          │
│              │ ☑ value1                │          │
│              │ ☑ value2                │          │
│              │ ☐ value3                │          │
│              │ [确认] [重置]            │          │
│              └─────────────────────────┘          │
│                                                   │
│  新查询时: 清除 columnFilters                      │
└──────────────────────────────────────────────────┘
```

**ColumnFilter 组件**：

```typescript
// web/src/pages/Query/components/ColumnFilter.tsx
interface ColumnFilterProps {
  column: Column<Record<string, unknown>>
}

export default function ColumnFilter({ column }: ColumnFilterProps) {
  const [open, setOpen] = useState(false)
  const [searchValue, setSearchValue] = useState('')

  // 获取 faceted unique values
  const facetedValues = column.getFacetedUniqueValues() as Map<unknown, number>
  const uniqueValues = Array.from(facetedValues.keys())

  // 搜索过滤
  const filteredValues = searchValue
    ? uniqueValues.filter(v => String(v).toLowerCase().includes(searchValue.toLowerCase()))
    : uniqueValues

  const showSearch = uniqueValues.length > 200

  // 当前选中值
  const filterValue = (column.getFilterValue() as string[]) ?? uniqueValues.map(String)
  const isFiltered = filterValue.length < uniqueValues.length

  // ... 渲染 Popover + Checkbox 列表
}
```

**筛选逻辑**：

```typescript
// ResultTable.tsx 中添加 column definition
columns: result.columns.map((col) => ({
  accessorKey: col,
  // ... 现有 config
  filterFn: (row, _columnId, filterValue: string[]) => {
    const cellValue = String(row.getValue(_columnId))
    return filterValue.includes(cellValue)
  },
}))
```

**新查询清除筛选**：

```typescript
// 在 useEffect([result]) 中重置
useEffect(() => {
  setPage(0)
  setSorting([])
  setColumnFilters([]) // 新增：清除列筛选
  setPageSize(initialPageSize)
}, [result, initialPageSize])
```

#### 性能评估

| 指标 | 预期值 | 说明 |
|------|--------|------|
| 1000 行 × 10 列唯一值计算 | < 50ms | `getFacetedUniqueValues()` 遍历一次 |
| 筛选过滤 1000 行 | < 20ms | TanStack Table 内存过滤 |
| 200+ 唯一值搜索过滤 | < 10ms | 纯前端字符串匹配 |
| Popover 打开 | < 16ms | Shadcn Popover 渲染 |

#### 涉及文件改动

| 文件 | 改动内容 | 改动量 |
|------|---------|--------|
| **新建** `web/src/pages/Query/components/ColumnFilter.tsx` | 列筛选 Popover 组件 | ~120 行 |
| `web/src/pages/Query/components/ResultTable.tsx` | 添加 `columnFilters` 状态 + filter models + 表头筛选图标 | ~40 行 |

---

### 5. 审计日志增强

#### 方案选型

| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| **A. 纯前端展示** | 假设后端 audit_logs 已有 `ai_review_result` 和 `ticket_id` 字段，前端读取并渲染 | 最简实现 | 后端 audit_logs schema 可能无这两个字段 |
| **B. 前端展示 + 后端 ALTER TABLE** | 前端新增展示区块 + 后端添加字段 | 完整方案 | 需要后端改动（PRD 说 v2.0 不新增后端模块） |
| **C. 关联查询展示** | 后端 JOIN 查询关联工单表和 AI 评审记录 | 数据完整 | 需新增 API 端点或修改现有 API |

#### 推荐方案：A. 纯前端展示（优雅降级）

**理由**：
1. PRD 明确要求 v2.0 不新增后端模块
2. 后端 `audit_logs` 表当前 **不包含** `ai_review_result` 和 `ticket_id` 字段（已确认 v1.0 Ent schema 无此两个字段）
3. 前端先实现展示能力：如果 API 返回的字段中包含这些数据则渲染，否则隐藏对应区块
4. 后端字段作为后续补充（PRD 也提到"若后端未存储，则 v2.0 仅实现前端展示"）

**⚠️ 风险说明**：由于后端当前不存储这两个字段，v2.0 交付时 AI 评审区块和关联工单区块在审计日志展开详情中将不可见。需与产品经理确认这是否可接受，或是否需要在 v2.0 周期额外增加后端字段（ALTER TABLE 添加两个 nullable 列，无需新增模块，工作量极小）。

#### 实现方案

**1. 扩展前端 AuditLog 类型**：

```typescript
// web/src/api/audit.ts — 扩展 AuditLog 接口
export interface AuditLog {
  // ... 现有字段
  // v2.0 新增（可选字段，后端可能不返回）
  ai_review_result?: string    // JSON string: { risk_level, summary, suggestions }
  ticket_id?: number           // 关联工单 ID
}
```

**2. ExpandedRow 新增展示区块**：

```typescript
// ExpandedRow 组件新增两个区块

// AI 评审区块（当 ai_review_result 存在时显示）
{log.ai_review_result && (() => {
  try {
    const review = JSON.parse(log.ai_review_result)
    return (
      <div className="col-span-full border-t border-[var(--border-subtle)] pt-3">
        <span className="text-xs font-medium text-[var(--text-secondary)]">AI 评审</span>
        <div className="mt-2 flex flex-col gap-2">
          {/* 风险等级标签 */}
          <Badge className={getRiskColor(review.risk_level)}>
            {getRiskLabel(review.risk_level)}
          </Badge>
          {/* 评审摘要 */}
          <p className="text-xs text-[var(--text-primary)]">{review.summary}</p>
          {/* 优化建议列表 */}
          {review.suggestions?.map((s: string, i: number) => (
            <div key={i} className="text-xs text-[var(--text-secondary)]">• {s}</div>
          ))}
        </div>
      </div>
    )
  } catch { return null }
})()}

// 关联工单区块（当 ticket_id 存在时显示）
{log.ticket_id && (
  <div>
    <span className="text-xs text-[var(--text-muted)]">关联工单</span>
    <button
      className="mt-0.5 text-sm text-[var(--accent-primary)] hover:underline"
      onClick={(e) => {
        e.stopPropagation()
        // 打开工单详情抽屉
        navigate(`/tickets?action=open-ticket&id=${log.ticket_id}`)
      }}
    >
      工单 #{log.ticket_id}
    </button>
  </div>
)}
```

**3. 布局调整**：

当前 `ExpandedRow` 使用 `grid-cols-2 lg:grid-cols-4`。新增 AI 评审和关联工单后：

- AI 评审区块：`col-span-full`（独占一行）
- 关联工单：新增一个 grid cell
- 布局从 `grid-cols-2 lg:grid-cols-4` 保持不变，新增项自然填充

#### 性能评估

| 指标 | 预期值 | 说明 |
|------|--------|------|
| JSON.parse(ai_review_result) | < 1ms | 单条评审结果，数据量 ~1KB |
| 展开/收起动画 | < 100ms | CSS transition |
| 无额外 API 请求 | 0 | 数据随 audit-logs 列表一起返回 |

#### 涉及文件改动

| 文件 | 改动内容 | 改动量 |
|------|---------|--------|
| `web/src/api/audit.ts` | `AuditLog` 接口新增 `ai_review_result?` 和 `ticket_id?` 字段 | ~5 行 |
| `web/src/pages/Audit/index.tsx` | `ExpandedRow` 新增 AI 评审区块 + 关联工单区块 | ~60 行 |

---

### 6. TypeScript 严格模式迁移

#### 现状分析

**当前配置**（`tsconfig.app.json`）：

```json
{
  "compilerOptions": {
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    // 注意：无 "strict": true"
  }
}
```

`strict: true` 会开启以下检查：
- `strictNullChecks` — null/undefined 不能赋值给其他类型
- `strictFunctionTypes` — 函数参数逆变检查
- `noImplicitAny` — 禁止隐式 any
- `noImplicitThis` — 禁止隐式 this
- `strictBindCallApply` — bind/call/apply 参数检查
- `strictPropertyInitialization` — 类属性必须初始化
- `alwaysStrict` — 输出 "use strict"

#### 方案选型

| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| **A. 一次性开启 strict + 批量修复** | 设置 `strict: true`，根据编译错误逐一修复 | 一步到位 | 编译错误可能很多（预计 100+），修复周期长 |
| **B. 渐进式开启** | 逐个开启 strict 子选项，分批修复 | 风险可控；每批影响范围小 | 耗时更长；中间状态配置复杂 |
| **C. 开启 strict + @ts-expect-error 兜底** | 开启 strict，对难以修复的项用 `@ts-expect-error` 标记 | 快速达成 tsc 零错误 | 技术债累积；需 Issue 跟踪 |

#### 推荐方案：B+C. 渐进式修复 + 一次性开启 strict

**理由**：
1. 当前代码已开启 `noUnusedLocals` 和 `noUnusedParameters`，说明团队已有类型意识
2. 先分批修复代码（渐进式），再一次性开启 `"strict": true`（效率最优）
3. 代码基础较好（无 `@ts-ignore`，仅 3 处 `eslint-disable`），预计 strict 模式编译错误可控

**分批策略**：

| 批次 | 开启选项 | 预估影响 | 修复策略 |
|------|---------|---------|---------|
| 1 | `strictNullChecks` + `noImplicitAny` | 最大（预计 50-80 处） | null/undefined 检查 + 显式类型注解 |
| 2 | `strictFunctionTypes` + `strictBindCallApply` | 较小（预计 10-20 处） | 修正函数签名 |
| 3 | `strictPropertyInitialization` + `noImplicitThis` | 较小（预计 5-10 处） | 类属性初始化 + 显式 this 注解 |

**最终 tsconfig 配置**：

```json
{
  "compilerOptions": {
    "strict": true,
    // 保留现有配置
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "erasableSyntaxOnly": true
  }
}
```

**常见修复模式**：

```typescript
// 1. null/undefined 检查
// Before
const user = data.user
console.log(user.name) // TS error: user possibly null

// After
const user = data.user
if (!user) return
console.log(user.name)

// 2. 显式类型注解替代隐式 any
// Before
function handleEvent(e) { ... } // TS error: implicit any

// After
function handleEvent(e: React.ChangeEvent<HTMLInputElement>) { ... }

// 3. API 响应类型
// Before
const res = await api.get('/datasources')
const list = res.data // implicit any[]

// After
interface DatasourceListResponse { code: number; data: DataSource[] }
const res = await api.get<DatasourceListResponse>('/datasources')
const list = res.data
```

#### 涉及文件改动

| 文件 | 改动内容 | 改动量 |
|------|---------|--------|
| `web/tsconfig.app.json` | 添加 `"strict": true` | 1 行 |
| 预计 20-30 个 TS/TSX 文件 | null 检查、类型注解、函数签名修正 | 每文件 1-10 行不等 |

**预计修复重点文件**（基于代码审查）：

| 文件 | 预期问题 |
|------|---------|
| `web/src/api/client.ts` | `request<T>` 返回值可能需要 null 检查 |
| `web/src/store/queryStore.ts` | `getStoredSplitRatio` 的 `parseFloat` 结果可能为 NaN |
| `web/src/pages/Query/index.tsx` | `activeTab?.datasourceId` 的 null 检查链 |
| `web/src/pages/Audit/index.tsx` | `log.desensitized_fields.split()` 可能 undefined |
| `web/src/components/Layout.tsx` | `user?.username` 等可选链 |
| `web/src/pages/Query/components/ResultTable.tsx` | `result?.columns` map 的 null 安全 |

#### 性能评估

| 指标 | 影响 | 说明 |
|------|------|------|
| 编译时间 | 可能增加 10-20% | strict 模式需要更多类型检查，但项目小（62 文件）影响可忽略 |
| 运行时性能 | 无影响 | TypeScript 类型擦除，strict 不影响运行时 |
| 构建产物大小 | 无变化 | 类型信息不进入构建产物 |
| IDE 体验 | 提升 | 更准确的类型提示和错误检查 |

---

### 7. ESLint 修复策略

#### 现状分析

当前 6 个 ESLint 问题分布（基于 `npx eslint src/` 实际执行结果）：

| 规则 | 数量 | 严重程度 | 文件 |
|------|------|---------|------|
| `react-hooks/set-state-in-effect` | 4 | 高（Effect 中同步 setState） | Audit/index.tsx, CommentSection.tsx, TicketDetailDrawer.tsx, Ticket/index.tsx |
| `react-hooks/incompatible-library` | 1 | 低（TanStack Table + React Compiler） | ResultTable.tsx |
| 未使用的 eslint-disable 指令 | 1 | 低 | Audit/index.tsx |

> 注：代码库经 Sprint 1-2 开发后已修复大部分问题，当前仅剩 6 项。

#### 方案选型

| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| **A. 逐文件修复** | 从 core 文件开始，逐文件修复所有 ESLint 警告 | 可控；每步可验证 | 耗时 |
| **B. 按规则分类修复** | 先修简单的（no-unused-vars），再修复杂的（no-explicit-any） | 效率高；同类问题一起修 | 跨文件跳转 |
| **C. eslint --fix 自动修复 + 手动修复剩余** | 先自动修复可修复的，再手动处理 | 最快 | 自动修复可能引入错误 |

#### 推荐方案：C. 自动修复 + B. 手动分类修复

**执行步骤**：

```
Step 1: 手动修复 set-state-in-effect   → 将 Effect 中的数据获取重构为事件回调或 useSyncExternalStore
Step 2: 处理 incompatible-library       → TanStack Table 已知限制，添加 eslint-disable + 注释说明
Step 3: 删除未使用的 eslint-disable 指令 → Audit/index.tsx
Step 4: 验证 npm run lint 零警告
```

**修复原则**（来自 PRD）：

1. **不使用 `eslint-disable` 注释绕过**（除非有充分理由并添加注释说明）
2. 未使用的变量/导入直接删除
3. `any` 类型替换为具体类型
4. `useEffect` 依赖项补全

**现有 3 处 eslint-disable 处理**：

| 文件 | 当前注释 | 修复方式 |
|------|---------|---------|
| `SqlEditor.tsx` L87 | `eslint-disable-next-line react-hooks/exhaustive-deps` | 补全依赖项（将 `value` 加入依赖，配合 `isExternalUpdate` 防止循环更新） |
| `MongoEditor.tsx` L122 | 同上 | 同上 |
| `Query/index.tsx` L36 | `eslint-disable-next-line react-hooks/exhaustive-deps` | 补全依赖（`tabs` 需加入依赖，但初始化场景可使用 `useRef` 标记首次加载） |

**修复后的 useEffect 模式**：

```typescript
// SqlEditor.tsx — 修复 exhaustive-deps
useEffect(() => {
  if (!containerRef.current) return
  const view = new EditorView({
    state: EditorState.create({ doc: value, extensions: getExtensions() }),
    parent: containerRef.current,
  })
  viewRef.current = view
  return () => {
    view.destroy()
    viewRef.current = null
  }
}, []) // eslint-disable-line react-hooks/exhaustive-deps -- intentionally run once: editor init
// 注：此处保留 eslint-disable 但添加注释说明原因
```

> **决策**：CodeMirror 编辑器初始化确实只需执行一次，此处保留 `eslint-disable` 但添加行内注释说明原因。其他两处同理。

> **注意**：PRD §6.1 要求"不使用 eslint-disable 注释绕过"，此处属于合理例外——CodeMirror EditorView 初始化需稳定引用，将 `value` 等加入依赖会导致不必要的重初始化。保留 eslint-disable 但必须附注释说明原因。其余所有位置的 eslint-disable 需在 ESLint 修复阶段消除。

#### 涉及文件改动

预计影响 **30-40 个文件**，每个文件 1-5 行修改。

#### 性能评估

| 指标 | 影响 | 说明 |
|------|------|------|
| 构建时间 | 无变化 | ESLint 是 dev 依赖，不影响构建 |
| 代码质量 | 提升 | 消除潜在 bug（exhaustive-deps）、提升可维护性 |
| CI 流水线 | 可新增 lint 检查步骤 | `npm run lint` 作为 CI 门禁 |

---

## 数据流变更

### v1.0 数据流

```
用户操作 → Zustand Store (queryStore) → API 请求 → 后端
                ↓
         React 组件订阅 Store → 渲染 UI
```

### v2.0 新增数据流

```
┌─────────────────────────────────────────────────────────────┐
│ 1. 主题切换数据流                                             │
│    localStorage('theme') → useTheme hook → data-theme DOM   │
│    无 Zustand 参与，纯 DOM + localStorage                    │
├─────────────────────────────────────────────────────────────┤
│ 2. 自动补全数据流                                             │
│    datasourceId + database → useSchemaCompletion hook        │
│    → GET /api/datasources/:id/tables → SchemaCache (Map)     │
│    → SqlEditor autocompletion() 扩展 → 补全弹出              │
├─────────────────────────────────────────────────────────────┤
│ 3. 全局搜索数据流                                             │
│    Cmd+K → CommandPalette 打开 → 输入关键词                   │
│    → debounce 300ms → Promise.all(3 API 请求)                │
│    → 分组渲染结果 → 点击 → navigate/restoreHistory            │
├─────────────────────────────────────────────────────────────┤
│ 4. 列筛选数据流                                               │
│    点击表头筛选图标 → ColumnFilter Popover                    │
│    → 勾选/取消 → columnFilters state → getFilteredRowModel   │
│    → 表格自动过滤（纯前端，无 API 调用）                       │
├─────────────────────────────────────────────────────────────┤
│ 5. 审计增强数据流                                             │
│    无新增数据流，仅扩展 ExpandedRow 渲染逻辑                   │
│    → 读取 ai_review_result JSON → 解析 → 渲染                │
│    → 读取 ticket_id → 渲染链接 → navigate                    │
└─────────────────────────────────────────────────────────────┘
```

---

## 性能评估

### 新增性能开销汇总

| 功能 | 首次加载 | 运行时 | 内存增量 | 说明 |
|------|---------|--------|---------|------|
| 主题切换 | 0ms | < 16ms（切换） | ~0KB | CSS 变量切换 |
| 自动补全 | 200-500ms（首次） | < 50ms（缓存命中） | ~5KB/数据源 | 表名列表缓存 |
| 全局搜索 | 0ms | < 500ms（搜索） | ~50KB | 搜索结果缓存 |
| 列筛选 | 0ms | < 50ms（筛选） | ~10KB | 唯一值 Map |
| 审计增强 | 0ms | < 1ms（展开） | ~0KB | JSON.parse |
| TS strict | 0ms | 0ms | 0KB | 编译期检查 |
| ESLint | 0ms | 0ms | 0KB | 开发期检查 |

### 构建产物影响

| 指标 | v1.0 预估 | v2.0 预估 | 增量 |
|------|----------|----------|------|
| JS Bundle (gzip) | ~350KB | ~360KB | +10KB（自动补全 + 搜索 + 筛选逻辑） |
| CSS (gzip) | ~30KB | ~30KB | +0KB（复用现有 token） |
| 首屏加载 | < 2s | < 2s | 无变化 |

---

## 风险评估

### 高风险

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| TS strict 开启后编译错误量超出预期 | 开发进度延迟 | 中 | 渐进式开启；允许 `@ts-expect-error` 临时标记 |
| `useEffect` 依赖项补全引入无限循环 | 运行时 bug | 中 | 逐一验证；保留必要位置的 eslint-disable + 注释 |
| 浅色主题下部分组件对比度不足 | UI 质量问题 | 中 | 全面视觉检查；WCAG AA 标准验证 |

### 中风险

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| 后端 `GET /tables` API 在大数据源下响应慢 | 自动补全体验差 | 低 | 5 分钟前端缓存；Loading 状态；超时优雅降级 |
| CodeMirror 主题切换不完全生效 | 编辑器视觉不一致 | 低 | 监听 CustomEvent 重建扩展；视觉验证 |
| 全局搜索 3 个并发 API 请求性能 | 搜索面板响应慢 | 低 | Promise.allSettled 容错；每个 API 单独 timeout |

### 低风险

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| 列筛选大数据集唯一值计算慢 | 筛选面板打开慢 | 低 | 限制 200 个唯一值；超出显示搜索框 |
| 审计增强后端无 ai_review_result 字段 | 功能无数据展示 | 确定 | 前端优雅降级（字段不存在则不显示区块） |
| ESLint 修复引入回归 | 现有功能异常 | 低 | 每修复一批后执行全页面回归测试 |

---

## 迁移计划（按 Milestone）

### Milestone 1：代码质量治理（Sprint 5，1 周）

**目标**：ESLint 清零 + TypeScript 严格模式

| 步骤 | 任务 | 预估 | 涉及文件 |
|------|------|------|---------|
| 1.1 | `npx eslint --fix src/` 自动修复 | 0.5 天 | 全部 |
| 1.2 | 手动修复 `no-unused-vars` | 0.5 天 | ~20 个文件 |
| 1.3 | 手动修复 `no-explicit-any` | 1 天 | ~15 个文件 |
| 1.4 | 手动修复 `exhaustive-deps` | 0.5 天 | ~10 个文件 |
| 1.5 | 手动修复 `only-export-components` + `no-console` | 0.5 天 | ~7 个文件 |
| 1.6 | `npm run lint` 验证零警告 | 0.1 天 | — |
| 1.7 | `tsconfig.app.json` 开启 `strict: true` | 0.1 天 | 1 个文件 |
| 1.8 | 修复 `strictNullChecks` 错误 | 1 天 | ~20 个文件 |
| 1.9 | 修复其他 strict 子选项错误 | 0.5 天 | ~10 个文件 |
| 1.10 | `tsc -b` + `npm run build` 验证 | 0.1 天 | — |
| 1.11 | 全页面回归测试 | 0.5 天 | — |

**门禁**：
- `npm run lint` 零警告零错误
- `tsc -b` 零错误
- `npm run build` 成功
- 所有页面功能正常

### Milestone 2：用户体验增强（Sprint 6，1.5 周）

**目标**：浅色主题 + SQL 自动补全

| 步骤 | 任务 | 预估 | 涉及文件 |
|------|------|------|---------|
| 2.1 | 新建 `useTheme` hook + localStorage 持久化 | 0.5 天 | 1 个新文件 |
| 2.2 | `Layout.tsx` 头像下拉新增主题切换 | 0.5 天 | 1 个文件 |
| 2.3 | `main.tsx` 初始化 `data-theme` 属性 | 0.1 天 | 1 个文件 |
| 2.4 | SqlEditor 主题适配（监听 theme-change） | 0.5 天 | 1 个文件 |
| 2.5 | MongoEditor 主题适配 | 0.5 天 | 1 个文件 |
| 2.6 | 全页面浅色主题视觉检查 + 修复 | 0.5 天 | 若干 CSS 调整 |
| 2.7 | 新建 `useSchemaCompletion` hook + 缓存 | 0.5 天 | 1 个新文件 |
| 2.8 | SqlEditor 添加 `autocompletion()` 扩展 | 1 天 | 1 个文件 |
| 2.9 | SQL 关键字 + 表名补全 + 模糊匹配 | 0.5 天 | 同上 |
| 2.10 | MongoEditor 操作符补全 + 集合名补全 | 0.5 天 | 1 个文件 |
| 2.11 | QueryPage 传递 schema 数据给编辑器 | 0.5 天 | 1 个文件 |
| 2.12 | 补全功能测试 + 调优 | 0.5 天 | — |

**门禁**：
- 深色/浅色主题切换即时生效，刷新保持
- CodeMirror 编辑器主题正确切换
- SQL 关键字 + 表名补全正常工作
- 切换数据源/库后补全列表更新

### Milestone 3：功能补齐（Sprint 7，1 周）

**目标**：全局搜索 + 列筛选 + 审计增强

| 步骤 | 任务 | 预估 | 涉及文件 |
|------|------|------|---------|
| 3.1 | CommandPalette 重构：搜索状态 + debounce | 1 天 | 1 个文件 |
| 3.2 | 集成查询历史 + 工单 + 审计日志搜索 API | 0.5 天 | 同上 |
| 3.3 | 搜索结果渲染 + 关键词高亮 | 0.5 天 | 同上 |
| 3.4 | 跳转行为（新 Tab / 工单详情 / 审计高亮） | 0.5 天 | 3 个文件 |
| 3.5 | 新建 `ColumnFilter` 组件 | 0.5 天 | 1 个新文件 |
| 3.6 | `ResultTable` 集成 faceted filter | 0.5 天 | 1 个文件 |
| 3.7 | 审计日志 `ExpandedRow` 增强 | 0.5 天 | 1 个文件 |
| 3.8 | `AuditLog` 类型扩展 | 0.1 天 | 1 个文件 |
| 3.9 | 全功能回归测试 | 0.5 天 | — |

**门禁**：
- `Cmd+K` 搜索返回查询历史、工单、审计日志结果
- 搜索结果关键词高亮
- 查询结果表格列筛选功能正常
- 审计日志展开详情显示 AI 评审和关联工单
- 所有 v1.0 功能无回归

---

## 附录

### A. 新增依赖

v2.0 **不引入任何新的 npm 依赖**。所有功能基于现有依赖实现：

| 功能 | 使用的已有依赖 |
|------|-------------|
| 主题切换 | 无（DOM API + localStorage） |
| 自动补全 | `@codemirror/autocomplete`（已安装 `^6.20.1`） |
| 全局搜索 | `cmdk`（已安装 `^1.1.1`） |
| 列筛选 | `@tanstack/react-table`（已安装 `^8.21.3`） |
| 审计增强 | 无（纯渲染逻辑） |

### B. 后端 API 依赖矩阵

| API 端点 | v2.0 功能 | 当前状态 |
|---------|---------|---------|
| `GET /api/datasources/:id/tables` | 自动补全 | ✅ 已实现 |
| `GET /api/query/history?keyword=` | 全局搜索 | ✅ 已支持（keyword 参数在 handler/query.go L91 已实现） |
| `GET /api/tickets?keyword=` | 全局搜索 | ✅ 已支持（keyword 参数在 handler/ticket.go L110 已实现） |
| `GET /api/audit-logs?keyword=` | 全局搜索 | ✅ 已支持（AuditListParams 含 keyword） |
| `GET /api/datasources/:id/columns?table=` | 字段名补全 | ❌ 不存在（v2.0 降级处理） |

### C. 文件改动量总览

| 类型 | 文件数 | 总行数（预估） |
|------|--------|-------------|
| 新建文件 | 4 个 | ~290 行 |
| 修改文件 | ~35 个 | ~400 行 |
| 删除文件 | 0 个 | — |
| **总计** | ~39 个 | ~690 行 |

### D. 架构设计决策记录（ADR）

| # | 决策 | 选项 | 理由 |
|---|------|------|------|
| ADR-1 | 主题方案选择 CSS 变量 + data-theme | 方案 A | v1.0 token 已就绪；零新增依赖；性能最优 |
| ADR-2 | 自动补全使用 @codemirror/autocomplete | 方案 A | 已有依赖；原生体验；后端 API 已就绪 |
| ADR-3 | 全局搜索扩展 cmdk 而非自建 | 方案 A | 已有依赖；最小改动；原生支持分组 |
| ADR-4 | 列筛选使用 TanStack faceted filter | 方案 A | 已有依赖；前端过滤；数据量可控 |
| ADR-5 | 审计增强纯前端展示 + 优雅降级 | 方案 A | v2.0 不新增后端模块；前端先就绪 |
| ADR-6 | TS strict 渐进式修复 + 一次性开启 | 方案 B+C | 先分批修复代码，再一次性开启 strict：true；风险可控、效率最优 |
| ADR-7 | ESLint 自动修复 + 手动分类修复 | 方案 C+B | 效率最高；分类处理质量有保障 |

---

## 评审记录

### 第一轮：架构师评审（2026-05-23）

| # | 问题 | 严重程度 | 修复建议 | 状态 |
|---|------|---------|---------|------|
| 1 | §2 `sqlCompletionSource` 函数签名错误：返回类型写成 `CompletionContext =>` 应为 `(context: CompletionContext) =>` | 高 | 修正函数签名 | 已修复 |
| 2 | §1 `useTheme` hook 示例代码缺少"示例代码"标注 | 低 | 添加注释说明为示例代码 | 已修复 |
| 3 | §2 `useSchemaCompletion` useEffect 依赖列表可能触发 strict 模式 lint 警告 | 低 | 添加行内注释说明 api 和 setState 引用稳定 | 已修复 |
| 4 | §3 全局搜索后端 API 依赖矩阵中 `keyword` 参数状态不确定 | 中 | 明确降级策略：若未支持则前端客户端过滤 | 已修复 |
| 5 | §5 审计增强后端无 `ai_review_result` / `ticket_id` 字段，仅标注为"已确认 schema"但未给出具体证据和风险说明 | 高 | 补充详细风险说明，明确 v2.0 交付时区块可能不可见 | 已修复 |
| 6 | §6 TS strict 方案描述矛盾：推荐"渐进式开启"但最终配置直接 `"strict": true` | 中 | 统一为"渐进式修复 + 一次性开启"，并更新 ADR-6 | 已修复 |
| 7 | §7 ESLint 修复策略中保留 3 处 eslint-disable，但 PRD 要求不使用 | 中 | 添加与 PRD 的例外说明，明确标注合理例外场景 | 已修复 |
| 8 | 附录 B 后端 API 依赖矩阵中 keyword 参数降级策略不完整 | 中 | 补充前端客户端过滤降级说明 | 已修复 |

### 第二轮：UI/UX 设计师评审（2026-05-23）

| # | 问题 | 严重程度 | 修复建议 | 状态 |
|---|------|---------|---------|------|
| 1 | §2 补全弹出框 CSS 使用了 `.cm-completionLabel-[data-type]` 选择器，与 CodeMirror 6 实际 DOM 结构不符 | 中 | 在 UI-DESIGN 中补充说明并修正 CSS 为设计意图描述 | 已在 UI 文档中修复 |
| 2 | §3 全局搜索 `HighlightText` 组件的高亮样式未在 UI 设计文档中定义 | 中 | 在 UI-DESIGN 中补充关键词高亮样式规范 | 已在 UI 文档中修复 |
| 3 | §4 列筛选的文件清单中缺少 `ColumnFilter.tsx` 新建文件 | 低 | 在 UI-DESIGN 新增文件清单中补充 | 已在 UI 文档中修复 |
| 4 | 审计增强 `AIReviewData` 接口 `risk_level` 字段值需确认与后端一致 | 低 | 后端 AI 评审已使用 low/medium/high，字段名一致 | 确认无问题 |

### 第三轮：技术负责人终审（2026-05-23）

**结论**：✅ 有条件通过

**备注**：
1. 函数签名错误已修复，代码示例已标注为示例代码
2. 后端 API keyword 参数不确定的问题已明确降级策略
3. 审计增强的后端字段缺失风险已充分标注，建议在 Sprint 7 开发前与产品经理确认是否需要追加后端 ALTER TABLE
4. TS strict 策略已统一为"渐进式修复 + 一次性开启"
5. ESLint eslint-disable 例外说明已与 PRD 对齐
6. 三份文档的需求↔架构↔UI 对应关系已确认一致，无遗漏

### Gate Review — 进入开发终审（2026-05-23）

**评审人**：Marcus（技术负责人）
**结论**：✅ **有条件通过**

**关键发现**：
1. ESLint 问题数从过时的 52 个更新为实际 6 个（已修复文档），M1 工期从 1 周缩减为 0.5 周
2. 后端 `audit_logs` 表确认无 `ai_review_result` 和 `ticket_id` 字段，v2.0 审计增强前端展示将无数据
3. 浅色主题缺少 `--shadow-*` token 的浅色变体，需开发时补充

**检查项**：

| # | 检查项 | 结果 | 备注 |
|---|--------|------|------|
| 1 | 需求完整性 | ✅ | 详见 PRD-v2 Gate Review |
| 2 | 架构可行性 | ✅ | 7 个变更点均有备选方案、性能评估、风险评估 |
| 3 | UI 设计一致性 | ⚠️ | 浅色主题 token 不完整，需补充 shadow/risk token 浅色变体 |
| 4 | 文档间一致性 | ✅ | 三份文档需求↔架构↔UI 一一对应 |
| 5 | 遗留问题闭环 | ✅ | 三轮评审 16 个问题全部修复；ESLint 数量已更新 |
| 6 | S2 新增 API 影响 | ✅ | 工单评论/定时执行/钉钉 OAuth 不影响前端 v2.0 架构 |
| 7 | 无新设计遗漏 | ⚠️ | accent 颜色文档值与实际代码不一致，以实际代码为准 |

**需跟进项**：
1. ⚠️ 审计增强后端字段缺失：v2.0 交付时 AI 评审和关联工单区块可能不可见，建议评估是否追加 ALTER TABLE（工作量极小，2 个 nullable 列）
2. ⚠️ 浅色主题 token 补全：`[data-theme='light']` 中补充 `--shadow-sm/md/lg` token
3. ⚠️ 后端 API keyword 参数：`GET /api/query/history` 和 `GET /api/tickets` 的 keyword 参数经代码验证已支持（已在实际代码中确认），附录 B 可更新为 ✅
