import type { BuildVariableSet } from '@/api'
import type { KeyValueRow } from '@/components/common/key-value-rows-editor'

export function emptyKeyValueRow(): KeyValueRow {
  return { id: crypto.randomUUID(), key: '', value: '' }
}

export function buildVariableRecordToRows(value: BuildVariableSet['variables']): KeyValueRow[] {
  const record = buildVariableRecord(value)
  const rows = Object.entries(record).map(([key, raw]) => ({ id: crypto.randomUUID(), key, value: String(raw) }))
  return rows.length ? rows : [emptyKeyValueRow()]
}

export function secretStateToRows(value: BuildVariableSet['secrets']): KeyValueRow[] {
  const rows = Object.keys(value ?? {}).map(key => ({ existing: true, id: crypto.randomUUID(), key, value: '' }))
  return rows.length ? rows : [emptyKeyValueRow()]
}

export function buildVariableRowsToRecord(rows: KeyValueRow[]) {
  return Object.fromEntries(rows.map(row => [row.key.trim(), row.value.trim()]).filter(([key, value]) => key || value))
}

function buildVariableRecord(value: BuildVariableSet['variables']) {
  if (typeof value === 'string') {
    try {
      const parsed = JSON.parse(value)
      if (parsed && typeof parsed === 'object')
        return Object.fromEntries(Object.entries(parsed).map(([key, raw]) => [key, String(raw)]))
    }
    catch {
      return {}
    }
  }
  return Object.fromEntries(Object.entries(value ?? {}).map(([key, raw]) => [key, String(raw)]))
}

export function buildVariableCount(value: BuildVariableSet['variables']) {
  if (typeof value === 'string') {
    try {
      const parsed = JSON.parse(value)
      return parsed && typeof parsed === 'object' ? Object.keys(parsed).length : 0
    }
    catch {
      return value.split('\n').filter(line => line.includes('=')).length
    }
  }
  return Object.keys(value ?? {}).length
}
