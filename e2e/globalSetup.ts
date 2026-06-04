/**
 * Global setup — runs once before all test projects.
 *
 * Waits for backend to be healthy, then seeds a shared MySQL datasource.
 */
async function globalSetup() {
  const baseURL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'

  // Poll backend health endpoint
  const start = Date.now()
  const timeout = 90_000 // 90s — Docker build + startup can be slow

  while (Date.now() - start < timeout) {
    try {
      const resp = await fetch(`${baseURL}/health`)
      if (resp.ok) {
        console.log(`[globalSetup] Backend healthy at ${baseURL}`)
        break
      }
    } catch {
      // Not ready yet
    }
    await new Promise((r) => setTimeout(r, 3_000))
  }

  if (Date.now() - start >= timeout) {
    throw new Error(
      `[globalSetup] Backend at ${baseURL} did not become healthy within ${timeout / 1000}s`,
    )
  }

  // Login and seed a shared datasource
  const username = process.env.E2E_USERNAME ?? 'e2eadmin'
  const password = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

  const loginRes = await fetch(`${baseURL}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!loginRes.ok) {
    const errBody = await loginRes.text().catch(() => '')
    throw new Error(
      `[globalSetup] Login failed: ${loginRes.status} — ${errBody}`,
    )
  }
  const { data } = await loginRes.json()
  const token = data.access_token

  // Check if e2e-shared-mysql datasource already exists
  const dsRes = await fetch(`${baseURL}/api/datasources`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (dsRes.ok) {
    const dsList = await dsRes.json()
    const existing = (dsList.data ?? []).find((d: { name: string }) => d.name === 'e2e-shared-mysql')
    if (existing) {
      if (existing.status === 'active') {
        console.log(`[globalSetup] Shared datasource already exists and active (id=${existing.id})`)
        return
      }
      // Try to delete and recreate if disabled
      await fetch(`${baseURL}/api/datasources/${existing.id}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${token}` },
      }).catch(() => {})
      console.log(`[globalSetup] Deleted stale datasource (id=${existing.id})`)
    }
  }

  // Create shared datasource with fixed name 'e2e-shared-mysql'
  const dsName = 'e2e-shared-mysql'
  const createRes = await fetch(`${baseURL}/api/datasources`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name: dsName,
      type: 'mysql',
      host: 'mysql-test',
      port: 3306,
      username: 'root',
      password: 'e2e-root-pass-123',
      database: 'testdb',
    }),
  })
  if (createRes.ok || createRes.status === 201) {
    console.log(`[globalSetup] Shared datasource '${dsName}' created`)
  } else {
    const err = await createRes.text()
    console.error(`[globalSetup] Failed to create shared datasource: ${err}`)
  }
}

export default globalSetup
