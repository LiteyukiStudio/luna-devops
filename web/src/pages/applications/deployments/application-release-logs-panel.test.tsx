import type { Release } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '@/api'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'
import { ApplicationReleaseLogsPanel } from './application-release-logs-panel'

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getReleaseLogs: vi.fn(),
      getReleaseRuntimeLogs: vi.fn(),
      listDeploymentTargetRuntimePods: vi.fn(),
    },
  }
})

const getReleaseLogs = vi.mocked(api.getReleaseLogs)
const getReleaseRuntimeLogs = vi.mocked(api.getReleaseRuntimeLogs)
const listDeploymentTargetRuntimePods = vi.mocked(api.listDeploymentTargetRuntimePods)
const runtimePodPage = {
  items: [
    { name: 'app-abc', phase: 'Running', ready: true, terminating: false, restartCount: 0, releaseId: 'rel_123', containers: ['app'], createdAt: '2026-09-06T00:00:00Z' },
    { name: 'app-def', phase: 'Running', ready: false, terminating: false, restartCount: 3, releaseId: 'rel_123', containers: ['app'], createdAt: '2026-09-05T00:00:00Z' },
  ],
  page: 1,
  pageSize: 100,
  sortBy: 'recommended',
  sortOrder: 'desc' as const,
  total: 2,
  totalPages: 1,
  status: 'ready' as const,
  observationCode: 'deployment_target.runtime_pods_ready',
  observedAt: '2026-09-06T00:00:00Z',
}
const release: Release = {
  id: 'rel_123',
  projectId: 'prj_123',
  applicationId: 'app_456',
  deploymentTargetId: 'dplt_789',
  buildRunId: 'build_123',
  imageRef: 'registry.example.com/app:latest',
  forceImagePull: false,
  type: 'deploy',
  status: 'succeeded',
  failureCode: '',
  revision: 1,
  rollbackFromId: '',
  message: '',
  createdBy: 'usr_123',
  createdAt: '2026-09-06T00:00:00Z',
}

function renderPanel(onClose = vi.fn(), initialView?: 'deployment' | 'runtime') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <ApplicationReleaseLogsPanel
          initialView={initialView}
          projectId="prj_123"
          release={release}
          onClose={onClose}
        />
      </TooltipProvider>
    </QueryClientProvider>,
  )
  return onClose
}

describe('application release logs panel', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
    getReleaseLogs.mockResolvedValue({
      id: 'log_123',
      releaseId: release.id,
      projectId: release.projectId,
      content: 'deployment complete',
      createdAt: release.createdAt,
      updatedAt: release.createdAt,
    })
    getReleaseRuntimeLogs.mockResolvedValue({ pod: 'app-abc', container: 'app', content: 'server ready' })
    listDeploymentTargetRuntimePods.mockResolvedValue(runtimePodPage)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('orders Runtime logs before Deploy logs and loads runtime logs by default without fetching deployment logs', async () => {
    renderPanel()

    const tabs = screen.getAllByRole('tab')
    expect(tabs.map(tab => tab.textContent)).toEqual(['Runtime logs', 'Deploy logs'])
    expect(tabs[0]).toHaveAttribute('aria-selected', 'true')
    expect(await screen.findByText('server ready')).toBeInTheDocument()
    expect(screen.getByText('Pod app-abc / container app')).toBeInTheDocument()
    expect(getReleaseRuntimeLogs).toHaveBeenCalledWith('prj_123', release.id, { tailLines: 500 }, expect.any(AbortSignal))
    expect(getReleaseLogs).not.toHaveBeenCalled()
  })

  it('loads deployment logs only after selecting the Deploy logs tab', async () => {
    const user = userEvent.setup()
    renderPanel()

    await user.click(screen.getByRole('tab', { name: 'Deploy logs' }))

    expect(await screen.findByText('deployment complete')).toBeInTheDocument()
    expect(getReleaseLogs).toHaveBeenCalledWith('prj_123', release.id, expect.any(AbortSignal))
  })

  it('reloads runtime logs from the replica the user selects', async () => {
    const user = userEvent.setup()
    renderPanel()

    expect(await screen.findByText('server ready')).toBeInTheDocument()

    getReleaseRuntimeLogs.mockResolvedValue({ pod: 'app-def', container: 'app', content: 'panic: config missing' })
    await user.click(screen.getByRole('button', { name: 'Replica' }))
    await user.click(await screen.findByRole('button', { name: /app-def.*Not ready.*Restarts: 3.*This release/ }))

    expect(await screen.findByText('panic: config missing')).toBeInTheDocument()
    expect(screen.getByText('Pod app-def / container app')).toBeInTheDocument()
    expect(getReleaseRuntimeLogs).toHaveBeenLastCalledWith('prj_123', release.id, { pod: 'app-def', tailLines: 500 }, expect.any(AbortSignal))
  })

  it('asks for another replica when the selected one is gone', async () => {
    const user = userEvent.setup()
    renderPanel()

    await user.click(await screen.findByRole('button', { name: 'Replica' }))
    getReleaseRuntimeLogs.mockRejectedValue(new ApiError('runtime pod not found', { code: 'runtime.pod_not_found', path: '/runtime-logs', status: 404 }))
    listDeploymentTargetRuntimePods.mockResolvedValue({ ...runtimePodPage, items: [runtimePodPage.items[0]], total: 1 })
    await user.click(await screen.findByRole('button', { name: /app-def/ }))

    expect((await screen.findAllByText('The selected replica no longer exists. Choose another one.')).length).toBeGreaterThanOrEqual(1)
    expect(screen.queryByText('server ready')).not.toBeInTheDocument()
  })

  it('shows unavailable instead of stale-looking log content when loading fails', async () => {
    getReleaseRuntimeLogs.mockRejectedValue(new ApiError('upstream unavailable', { code: 'runtime.unavailable', path: '/runtime-logs', status: 422 }))
    const { unmount } = render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <TooltipProvider>
          <ApplicationReleaseLogsPanel projectId="prj_123" release={release} onClose={vi.fn()} />
        </TooltipProvider>
      </QueryClientProvider>,
    )

    expect((await screen.findAllByText('Unavailable')).length).toBeGreaterThanOrEqual(1)
    expect(screen.queryByText('server ready')).not.toBeInTheDocument()
    unmount()

    getReleaseLogs.mockRejectedValue(new Error('upstream unavailable'))
    renderPanel(vi.fn(), 'deployment')

    expect((await screen.findAllByText('Unavailable')).length).toBeGreaterThanOrEqual(1)
    expect(screen.queryByText('deployment complete')).not.toBeInTheDocument()
  })

  it('keeps refreshing runtime logs after the release has succeeded', async () => {
    vi.useFakeTimers()
    renderPanel()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(getReleaseRuntimeLogs).toHaveBeenCalledOnce()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(WORKFLOW_STATUS_REFETCH_INTERVAL_MS)
    })
    expect(getReleaseRuntimeLogs.mock.calls.length).toBeGreaterThanOrEqual(2)
  })

  it('supports switching to previous crash logs and handles missing previous logs', async () => {
    const user = userEvent.setup()
    renderPanel()

    expect(await screen.findByText('server ready')).toBeInTheDocument()

    getReleaseRuntimeLogs.mockResolvedValueOnce({ pod: 'app-def', container: 'app', content: 'fatal error: out of memory', previous: true })
    await user.click(screen.getByRole('button', { name: 'View previous crash logs' }))

    expect(await screen.findByText('fatal error: out of memory')).toBeInTheDocument()
    expect(getReleaseRuntimeLogs).toHaveBeenLastCalledWith('prj_123', release.id, { pod: 'app-def', previous: true, tailLines: 500 }, expect.any(AbortSignal))

    getReleaseRuntimeLogs.mockRejectedValueOnce(new ApiError('previous log not found', { code: 'runtime.previous_log_not_found', path: '/runtime-logs', status: 409 }))
    await user.click(screen.getByRole('button', { name: 'Timestamps' }))

    expect(await screen.findByText('No previous crash logs were found for this container.')).toBeInTheDocument()
  })
})
