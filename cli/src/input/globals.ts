import { invalidInput } from '../errors/index.js'
import { parseBoolean, parseDurationMilliseconds } from './primitives.js'

export const OUTPUT_FORMATS = Object.freeze([
  'table',
  'json',
  'raw-json',
  'yaml',
  'jsonl',
  'name',
] as const)
export type OutputFormat = (typeof OUTPUT_FORMATS)[number]

export const GLOBAL_CONTROL_KEYS = Object.freeze([
  'server',
  'project',
  'output',
  'lang',
  'color',
  'interactive',
  'yes',
  'quiet',
  'agent',
  'dryRun',
  'timeout',
  'debug',
  'requestId',
  'idempotencyKey',
  'insecureSkipTlsVerify',
] as const)

export type GlobalControlKey = (typeof GLOBAL_CONTROL_KEYS)[number]

export interface GlobalControls {
  readonly server?: string
  readonly project?: string
  readonly output?: OutputFormat
  readonly lang?: string
  readonly color?: boolean
  readonly interactive?: boolean
  readonly yes?: boolean
  readonly quiet?: boolean
  readonly agent?: boolean
  readonly dryRun?: 'client' | 'server'
  readonly timeoutMs?: number
  readonly debug?: boolean
  readonly requestId?: string
  readonly idempotencyKey?: string
  readonly insecureSkipTlsVerify?: boolean
}

export interface ExtractedGlobalControls {
  readonly controls: GlobalControls
  readonly businessTokens: readonly string[]
  readonly explicitKeys: ReadonlySet<GlobalControlKey>
}

const GLOBAL_KEY_SET = new Set<string>(GLOBAL_CONTROL_KEYS)
const BOOLEAN_GLOBALS = new Set<GlobalControlKey>([
  'color',
  'interactive',
  'yes',
  'quiet',
  'agent',
  'debug',
  'insecureSkipTlsVerify',
])

export function extractGlobalControls(tokens: readonly string[]): ExtractedGlobalControls {
  const businessTokens: string[] = []
  const rawControls = new Map<GlobalControlKey, string>()

  for (const token of tokens) {
    const separator = token.indexOf('=')
    if (separator <= 0) {
      businessTokens.push(token)
      continue
    }
    const key = token.slice(0, separator)
    if (!GLOBAL_KEY_SET.has(key)) {
      businessTokens.push(token)
      continue
    }
    const typedKey = key as GlobalControlKey
    if (rawControls.has(typedKey)) {
      throw invalidInput('duplicate_global_control', `${key} may only be specified once.`, {
        fields: [{ key, code: 'duplicate' }],
      })
    }
    rawControls.set(typedKey, token.slice(separator + 1))
  }

  const controls: Record<string, unknown> = {}
  for (const [key, value] of rawControls) {
    if (BOOLEAN_GLOBALS.has(key)) {
      controls[key] = parseBoolean(value, key)
    }
    else if (key === 'timeout') {
      controls.timeoutMs = parseDurationMilliseconds(value, key)
    }
    else if (key === 'output') {
      if (!OUTPUT_FORMATS.includes(value as OutputFormat)) {
        throw invalidInput('invalid_output_format', `Unsupported output format "${value}".`, {
          fields: [{ key, code: 'enum', expected: OUTPUT_FORMATS, actual: value }],
        })
      }
      controls.output = value
    }
    else if (key === 'dryRun') {
      if (value !== 'client' && value !== 'server') {
        throw invalidInput('invalid_dry_run_mode', 'dryRun must be client or server.', {
          fields: [{ key, code: 'enum', expected: ['client', 'server'], actual: value }],
        })
      }
      controls.dryRun = value
    }
    else {
      controls[key] = value
    }
  }

  return {
    controls: controls as GlobalControls,
    businessTokens,
    explicitKeys: new Set(rawControls.keys()),
  }
}
