import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'

import enUS from './locales/en-US'
import jaJP from './locales/ja-JP'
import koKR from './locales/ko-KR'
import zhCN from './locales/zh-CN'
import zhTW from './locales/zh-TW'

const resources = {
  'zh-CN': { translation: zhCN },
  'zh-TW': { translation: zhTW },
  'en-US': { translation: enUS },
  'ja-JP': { translation: jaJP },
  'ko-KR': { translation: koKR },
}

type SupportedLanguage = 'zh-CN' | 'zh-TW' | 'en-US' | 'ja-JP' | 'ko-KR'

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

i18next.use(initReactI18next).init({
  lng: detectBrowserLanguage(),
  fallbackLng: 'zh-CN',
  interpolation: {
    escapeValue: false,
  },
  resources,
})

export default i18next
