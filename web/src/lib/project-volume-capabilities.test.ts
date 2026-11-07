import { describe, expect, it } from 'vitest'
import { ProjectAction } from '@/lib/roles'
import { canCancelVolumeTransfer, canRetryProjectVolume, canRetryVolumeTransfer, projectVolumeCapabilities } from './project-volume-capabilities'

const transfer = (overrides: Record<string, unknown> = {}) => ({ actorId: 'user_creator', direction: 'import', ...overrides }) as never
const volume = (pendingOperation: 'delete' | 'expand' | 'provision') => ({ pendingOperation }) as never

const viewerActions = ['project:read', 'volume:read']
const developerActions = [...viewerActions, ProjectAction.ProjectWrite, ProjectAction.VolumeWrite, ProjectAction.VolumeImport]
const adminActions = [...developerActions, ProjectAction.VolumeExport, ProjectAction.VolumeDelete]

describe('project volume capabilities', () => {
  it.each([
    ['viewer', viewerActions, false, false, false, false],
    ['developer', developerActions, true, true, false, false],
    ['admin', adminActions, true, true, true, true],
  ] as const)('projects %s server actions onto volume operations', (_name, actions, canWrite, canImport, canExport, canDelete) => {
    const capabilities = projectVolumeCapabilities(actions, 'user_current', true)
    expect(capabilities).toMatchObject({ canWrite, canImport, canExport, canDelete, canManageOtherTransfers: canExport })
    expect(canRetryProjectVolume(capabilities, volume('provision'))).toBe(canWrite)
    expect(canRetryProjectVolume(capabilities, volume('expand'))).toBe(canWrite)
    expect(canRetryProjectVolume(capabilities, volume('delete'))).toBe(canDelete)
    expect(canRetryVolumeTransfer(capabilities, transfer({ direction: 'import' }))).toBe(false)
    expect(canRetryVolumeTransfer(capabilities, transfer({ direction: 'export' }))).toBe(canExport)
  })

  it('denies every operation before the project detail is loaded', () => {
    expect(projectVolumeCapabilities(undefined, 'user_current', undefined)).toMatchObject({
      canWrite: false,
      canImport: false,
      canExport: false,
      canDelete: false,
      canManageOtherTransfers: false,
    })
  })

  it('disables transfer only when the server reports it is not configured', () => {
    expect(projectVolumeCapabilities(adminActions, 'user_admin', undefined).transferEnabled).toBe(true)
    expect(projectVolumeCapabilities(adminActions, 'user_admin', true).transferEnabled).toBe(true)
    expect(projectVolumeCapabilities(adminActions, 'user_admin', false).transferEnabled).toBe(false)
  })

  it('allows transfer creators to cancel their own transfer without exposing other users transfers', () => {
    const viewer = projectVolumeCapabilities(viewerActions, 'user_creator', true)
    expect(canCancelVolumeTransfer(viewer, transfer())).toBe(true)
    expect(canCancelVolumeTransfer(viewer, transfer({ actorId: 'user_other' }))).toBe(false)
    const admin = projectVolumeCapabilities(adminActions, 'user_admin', true)
    expect(canCancelVolumeTransfer(admin, transfer({ actorId: 'user_other' }))).toBe(true)
  })
})
