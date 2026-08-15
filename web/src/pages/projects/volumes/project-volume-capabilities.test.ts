import { describe, expect, it } from 'vitest'
import { PlatformRole, ProjectRole } from '@/lib/roles'
import { canCancelVolumeTransfer, canRetryProjectVolume, canRetryVolumeTransfer, projectVolumeCapabilities } from './project-volume-capabilities'

const transfer = (overrides: Record<string, unknown> = {}) => ({ actorId: 'user_creator', direction: 'import', ...overrides }) as never
const volume = (pendingOperation: 'delete' | 'expand' | 'provision') => ({ pendingOperation }) as never

describe('project volume capabilities', () => {
  it.each([
    [ProjectRole.Viewer, false, false, false, false],
    [ProjectRole.Developer, true, true, false, false],
    [ProjectRole.Admin, true, true, true, true],
    [ProjectRole.Owner, true, true, true, true],
  ] as const)('maps project role %s to the backend volume action matrix', (role, canWrite, canImport, canExport, canDelete) => {
    const capabilities = projectVolumeCapabilities(PlatformRole.User, role, 'user_current')
    expect(capabilities).toMatchObject({ canWrite, canImport, canExport, canDelete })
    expect(canRetryProjectVolume(capabilities, volume('provision'))).toBe(canWrite)
    expect(canRetryProjectVolume(capabilities, volume('expand'))).toBe(canWrite)
    expect(canRetryProjectVolume(capabilities, volume('delete'))).toBe(canDelete)
    expect(canRetryVolumeTransfer(capabilities, transfer({ direction: 'import' }))).toBe(canImport)
    expect(canRetryVolumeTransfer(capabilities, transfer({ direction: 'export' }))).toBe(canExport)
  })

  it('grants platform administrators all volume capabilities', () => {
    expect(projectVolumeCapabilities(PlatformRole.Admin, undefined, 'platform_admin')).toMatchObject({
      canWrite: true,
      canImport: true,
      canExport: true,
      canDelete: true,
      canManageOtherTransfers: true,
    })
  })

  it('allows transfer creators to cancel their own transfer without exposing other users transfers', () => {
    const viewer = projectVolumeCapabilities(PlatformRole.User, ProjectRole.Viewer, 'user_creator')
    expect(canCancelVolumeTransfer(viewer, transfer())).toBe(true)
    expect(canCancelVolumeTransfer(viewer, transfer({ actorId: 'user_other' }))).toBe(false)
    const admin = projectVolumeCapabilities(PlatformRole.User, ProjectRole.Admin, 'user_admin')
    expect(canCancelVolumeTransfer(admin, transfer({ actorId: 'user_other' }))).toBe(true)
  })
})
