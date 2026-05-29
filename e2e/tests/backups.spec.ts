/**
 * backups.spec.ts — E2E: Backup management APIs (SF-QA0031)
 *
 * Tests 4 APIs (adminGroup):
 *   POST   /api/backups                    — trigger backup
 *   GET    /api/backups                    — list backups
 *   DELETE /api/backups/:filename           — delete backup
 *   GET    /api/backups/:filename/download  — download backup
 *
 * Note: backup writes to filesystem. Cleanup is essential.
 */
import { test, expect, BASE_URL, loginViaUI } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('Backup Management (admin)', () => {
  let createdBackupFilename: string | undefined

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test.afterEach(async ({ page }) => {
    // Cleanup: delete the backup we created
    if (createdBackupFilename) {
      const token = await page.evaluate(() => localStorage.getItem('token')!)
      await page.request.delete(`${BASE_URL}/api/backups/${encodeURIComponent(createdBackupFilename!)}`, {
        headers: { Authorization: `Bearer ${token}` },
      }).catch(() => {})
      createdBackupFilename = undefined
    }
  })

  test('should trigger a backup', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.post(`${BASE_URL}/api/backups`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(res.status()).toBe(200)
    const body: { code: number; data: { message: string } } = await res.json()
    expect(body.code).toBe(0)
    expect(body.data.message).toBeTruthy()
  })

  test('should list backups', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(`${BASE_URL}/api/backups`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(res.status()).toBe(200)
    const body: { code: number; data: Array<{ filename: string }> } = await res.json()
    expect(body.code).toBe(0)
    expect(Array.isArray(body.data)).toBeTruthy()
  })

  test('should create, list, and find the backup', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)

    // Trigger backup
    const triggerRes = await page.request.post(`${BASE_URL}/api/backups`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(triggerRes.status()).toBe(200)

    // List and find it
    const listRes = await page.request.get(`${BASE_URL}/api/backups`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const listBody: { code: number; data: Array<{ filename: string }> } = await listRes.json()
    expect(listBody.code).toBe(0)
    expect(listBody.data.length).toBeGreaterThanOrEqual(1)

    // Track for cleanup
    createdBackupFilename = listBody.data[0].filename
  })

  test('should download a backup file', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)

    // Trigger backup
    await page.request.post(`${BASE_URL}/api/backups`, {
      headers: { Authorization: `Bearer ${token}` },
    })

    // List to get filename
    const listRes = await page.request.get(`${BASE_URL}/api/backups`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const listBody: { code: number; data: Array<{ filename: string }> } = await listRes.json()
    if (listBody.data.length === 0) return // no backups to download
    const filename = listBody.data[0].filename
    createdBackupFilename = filename

    // Download
    const dlRes = await page.request.get(`${BASE_URL}/api/backups/${encodeURIComponent(filename)}/download`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(dlRes.status()).toBe(200)
  })

  test('should return 404 for downloading non-existent backup', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/backups/nonexistent_backup_file.db/download`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBe(404)
  })

  test('should delete a backup', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)

    // Trigger backup
    await page.request.post(`${BASE_URL}/api/backups`, {
      headers: { Authorization: `Bearer ${token}` },
    })

    // List to get filename
    const listRes = await page.request.get(`${BASE_URL}/api/backups`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const listBody: { code: number; data: Array<{ filename: string }> } = await listRes.json()
    if (listBody.data.length === 0) return
    const filename = listBody.data[0].filename

    // Delete
    const delRes = await page.request.delete(`${BASE_URL}/api/backups/${encodeURIComponent(filename)}`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(delRes.status()).toBe(200)

    // Verify it's gone
    const dlRes = await page.request.get(`${BASE_URL}/api/backups/${encodeURIComponent(filename)}/download`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(dlRes.status()).toBe(404)
  })

  test('should return error when deleting non-existent backup', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.delete(
      `${BASE_URL}/api/backups/nonexistent_file.db`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBe(400)
  })
})
