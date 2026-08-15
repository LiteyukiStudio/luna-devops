import { describe, expect, it } from 'vitest'
import { parseRuntimeDataVolumes, projectVolumeRowsAreValid, serializeRuntimeDataVolumes } from './runtime-data-volumes'

describe('runtime project volume contract', () => {
  it('round trips the stable projectVolumeId without a PVC fallback', () => {
    const serialized = serializeRuntimeDataVolumes([{
      id: 'row-1',
      emptyDirMedium: '',
      emptyDirSizeLimit: '',
      devicePath: '',
      projectVolumeId: 'pvol_123',
      readOnly: true,
      mountPath: '/data',
      name: 'data',
      sourceType: 'projectVolume',
    }])

    expect(serialized).toEqual([expect.objectContaining({
      logicalName: 'data',
      sourceType: 'projectVolume',
      projectVolumeId: 'pvol_123',
      readOnly: true,
    })])
    expect(parseRuntimeDataVolumes(serialized)[0]).toMatchObject({ projectVolumeId: 'pvol_123', readOnly: true })
  })

  it('keeps block devicePath mutually exclusive with mountPath', () => {
    const serialized = serializeRuntimeDataVolumes([{
      id: 'row-1',
      emptyDirMedium: '',
      emptyDirSizeLimit: '',
      devicePath: '/dev/data',
      projectVolumeId: 'pvol_block',
      readOnly: false,
      mountPath: '/data',
      name: 'data',
      sourceType: 'projectVolume',
    }])

    expect(serialized[0]).toMatchObject({ devicePath: '/dev/data', mountPath: undefined })
  })

  it('requires a stable volume ID and exactly one absolute runtime path', () => {
    const row = {
      ...parseRuntimeDataVolumes([{ logicalName: 'data', sourceType: 'projectVolume' }])[0]!,
      projectVolumeId: 'pvol_123',
      mountPath: '/data',
    }
    expect(projectVolumeRowsAreValid([row])).toBe(true)
    expect(projectVolumeRowsAreValid([{ ...row, devicePath: '/dev/data' }])).toBe(false)
    expect(projectVolumeRowsAreValid([{ ...row, mountPath: 'data' }])).toBe(false)
    expect(projectVolumeRowsAreValid([{ ...row, projectVolumeId: '' }])).toBe(false)
  })
})
