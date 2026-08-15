import type { RefObject } from 'react'
import { useCallback, useEffect, useRef } from 'react'

interface InfiniteLoadTriggerOptions {
  enabled: boolean
  loading: boolean
  observe?: boolean
  onLoad: () => Promise<unknown> | unknown
  rootRef: RefObject<Element | null>
  rootMargin?: string
}

/**
 * IntersectionObserver is an enhancement, not the only way to paginate. Call
 * `load` from an accessible button as a fallback for browsers and tests that do
 * not expose IntersectionObserver.
 */
export function useInfiniteLoadTrigger({ enabled, loading, observe = enabled, onLoad, rootRef, rootMargin = '240px 0px' }: InfiniteLoadTriggerOptions) {
  const sentinelRef = useRef<HTMLDivElement>(null)
  const pendingRef = useRef(false)
  const onLoadRef = useRef(onLoad)
  onLoadRef.current = onLoad

  const load = useCallback(async () => {
    if (!enabled || loading || pendingRef.current)
      return
    pendingRef.current = true
    try {
      await onLoadRef.current()
    }
    finally {
      pendingRef.current = false
    }
  }, [enabled, loading])

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!enabled || !observe || !sentinel || typeof IntersectionObserver === 'undefined')
      return
    const observer = new IntersectionObserver((entries) => {
      if (entries.some(entry => entry.isIntersecting))
        void load()
    }, { root: rootRef.current, rootMargin })
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [enabled, load, observe, rootMargin, rootRef])

  return { sentinelRef, load }
}
