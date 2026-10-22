export interface RuntimeConfigFileRow {
  id: string
  path: string
  content: string
}

export function emptyRuntimeConfigFileRow(): RuntimeConfigFileRow {
  return { id: crypto.randomUUID(), content: '', path: '' }
}

export function parseRuntimeConfigFiles(value: string): RuntimeConfigFileRow[] {
  const trimmed = value.trim()
  if (!trimmed || trimmed === '[]')
    return []
  try {
    const parsed = JSON.parse(trimmed)
    if (Array.isArray(parsed)) {
      return parsed.map(item => ({
        id: crypto.randomUUID(),
        content: String(item?.content ?? ''),
        path: String(item?.path ?? ''),
      })).filter(row => row.path.trim() || row.content.trim())
    }
  }
  catch {
    return []
  }
  return []
}

export function serializeRuntimeConfigFiles(rows: RuntimeConfigFileRow[]) {
  const files = rows
    .map(row => ({ content: row.content, path: row.path.trim() }))
    .filter(row => row.path || row.content)
  return files.length > 0 ? JSON.stringify(files) : ''
}

/** 返回挂载路径冲突的行 ID；路径按容器内绝对路径语义归一化后比较。 */
export function duplicateRuntimeConfigPathRowIds(rows: RuntimeConfigFileRow[]) {
  const pathRows = new Map<string, string[]>()
  for (const row of rows) {
    const path = normalizeRuntimeConfigPathForConflict(row.path)
    if (!path)
      continue
    pathRows.set(path, [...(pathRows.get(path) ?? []), row.id])
  }
  const duplicated = new Set<string>()
  for (const rowIds of pathRows.values()) {
    if (rowIds.length <= 1)
      continue
    for (const rowId of rowIds)
      duplicated.add(rowId)
  }
  return duplicated
}

function normalizeRuntimeConfigPathForConflict(value: string) {
  const trimmed = value.trim()
  if (!trimmed)
    return ''
  if (!trimmed.startsWith('/'))
    return trimmed
  const parts: string[] = []
  for (const part of trimmed.split('/')) {
    if (!part || part === '.')
      continue
    if (part === '..') {
      parts.pop()
      continue
    }
    parts.push(part)
  }
  return `/${parts.join('/')}`
}

export function runtimeConfigFileCount(value: string) {
  return parseRuntimeConfigFiles(value).filter(row => row.path.trim()).length
}
