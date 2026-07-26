import type { SupportedLocale } from './resources.js'
import process from 'node:process'

export interface LocaleDetectionOptions {
  readonly explicit?: string
  readonly configured?: string
  readonly env?: Readonly<Record<string, string | undefined>>
  readonly runtimeLocale?: string
}

export function normalizeLocale(locale: string | undefined): SupportedLocale | undefined {
  if (!locale)
    return undefined
  const normalized = locale
    .trim()
    .replace(/_/gu, '-')
    .replace(/[.@].*$/u, '')
  if (!normalized)
    return undefined
  const lower = normalized.toLocaleLowerCase()
  if (
    lower === 'cn'
    || lower === 'zh'
    || lower.startsWith('zh-cn')
    || lower.startsWith('zh-hans')
  ) {
    return 'zh-CN'
  }
  if (lower === 'en' || lower.startsWith('en-'))
    return 'en-US'
  return undefined
}

export function detectLocale(options: LocaleDetectionOptions = {}): SupportedLocale {
  const env = options.env ?? process.env
  const preferredCandidates = [
    options.explicit,
    env.LUNA_LANG,
    options.configured,
  ]
  for (const candidate of preferredCandidates) {
    if (candidate?.trim())
      return normalizeLocale(candidate) ?? 'en-US'
  }

  const runtimeCandidates = [
    env.LC_ALL,
    env.LC_MESSAGES,
    env.LANG,
    options.runtimeLocale ?? runtimeLocale(),
  ]
  for (const candidate of runtimeCandidates) {
    const locale = normalizeLocale(candidate)
    if (locale)
      return locale
  }
  return 'en-US'
}

function runtimeLocale(): string | undefined {
  try {
    return new Intl.DateTimeFormat().resolvedOptions().locale
  }
  catch {
    return undefined
  }
}
