import type { ProjectVolumeStorageClass } from '@/api'
import { describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { volumeSnapshotNotice, volumeSnapshotSupport } from './volume-export-consistency'

const storageClass = (name: string, snapshotSupported: boolean, isDefault = false) => ({ name, snapshotSupported, isDefault }) as ProjectVolumeStorageClass
const volume = (storageClassName: string, observed = '') => ({ storageClassName, observation: { storageClassName: observed } }) as never
const loaded = (items: ProjectVolumeStorageClass[], complete = true) => ({ items, complete, status: 'success' as const })

describe('volume export snapshot support', () => {
  it('uses the observed PVC storage class before the registered one', () => {
    const classes = loaded([storageClass('fast', true), storageClass('slow', false)])
    expect(volumeSnapshotSupport(volume('slow', 'fast'), classes)).toBe('supported')
    expect(volumeSnapshotSupport(volume('fast', 'slow'), classes)).toBe('unsupported')
  })

  it('resolves a blank class name to the default storage class', () => {
    expect(volumeSnapshotSupport(volume(''), loaded([storageClass('standard', false, true), storageClass('fast', true)]))).toBe('unsupported')
    expect(volumeSnapshotSupport(volume(''), loaded([storageClass('standard', true, true)]))).toBe('supported')
  })

  it('does not claim support while the live list is pending, failed, or incomplete', () => {
    expect(volumeSnapshotSupport(volume('fast'), { items: [], complete: false, status: 'pending' })).toBe('loading')
    expect(volumeSnapshotSupport(volume('fast'), { items: [], complete: false, status: 'error' })).toBe('unknown')
    expect(volumeSnapshotSupport(volume('fast'), loaded([storageClass('other', true)], false))).toBe('unknown')
    expect(volumeSnapshotSupport(volume('fast'), loaded([storageClass('other', true)]))).toBe('unsupported')
  })

  it('explains the automatic fallback per volume mode', () => {
    const t = i18next.t.bind(i18next)
    expect(volumeSnapshotNotice(t, 'Filesystem', 'unsupported')).toBe(i18next.t('projectVolumes.consistencySnapshotUnsupported'))
    expect(volumeSnapshotNotice(t, 'Block', 'unsupported')).toBe(i18next.t('projectVolumes.consistencySnapshotUnsupportedBlock'))
    expect(volumeSnapshotNotice(t, 'Filesystem', 'unknown')).toBe(i18next.t('projectVolumes.consistencySnapshotUnknown'))
    expect(volumeSnapshotNotice(t, 'Filesystem', 'supported')).toBeNull()
  })
})
