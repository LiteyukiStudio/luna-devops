import { describe, expect, it } from 'vitest'
import {
  isPlatformAdmin,
  PlatformRole,
} from '@/lib/roles'

describe('roles', () => {
  it('recognizes platform admins', () => {
    expect(isPlatformAdmin(PlatformRole.Admin)).toBe(true)
    expect(isPlatformAdmin(PlatformRole.User)).toBe(false)
  })
})
