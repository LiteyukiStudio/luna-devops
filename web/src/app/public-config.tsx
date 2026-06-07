import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo } from 'react'
import { api } from '@/api/client'
import { defaultPublicConfigs, PublicConfigContext } from './public-config-context'

const publicConfigKeys = ['site.title', 'site.logoUrl', 'site.faviconUrl', 'site.loginSubtitle']

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

  return <PublicConfigContext value={value}>{children}</PublicConfigContext>
}
