import { describe, it, expect } from 'vitest'
import {
  ROLE_LABEL_MAP,
  ROLE_BADGE_CLASS,
  ROLE_OPTIONS,
} from '@/api/user'

describe('User API Helpers', () => {
  describe('ROLE_LABEL_MAP', () => {
    it('maps admin to 管理员', () => {
      expect(ROLE_LABEL_MAP.admin).toBe('管理员')
    })

    it('maps dba to DBA', () => {
      expect(ROLE_LABEL_MAP.dba).toBe('DBA')
    })

    it('maps developer to 开发人员', () => {
      expect(ROLE_LABEL_MAP.developer).toBe('开发人员')
    })
  })

  describe('ROLE_BADGE_CLASS', () => {
    it('maps admin to red badge', () => {
      expect(ROLE_BADGE_CLASS.admin).toBe('bg-orange-500/20 text-orange-400')
    })

    it('maps dba to blue badge', () => {
      expect(ROLE_BADGE_CLASS.dba).toBe('bg-violet-500/20 text-violet-400')
    })

    it('maps developer to green badge', () => {
      expect(ROLE_BADGE_CLASS.developer).toBe('bg-blue-500/20 text-blue-400')
    })
  })

  describe('ROLE_OPTIONS', () => {
    it('has 3 entries', () => {
      expect(ROLE_OPTIONS).toHaveLength(3)
    })

    it('contains admin, dba, developer', () => {
      const values = ROLE_OPTIONS.map(o => o.value)
      expect(values).toEqual(['admin', 'dba', 'developer'])
    })

    it('each entry has value and label', () => {
      for (const opt of ROLE_OPTIONS) {
        expect(opt.value).toBeTruthy()
        expect(opt.label).toBeTruthy()
      }
    })
  })
})
