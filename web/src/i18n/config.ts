export const supportedLanguages = ['zh-CN', 'zh-TW', 'en-US', 'ja-JP', 'ko-KR'] as const

export type SupportedLanguage = typeof supportedLanguages[number]

export const coreTranslationBundles = [
  'root',
  'languages',
  'common',
  'time',
  'errors',
  'auth',
  'pagination',
  'oauthApps',
  'theme',
  'loginPage',
  'bootstrap',
] as const

export const spreadFeatureBundles = ['aiAssistant', 'inbox'] as const
