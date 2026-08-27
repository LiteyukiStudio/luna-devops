import type { BackendModule, ResourceKey } from 'i18next'
import type { SupportedLanguage } from './config'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { safeStorageGet } from '@/lib/safe-storage'
import { spreadFeatureBundles, supportedLanguages } from './config'

interface LocaleModule {
  default: ResourceKey
}

const localeLoaders: Record<SupportedLanguage, () => Promise<{ default: ResourceKey }>> = {
  'zh-CN': () => import('./locales/zh-CN'),
  'zh-TW': () => import('./locales/zh-TW'),
  'en-US': () => import('./locales/en-US'),
  'ja-JP': () => import('./locales/ja-JP'),
  'ko-KR': () => import('./locales/ko-KR'),
}

const featureLocaleLoaders = import.meta.glob<LocaleModule>([
  './locales/*/*.ts',
  '!./locales/*/root.ts',
  '!./locales/*/languages.ts',
  '!./locales/*/common.ts',
  '!./locales/*/time.ts',
  '!./locales/*/errors.ts',
  '!./locales/*/auth.ts',
  '!./locales/*/pagination.ts',
  '!./locales/*/oauthApps.ts',
  '!./locales/*/theme.ts',
  '!./locales/*/loginPage.ts',
  '!./locales/*/bootstrap.ts',
])
const activeFeatureBundles = new Set<string>()
const spreadFeatureBundleSet = new Set<string>(spreadFeatureBundles)

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
  const storedLanguage = normalizeLanguage(safeStorageGet('luna-devops-language'))
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

export async function loadTranslationBundles(bundleNames: readonly string[]) {
  for (const bundleName of bundleNames)
    activeFeatureBundles.add(bundleName)

  await i18nextReady
  await loadFeatureBundles(normalizeLanguage(i18next.language) ?? 'zh-CN', bundleNames)
}

export async function loadAllTranslationBundlesForTests() {
  const bundleNames = [...new Set(Object.keys(featureLocaleLoaders).map(modulePath => modulePath.split('/').at(-1)?.replace(/\.ts$/, '')).filter((name): name is string => Boolean(name)))]
  for (const bundleName of bundleNames)
    activeFeatureBundles.add(bundleName)
  await i18nextReady
  await Promise.all(supportedLanguages.map(async (language) => {
    const core = await localeLoaders[language]()
    i18next.addResourceBundle(language, 'translation', core.default, true, true)
    await loadFeatureBundles(language, bundleNames)
  }))
}

async function loadFeatureBundles(language: SupportedLanguage, bundleNames: readonly string[]) {
  await Promise.all(bundleNames.map(async (bundleName) => {
    const loader = featureLocaleLoaders[`./locales/${language}/${bundleName}.ts`]
    if (!loader)
      throw new Error(`Missing translation bundle: ${language}/${bundleName}`)
    const module = await loader()
    const resources = spreadFeatureBundleSet.has(bundleName)
      ? module.default
      : { [bundleName]: module.default }
    i18next.addResourceBundle(language, 'translation', resources, true, true)
  }))
}

i18next.on('languageChanged', (language) => {
  const normalized = normalizeLanguage(language)
  if (normalized && activeFeatureBundles.size > 0)
    void loadFeatureBundles(normalized, [...activeFeatureBundles])
})

export default i18next
