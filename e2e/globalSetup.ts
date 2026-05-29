/**
 * Global setup — runs once before all test projects.
 *
 * For real tests: waits for backend to be healthy and creates test datasource.
 * For mock tests: no-op (route mocks don't need a backend).
 */
async function globalSetup() {
  const project = process.env.PLAYWRIGHT_PROJECT

  // Skip setup for mock-only runs
  if (project === 'mock') {
    console.log('[globalSetup] Skipping setup for mock project')
    return
  }

  const baseURL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'

  // Poll backend health endpoint
  const start = Date.now()
  const timeout = 90_000 // 90s — Docker build + startup can be slow

  while (Date.now() - start < timeout) {
    try {
      const resp = await fetch(`${baseURL}/health`)
      if (resp.ok) {
        console.log(`[globalSetup] Backend healthy at ${baseURL}`)
        return
      }
    } catch {
      // Not ready yet
    }
    await new Promise((r) => setTimeout(r, 3_000))
  }

  throw new Error(
    `[globalSetup] Backend at ${baseURL} did not become healthy within ${timeout / 1000}s`,
  )
}

export default globalSetup
