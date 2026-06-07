import { useEffect } from 'react'
import { defaultPublicConfigs, usePublicConfig } from '@/app/public-config-context'

export function formatDocumentTitle(pageTitle: string, siteTitle: string) {
  const normalizedPageTitle = pageTitle.trim()
  const normalizedSiteTitle = siteTitle.trim() || defaultPublicConfigs['site.title']

  if (!normalizedPageTitle || normalizedPageTitle === normalizedSiteTitle)
    return normalizedSiteTitle

  return `${normalizedPageTitle} - ${normalizedSiteTitle}`
}

export function useDocumentTitle(pageTitle: string) {
  const configs = usePublicConfig()
  const siteTitle = configs['site.title'] || defaultPublicConfigs['site.title']

  useEffect(() => {
    document.title = formatDocumentTitle(pageTitle, siteTitle)
  }, [pageTitle, siteTitle])
}
