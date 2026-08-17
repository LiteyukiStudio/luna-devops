export class KeyValueTextError extends Error {
  readonly code: 'empty_key' | 'duplicate_key' | 'invalid_line'

  constructor(code: 'empty_key' | 'duplicate_key' | 'invalid_line') {
    super(code)
    this.code = code
    this.name = 'KeyValueTextError'
  }
}

export function parseKeyValueText(value: string): Record<string, string> {
  const output: Record<string, string> = {}
  for (const rawLine of value.split('\n')) {
    const line = rawLine.trim()
    if (!line)
      continue
    const separator = rawLine.indexOf('=')
    if (separator < 0)
      throw new KeyValueTextError('invalid_line')
    const key = rawLine.slice(0, separator).trim()
    if (!key)
      throw new KeyValueTextError('empty_key')
    if (Object.hasOwn(output, key))
      throw new KeyValueTextError('duplicate_key')
    output[key] = rawLine.slice(separator + 1).replace(/[ \t\r]+$/, '')
  }
  return output
}

export function formatKeyValueText(value: Record<string, string> | null | undefined): string {
  return Object.keys(value ?? {})
    .sort((left, right) => left.localeCompare(right))
    .map(key => `${key}=${value?.[key] ?? ''}`)
    .join('\n')
}
