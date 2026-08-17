import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AppUpdateChecker } from './app-update-checker'
import '@/i18n'

const { getAPIMeta, toastInfo } = vi.hoisted(() => ({
  getAPIMeta: vi.fn(),
  toastInfo: vi.fn(),
}))

vi.mock('@/api', () => ({
  api: { getAPIMeta },
}))

vi.mock('sonner', () => ({
  toast: { info: toastInfo },
}))

describe('app update checker', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    getAPIMeta.mockReset()
    toastInfo.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('does not show a toast when versions match', async () => {
    getAPIMeta.mockResolvedValue({ serverVersion: 'commit-1' })

    render(<AppUpdateChecker commitSha="commit-1" enabled />)
    await act(async () => {})

    expect(getAPIMeta).toHaveBeenCalledOnce()
    expect(toastInfo).not.toHaveBeenCalled()
  })

  it('shows one persistent toast when the server has a newer version', async () => {
    getAPIMeta.mockResolvedValue({ serverVersion: 'commit-2' })

    render(<AppUpdateChecker commitSha="commit-1" enabled />)
    await act(async () => {})

    expect(toastInfo).toHaveBeenCalledOnce()
    expect(toastInfo).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({
        dismissible: false,
        duration: Infinity,
        id: 'app-update-available',
      }),
    )
  })

  it('does not create another toast during repeated polling', async () => {
    getAPIMeta.mockResolvedValue({ serverVersion: 'commit-2' })

    render(<AppUpdateChecker commitSha="commit-1" enabled />)
    await act(async () => {})
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000)
    })

    expect(getAPIMeta).toHaveBeenCalledTimes(2)
    expect(toastInfo).toHaveBeenCalledOnce()
  })

  it('reloads the page from the toast action', async () => {
    getAPIMeta.mockResolvedValue({ serverVersion: 'commit-2' })
    const reload = vi.fn()
    const testWindow = Object.create(window) as Window & typeof globalThis
    Object.defineProperty(testWindow, 'location', { configurable: true, value: { reload } })
    vi.stubGlobal('window', testWindow)

    render(<AppUpdateChecker commitSha="commit-1" enabled />)
    await act(async () => {})

    const options = toastInfo.mock.calls[0]?.[1]
    if (!options || typeof options !== 'object' || !('action' in options) || !options.action || typeof options.action !== 'object' || !('onClick' in options.action) || typeof options.action.onClick !== 'function')
      throw new Error('update toast action is missing')
    options.action.onClick()

    expect(reload).toHaveBeenCalledOnce()
  })
})
