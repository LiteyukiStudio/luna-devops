import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  AI_ASSISTANT_FINE_POINTER_MEDIA_QUERY,
  AI_ASSISTANT_HOVER_MEDIA_QUERY,
  canUseAIAssistantDesktopWindow,
  resolveAIAssistantPresentationMode,
  useAIAssistantPresentationMode,
} from './presentation-mode'

const originalMatchMedia = window.matchMedia
const originalInnerWidth = window.innerWidth
const originalInnerHeight = window.innerHeight

afterEach(() => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: originalInnerWidth })
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: originalInnerHeight })
  Object.defineProperty(window, 'matchMedia', { configurable: true, value: originalMatchMedia })
})

describe('ai assistant presentation mode', () => {
  it('allows a desktop window only at the complete capability boundary', () => {
    expect(canUseAIAssistantDesktopWindow({
      viewportWidth: 1024,
      viewportHeight: 600,
      finePointer: true,
      hover: true,
    })).toBe(true)

    const unavailable = [
      { viewportWidth: 1023, viewportHeight: 600, finePointer: true, hover: true },
      { viewportWidth: 1024, viewportHeight: 599, finePointer: true, hover: true },
      { viewportWidth: 1024, viewportHeight: 600, finePointer: false, hover: true },
      { viewportWidth: 1024, viewportHeight: 600, finePointer: true, hover: false },
    ]
    unavailable.forEach(environment => expect(canUseAIAssistantDesktopWindow(environment)).toBe(false))
  })

  it('forces the assistant route to page mode on a desktop-capable device', () => {
    const desktop = {
      viewportWidth: 1440,
      viewportHeight: 900,
      finePointer: true,
      hover: true,
    }
    expect(resolveAIAssistantPresentationMode({ ...desktop, pathname: '/dashboard' })).toBe('window')
    expect(resolveAIAssistantPresentationMode({ ...desktop, pathname: '/ai-assistant' })).toBe('page')
    expect(resolveAIAssistantPresentationMode({ ...desktop, pathname: '/ai-assistant/' })).toBe('page')
  })

  it('reacts to viewport and input capability changes through the shared resolver', () => {
    const media = installPresentationEnvironment({
      viewportWidth: 1440,
      viewportHeight: 900,
      finePointer: true,
      hover: true,
    })
    const { result } = renderHook(() => useAIAssistantPresentationMode('/dashboard'))
    expect(result.current).toBe('window')

    act(() => media.resize(1023, 900))
    expect(result.current).toBe('page')

    act(() => media.resize(1280, 800))
    expect(result.current).toBe('window')

    act(() => media.setFinePointer(false))
    expect(result.current).toBe('page')

    act(() => media.setFinePointer(true))
    act(() => media.setHover(false))
    expect(result.current).toBe('page')
  })

  it('updates to page mode when routing to the assistant', () => {
    installPresentationEnvironment({
      viewportWidth: 1440,
      viewportHeight: 900,
      finePointer: true,
      hover: true,
    })
    const { result, rerender } = renderHook(
      ({ pathname }) => useAIAssistantPresentationMode(pathname),
      { initialProps: { pathname: '/dashboard' } },
    )
    expect(result.current).toBe('window')

    rerender({ pathname: '/ai-assistant' })
    expect(result.current).toBe('page')
  })
})

interface PresentationEnvironmentMock {
  viewportWidth: number
  viewportHeight: number
  finePointer: boolean
  hover: boolean
}

function installPresentationEnvironment(initial: PresentationEnvironmentMock) {
  const environment = { ...initial }
  const listeners = new Map<string, Set<() => void>>()

  const matches = (query: string) => {
    if (query === AI_ASSISTANT_FINE_POINTER_MEDIA_QUERY)
      return environment.finePointer
    if (query === AI_ASSISTANT_HOVER_MEDIA_QUERY)
      return environment.hover
    return false
  }

  Object.defineProperty(window, 'innerWidth', { configurable: true, get: () => environment.viewportWidth })
  Object.defineProperty(window, 'innerHeight', { configurable: true, get: () => environment.viewportHeight })
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: vi.fn((query: string) => ({
      get matches() {
        return matches(query)
      },
      media: query,
      onchange: null,
      addEventListener: (_type: string, listener: () => void) => {
        const queryListeners = listeners.get(query) ?? new Set()
        queryListeners.add(listener)
        listeners.set(query, queryListeners)
      },
      removeEventListener: (_type: string, listener: () => void) => listeners.get(query)?.delete(listener),
    })),
  })

  return {
    resize: (viewportWidth: number, viewportHeight: number) => {
      environment.viewportWidth = viewportWidth
      environment.viewportHeight = viewportHeight
      window.dispatchEvent(new Event('resize'))
    },
    setFinePointer: (finePointer: boolean) => {
      environment.finePointer = finePointer
      listeners.get(AI_ASSISTANT_FINE_POINTER_MEDIA_QUERY)?.forEach(listener => listener())
    },
    setHover: (hover: boolean) => {
      environment.hover = hover
      listeners.get(AI_ASSISTANT_HOVER_MEDIA_QUERY)?.forEach(listener => listener())
    },
  }
}
