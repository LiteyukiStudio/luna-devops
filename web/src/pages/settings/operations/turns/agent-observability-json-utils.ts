type JsonTokenKind = 'boolean' | 'key' | 'null' | 'number' | 'plain' | 'string'

export interface JsonSyntaxToken {
  id: string
  kind: JsonTokenKind
  value: string
}

const jsonTokenPattern = /"(?:\\.|[^"\\])*"(?=\s*:)|"(?:\\.|[^"\\])*"|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|\b(?:true|false)\b|\bnull\b/g

export function formatObservabilityJSON(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2) ?? String(value)
  }
  catch {
    return String(value)
  }
}

export function jsonSyntaxTokens(value: unknown): JsonSyntaxToken[] {
  const formatted = formatObservabilityJSON(value)
  const tokens: JsonSyntaxToken[] = []
  let offset = 0
  for (const match of formatted.matchAll(jsonTokenPattern)) {
    const index = match.index
    if (index > offset)
      tokens.push({ id: `${offset}-plain`, kind: 'plain', value: formatted.slice(offset, index) })
    const token = match[0]
    const kind = jsonTokenKind(token, formatted.slice(index + token.length))
    tokens.push({ id: `${index}-${kind}`, kind, value: token })
    offset = index + token.length
  }
  if (offset < formatted.length)
    tokens.push({ id: `${offset}-plain`, kind: 'plain', value: formatted.slice(offset) })
  return tokens
}

function jsonTokenKind(token: string, remainder: string): JsonTokenKind {
  if (token.startsWith('"'))
    return /^\s*:/.test(remainder) ? 'key' : 'string'
  if (token === 'true' || token === 'false')
    return 'boolean'
  if (token === 'null')
    return 'null'
  return 'number'
}

export function jsonTokenClassName(kind: JsonTokenKind): string | undefined {
  const classes: Record<JsonTokenKind, string | undefined> = {
    boolean: 'text-warning',
    key: 'text-primary-text',
    null: 'text-muted-foreground italic',
    number: 'text-info',
    plain: undefined,
    string: 'text-success',
  }
  return classes[kind]
}
