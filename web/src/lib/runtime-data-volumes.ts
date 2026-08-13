export interface RuntimeDataVolumeRow {
  emptyDirMedium: string
  emptyDirSizeLimit: string
  existingClaimName: string
  retainedVolumeId: string
  id: string
  name: string
  mountPath: string
  capacity: string
  sourceType: 'managed' | 'existingClaim' | 'retainedClaim' | 'emptyDir'
}

export function defaultRuntimeDataVolumeRow(): RuntimeDataVolumeRow {
  return { id: runtimeDataVolumeRowId(0), capacity: '1Gi', emptyDirMedium: '', emptyDirSizeLimit: '', existingClaimName: '', retainedVolumeId: '', mountPath: '/data', name: 'data', sourceType: 'managed' }
}

export function emptyRuntimeDataVolumeRow(index: number): RuntimeDataVolumeRow {
  return { id: runtimeDataVolumeRowId(index), capacity: '1Gi', emptyDirMedium: '', emptyDirSizeLimit: '', existingClaimName: '', retainedVolumeId: '', mountPath: '', name: `data-${index + 1}`, sourceType: 'managed' }
}

export function parseRuntimeDataVolumes(value?: string, fallbackMountPath = '/data', fallbackCapacity = '1Gi'): RuntimeDataVolumeRow[] {
  const trimmed = value?.trim() ?? ''
  if (!trimmed || trimmed === '[]') {
    return [{
      id: runtimeDataVolumeRowId(0),
      capacity: fallbackCapacity || '1Gi',
      emptyDirMedium: '',
      emptyDirSizeLimit: '',
      existingClaimName: '',
      retainedVolumeId: '',
      mountPath: fallbackMountPath || '/data',
      name: 'data',
      sourceType: 'managed',
    }]
  }
  try {
    const parsed = JSON.parse(trimmed)
    if (Array.isArray(parsed)) {
      const rows = parsed.map((item, index) => ({
        id: runtimeDataVolumeRowId(index),
        capacity: String(item?.capacity ?? '1Gi'),
        emptyDirMedium: String(item?.emptyDirMedium ?? ''),
        emptyDirSizeLimit: String(item?.emptyDirSizeLimit ?? ''),
        existingClaimName: String(item?.existingClaimName ?? ''),
        retainedVolumeId: String(item?.retainedVolumeId ?? ''),
        mountPath: String(item?.mountPath ?? ''),
        name: String(item?.name ?? `data-${index + 1}`),
        sourceType: normalizeRuntimeDataVolumeSourceType(item?.sourceType),
      })).filter(row => row.name.trim() || row.mountPath.trim() || row.capacity.trim())
      return rows.length > 0 ? rows : [defaultRuntimeDataVolumeRow()]
    }
  }
  catch {
    return [defaultRuntimeDataVolumeRow()]
  }
  return [defaultRuntimeDataVolumeRow()]
}

export function serializeRuntimeDataVolumes(rows: RuntimeDataVolumeRow[]) {
  const volumes = rows
    .map(row => ({
      capacity: row.sourceType === 'managed' ? (row.capacity.trim() || '1Gi') : '',
      emptyDirMedium: row.sourceType === 'emptyDir' ? row.emptyDirMedium.trim() : '',
      emptyDirSizeLimit: row.sourceType === 'emptyDir' ? row.emptyDirSizeLimit.trim() : '',
      existingClaimName: row.sourceType === 'existingClaim' || row.sourceType === 'retainedClaim' ? row.existingClaimName.trim() : '',
      retainedVolumeId: row.sourceType === 'retainedClaim' ? row.retainedVolumeId.trim() : '',
      mountPath: row.mountPath.trim(),
      name: row.name.trim(),
      sourceType: row.sourceType || 'managed',
    }))
    .filter(row => row.name || row.mountPath)
  return volumes.length > 0 ? JSON.stringify(volumes) : ''
}

function normalizeRuntimeDataVolumeSourceType(value: unknown): RuntimeDataVolumeRow['sourceType'] {
  if (value === 'existingClaim' || value === 'retainedClaim' || value === 'emptyDir')
    return value
  return 'managed'
}

function runtimeDataVolumeRowId(index: number) {
  return `runtime-data-volume-${index}`
}
