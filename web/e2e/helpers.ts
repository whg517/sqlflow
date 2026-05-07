import { type Page, type Route, test as base } from '@playwright/test'

// --- Mock Data ---

export const MOCK_USERS = {
  admin: { id: 1, username: 'admin', role: 'admin' },
  developer: { id: 2, username: 'developer', role: 'developer' },
  dba: { id: 3, username: 'dba', role: 'dba' },
}

export const MOCK_DATASOURCES = [
  { id: 1, name: 'test-mysql', type: 'mysql', status: 'active' },
  { id: 2, name: 'test-mongo', type: 'mongodb', status: 'active' },
]

export const MOCK_TOKEN = 'mock-jwt-token-for-e2e-testing'

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

export function mockApiRoutes(page: Page, opts?: MockOptions) {
  const role = defaultRole(opts)
  const user = MOCK_USERS[role]

  // Auth
  page.route('**/api/auth/login', async (route: Route) => {
    const req = route.request()
    const postData = req.postDataJSON()
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

  // Datasources
  page.route(/\/api\/datasources(\?.*)?$/, async (route: Route) => {
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

  page.route('**/api/datasources/*/tables', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: ['users', 'orders', 'products'] }),
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

  // Tickets (match with or without query params)
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
          data: { ...MOCK_TICKET, status: 'APPROVED', reviewer_id: user.id, reviewer_name: user.username },
        }),
      })
    } else if (url.includes('/reject')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: { ...MOCK_TICKET, status: 'REJECTED', reviewer_id: user.id, reviewer_name: user.username },
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

  // Users (admin-only)
  page.route(/\/api\/users(\?.*)?$/, async (route: Route) => {
    if (role === 'admin') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: [MOCK_USERS.admin, MOCK_USERS.developer, MOCK_USERS.dba],
        }),
      })
    } else {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    }
  })

  // Roles & policies (admin-only)
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
            { id: 1, role: 'developer', datasource: 'test-mysql', table: 'users', action: 'select' },
          ],
        }),
      })
    } else {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    }
  })

  // Mask rules (admin-only)
  page.route('**/api/mask-rules**', async (route: Route) => {
    if (role === 'admin') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: [] }),
      })
    } else {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    }
  })

  // Sensitive tables (admin-only)
  page.route('**/api/sensitive-tables**', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: [] }),
    })
  })

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
}

// Helper: login via UI
export async function loginViaUI(page: Page, username = 'admin', password = 'admin123') {
  await page.goto('/login')
  await page.getByPlaceholder('用户名').fill(username)
  await page.getByPlaceholder('密码').fill(password)
  await page.getByRole('button', { name: '登 录' }).click()
  await page.waitForURL('**/query**')
}

// Helper: set token in localStorage directly
export async function setToken(page: Page, role: 'admin' | 'developer' | 'dba' = 'admin') {
  await page.goto('/login')
  await page.evaluate((token) => {
    localStorage.setItem('token', token)
  }, MOCK_TOKEN)
}
