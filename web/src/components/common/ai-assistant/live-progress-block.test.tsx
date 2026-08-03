import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { LiveProgressBlock } from './live-progress-block'

class EventSourceMock extends EventTarget {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2
  static readonly instances: EventSourceMock[] = []

  readonly CONNECTING = EventSourceMock.CONNECTING
  readonly OPEN = EventSourceMock.OPEN
  readonly CLOSED = EventSourceMock.CLOSED
  readonly url: string
  readonly withCredentials: boolean
  readyState = EventSourceMock.CONNECTING

  constructor(url: string | URL, options?: EventSourceInit) {
    super()
    this.url = String(url)
    this.withCredentials = options?.withCredentials ?? false
    EventSourceMock.instances.push(this)
  }

  close() {
    this.readyState = EventSourceMock.CLOSED
  }
}

beforeAll(async () => {
  await i18next.changeLanguage('zh-CN')
})

beforeEach(() => {
  EventSourceMock.instances.length = 0
  vi.stubGlobal('EventSource', EventSourceMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('live progress block', () => {
  it('reconciles authoritative snapshots and closes after the terminal event', async () => {
    const { container } = render(
      <LiveProgressBlock
        block={{
          id: 'release-progress',
          type: 'live_progress',
          binding: { operationType: 'release', projectId: 'prj_1', operationId: 'rel_1' },
        }}
      />,
    )

    expect(EventSourceMock.instances).toHaveLength(1)
    const source = EventSourceMock.instances[0]!
    expect(source.url).toContain('/ai/progress/projects/prj_1/release/rel_1/stream')
    expect(source.withCredentials).toBe(true)

    await act(async () => {
      source.dispatchEvent(new MessageEvent('progress.snapshot', { data: JSON.stringify(snapshot(1, 'running')) }))
    })
    expect(container.querySelector('[data-ai-live-progress="running"]')).not.toBeNull()
    expect(screen.getAllByText('正在执行发布')).toHaveLength(2)
    expect(source.readyState).not.toBe(EventSourceMock.CLOSED)

    await act(async () => {
      source.dispatchEvent(new MessageEvent('progress.snapshot', { data: JSON.stringify(snapshot(2, 'succeeded')) }))
    })
    expect(container.querySelector('[data-ai-live-progress="succeeded"]')).not.toBeNull()
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '100')
    expect(source.readyState).toBe(EventSourceMock.CLOSED)
  })

  it('ignores snapshots older than the latest observed revision', async () => {
    const { container } = render(
      <LiveProgressBlock
        block={{
          id: 'build-progress',
          type: 'live_progress',
          binding: { operationType: 'build_run', projectId: 'prj_1', operationId: 'bldrun_1' },
        }}
      />,
    )
    const source = EventSourceMock.instances[0]!

    await act(async () => {
      source.dispatchEvent(new MessageEvent('progress.snapshot', { data: JSON.stringify(snapshot(4, 'running')) }))
      source.dispatchEvent(new MessageEvent('progress.snapshot', { data: JSON.stringify(snapshot(3, 'queued')) }))
    })

    expect(container.querySelector('[data-ai-live-progress="running"]')).not.toBeNull()
  })
})

function snapshot(sequence: number, state: 'queued' | 'running' | 'succeeded') {
  return {
    operationId: 'rel_1',
    operationType: 'release',
    revision: `19700101T0000${String(sequence).padStart(2, '0')}.000000000Z`,
    state,
    stageCode: `ai.progress.release.${state}`,
    progress: state === 'succeeded' ? { mode: 'determinate', value: 100 } : { mode: 'indeterminate' },
    steps: [
      { id: 'queued', labelCode: 'ai.progress.release.queued', status: state === 'queued' ? 'running' : 'success' },
      { id: 'running', labelCode: 'ai.progress.release.running', status: state === 'running' ? 'running' : state === 'succeeded' ? 'success' : 'pending' },
      { id: 'completed', labelCode: 'ai.progress.release.succeeded', status: state === 'succeeded' ? 'success' : 'pending' },
    ],
    updatedAt: new Date(sequence * 1_000).toISOString(),
  }
}
