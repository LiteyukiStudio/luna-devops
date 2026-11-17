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

  it('localizes real failure codes, hides internal fallback code, localizes prompt task type, and locks diagnose after one trigger', async () => {
    const onAction = vi.fn(async () => true)
    const { container } = render(
      <LiveProgressBlock
        block={{
          id: 'build-failed-progress',
          type: 'live_progress',
          binding: { operationType: 'build_run', projectId: 'prj_demo', operationId: 'bldrun_42' },
        }}
        onAction={onAction}
      />,
    )
    const source = EventSourceMock.instances[0]!

    await act(async () => {
      source.dispatchEvent(new MessageEvent('progress.snapshot', {
        data: JSON.stringify({
          operationId: 'bldrun_42',
          operationType: 'build_run',
          revision: '19700101T000010.000000000Z',
          state: 'failed',
          stageCode: 'ai.progress.build.failed',
          progress: { mode: 'indeterminate' },
          steps: [
            { id: 'queued', labelCode: 'ai.progress.build.queued', status: 'success' },
            { id: 'running', labelCode: 'ai.progress.build.running', status: 'error' },
          ],
          updatedAt: new Date(10_000).toISOString(),
          finishedAt: new Date(10_000).toISOString(),
          error: { code: 'job_failed' },
        }),
      }))
    })

    expect(container.querySelector('[data-ai-live-progress="failed"]')).not.toBeNull()
    expect(screen.getByText('构建执行失败')).toBeInTheDocument()
    expect(screen.queryByText('job_failed')).not.toBeInTheDocument()
    const diagnoseButton = screen.getByRole('button', { name: '排查失败原因' })
    expect(diagnoseButton).toBeEnabled()

    await act(async () => {
      diagnoseButton.click()
    })
    expect(onAction).toHaveBeenCalledTimes(1)
    expect(onAction).toHaveBeenCalledWith(expect.objectContaining({
      version: 1,
      type: 'send_message',
      payload: expect.objectContaining({
        message: expect.stringContaining('构建任务（bldrun_42）'),
      }),
    }))
    // 防止连点重发：成功触发一次后按钮置为 disabled
    expect(diagnoseButton).toBeDisabled()
    await act(async () => {
      diagnoseButton.click()
    })
    expect(onAction).toHaveBeenCalledTimes(1)
  })

  it('never renders internal fallback ai.progress.operation_failed raw and respects actionDisabled', async () => {
    render(
      <LiveProgressBlock
        actionDisabled
        block={{
          id: 'release-fallback-failed',
          type: 'live_progress',
          binding: { operationType: 'release', projectId: 'prj_demo', operationId: 'rel_77' },
        }}
        onAction={vi.fn(async () => true)}
      />,
    )
    const source = EventSourceMock.instances[0]!
    await act(async () => {
      source.dispatchEvent(new MessageEvent('progress.snapshot', {
        data: JSON.stringify({
          operationId: 'rel_77',
          operationType: 'release',
          revision: '19700101T000011.000000000Z',
          state: 'failed',
          stageCode: 'ai.progress.release.failed',
          progress: { mode: 'indeterminate' },
          steps: [],
          updatedAt: new Date(11_000).toISOString(),
          finishedAt: new Date(11_000).toISOString(),
          error: { code: 'ai.progress.operation_failed' },
        }),
      }))
    })
    expect(screen.queryByText('ai.progress.operation_failed')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '排查失败原因' })).toBeDisabled()
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
