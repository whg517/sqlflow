import { describe, it, expect, vi } from 'vitest'
import { buildMongoSql } from '@/api/query'

describe('Query API Helpers', () => {
  describe('buildMongoSql', () => {
    it('builds find query JSON', () => {
      const result = buildMongoSql({
        collection: 'users',
        operation: 'find',
        filter: { age: { $gt: 18 } },
      })
      const parsed = JSON.parse(result)
      expect(parsed.collection).toBe('users')
      expect(parsed.operation).toBe('find')
      expect(parsed.filter).toEqual({ age: { $gt: 18 } })
    })

    it('builds aggregate query JSON', () => {
      const result = buildMongoSql({
        collection: 'orders',
        operation: 'aggregate',
        pipeline: [{ $match: { status: 'active' } }],
      })
      const parsed = JSON.parse(result)
      expect(parsed.collection).toBe('orders')
      expect(parsed.operation).toBe('aggregate')
      expect(parsed.pipeline).toHaveLength(1)
    })

    it('builds update query JSON', () => {
      const result = buildMongoSql({
        collection: 'products',
        operation: 'update',
        filter: { id: 1 },
        options: { $set: { name: 'updated' } },
      })
      const parsed = JSON.parse(result)
      expect(parsed.operation).toBe('update')
      expect(parsed.options).toEqual({ $set: { name: 'updated' } })
    })

    it('returns formatted JSON string', () => {
      const result = buildMongoSql({ collection: 'test', operation: 'find' })
      expect(result).toContain('\n') // Pretty-printed
    })
  })
})
