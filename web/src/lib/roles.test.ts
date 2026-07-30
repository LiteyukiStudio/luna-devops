import { describe, expect, it } from 'vitest'
import {
  isPlatformAdmin,
  isPlatformRole,
  isProjectRole,
  PlatformRole,
  ProjectRole,
} from '@/lib/roles'

describe('roles', () => {
  it('recognizes platform roles', () => {
    expect(isPlatformRole(PlatformRole.Admin)).toBe(true)
    expect(isPlatformRole(PlatformRole.User)).toBe(true)
    expect(isPlatformRole(ProjectRole.Owner)).toBe(false)
    expect(isPlatformAdmin(PlatformRole.Admin)).toBe(true)
  })

  it('recognizes project roles', () => {
    expect(isProjectRole(ProjectRole.Owner)).toBe(true)
    expect(isProjectRole(ProjectRole.Admin)).toBe(true)
    expect(isProjectRole(ProjectRole.Developer)).toBe(true)
    expect(isProjectRole(ProjectRole.Viewer)).toBe(true)
    expect(isProjectRole(PlatformRole.Admin)).toBe(false)
  })
})
