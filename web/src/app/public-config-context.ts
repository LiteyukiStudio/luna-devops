import { createContext, use } from 'react'
import i18next from '@/i18n'

export const defaultPublicConfigs: Record<string, string> = {
  'site.title': 'Liteyuki DevOps',
  'site.logoUrl': '/liteyuki-logo.svg',
  'site.faviconUrl': '/liteyuki-logo.svg',
  'site.loginSubtitle': i18next.t('loginPage.subtitle'),
}

export const PublicConfigContext = createContext(defaultPublicConfigs)

export function usePublicConfig() {
  return use(PublicConfigContext)
}
