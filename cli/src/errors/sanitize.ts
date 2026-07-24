const ESCAPE_CHARACTER = String.fromCharCode(0x1B)
const BELL_CHARACTER = String.fromCharCode(0x07)
const ANSI_SEQUENCE_PATTERN = new RegExp(
  [
    ESCAPE_CHARACTER,
    `\\][^${BELL_CHARACTER}${ESCAPE_CHARACTER}]*`,
    `(?:${BELL_CHARACTER}|${ESCAPE_CHARACTER}\\\\)`,
    '|',
    ESCAPE_CHARACTER,
    '(?:\\[[\\x30-\\x3F]*[\\x20-\\x2F]*[\\x40-\\x7E]|[\\x40-\\x5F])',
  ].join(''),
  'gu',
)
const BIDI_CONTROL_PATTERN = /[\u061C\u200E\u200F\u202A-\u202E\u2066-\u2069]/gu
const SENSITIVE_KEY_PATTERN
  = /(?:authorization|cookie|credential|kubeconfig|otp|pass(?:word|phrase|wd)?|private[-_]?key|recovery[-_]?code|refresh[-_]?token|secret|session[-_]?id)$/iu
const VALUE_SENSITIVE_KEY_PATTERN = /(?:access[-_]?token|id[-_]?token|token)$/iu
const URL_PATTERN = /\bhttps?:\/\/[^\s"'<>]+/giu
const AUTHORIZATION_PATTERN = /\b(Bearer|Basic)\s+[\w.~+/=-]+/giu
const ASSIGNMENT_PATTERN
  = /\b(access[_-]?token|api[_-]?key|client[_-]?secret|password|refresh[_-]?token|secret|token)(?:[ \t]*=[ \t]*|[ \t]*:[ \t]+)([^\s,;]+)/giu

const SENSITIVE_QUERY_KEYS = new Set([
  'access_token',
  'api_key',
  'apikey',
  'authorization',
  'client_secret',
  'code',
  'id_token',
  'key',
  'password',
  'refresh_token',
  'secret',
  'sig',
  'signature',
  'token',
])

export const REDACTED_VALUE = '[REDACTED]'

export interface RedactionOptions {
  readonly maxDepth?: number
  readonly maxEntries?: number
  readonly sensitiveKeys?: ReadonlySet<string>
}

export function isSensitiveKey(key: string, additionalKeys?: ReadonlySet<string>): boolean {
  const normalized = key.trim().toLocaleLowerCase()
  return Boolean(additionalKeys?.has(normalized)) || SENSITIVE_KEY_PATTERN.test(normalized)
}

export function redactValue(value: unknown, options: RedactionOptions = {}): unknown {
  const maxDepth = options.maxDepth ?? 12
  const maxEntries = options.maxEntries ?? 10_000
  const seen = new WeakSet<object>()
  let entries = 0

  function visit(current: unknown, depth: number, key?: string): unknown {
    entries += 1
    if (entries > maxEntries)
      return '[TRUNCATED]'
    if (
      key
      && (
        (typeof current !== 'boolean' && isSensitiveKey(key, options.sensitiveKeys))
        || (typeof current === 'string' && VALUE_SENSITIVE_KEY_PATTERN.test(key))
      )
    ) {
      return REDACTED_VALUE
    }
    if (typeof current === 'string')
      return redactSensitiveText(current)
    if (typeof current !== 'object' || current === null)
      return current
    if (depth >= maxDepth)
      return '[MAX_DEPTH]'
    if (seen.has(current)) {
      return '[CIRCULAR]'
    }
    seen.add(current)

    if (Array.isArray(current)) {
      const result = current.map(item => visit(item, depth + 1))
      seen.delete(current)
      return result
    }

    const result: Record<string, unknown> = {}
    for (const [childKey, childValue] of Object.entries(current)) {
      result[childKey] = visit(childValue, depth + 1, childKey)
    }
    seen.delete(current)
    return result
  }

  return visit(value, 0)
}

export function redactSensitiveText(value: string): string {
  return value
    .replace(AUTHORIZATION_PATTERN, '$1 [REDACTED]')
    .replace(ASSIGNMENT_PATTERN, '$1=[REDACTED]')
    .replace(URL_PATTERN, redactUrl)
}

export function sanitizeTerminalText(value: string): string {
  const withoutSequences = redactSensitiveText(value)
    .replace(ANSI_SEQUENCE_PATTERN, '')
    .replace(BIDI_CONTROL_PATTERN, '')
  return [...withoutSequences]
    .filter(character => !isUnsafeControlCharacter(character.codePointAt(0)!))
    .join('')
}

export function escapeUnsafeJsonCharacters(value: string): string {
  return value.replace(/[\u007F-\u009F\u061C\u200E\u200F\u2028\u2029\u202A-\u202E\u2066-\u2069]/gu, character =>
    `\\u${character.codePointAt(0)!.toString(16).padStart(4, '0')}`)
}

function redactUrl(value: string): string {
  try {
    const url = new URL(value)
    if (url.username)
      url.username = REDACTED_VALUE
    if (url.password)
      url.password = REDACTED_VALUE
    for (const key of [...url.searchParams.keys()]) {
      if (SENSITIVE_QUERY_KEYS.has(key.toLocaleLowerCase())) {
        url.searchParams.set(key, REDACTED_VALUE)
      }
    }
    return url.toString()
  }
  catch {
    return value
  }
}

function isUnsafeControlCharacter(codePoint: number): boolean {
  return (
    (codePoint >= 0x00 && codePoint <= 0x08)
    || codePoint === 0x0B
    || codePoint === 0x0C
    || (codePoint >= 0x0E && codePoint <= 0x1F)
    || (codePoint >= 0x7F && codePoint <= 0x9F)
  )
}
