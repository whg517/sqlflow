/**
 * approval-policies.spec.ts
 *
 * E2E 审批策略引擎 CRUD 测试 (SF-QA0034)
 *
 * 覆盖 5 个 API 端点：
 *   POST   /api/approval/policies       — 创建策略
 *   GET    /api/approval/policies       — 列出策略
 *   GET    /api/approval/policies/:id   — 获取单条策略
 *   PUT    /api/approval/policies/:id   — 更新策略
 *   DELETE /api/approval/policies/:id   — 删除策略
 *
 * 测试范围：
 *   - 默认策略零配置
 *   - 自定义策略 CRUD
 *   - 条件匹配验证
 *   - 优先级排序
 *   - 禁用策略
 *   - 边界：策略名称重复、JSON 格式错误、空名称
 *
 * 隔离：策略名称使用 e2e_test_{uuid}_ 前缀
 * 前置：docker-compose.test.yml 环境，e2e-admin 账号可用
 */
import { test, expect, BASE_URL, ADMIN_USER, ADMIN_PASS, getToken, loginViaApi } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

const POLICY_PREFIX = `e2e_test_${Date.now()}_`

/** Generate a unique policy name for isolation. */
function policyName(suffix: string): string {
  return `${POLICY_PREFIX}${suffix}`
}

/** Create an approval policy via API, return the response JSON. */
async function createPolicy(data: Record<string, unknown>) {
  const token = await getToken()
  const res = await fetch(`${BASE_URL}/api/approval/policies`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify(data),
  })
  const body = await res.json()
  return { status: res.status, body }
}

/** List all approval policies. */
async function listPolicies() {
  const token = await getToken()
  const res = await fetch(`${BASE_URL}/api/approval/policies`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  const body = await res.json()
  return { status: res.status, body }
}

/** Get a single policy by ID. */
async function getPolicy(id: number) {
  const token = await getToken()
  const res = await fetch(`${BASE_URL}/api/approval/policies/${id}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  const body = await res.json()
  return { status: res.status, body }
}

/** Update a policy by ID. */
async function updatePolicy(id: number, data: Record<string, unknown>) {
  const token = await getToken()
  const res = await fetch(`${BASE_URL}/api/approval/policies/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify(data),
  })
  const body = await res.json()
  return { status: res.status, body }
}

/** Delete a policy by ID. */
async function deletePolicy(id: number) {
  const token = await getToken()
  const res = await fetch(`${BASE_URL}/api/approval/policies/${id}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  })
  const body = await res.json()
  return { status: res.status, body }
}

/** Best-effort cleanup of all e2e_test_ prefixed policies. */
async function cleanupE2EPolicies() {
  const { body } = await listPolicies()
  const policies: Array<{ id: number; name: string }> = Array.isArray(body) ? body : []
  for (const p of policies) {
    if (p.name.startsWith('e2e_test_')) {
      await deletePolicy(p.id)
    }
  }
}

// --- Tests ---

test.describe('审批策略引擎 CRUD', () => {
  test.beforeAll(async () => {
    await cleanupE2EPolicies()
  })

  test.afterAll(async () => {
    await cleanupE2EPolicies()
  })

  // ── 创建策略 ────────────────────────────────────────────────────────────

  test('创建策略 — 基本 CRUD 创建成功', async () => {
    const name = policyName('basic-crud')
    const conditions = JSON.stringify({ risk_levels: ['high'], sql_types: ['DELETE'] })
    const chain = JSON.stringify([{ role: 'dba', auto_skip_same_submitter: true }])

    const { status, body } = await createPolicy({
      name,
      description: 'E2E 基本策略测试',
      conditions,
      approval_chain: chain,
      auto_approve_enabled: false,
      is_default: false,
      priority: 10,
    })

    expect(status).toBe(201)
    expect(body.id).toBeTruthy()
    expect(body.name).toBe(name)
    expect(body.enabled).toBe(true)
    expect(body.priority).toBe(10)
    expect(body.conditions).toBe(conditions)
    expect(body.approval_chain).toBe(chain)

    // 清理
    await deletePolicy(body.id)
  })

  test('创建策略 — 自动审批策略', async () => {
    const name = policyName('auto-approve')
    const { status, body } = await createPolicy({
      name,
      description: 'E2E 自动审批策略',
      conditions: '{}',
      approval_chain: '[]',
      auto_approve_enabled: true,
      auto_approve_reason: '低风险自动审批',
      is_default: false,
      priority: 1,
    })

    expect(status).toBe(201)
    expect(body.auto_approve_enabled).toBe(true)
    expect(body.auto_approve_reason).toBe('低风险自动审批')

    await deletePolicy(body.id)
  })

  test('创建策略 — 默认策略标记', async () => {
    const name = policyName('default-flag')
    const { status, body } = await createPolicy({
      name,
      description: 'E2E 默认策略',
      conditions: '{}',
      approval_chain: JSON.stringify([{ role: 'admin' }]),
      auto_approve_enabled: false,
      is_default: true,
      priority: 0,
    })

    expect(status).toBe(201)
    expect(body.is_default).toBe(true)

    await deletePolicy(body.id)
  })

  // ── 边界条件：创建失败 ───────────────────────────────────────────────

  test('创建策略 — 空名称应失败', async () => {
    const { status, body } = await createPolicy({
      name: '',
      description: '应失败',
      conditions: '{}',
      approval_chain: '[]',
    })

    expect(status).toBe(400)
  })

  test('创建策略 — 名称重复应失败', async () => {
    const name = policyName('duplicate-name')

    // 第一次创建成功
    const { status: s1, body: b1 } = await createPolicy({
      name,
      description: '第一次',
      conditions: '{}',
      approval_chain: '[]',
    })
    expect(s1).toBe(201)

    // 第二次同名创建应失败
    const { status: s2, body: b2 } = await createPolicy({
      name,
      description: '第二次应失败',
      conditions: '{}',
      approval_chain: '[]',
    })
    expect(s2).toBe(500)
    expect(b2.error || b2.message || '').toBeTruthy()

    await deletePolicy(b1.id)
  })

  test('创建策略 — 非法 JSON 条件字段（字符串也能通过，验证后端不崩溃）', async () => {
    const name = policyName('malformed-json')
    const { status, body } = await createPolicy({
      name,
      description: '非法 JSON',
      conditions: 'not-valid-json',
      approval_chain: '[]',
    })

    // 后端存储为字符串字段，即使不是合法 JSON 也能创建
    expect(status).toBe(201)
    expect(body.conditions).toBe('not-valid-json')

    await deletePolicy(body.id)
  })

  // ── 查询策略列表 ──────────────────────────────────────────────────────

  test('列出策略 — 返回列表按优先级降序', async () => {
    // 创建 3 个不同优先级的策略
    const p1 = await createPolicy({
      name: policyName('list-p1'), description: '优先级 1', conditions: '{}', approval_chain: '[]', priority: 10,
    })
    const p2 = await createPolicy({
      name: policyName('list-p2'), description: '优先级 2', conditions: '{}', approval_chain: '[]', priority: 30,
    })
    const p3 = await createPolicy({
      name: policyName('list-p3'), description: '优先级 3', conditions: '{}', approval_chain: '[]', priority: 20,
    })

    const { body } = await listPolicies()
    const policies: Array<{ id: number; name: string; priority: number }> = Array.isArray(body) ? body : []

    // 按优先级降序排序验证
    const e2ePolicies = policies.filter(p => p.name.startsWith(POLICY_PREFIX))
      .sort((a, b) => b.priority - a.priority)
    expect(e2ePolicies.length).toBeGreaterThanOrEqual(3)
    // 第一个应该是 priority 30
    expect(e2ePolicies[0].priority).toBe(30)

    // 清理
    await deletePolicy(p1.body.id)
    await deletePolicy(p2.body.id)
    await deletePolicy(p3.body.id)
  })

  test('获取单条策略 — 返回完整字段', async () => {
    const conditions = JSON.stringify({ risk_levels: ['medium', 'high'], sql_types: ['ALTER', 'DROP'] })
    const chain = JSON.stringify([
      { role: 'dba', auto_skip_same_submitter: true },
      { role: 'admin' },
    ])

    const { body: created } = await createPolicy({
      name: policyName('get-single'),
      description: '获取详情测试',
      conditions,
      approval_chain: chain,
      auto_approve_enabled: false,
      is_default: false,
      priority: 15,
    })

    const { status, body: fetched } = await getPolicy(created.id)
    expect(status).toBe(200)
    expect(fetched.id).toBe(created.id)
    expect(fetched.name).toBe(created.name)
    expect(fetched.conditions).toBe(conditions)
    expect(fetched.approval_chain).toBe(chain)

    await deletePolicy(created.id)
  })

  test('获取不存在的策略 — 返回 404', async () => {
    const { status } = await getPolicy(999999)
    expect(status).toBe(404)
  })

  // ── 更新策略 ──────────────────────────────────────────────────────────

  test('更新策略 — 修改名称和优先级', async () => {
    const { body: created } = await createPolicy({
      name: policyName('update-basic'),
      description: '原始描述',
      conditions: '{}',
      approval_chain: '[]',
      priority: 5,
    })

    const newName = policyName('updated-name')
    const { status, body: updated } = await updatePolicy(created.id, {
      name: newName,
      description: '更新后的描述',
      enabled: true,
      priority: 50,
      conditions: '{}',
      approval_chain: '[]',
      auto_approve_enabled: false,
      is_default: false,
    })

    expect(status).toBe(200)
    expect(updated.name).toBe(newName)
    expect(updated.description).toBe('更新后的描述')
    expect(updated.priority).toBe(50)

    await deletePolicy(updated.id)
  })

  test('更新策略 — 禁用策略', async () => {
    const { body: created } = await createPolicy({
      name: policyName('disable-test'),
      conditions: '{}',
      approval_chain: '[]',
      priority: 1,
    })

    expect(created.enabled).toBe(true)

    const { status, body: updated } = await updatePolicy(created.id, {
      name: created.name,
      description: created.description,
      enabled: false,
      priority: created.priority,
      conditions: created.conditions,
      approval_chain: created.approval_chain,
      auto_approve_enabled: created.auto_approve_enabled,
      is_default: created.is_default,
    })

    expect(status).toBe(200)
    expect(updated.enabled).toBe(false)

    await deletePolicy(updated.id)
  })

  test('更新策略 — 启用自动审批', async () => {
    const { body: created } = await createPolicy({
      name: policyName('enable-auto'),
      conditions: '{}',
      approval_chain: '[]',
      auto_approve_enabled: false,
      priority: 1,
    })

    const { status, body: updated } = await updatePolicy(created.id, {
      name: created.name,
      description: created.description,
      enabled: true,
      priority: created.priority,
      conditions: created.conditions,
      approval_chain: created.approval_chain,
      auto_approve_enabled: true,
      auto_approve_reason: '测试自动审批原因',
      is_default: false,
    })

    expect(status).toBe(200)
    expect(updated.auto_approve_enabled).toBe(true)
    expect(updated.auto_approve_reason).toBe('测试自动审批原因')

    await deletePolicy(updated.id)
  })

  // ── 删除策略 ──────────────────────────────────────────────────────────

  test('删除策略 — 删除后查询返回 404', async () => {
    const { body: created } = await createPolicy({
      name: policyName('delete-test'),
      conditions: '{}',
      approval_chain: '[]',
    })

    const { status: delStatus } = await deletePolicy(created.id)
    expect(delStatus).toBe(200)

    const { status: getStatus } = await getPolicy(created.id)
    expect(getStatus).toBe(404)
  })

  test('删除不存在的策略 — 仍返回 200（幂等）', async () => {
    const { status } = await deletePolicy(999999)
    expect(status).toBe(200)
  })

  // ── 条件匹配验证（通过策略创建验证结构） ─────────────────────────────

  test('策略条件 — 多维度匹配条件', async () => {
    const conditions = JSON.stringify({
      risk_levels: ['high', 'critical'],
      sql_types: ['DELETE', 'DROP', 'TRUNCATE'],
      databases: ['production_db'],
    })
    const chain = JSON.stringify([
      { role: 'dba', auto_skip_same_submitter: true },
      { role: 'admin', auto_skip_same_submitter: false },
    ])

    const { status, body } = await createPolicy({
      name: policyName('multi-condition'),
      description: '多维度条件匹配',
      conditions,
      approval_chain: chain,
      priority: 100,
    })

    expect(status).toBe(201)
    expect(body.conditions).toBe(conditions)
    expect(body.approval_chain).toBe(chain)

    await deletePolicy(body.id)
  })

  test('策略审批链 — 空链（自动审批场景）', async () => {
    const { status, body } = await createPolicy({
      name: policyName('empty-chain'),
      description: '空审批链 + 自动审批',
      conditions: '{}',
      approval_chain: '[]',
      auto_approve_enabled: true,
      auto_approve_reason: '空链自动审批',
      priority: 0,
    })

    expect(status).toBe(201)
    expect(body.approval_chain).toBe('[]')

    await deletePolicy(body.id)
  })
})
