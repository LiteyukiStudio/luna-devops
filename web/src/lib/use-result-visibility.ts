import type { ResultVisibility } from '@/api'
import { useSearchParams } from 'react-router-dom'
import { parseResultVisibility } from '@/lib/result-visibility'

export function useResultVisibility(canViewAll: boolean) {
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedVisibility = parseResultVisibility(searchParams.get('visibility'))
  const visibility: ResultVisibility = canViewAll ? requestedVisibility : 'related'

  const updateVisibility = (nextVisibility: ResultVisibility) => {
    const nextSearchParams = new URLSearchParams(searchParams)
    if (nextVisibility === 'related')
      nextSearchParams.delete('visibility')
    else
      nextSearchParams.set('visibility', nextVisibility)
    setSearchParams(nextSearchParams, { replace: true })
  }

  return [visibility, updateVisibility] as const
}
