import { useState } from 'react'

const storageKey = 'luna-devops.ai.internal-tools.visible-users'

export function useAIToolDebugMode(userId: string | undefined, allowed: boolean) {
  const [enabledUserIds, setEnabledUserIds] = useState(readEnabledUserIds)
  const enabled = Boolean(allowed && userId && enabledUserIds.has(userId))
  const toggle = () => {
    if (!allowed || !userId)
      return
    setEnabledUserIds((current) => {
      const next = new Set(current)
      if (next.has(userId))
        next.delete(userId)
      else
        next.add(userId)
      writeEnabledUserIds(next)
      return next
    })
  }
  return { enabled, toggle }
}

function readEnabledUserIds(): Set<string> {
  if (typeof window === 'undefined')
    return new Set()
  try {
    const value: unknown = JSON.parse(window.localStorage.getItem(storageKey) ?? '[]')
    return new Set(Array.isArray(value) ? value.filter(item => typeof item === 'string') : [])
  }
  catch {
    return new Set()
  }
}

function writeEnabledUserIds(userIds: Set<string>) {
  window.localStorage.setItem(storageKey, JSON.stringify([...userIds].sort()))
}
