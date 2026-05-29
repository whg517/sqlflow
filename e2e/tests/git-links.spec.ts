/**
 * git-links.spec.ts — E2E: Git link management APIs (SF-QA0031)
 *
 * Tests 3 APIs (authGroup):
 *   POST   /api/git-links              — create git link
 *   GET    /api/git-links              — list git links (by entity)
 *   DELETE /api/git-links/:id          — delete git link
 *
 * Links associate git commits/PRs with tickets or audit logs.
 */
import { test, expect, BASE_URL, loginViaUI, apiRequest } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

let createdGitLinkIds: number[] = []

async function cleanupGitLinks(page: import('@playwright/test').Page): Promise<void> {
  // No batch delete API — cleanup individual links created in tests
  for (const id of createdGitLinkIds) {
    await apiRequest(page, 'DELETE', `/git-links/${id}`).catch(() => {})
  }
  createdGitLinkIds = []
}

test.describe('Git Links — CRUD', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
    createdGitLinkIds = []
  })

  test.afterEach(async ({ page }) => {
    await cleanupGitLinks(page)
  })

  test('should create a git link (commit type)', async ({ page }) => {
    const { status, body } = await apiRequest(page, 'POST', '/git-links', {
      entity_type: 'ticket',
      entity_id: 1,
      link_type: 'commit',
      commit_hash: `e2e${Date.now().toString(16)}`,
      commit_message: 'E2E test commit',
      author_name: 'e2e-tester',
      repo_url: 'https://github.com/e2e/test',
    })
    expect(status).toBe(201)
    const data = body as { code: number; data: { id: number } }
    expect(data.code).toBe(0)
    expect(data.data.id).toBeTruthy()
    createdGitLinkIds.push(data.data.id)
  })

  test('should create a git link (PR type)', async ({ page }) => {
    const { status, body } = await apiRequest(page, 'POST', '/git-links', {
      entity_type: 'audit_log',
      entity_id: 1,
      link_type: 'pr',
      pr_number: 42,
      pr_title: 'E2E test PR',
      pr_url: 'https://github.com/e2e/test/pull/42',
    })
    expect(status).toBe(201)
    const data = body as { code: number; data: { id: number } }
    expect(data.code).toBe(0)
    createdGitLinkIds.push(data.data.id)
  })

  test('should reject invalid entity_type', async ({ page }) => {
    const { status } = await apiRequest(page, 'POST', '/git-links', {
      entity_type: 'invalid_type',
      entity_id: 1,
      link_type: 'commit',
      commit_hash: 'abc123',
    })
    expect(status).toBe(400)
  })

  test('should reject invalid link_type', async ({ page }) => {
    const { status } = await apiRequest(page, 'POST', '/git-links', {
      entity_type: 'ticket',
      entity_id: 1,
      link_type: 'invalid_link_type',
      commit_hash: 'abc123',
    })
    expect(status).toBe(400)
  })

  test('should reject missing commit_hash and pr_number', async ({ page }) => {
    const { status } = await apiRequest(page, 'POST', '/git-links', {
      entity_type: 'ticket',
      entity_id: 1,
      link_type: 'commit',
    })
    expect(status).toBe(400)
  })

  test('should reject entity_id <= 0', async ({ page }) => {
    const { status } = await apiRequest(page, 'POST', '/git-links', {
      entity_type: 'ticket',
      entity_id: 0,
      link_type: 'commit',
      commit_hash: 'abc123',
    })
    expect(status).toBe(400)
  })

  test('should list git links by entity', async ({ page }) => {
    // Create a link first
    const entityId = 1
    const createRes = await apiRequest(page, 'POST', '/git-links', {
      entity_type: 'ticket',
      entity_id: entityId,
      link_type: 'commit',
      commit_hash: `e2e-list-${Date.now().toString(16)}`,
    })
    const created = createRes.body as { data: { id: number } }
    createdGitLinkIds.push(created.data.id)

    // List
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/git-links?entity_type=ticket&entity_id=${entityId}`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBe(200)
    const body: { code: number; data: Array<{ id: number }> } = await res.json()
    expect(body.code).toBe(0)
    expect(body.data.length).toBeGreaterThanOrEqual(1)
    expect(body.data.some((l) => l.id === created.data.id)).toBeTruthy()
  })

  test('should reject list without entity_type', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/git-links?entity_id=1`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBe(400)
  })

  test('should delete a git link', async ({ page }) => {
    // Create
    const createRes = await apiRequest(page, 'POST', '/git-links', {
      entity_type: 'ticket',
      entity_id: 1,
      link_type: 'commit',
      commit_hash: `e2e-del-${Date.now().toString(16)}`,
    })
    const created = createRes.body as { data: { id: number } }
    const id = created.data.id

    // Delete
    const { status, body } = await apiRequest(page, 'DELETE', `/git-links/${id}`)
    expect(status).toBe(200)
    expect((body as { code: number }).code).toBe(0)
    // Remove from cleanup since already deleted
    createdGitLinkIds = createdGitLinkIds.filter((x) => x !== id)
  })

  test('should return 404 when deleting non-existent git link', async ({ page }) => {
    const { status } = await apiRequest(page, 'DELETE', '/git-links/99999')
    expect(status).toBe(404)
  })
})
