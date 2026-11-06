import { useCallback, useEffect, useRef } from 'react'
import { useBlocker } from 'react-router-dom'

/**
 * 表单存在未保存修改时拦截离开：站内导航交给 useBlocker 由页面弹确认，刷新或关闭标签页走浏览器的
 * beforeunload 提示。保存成功或用户主动取消后调用 allowNextNavigation，紧随其后的跳转不再拦截。
 */
export function useUnsavedChangesGuard(dirty: boolean) {
  const bypassRef = useRef(false)
  const blocker = useBlocker(({ currentLocation, nextLocation }) => dirty
    && !bypassRef.current
    && (currentLocation.pathname !== nextLocation.pathname || currentLocation.search !== nextLocation.search))

  useEffect(() => {
    if (!dirty)
      return
    const warnBeforeUnload = (event: BeforeUnloadEvent) => {
      if (bypassRef.current)
        return
      event.preventDefault()
    }
    window.addEventListener('beforeunload', warnBeforeUnload)
    return () => window.removeEventListener('beforeunload', warnBeforeUnload)
  }, [dirty])

  const allowNextNavigation = useCallback(() => {
    bypassRef.current = true
  }, [])

  return { allowNextNavigation, blocker }
}
