import { createContext, use } from 'react'

export const defaultPublicConfigs: Record<string, string> = {
  'billing.creditsDisplayName': 'Credits',
  'billing.creditsPerFiatUnit': '1000',
  'billing.fiatCurrencyUnit': 'CNY',
  'site.title': 'Luna DevOps',
  'site.logoUrl': '/luna-devops-logo.svg',
  'site.faviconUrl': '/luna-devops-logo.svg',
  'site.brandColorPreset': 'blue',
  'site.minimalModeDefault': 'false',
}

export const PublicConfigContext = createContext(defaultPublicConfigs)

export function usePublicConfig() {
  return use(PublicConfigContext)
}
