import type { DeploymentDataVolumeInput } from '@/api'

export interface RuntimeDataVolumeRow {
  emptyDirMedium: '' | 'Memory'
  emptyDirSizeLimit: string
  devicePath: string
  projectVolumeId: string
  readOnly: boolean
  id: string
  name: string
  mountPath: string
  sourceType: 'projectVolume' | 'emptyDir'
}

export function defaultRuntimeDataVolumeRow(): RuntimeDataVolumeRow {
  return {
    id: runtimeDataVolumeRowId(0),
    emptyDirMedium: '',
    emptyDirSizeLimit: '',
    devicePath: '',
    projectVolumeId: '',
    readOnly: false,
    mountPath: '/data',
    name: 'data',
    sourceType: 'projectVolume',
  }
}

export function emptyRuntimeDataVolumeRow(index: number): RuntimeDataVolumeRow {
  return {
    ...defaultRuntimeDataVolumeRow(),
    id: runtimeDataVolumeRowId(index),
    mountPath: '',
    name: `data-${index + 1}`,
  }
}

export function parseRuntimeDataVolumes(value?: DeploymentDataVolumeInput[]): RuntimeDataVolumeRow[] {
  if (!Array.isArray(value))
    return []
  return value
    .filter(item => item?.sourceType === 'projectVolume' || item?.sourceType === 'emptyDir')
    .map((item, index) => ({
      id: runtimeDataVolumeRowId(index),
      emptyDirMedium: item.emptyDir?.medium === 'Memory' ? 'Memory' : '',
      emptyDirSizeLimit: String(item.emptyDir?.sizeLimit ?? ''),
      devicePath: String(item.devicePath ?? ''),
      projectVolumeId: String(item.projectVolumeId ?? ''),
      readOnly: Boolean(item.readOnly),
      mountPath: String(item.mountPath ?? ''),
      name: String(item.logicalName ?? `data-${index + 1}`),
      sourceType: item.sourceType,
    }))
}

export function serializeRuntimeDataVolumes(rows: RuntimeDataVolumeRow[]): DeploymentDataVolumeInput[] {
  return rows.map(row => row.sourceType === 'emptyDir'
    ? {
        logicalName: row.name.trim(),
        sourceType: 'emptyDir',
        mountPath: row.mountPath.trim(),
        emptyDir: {
          medium: row.emptyDirMedium,
          sizeLimit: row.emptyDirSizeLimit.trim(),
        },
      }
    : {
        logicalName: row.name.trim(),
        sourceType: 'projectVolume',
        projectVolumeId: row.projectVolumeId.trim(),
        mountPath: row.devicePath.trim() ? undefined : row.mountPath.trim(),
        devicePath: row.devicePath.trim() || undefined,
        readOnly: row.readOnly,
      })
}

export function projectVolumeRowsAreValid(rows: RuntimeDataVolumeRow[]) {
  return rows.every((row) => {
    if (!row.name.trim())
      return false
    if (row.sourceType === 'emptyDir')
      return row.mountPath.trim().startsWith('/') && !row.devicePath.trim() && !row.projectVolumeId.trim()
    const mountPath = row.mountPath.trim()
    const devicePath = row.devicePath.trim()
    return row.projectVolumeId.trim().startsWith('pvol_')
      && Boolean(mountPath) !== Boolean(devicePath)
      && (mountPath || devicePath).startsWith('/')
  })
}

function runtimeDataVolumeRowId(index: number) {
  return `runtime-data-volume-${index}`
}
