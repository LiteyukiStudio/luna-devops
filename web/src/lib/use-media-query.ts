import { useSyncExternalStore } from 'react'

/** 订阅 CSS 媒体查询结果；服务端或不支持 matchMedia 的环境返回 false。 */
export function useMediaQuery(query: string) {
  return useSyncExternalStore(
    (callback) => {
      if (typeof window.matchMedia !== 'function')
        return () => undefined
      const media = window.matchMedia(query)
      media.addEventListener('change', callback)
      return () => media.removeEventListener('change', callback)
    },
    () => typeof window.matchMedia === 'function' && window.matchMedia(query).matches,
    () => false,
  )
}
