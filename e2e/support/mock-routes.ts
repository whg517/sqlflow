/**
 * support/mock-routes.ts — Playwright route mocks for mock E2E tests
 *
 * Intercepts all /api/* calls and returns mock responses.
 * Migrated from web/e2e/helpers.ts.
 */
import { type Page, type Route } from '@playwright/test'

// --- Mock Data ---

export const MOCK_USERS = {
  admin: { id: 1, username: 'admin', role: 'admin' },
  developer: { id: 2, username: 'developer', role: 'developer' },
  dba: { id: 3, username: 'dba', role: 'dba' },
}

export const MOCK_DATASOURCES = [
  { id: 1, name: 'test-mysql', type: 'mysql', status: 'active', host: '127.0.0.1', port: 3306, username: 'root', database: 'testdb', max_open: 10, created_at: '2025-01-01T00:00:00Z' },
  { id: 2, name: 'test-mongo', type: 'mongodb', status: 'active', host: '127.0.0.1', port: 27017, username: 'mongo', database: 'mongodb', max_open: 10, created_at: '2025-01-15T00:00:00Z' },
]

export const MOCK_TOKEN = 'mock-jwt-e2e-testing-token'

export const MOCK_QUERY_RESULT = {
  code: 0,
  message: 'ok',
  data: {
    columns: ['id', 'name', 'email'],
    rows: [
      { id: 1, name: 'Alice', email: 'ali***@example.com' },
      { id: 2, name: 'Bob', email: 'b**@example.com' },
    ],
    total: 2,
    execution_time_ms: 15,
    affected_rows: 0,
    desensitized: true,
    desensitized_fields: ['email'],
    warnings: [],
  },
}

export const MOCK_TICKET = {
  id: 1,
  submitter_id: 1,
  submitter_name: 'admin',
  datasource_id: 1,
  database: 'testdb',
  sql_content: 'ALTER TABLE users ADD COLUMN phone VARCHAR(20)',
  sql_summary: 'ALTER TABLE users ADD ...',
  db_type: 'mysql',
  change_reason: 'Add phone column for user profiles',
  status: 'PENDING_APPROVAL',
  risk_level: 'high',
  ai_review_result: '{"risk_level":"high","decision":"ticket"}',
  reviewer_id: 0,
  reviewer_name: '',
  review_comment: '',
  executed_at: null,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
}

export const MOCK_TICKET_LIST = {
  code: 0,
  message: 'ok',
  data: [MOCK_TICKET],
  page: 1,
  page_size: 50,
  total: 1,
}

export const MOCK_TICKET_DETAIL = {
  code: 0,
  message: 'ok',
  data: MOCK_TICKET,
}

export const MOCK_CREATED_TICKET = {
  code: 0,
  message: 'ok',
  data: {
    ...MOCK_TICKET,
    id: 2,
    status: 'SUBMITTED',
    sql_content: 'SELECT 1',
    sql_summary: 'SELECT 1',
    change_reason: 'Test ticket for E2E testing flow',
  },
}

export const MOCK_PERFORMANCE_STATS = {
  code: 0,
  message: 'ok',
  data: {
    total_queries: 1500,
    slow_queries: 23,
    avg_time: 85,
    slow_query_rate: 1.5,
    daily_trend: [
      { date: '2025-05-19', count: 200, avg_time: 78, slow_count: 2 },
      { date: '2025-05-20', count: 215, avg_time: 92, slow_count: 5 },
      { date: '2025-05-21', count: 180, avg_time: 65, slow_count: 1 },
      { date: '2025-05-22', count: 250, avg_time: 110, slow_count: 8 },
      { date: '2025-05-23', count: 190, avg_time: 72, slow_count: 3 },
      { date: '2025-05-24', count: 220, avg_time: 88, slow_count: 2 },
      { date: '2025-05-25', count: 245, avg_time: 95, slow_count: 2 },
    ],
    datasource_stats: [
      { datasource_id: 1, datasource_name: 'test-mysql', count: 1200, avg_time: 90 },
      { datasource_id: 2, datasource_name: 'test-mongo', count: 300, avg_time: 60 },
    ],
    top_slow_queries: [
      { id: 1, sql_summary: 'SELECT * FROM orders WHERE ...', execution_time: 5200, datasource_name: 'test-mysql', created_at: '2025-05-22T10:30:00Z' },
      { id: 2, sql_summary: 'UPDATE users SET ...', execution_time: 3800, datasource_name: 'test-mysql', created_at: '2025-05-22T14:15:00Z' },
      { id: 3, sql_summary: 'SELECT o.*, u.name FROM orders ...', execution_time: 2500, datasource_name: 'test-mysql', created_at: '2025-05-22T09:20:00Z' },
    ],
  },
}

export const MOCK_SLOW_QUERIES = {
  code: 0,
  message: 'ok',
  data: [
    { id: 1, user_id: 1, datasource_id: 1, database: 'testdb', sql_content: 'SELECT * FROM orders JOIN users ON ...', sql_summary: 'SELECT * FROM orders JOIN ...', db_type: 'mysql', execution_time: 5200, result_rows: 50000, affected_rows: 0, created_at: '2025-05-22T10:30:00Z' },
    { id: 2, user_id: 1, datasource_id: 1, database: 'testdb', sql_content: 'UPDATE users SET status = ... WHERE id IN (...)', sql_summary: 'UPDATE users SET status ...', db_type: 'mysql', execution_time: 3800, result_rows: 0, affected_rows: 1500, created_at: '2025-05-22T14:15:00Z' },
    { id: 3, user_id: 2, datasource_id: 1, database: 'testdb', sql_content: 'SELECT o.*, u.name FROM orders o LEFT JOIN ...', sql_summary: 'SELECT o.*, u.name FROM ...', db_type: 'mysql', execution_time: 2500, result_rows: 80000, affected_rows: 0, created_at: '2025-05-22T09:20:00Z' },
    { id: 4, user_id: 1, datasource_id: 2, database: 'mongodb', sql_content: 'db.orders.find({...}).sort(...)', sql_summary: 'db.orders.find({...}).sort(...)', db_type: 'mongodb', execution_time: 1800, result_rows: 25000, affected_rows: 0, created_at: '2025-05-23T11:00:00Z' },
    { id: 5, user_id: 3, datasource_id: 1, database: 'testdb', sql_content: 'DELETE FROM temp_logs WHERE created_at < ...', sql_summary: 'DELETE FROM temp_logs WHERE ...', db_type: 'mysql', execution_time: 1500, result_rows: 0, affected_rows: 100000, created_at: '2025-05-23T08:00:00Z' },
  ],
  page: 1,
  page_size: 20,
  total: 5,
}

export const MOCK_MASK_RULES = [
  { id: 1, datasource_id: 1, table_name: 'users', column_name: 'email', mask_type: 'partial', mask_length: 3, sensitivity: 'high', description: '邮箱脱敏' },
  { id: 2, datasource_id: 1, table_name: 'users', column_name: 'phone', mask_type: 'partial', mask_length: 3, sensitivity: 'high', description: '手机号脱敏' },
  { id: 3, datasource_id: 1, table_name: 'users', column_name: 'id_card', mask_type: 'full', mask_length: 0, sensitivity: 'critical', description: '身份证全脱敏' },
]

export const MOCK_SENSITIVE_TABLES = [
  { id: 1, datasource_id: 1, table_name: 'users', column_count: 3, created_at: '2025-05-20T00:00:00Z' },
  { id: 2, datasource_id: 1, table_name: 'payments', column_count: 5, created_at: '2025-05-21T00:00:00Z' },
]

export const MOCK_MASK_TABLES = ['users', 'payments', 'orders', 'products', 'temp_logs']

export const MOCK_MASK_COLUMNS = [
  { column_name: 'email', column_type: 'varchar', is_sensitive: true },
  { column_name: 'phone', column_type: 'varchar', is_sensitive: true },
  { column_name: 'id_card', column_type: 'varchar', is_sensitive: true },
  { column_name: 'name', column_type: 'varchar', is_sensitive: false },
  { column_name: 'id', column_type: 'int', is_sensitive: false },
]

export const MOCK_HISTORY = {
  code: 0,
  message: 'ok',
  data: [
    {
      id: 1,
      user_id: 1,
      datasource_id: 1,
      database: 'testdb',
      sql_content: 'SELECT * FROM users LIMIT 10',
      sql_summary: 'SELECT * FROM users ...',
      db_type: 'mysql',
      execution_time: 15,
      result_rows: 10,
      affected_rows: 0,
      created_at: new Date().toISOString(),
    },
  ],
  page: 1,
  page_size: 50,
  total: 1,
}

// --- Route Handlers ---

type MockOptions = {
  role?: 'admin' | 'developer' | 'dba'
  denyDatasources?: boolean
}

function defaultRole(opts?: MockOptions) {
  return opts?.role ?? 'admin'
}

/**
 * Mock all API routes for a page.
 * Call this in beforeEach or via the authenticatedPage fixture.
 */
export function mockApiRoutes(page: Page, opts?: MockOptions) {
  const role = defaultRole(opts)
  const user = MOCK_USERS[role]

  // Auth
  page.route('**/api/auth/login', async (route: Route) => {
    const postData = route.request().postDataJSON()
    if (postData?.username && postData?.password) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: { token: MOCK_TOKEN } }),
      })
    } else {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 1, message: '用户名或密码错误' }),
      })
    }
  })

  page.route('**/api/auth/me', async (route: Route) => {
    const authHeader = await route.request().headerValue('authorization')
    if (authHeader === `Bearer ${MOCK_TOKEN}`) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: user }),
      })
    } else {
      await route.fulfill({ status: 401, contentType: 'application/json', body: '{}' })
    }
  })

  page.route('**/api/auth/password', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, message: 'ok' }),
    })
  })

  // Datasources (GET only — CRUD handled below)
  page.route(/\/api\/datasources(\?.*)?$/, async (route: Route) => {
    if (route.request().method() !== 'GET') return // let CRUD handler below handle
    if (opts?.denyDatasources) {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    } else {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: MOCK_DATASOURCES }),
      })
    }
  })

  // Datasource tables (simple list — column detail handled below)
  page.route('**/api/datasources/*/tables', async (route: Route) => {
    const url = route.request().url()
    // If URL has more path segments (e.g. .../tables/users/columns), skip to column handler
    if (/\/tables\/[^/]+\/columns/.test(url)) return
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: MOCK_MASK_TABLES }),
    })
  })

  // Query
  page.route('**/api/query/execute', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(MOCK_QUERY_RESULT),
    })
  })

  page.route('**/api/query/review', async (route: Route) => {
    const body =
      'event: content\ndata: "Analyzing SQL..."\n\n' +
      'event: result\ndata: ' +
      JSON.stringify({
        risk_level: 'low',
        risk_score: 10,
        decision: 'execute',
        summary: 'Low risk SELECT query',
        suggestions: [],
        impact_analysis: 'Read-only query, no data modification',
        rollback_sql: '',
        warnings: [],
        review_source: 'ai',
        reviewed_at: new Date().toISOString(),
        expires_at: new Date(Date.now() + 30000).toISOString(),
        model_used: 'gpt-4',
      }) +
      '\n\n'
    await route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      body,
    })
  })

  page.route('**/api/query/export', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'text/csv',
      headers: { 'Content-Disposition': 'attachment; filename=export.csv' },
      body: 'id,name,email\n1,Alice,test@example.com\n',
    })
  })

  page.route(/\/api\/query\/history(\?.*)?$/, async (route: Route) => {
    if (route.request().method() === 'DELETE') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok' }),
      })
    } else {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_HISTORY),
      })
    }
  })

  // Tickets
  page.route(/\/api\/tickets(\?.*)?$/, async (route: Route) => {
    if (route.request().method() === 'POST') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_CREATED_TICKET),
      })
    } else {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_TICKET_LIST),
      })
    }
  })

  page.route('**/api/tickets/*', async (route: Route) => {
    const url = route.request().url()
    if (url.includes('/approve')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: {
            ...MOCK_TICKET,
            status: 'APPROVED',
            reviewer_id: user.id,
            reviewer_name: user.username,
          },
        }),
      })
    } else if (url.includes('/reject')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: {
            ...MOCK_TICKET,
            status: 'REJECTED',
            reviewer_id: user.id,
            reviewer_name: user.username,
          },
        }),
      })
    } else if (url.includes('/cancel')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: { ...MOCK_TICKET, status: 'CANCELLED' },
        }),
      })
    } else if (url.includes('/execute')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: { ...MOCK_TICKET, status: 'DONE', executed_at: new Date().toISOString() },
        }),
      })
    } else {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_TICKET_DETAIL),
      })
    }
  })

  // Users list (admin-only) — handled by CRUD block below

  page.route('**/api/roles**', async (route: Route) => {
    if (role === 'admin') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: [
            { role: 'admin', description: '管理员' },
            { role: 'dba', description: 'DBA' },
            { role: 'developer', description: '开发人员' },
          ],
        }),
      })
    } else {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    }
  })

  page.route('**/api/policies**', async (route: Route) => {
    if (role === 'admin') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: [
            {
              id: 1,
              role: 'developer',
              datasource: 'test-mysql',
              table: 'users',
              action: 'select',
            },
          ],
        }),
      })
    } else {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    }
  })

  // Mask rules & sensitive tables handled by new handlers below

  // Fallback sensitive-tables glob (non-regex) — no-op since regex handlers above take priority


  // Audit logs (admin-only)
  page.route('**/api/audit-logs**', async (route: Route) => {
    if (role === 'admin' || role === 'dba') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: [],
          page: 1,
          page_size: 50,
          total: 0,
        }),
      })
    } else {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    }
  })

  // Export audit logs (admin/dba)
  page.route(/\/api\/export\/audit/, async (route: Route) => {
    if (role === 'admin' || role === 'dba') {
      const BOM = '\uFEFF'
      const csv = BOM + 'ID,时间,用户,操作,数据源ID,数据库,SQL内容\n'
      + '1,2026-05-25 10:00:00,admin,query_execute,1,testdb,SELECT 1\n'
      + '# 导出水印: 导出人=' + user.username + ' | 导出时间=2026-05-25 10:00:00 UTC | 仅限内部使用\n'
      await route.fulfill({
        status: 200,
        contentType: 'text/csv; charset=utf-8',
        headers: {
          'Content-Disposition': 'attachment; filename="audit_logs_2026-05-25.csv"',
          'X-Export-Rows': '1',
        },
        body: csv,
      })
    } else {
      await route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ code: 403, message: '没有导出权限' }) })
    }
  })

  // Export tickets (all authenticated users)
  page.route(/\/api\/export\/tickets/, async (route: Route) => {
    const BOM = '\uFEFF'
    const csv = BOM + 'ID,提交人,状态,数据库\n'
    + '1,admin,SUBMITTED,testdb\n'
    + '# 导出水印: 导出人=' + user.username + ' | 导出时间=2026-05-25 10:00:00 UTC | 仅限内部使用\n'
    await route.fulfill({
      status: 200,
      contentType: 'text/csv; charset=utf-8',
      headers: {
        'Content-Disposition': 'attachment; filename="tickets_2026-05-25.csv"',
        'X-Export-Rows': '1',
      },
      body: csv,
    })
  })

  // Settings (admin-only)
  page.route('**/api/settings**', async (route: Route) => {
    if (role === 'admin') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: {
            dingtalk_webhook: '',
            dingtalk_secret: '',
            ai_provider: 'openai',
            ai_model: 'gpt-4',
            ai_api_key: '',
            ai_base_url: '',
            ai_timeout: 30,
          },
        }),
      })
    } else {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    }
  })

  // Health
  page.route('**/api/health', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: 'ok' }),
    })
  })

  // Dashboard
  page.route('**/api/dashboard**', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 0,
        data: {
          total_queries: 100,
          total_tickets: 10,
          active_datasources: 2,
          recent_queries: [],
        },
      }),
    })
  })

  // Refresh token
  page.route('**/api/auth/refresh', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: { token: MOCK_TOKEN } }),
    })
  })

  // Performance stats
  page.route(/\/api\/query\/performance\/stats/, async (route: Route) => {
    if (role === 'admin' || role === 'dba') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_PERFORMANCE_STATS),
      })
    } else {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    }
  })

  // Slow queries
  page.route(/\/api\/query\/performance\/slow/, async (route: Route) => {
    if (role === 'admin' || role === 'dba') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_SLOW_QUERIES),
      })
    } else {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    }
  })

  // Mask rules (admin-only)
  page.route(/\/api\/mask-rules(\?.*)?$/, async (route: Route) => {
    if (role !== 'admin') {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
      return
    }
    if (route.request().method() === 'POST') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok', data: { id: 99, ...route.request().postDataJSON() } }),
      })
    } else {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: MOCK_MASK_RULES }),
      })
    }
  })

  page.route('**/api/mask-rules/*', async (route: Route) => {
    if (role !== 'admin') {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
      return
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, message: 'ok' }),
    })
  })

  // Sensitive tables (admin-only)
  page.route(/\/api\/sensitive-tables(\?.*)?$/, async (route: Route) => {
    if (role !== 'admin') {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
      return
    }
    if (route.request().method() === 'POST') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok', data: { id: 99, ...route.request().postDataJSON() } }),
      })
    } else {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: MOCK_SENSITIVE_TABLES, total: MOCK_SENSITIVE_TABLES.length }),
      })
    }
  })

  page.route('**/api/sensitive-tables/*', async (route: Route) => {
    if (role !== 'admin') {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
      return
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, message: 'ok' }),
    })
  })

  // AI config
  page.route('**/api/ai-config/test', async (route: Route) => {
    if (role !== 'admin') {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
      return
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: { success: true, message: 'AI 连接测试成功', latency_ms: 350 } }),
    })
  })

  // Datasource table columns (for mask rules)
  page.route(/\/api\/datasources\/\d+\/tables\/[^/]+\/columns/, async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: MOCK_MASK_COLUMNS }),
    })
  })

  // Datasource test connection
  page.route(/\/api\/datasources\/\d+\/test/, async (route: Route) => {
    if (role !== 'admin') {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
      return
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: { message: '连接测试成功', success: true } }),
    })
  })

  // Individual datasource CRUD (PUT / DELETE)
  page.route(/\/api\/datasources\/\d+$/, async (route: Route) => {
    if (role !== 'admin') {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
      return
    }
    const method = route.request().method()
    if (method === 'PUT') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, message: 'ok' }) })
    } else if (method === 'DELETE') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, message: 'ok' }) })
    } else {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, data: MOCK_DATASOURCES[0] }) })
    }
  })

  // Users CRUD (list + POST)
  page.route(/\/api\/users(\?.*)?$/, async (route: Route) => {
    if (role !== 'admin') {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
      return
    }
    if (route.request().method() === 'POST') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, message: 'ok' }) })
    } else {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: { users: [MOCK_USERS.admin, MOCK_USERS.developer, MOCK_USERS.dba], total: 3 } }),
      })
    }
  })

  // Individual user CRUD (PUT / DELETE)
  page.route(/\/api\/users\/\d+$/, async (route: Route) => {
    if (role !== 'admin') {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
      return
    }
    const method = route.request().method()
    if (method === 'PUT') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, message: 'ok' }) })
    } else if (method === 'DELETE') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, message: 'ok' }) })
    }
  })

  // User reset password
  page.route(/\/api\/users\/\d+\/reset-password/, async (route: Route) => {
    if (role !== 'admin') {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
      return
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, message: 'ok' }) })
  })
}

/**
 * Helper: login via UI with mock routes active.
 */
export async function loginViaUI(
  page: Page,
  username = 'admin',
  password = 'admin123',
) {
  await page.goto('/login')
  await page.getByPlaceholder('用户名').fill(username)
  await page.getByPlaceholder('密码').fill(password)
  await page.getByRole('button', { name: '登 录' }).click()
  await page.waitForURL('**/query**')
}

/**
 * Helper: set token in localStorage directly (skip UI login).
 */
export async function setToken(
  page: Page,
  role: 'admin' | 'developer' | 'dba' = 'admin',
) {
  await page.goto('/login')
  await page.evaluate((token) => {
    localStorage.setItem('token', token)
  }, MOCK_TOKEN)
}
