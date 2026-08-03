import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { useAIToolDebugMode } from './tool-debug-mode'

describe('ai tool debug mode', () => {
  beforeEach(() => window.localStorage.clear())

  it('persists the preference per real administrator account', () => {
    const { result, unmount } = renderHook(() => useAIToolDebugMode('usr_admin', true))
    expect(result.current.enabled).toBe(false)
    act(() => result.current.toggle())
    expect(result.current.enabled).toBe(true)
    unmount()

    const restored = renderHook(() => useAIToolDebugMode('usr_admin', true))
    expect(restored.result.current.enabled).toBe(true)
    const otherUser = renderHook(() => useAIToolDebugMode('usr_other', true))
    expect(otherUser.result.current.enabled).toBe(false)
  })

  it('cannot be enabled for a non-administrator even with a stored preference', () => {
    window.localStorage.setItem('luna-devops.ai.internal-tools.visible-users', JSON.stringify(['usr_member']))
    const { result } = renderHook(() => useAIToolDebugMode('usr_member', false))
    expect(result.current.enabled).toBe(false)
    act(() => result.current.toggle())
    expect(result.current.enabled).toBe(false)
  })
})
