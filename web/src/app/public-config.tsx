import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo } from 'react'
import { api } from '@/api'
import { applySiteBrandColorPreset } from './brand-theme'
import { defaultPublicConfigs, PublicConfigContext } from './public-config-context'

const publicConfigKeys = ['site.title', 'site.logoUrl', 'site.faviconUrl', 'site.loginSubtitle', 'site.brandColorPreset', 'billing.creditsDisplayName', 'billing.fiatCurrencyUnit', 'billing.creditsPerFiatUnit']

export function PublicConfigProvider({ children }: { children: ReactNode }) {
  const configs = useQuery({
    queryKey: ['public-configs'],
    queryFn: () => api.getPublicConfigs(publicConfigKeys),
  })

  const value = useMemo(() => ({ ...defaultPublicConfigs, ...(configs.data ?? {}) }), [configs.data])

  useEffect(() => {
    const faviconUrl = value['site.faviconUrl']
    if (!faviconUrl)
      return

    let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    if (!link) {
      link = document.createElement('link')
      link.rel = 'icon'
      document.head.appendChild(link)
    }
    link.href = faviconUrl
  }, [value])

  useEffect(() => {
    const preset = configs.data?.['site.brandColorPreset']
    if (preset)
      applySiteBrandColorPreset(preset)
  }, [configs.data])

  return <PublicConfigContext value={value}>{children}</PublicConfigContext>
}
