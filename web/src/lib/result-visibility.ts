import type { ResultVisibility } from '@/api'

export function parseResultVisibility(value: string | null | undefined): ResultVisibility {
  return value === 'all' ? 'all' : 'related'
}

export function withResultVisibility(href: string, visibility: ResultVisibility) {
  const url = new URL(href, 'https://luna.devops.local')
  url.searchParams.set('visibility', visibility)
  return `${url.pathname}${url.search}${url.hash}`
}
