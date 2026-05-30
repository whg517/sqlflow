# SF-QA0038: UI 自动化测试方案设计

> **项目**：SQLFlow  
> **优先级**：P1🟠重要  
> **作者**：林夏（前端专家）  
> **日期**：2026-06-01  
> **状态**：方案设计  

---

## 1. 背景与目标

### 1.1 现状

SQLFlow 前端已有较完善的测试基础设施：

| 层级 | 工具 | 现有测试 | 位置 |
|------|------|---------|------|
| 单元测试 | Vitest + React Testing Library | 40 个测试文件 | `web/src/**/*.test.ts(x)` |
| E2E 测试 | Playwright | 63 个 spec 文件 | `e2e/tests/*.spec.ts` |

**问题**：
1. E2E 测试主要由后端驱动（API 级别验证），前端 UI 交互测试覆盖不足
2. 缺少 Page Object Model (POM) 抽象，测试维护成本高
3. 组件级测试覆盖率偏低（约 40%），关键路径未达叶青要求的 70%
4. 审批流程等新功能缺少 UI 自动化测试
5. CI 中前端测试缺少分层策略（smoke / regression / full）

### 1.2 目标

| 指标 | 当前 | 目标 |
|------|------|------|
| 前端关键路径自动化覆盖率 | ~40% | **≥70%** |
| E2E UI 交互测试场景数 | ~20 | **≥80** |
| 平均单次 E2E 运行时长 | ~8min | **≤5min（smoke）** |
| Page Object 复用率 | 0% | **≥90%** |
| CI 集成 | 手动触发 | **PR 自动 smoke + nightly full** |

---

## 2. 测试分层策略

### 2.1 三层测试金字塔

```
                    ┌─────────┐
                    │  E2E UI  │  ← Playwright（~80 场景）
                    │  ~15%    │
                ┌───┴─────────┴───┐
                │   组件测试/集成    │  ← Vitest + RTL（~150 用例）
                │      ~35%        │
            ┌───┴─────────────────┴───┐
            │       单元测试            │  ← Vitest（~200 用例）
            │          ~50%            │
            └─────────────────────────┘
```

### 2.2 各层职责

| 层级 | 工具 | 范围 | 运行时机 | 超时 |
|------|------|------|---------|------|
| **L1 单元** | Vitest | 工具函数、hooks、store 逻辑 | 每次提交 | 10s |
| **L2 组件** | Vitest + RTL | UI 组件交互、状态变化 | PR 合并前 | 60s |
| **L3 E2E** | Playwright | 完整用户流程 | PR smoke + nightly full | 5min / 30min |

---

## 3. E2E UI 测试架构

### 3.1 Page Object Model (POM) 设计

```
e2e/
├── pages/                    ← 新增：POM 层
│   ├── LoginPage.ts
│   ├── QueryPage.ts
│   ├── TicketPage.ts
│   ├── TicketDetailDrawer.ts
│   ├── ApprovalPoliciesPage.ts
│   ├── AuditPage.ts
│   ├── SettingsPage.ts
│   ├── UsersPage.ts
│   ├── DashboardPage.ts
│   └── BasePage.ts           ← 基类：通用导航/等待/断言
├── fixtures/
│   ├── index.ts              ← 扩展：注入 POM
│   └── auth.fixture.ts
├── support/
│   ├── real-test-helpers.ts
│   ├── real-api.ts
│   └── approval-helpers.ts
├── tests/
│   ├── smoke/                ← 新增：冒烟测试（~15 场景）
│   │   ├── login-logout.spec.ts
│   │   ├── navigation.spec.ts
│   │   ├── query-basic.spec.ts
│   │   └── ticket-crud.spec.ts
│   ├── approval/             ← 新增：审批模块分组
│   │   ├── approval-stepper.spec.ts
│   │   ├── resubmit-flow.spec.ts
│   │   ├── condition-builder.spec.ts
│   │   ├── policy-crud.spec.ts
│   │   └── approval-timeline.spec.ts
│   └── ... (现有 63 个 spec)
└── playwright.config.ts
```

### 3.2 BasePage 基类

```typescript
// e2e/pages/BasePage.ts
export abstract class BasePage {
  constructor(protected page: Page) {}

  // 通用导航
  async goto(path: string) { ... }
  async waitForPageLoad() { ... }

  // 通用断言
  async expectToast(message: string, type?: 'success' | 'error') { ... }
  async expectVisible(selector: string) { ... }
  async expectUrlContains(path: string) { ... }

  // 通用操作
  async fillInput(label: string, value: string) { ... }
  async clickButton(name: string) { ... }
  async confirmDialog() { ... }
  async cancelDialog() { ... }
}
```

### 3.3 Fixture 扩展

```typescript
// e2e/fixtures/index.ts — 扩展现有 fixture
import { test as base } from '@playwright/test';
import { LoginPage } from '../pages/LoginPage';
import { TicketPage } from '../pages/TicketPage';
import { ApprovalPoliciesPage } from '../pages/ApprovalPoliciesPage';

type Fixtures = {
  loginPage: LoginPage;
  ticketPage: TicketPage;
  approvalPoliciesPage: ApprovalPoliciesPage;
  authenticatedPage: Page;
};

export const test = base.extend<Fixtures>({
  authenticatedPage: async ({ page }, use) => {
    // 复用现有 loginViaApi
    await loginViaApi(page);
    await use(page);
  },
  loginPage: async ({ page }, use) => await use(new LoginPage(page)),
  ticketPage: async ({ authenticatedPage }, use) => await use(new TicketPage(authenticatedPage)),
  approvalPoliciesPage: async ({ authenticatedPage }, use) => await use(new ApprovalPoliciesPage(authenticatedPage)),
});
```

---

## 4. 审批流程 UI 测试用例设计

### 4.1 测试矩阵

基于 SF-FEAT0044 的 8 个模块，设计以下 UI 测试场景：

| # | 场景 | 模块 | 优先级 | 预估步骤 |
|---|------|------|--------|---------|
| 1 | Stepper 展示多级审批进度 | A | P0 | 8 |
| 2 | Stepper 脉冲动画（等待阶段） | A | P2 | 5 |
| 3 | Stepper Tooltip 显示审批详情 | A | P1 | 6 |
| 4 | Stepper 策略匹配信息栏 | A | P1 | 5 |
| 5 | 重提按钮仅 REJECTED+提交人可见 | B | P0 | 7 |
| 6 | 重提编辑态切换动画 | B | P2 | 4 |
| 7 | Diff 对比视图（新增/删除行） | B | P0 | 10 |
| 8 | SQL 未修改时警告提示 | B | P0 | 6 |
| 9 | 重提确认弹窗展示变更摘要 | B | P1 | 8 |
| 10 | 重提成功后刷新流程 | B | P0 | 9 |
| 11 | 条件构建器添加/删除条件 | C | P0 | 8 |
| 12 | 条件构建器 AND/OR 切换 | C | P0 | 5 |
| 13 | 条件构建器条件组嵌套 | C | P1 | 7 |
| 14 | 条件构建器 7 种字段类型 | C | P1 | 10 |
| 15 | 策略列表展示 + 拖拽排序 | D | P0 | 10 |
| 16 | 策略 Sheet 编辑器创建 | D | P0 | 12 |
| 17 | 策略编辑器审批链配置 | D | P1 | 8 |
| 18 | 策略启用/禁用切换 | D | P0 | 6 |
| 19 | 策略删除确认（>5 条需输入名称） | D | P1 | 8 |
| 20 | 策略空态展示 | D | P2 | 4 |
| 21 | 审批历史时间线 Revision 分组 | E | P0 | 8 |
| 22 | 审批历史颜色编码 | E | P1 | 5 |
| 23 | 审批历史 Markdown 评论渲染 | E | P1 | 6 |
| 24 | 审批历史长评论截断+展开 | E | P2 | 5 |
| 25 | 审批历史空态 | E | P2 | 3 |
| 26 | 工单列表 StageBadge 展示 | F | P1 | 4 |
| 27 | 工单详情集成 Stepper | A+B | P0 | 10 |
| 28 | 工单详情集成 ResubmitForm | B | P0 | 12 |
| 29 | 工单详情集成 Timeline | E | P0 | 8 |
| 30 | 10s 轮询实时更新 | F | P1 | 8 |
| 31 | Loading Skeleton 展示 | F | P2 | 3 |
| 32 | Error 状态 + 重试 | F | P1 | 5 |
| 33 | 并发冲突 409 AlertDialog | F | P1 | 7 |
| 34 | 权限不足 Error 展示 | F | P0 | 5 |
| 35 | 未保存离开确认 | D | P1 | 6 |

### 4.2 关键场景测试脚本骨架

#### 场景 7: Diff 对比视图

```typescript
test('重提 Diff 对比视图 — 显示新增/删除行', async ({ ticketPage }) => {
  // 1. 创建并驳回工单
  const ticketId = await ticketPage.createAndRejectTicket();
  
  // 2. 打开工单详情
  await ticketPage.openDetail(ticketId);
  
  // 3. 点击修改重提
  await ticketPage.clickResubmit();
  
  // 4. 修改 SQL
  const originalSql = await ticketPage.getSqlContent();
  await ticketPage.editSql(originalSql.replace('SELECT', 'DELETE'));
  
  // 5. 展开变更对比
  await ticketPage.toggleDiffView();
  
  // 6. 验证 Diff 显示
  await expect(ticketPage.diffAddedLine).toBeVisible();
  await expect(ticketPage.diffRemovedLine).toBeVisible();
  await expect(ticketPage.diffStats).toContainText('+1');
  await expect(ticketPage.diffStats).toContainText('-1');
});
```

#### 场景 15: 策略拖拽排序

```typescript
test('策略拖拽排序 — 优先级变更持久化', async ({ approvalPoliciesPage }) => {
  // 1. 创建 3 个策略
  const ids = await approvalPoliciesPage.createTestPolicies(3);
  
  // 2. 拖拽第一个到最后
  await approvalPoliciesPage.dragReorder(0, 2);
  
  // 3. 验证顺序
  const order = await approvalPoliciesPage.getPolicyOrder();
  expect(order).toEqual([ids[1], ids[2], ids[0]]);
  
  // 4. 刷新页面验证持久化
  await approvalPoliciesPage.reload();
  const orderAfter = await approvalPoliciesPage.getPolicyOrder();
  expect(orderAfter).toEqual([ids[1], ids[2], ids[0]]);
});
```

---

## 5. 组件测试方案

### 5.1 需要覆盖的核心组件

| 组件 | 测试文件 | 用例数 | 关键断言 |
|------|---------|--------|---------|
| `ApprovalStepper` | `ApprovalStepper.test.tsx` | 8 | 阶段渲染、颜色编码、脉冲动画、Tooltip |
| `ResubmitForm` | `ResubmitForm.test.tsx` | 12 | 编辑态切换、Diff 计算、表单校验、确认弹窗 |
| `ConditionBuilder` | `ConditionBuilder.test.tsx` | 15 | 条件增删、AND/OR 切换、嵌套、字段类型、空态 |
| `ApprovalTimeline` | `ApprovalTimeline.test.tsx` | 10 | Revision 分组、时间线渲染、折叠/展开、空态 |
| `StageBadge` | `StageBadge.test.tsx` | 4 | 自动通过/阶段展示 |
| `useApprovalFlow` | `useApprovalFlow.test.ts` | 6 | 获取/轮询/审批/驳回/重提 |
| `approvalStore` | `approvalStore.test.ts` | 8 | 状态管理、错误处理 |
| `policyStore` | `policyStore.test.ts` | 10 | CRUD、乐观更新、冲突处理 |

### 5.2 测试策略

```typescript
// 示例：ConditionBuilder 组件测试
describe('ConditionBuilder', () => {
  it('应渲染空态提示', () => {
    render(<ConditionBuilder value={EMPTY_GROUP} onChange={fn} />);
    expect(screen.getByText(/尚未添加条件/)).toBeInTheDocument();
  });

  it('应支持添加条件', async () => {
    const onChange = vi.fn();
    render(<ConditionBuilder value={EMPTY_GROUP} onChange={onChange} />);
    
    await userEvent.click(screen.getByText('添加条件'));
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ conditions: [expect.any(Object)] })
    );
  });

  it('应支持 AND/OR 切换', async () => {
    const twoConditions = { logic: 'AND', conditions: [cond1, cond2] };
    render(<ConditionBuilder value={twoConditions} onChange={vi.fn()} />);
    
    const toggle = screen.getByText('AND');
    await userEvent.click(toggle);
    // 验证 toggle 变为 OR
  });

  it('应支持 2 层嵌套', async () => {
    render(<ConditionBuilder value={EMPTY_GROUP} onChange={vi.fn()} />);
    await userEvent.click(screen.getByText('添加条件组'));
    // 验证嵌套 UI 出现
  });
});
```

---

## 6. CI 集成方案

### 6.1 分层执行策略

```yaml
# GitHub Actions 工作流
jobs:
  # L1: 每次提交 — 单元测试 + Lint
  unit-test:
    runs-on: ubuntu-latest
    steps:
      - run: cd web && npm ci && npm run test -- --coverage
      - run: cd web && npm run lint
    # 覆盖率门槛
    coverage-threshold:
      statements: 70%
      branches: 65%
      functions: 70%

  # L2: PR 合并前 — 组件测试 + Smoke E2E
  pr-check:
    runs-on: ubuntu-latest
    needs: unit-test
    steps:
      - run: cd web && npm ci && npm run test
      - run: cd e2e && npm ci && npx playwright install chromium
      - run: cd e2e && npm run setup
      - run: cd e2e && npx playwright test tests/smoke/
      - run: cd e2e && npm run teardown

  # L3: Nightly / staging — 全量 E2E
  nightly-full-e2e:
    runs-on: ubuntu-latest
    trigger: cron(0 2 * * *)
    steps:
      - run: cd e2e && npm run setup
      - run: cd e2e && npx playwright test
      - run: cd e2e && npm run teardown
      # 上传测试报告
      - uses: actions/upload-artifact@v4
        with:
          name: playwright-report
          path: e2e/playwright-report/
```

### 6.2 Smoke 测试清单（~15 场景，≤5min）

| # | 场景 | 页面 |
|---|------|------|
| 1 | 登录 → 首页加载 | Login |
| 2 | 侧边栏导航各页面 | Layout |
| 3 | SQL 查询基本执行 | Query |
| 4 | 工单列表加载 | Ticket |
| 5 | 工单详情抽屉打开 | Ticket |
| 6 | 审批操作（通过/拒绝） | Ticket |
| 7 | 审计页面加载+搜索 | Audit |
| 8 | 用户管理列表加载 | Users |
| 9 | 设置页面加载 | Settings |
| 10 | 主题切换 | Layout |
| 11 | 退出登录 | Layout |
| 12 | 审批策略页面加载 | ApprovalPolicies |
| 13 | 创建策略 | ApprovalPolicies |
| 14 | 全局搜索（⌘K） | Layout |
| 15 | 权限页面加载 | Permissions |

---

## 7. 实施计划

### Phase 1: 基础架构（Week 1）

| 任务 | 预估 | 产出 |
|------|------|------|
| BasePage + POM 基类 | 4h | `e2e/pages/BasePage.ts` |
| Fixture 扩展 | 2h | POM 注入 + authenticatedPage |
| Smoke 测试套件（15 场景） | 8h | `e2e/tests/smoke/` |
| CI 工作流配置 | 2h | GitHub Actions YAML |

### Phase 2: 审批流程测试（Week 2）

| 任务 | 预估 | 产出 |
|------|------|------|
| ApprovalPolicies POM | 4h | `e2e/pages/ApprovalPoliciesPage.ts` |
| TicketDetailDrawer POM | 4h | `e2e/pages/TicketDetailDrawer.ts` |
| 审批 Stepper/Timeline 测试 | 6h | 5 个 spec 文件 |
| 重提流程测试 | 6h | 3 个 spec 文件 |
| 策略 CRUD 测试 | 4h | 3 个 spec 文件 |

### Phase 3: 组件测试（Week 3）

| 任务 | 预估 | 产出 |
|------|------|------|
| ConditionBuilder 组件测试 | 4h | 15 用例 |
| ApprovalStepper 组件测试 | 3h | 8 用例 |
| ResubmitForm 组件测试 | 4h | 12 用例 |
| ApprovalTimeline 组件测试 | 3h | 10 用例 |
| Store + Hook 单元测试 | 4h | 24 用例 |

### Phase 4: CI 集成 + 报告（Week 3）

| 任务 | 预估 | 产出 |
|------|------|------|
| 覆盖率门槛配置 | 2h | vitest.config.ts 更新 |
| Nightly E2E 工作流 | 2h | GitHub Actions |
| 测试报告 + Badge | 2h | README 集成 |

**总预估工时**：~64h（8 人天）

---

## 8. 风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| Playwright 环境搭建复杂 | 延迟 1-2 天 | 复用现有 e2e/ Docker 环境 |
| 审批流程依赖后端 API | E2E 可能因后端不稳定失败 | API mock + 重试策略 |
| 拖拽排序测试不稳定 | Flaky test | 使用 Playwright dragTo + 重试 |
| 轮询测试超时 | 10s 轮询难以在测试中等待 | Mock timer + 快速触发 |
| 覆盖率计算偏差 | 虚高覆盖率 | 排除 types/api/ 声明文件 |

---

## 9. 验收标准

1. ✅ POM 基类 + 至少 5 个页面对象
2. ✅ Smoke 测试套件 15 场景，运行时间 ≤5min
3. ✅ 审批流程 E2E 测试覆盖 A-E 模块（≥30 场景）
4. ✅ 组件测试覆盖关键审批组件（≥50 用例）
5. ✅ 前端关键路径自动化覆盖率 ≥70%
6. ✅ CI 集成：PR smoke + nightly full
7. ✅ 测试报告可查看（HTML report）

---

## 10. 技术选型确认

| 工具 | 版本 | 用途 |
|------|------|------|
| Playwright | ^1.59.1 | E2E 测试 |
| Vitest | ^4.1.7 | 单元/组件测试 |
| React Testing Library | ^16.3.2 | 组件交互测试 |
| @vitest/coverage-v8 | ^4.1.7 | 覆盖率 |
| @testing-library/user-event | ^14.6.1 | 用户交互模拟 |

所有工具已在 `package.json` 中，无需额外安装。
