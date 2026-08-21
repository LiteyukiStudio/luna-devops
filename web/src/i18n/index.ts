import type { BackendModule, ResourceKey } from 'i18next'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'

type SupportedLanguage = 'zh-CN' | 'zh-TW' | 'en-US' | 'ja-JP' | 'ko-KR'

const supportedLanguages = ['zh-CN', 'zh-TW', 'en-US', 'ja-JP', 'ko-KR'] as const satisfies readonly SupportedLanguage[]
const localeLoaders: Record<SupportedLanguage, () => Promise<{ default: ResourceKey }>> = {
  'zh-CN': () => import('./locales/zh-CN'),
  'zh-TW': () => import('./locales/zh-TW'),
  'en-US': () => import('./locales/en-US'),
  'ja-JP': () => import('./locales/ja-JP'),
  'ko-KR': () => import('./locales/ko-KR'),
}

const localeBackend: BackendModule = {
  type: 'backend',
  init() {},
  read(language, _namespace, callback) {
    const normalized = normalizeLanguage(language)
    if (!normalized) {
      callback(new Error(`Unsupported language: ${language}`), false)
      return
    }

    localeLoaders[normalized]()
      .then(module => callback(null, module.default))
      .catch(error => callback(error instanceof Error ? error : new Error(String(error)), false))
  },
}

function detectBrowserLanguage() {
  const storedLanguage = normalizeLanguage(localStorage.getItem('luna-devops-language'))
  if (storedLanguage)
    return storedLanguage

  const browserLanguages = [...navigator.languages, navigator.language].filter(Boolean)
  return browserLanguages.map(normalizeLanguage).find(Boolean) ?? 'zh-CN'
}

function normalizeLanguage(language?: string | null): SupportedLanguage | undefined {
  const normalized = language?.trim().toLowerCase()
  if (!normalized)
    return undefined
  if (normalized === 'zh-tw' || normalized === 'zh-hk' || normalized === 'zh-mo' || normalized.startsWith('zh-hant'))
    return 'zh-TW'
  if (normalized === 'zh-cn' || normalized === 'zh' || normalized === 'zh-sg' || normalized.startsWith('zh-hans') || normalized.startsWith('zh-'))
    return 'zh-CN'
  if (normalized === 'en-us' || normalized === 'en' || normalized.startsWith('en-'))
    return 'en-US'
  if (normalized === 'ja-jp' || normalized === 'ja' || normalized.startsWith('ja-'))
    return 'ja-JP'
  if (normalized === 'ko-kr' || normalized === 'ko' || normalized.startsWith('ko-'))
    return 'ko-KR'
  return undefined
}

export const i18nextReady = i18next.use(localeBackend).use(initReactI18next).init({
  lng: detectBrowserLanguage(),
  fallbackLng: false,
  supportedLngs: supportedLanguages,
  load: 'currentOnly',
  interpolation: {
    escapeValue: false,
  },
})

export default i18next
