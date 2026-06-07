import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'

import enUS from './locales/en-US'
import zhCN from './locales/zh-CN'

const resources = {
  'zh-CN': { translation: zhCN },
  'en-US': { translation: enUS },
}

i18next.use(initReactI18next).init({
  lng: 'zh-CN',
  fallbackLng: 'zh-CN',
  interpolation: {
    escapeValue: false,
  },
  resources,
})

export default i18next
