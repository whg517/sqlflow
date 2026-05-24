/**
 * Global teardown — runs once after all test projects.
 *
 * For real tests: cleans up test-created resources via API.
 * For mock tests: no-op.
 */
async function globalTeardown() {
  const project = process.env.PLAYWRIGHT_PROJECT

  // Skip teardown for mock-only runs
  if (project === 'mock') {
    return
  }

  const baseURL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
  const username = process.env.E2E_USERNAME ?? 'e2e-admin'
  const password = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

  try {
    // Login to get a token
    const loginResp = await fetch(`${baseURL}/api/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    })

    if (!loginResp.ok) {
      console.log('[globalTeardown] Login failed, skipping cleanup')
      return
    }

    const loginBody = await loginResp.json()
    if (loginBody.code !== 0 || !loginBody.data?.token) {
      console.log('[globalTeardown] No token returned, skipping cleanup')
      return
    }

    const token = loginBody.data.token
    const headers = {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    }

    // Clean up e2e-prefixed datasources
    const dsResp = await fetch(`${baseURL}/api/datasources`, {
      headers: { Authorization: `Bearer ${token}` },
    })

    if (dsResp.ok) {
      const dsBody = await dsResp.json()
      const datasources = dsBody.data ?? []

      for (const ds of datasources) {
        if (
          typeof ds.name === 'string' &&
          (ds.name.startsWith('e2e-') || ds.name.includes('e2e-test'))
        ) {
          try {
            await fetch(`${baseURL}/api/datasources/${ds.id}`, {
              method: 'DELETE',
              headers,
            })
          } catch {
            // best-effort
          }
        }
      }
    }

    console.log('[globalTeardown] Cleanup complete')
  } catch (err) {
    console.log(`[globalTeardown] Cleanup error: ${err}`)
  }
}

export default globalTeardown
