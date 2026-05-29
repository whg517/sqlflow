/**
 * Global teardown — runs once after all test projects.
 *
 * For real tests: cleans up test-created resources via API.
 * For mock tests: no-op.
 */
async function globalTeardown() {
  const project = process.env.PLAYWRIGHT_PROJECT

  if (project === 'mock') {
    console.log('[globalTeardown] Skipping teardown for mock project')
    return
  }

  const baseURL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
  const username = process.env.E2E_USERNAME ?? 'e2e-admin'
  const password = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

  try {
    const loginResp = await fetch(`${baseURL}/api/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    })

    if (!loginResp.ok) {
      console.log('[globalTeardown] Login failed, skipping cleanup')
      return
    }

    const loginBody: { code: number; data?: { token?: string } } = await loginResp.json()
    if (loginBody.code !== 0 || !loginBody.data?.token) {
      console.log('[globalTeardown] No token returned, skipping cleanup')
      return
    }

    const token = loginBody.data.token
    const headers: Record<string, string> = {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    }

    // Helper: best-effort fetch
    async function cleanupFetch(url: string, method = 'GET') {
      try { await fetch(url, { method, headers }) } catch (e) { console.log(`[globalTeardown] cleanup error: ${e}`) }
    }

    // Clean up e2e-prefixed datasources
    try {
      const dsResp = await fetch(`${baseURL}/api/datasources`, { headers: { Authorization: `Bearer ${token}` } })
      if (dsResp.ok) {
        const dsBody: { data: Array<{ id: number; name: string }> } = await dsResp.json()
        for (const ds of dsBody.data ?? []) {
          if (ds.name.startsWith('e2e-') || ds.name.includes('e2e-test')) {
            await cleanupFetch(`${baseURL}/api/datasources/${ds.id}`, 'DELETE')
          }
        }
      }
    } catch (e) {
      console.log(`[globalTeardown] Datasource cleanup error: ${e}`)
    }

    // Clean up e2e-prefixed users
    try {
      const usersResp = await fetch(`${baseURL}/api/users`, { headers: { Authorization: `Bearer ${token}` } })
      if (usersResp.ok) {
        const usersBody: { data: { users: Array<{ id: number; username: string }> } } = await usersResp.json()
        for (const user of usersBody.data?.users ?? []) {
          if (user.username.startsWith('e2e_')) {
            await cleanupFetch(`${baseURL}/api/users/${user.id}`, 'DELETE')
          }
        }
      }
    } catch (e) {
      console.log(`[globalTeardown] User cleanup error: ${e}`)
    }

    console.log('[globalTeardown] Cleanup complete')
  } catch (err) {
    console.log(`[globalTeardown] Error: ${err}`)
  }
}

export default globalTeardown
