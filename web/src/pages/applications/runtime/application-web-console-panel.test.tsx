import type { Release } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ApplicationWebConsolePanel } from './application-web-console-panel'

vi.mock('@/components/common/runtime-terminal-panel', () => ({
  RuntimeTerminalPanel: ({ pod }: { pod?: string }) => <div>{`Terminal ${pod || 'auto'}`}</div>,
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return { ...actual, api: { ...actual.api, listDeploymentTargetRuntimePods: vi.fn() } }
})

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

describe('application web console panel', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
    vi.mocked(api.listDeploymentTargetRuntimePods).mockResolvedValue({
      items: [
        { name: 'api-7f9c-a', phase: 'Running', ready: true, terminating: false, restartCount: 0, releaseId: 'rel_123', containers: ['app', 'worker'], createdAt: '2026-09-06T00:00:00Z' },
        { name: 'api-7f9c-b', phase: 'Running', ready: false, terminating: false, restartCount: 4, releaseId: 'rel_old', containers: ['app', 'worker'], createdAt: '2026-09-05T00:00:00Z' },
      ],
      page: 1,
      pageSize: 100,
      sortBy: 'recommended',
      sortOrder: 'desc',
      total: 2,
      totalPages: 1,
      status: 'ready',
      observationCode: 'deployment_target.runtime_pods_ready',
      observedAt: '2026-09-06T00:00:00Z',
    })
  })

  it('tracks the selected container in the CLI command and toggles fullscreen', async () => {
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <TooltipProvider>
          <ApplicationWebConsolePanel projectId="prj_123" release={release} onClose={vi.fn()} />
        </TooltipProvider>
      </QueryClientProvider>,
    )

    expect(await screen.findByText('Terminal auto')).toBeInTheDocument()

    const command = screen.getByRole('group', { name: 'CLI command' })
    expect(command).toHaveTextContent(`luna deployment exec server=${window.location.origin} projectId=prj_123 applicationId=app_456 targetId=dplt_789`)

    const containerSelect = await screen.findByRole('combobox', { name: 'Container' })
    await userEvent.selectOptions(containerSelect, 'worker')
    expect(command).toHaveTextContent(`luna deployment exec server=${window.location.origin} projectId=prj_123 applicationId=app_456 targetId=dplt_789 container=worker`)

    await userEvent.click(screen.getByRole('button', { name: 'Replica' }))
    await userEvent.click(await screen.findByRole('button', { name: /api-7f9c-b.*Not ready.*Restarts: 4.*Other release/ }))
    expect(await screen.findByText('Terminal api-7f9c-b')).toBeInTheDocument()
    expect(command).toHaveTextContent(`targetId=dplt_789 pod=api-7f9c-b container=worker`)
    expect(api.listDeploymentTargetRuntimePods).toHaveBeenCalledWith('prj_123', 'app_456', 'dplt_789', { releaseId: 'rel_123', search: '' }, expect.any(AbortSignal))

    fireEvent.click(screen.getByRole('button', { name: 'Fullscreen' }))
    expect(screen.getByRole('button', { name: 'Exit fullscreen' })).toBeInTheDocument()
  })
})
