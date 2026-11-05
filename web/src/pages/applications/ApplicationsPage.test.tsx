import type { Application } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { ApplicationsPage } from './ApplicationsPage'
import { ApplicationRuntimeStatus } from './overview/application-summary'

const mocks = vi.hoisted(() => ({
  listApplicationsPage: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listApplicationsPage: mocks.listApplicationsPage,
    },
  }
})

const application: Application = {
  id: 'app-1',
  projectId: 'project-1',
  identifier: 'example',
  name: '示例应用',
  icon: 'box',
  deleteStatus: 'active',
  deleteMessage: '',
  createdAt: '2026-08-18T00:00:00Z',
  updatedAt: '2026-08-18T00:00:00Z',
}

describe('application runtime status', () => {
  beforeAll(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('shows runtime status separately for each deployment stage', () => {
    render(
      <ApplicationRuntimeStatus
        application={{
          ...application,
          deploymentSummary: {
            targetCount: 2,
            desiredReplicas: 3,
            readyReplicas: 2,
            status: 'degraded',
            targets: [
              { id: 'prod', stage: 'prod', desiredReplicas: 2, readyReplicas: 1, status: 'degraded' },
              { id: 'dev', stage: 'dev', desiredReplicas: 1, readyReplicas: 1, status: 'ready' },
            ],
          },
        }}
      />,
    )

    expect(screen.getByText('生产')).toBeVisible()
    expect(screen.getByText('异常').closest('[data-slot="status-dot"]')).toHaveAttribute('data-tone', 'warning')
    expect(screen.getByText('1/2')).toBeVisible()
    expect(screen.getByText('开发')).toBeVisible()
    expect(screen.getByText('就绪').closest('[data-slot="status-dot"]')).toHaveAttribute('data-tone', 'success')
    expect(screen.getByText('1/1')).toBeVisible()
    expect(screen.queryByText('2/3')).not.toBeInTheDocument()
  })

  it('keeps the undeployed state when the application has no deployment target', () => {
    render(
      <ApplicationRuntimeStatus
        application={{
          ...application,
          deploymentSummary: { targetCount: 0, desiredReplicas: 0, readyReplicas: 0, status: 'not-deployed', targets: [] },
        }}
      />,
    )

    expect(screen.getByText('未部署').closest('[data-slot="status-dot"]')).toHaveAttribute('data-tone', 'neutral')
    expect(screen.queryByText('0/0')).not.toBeInTheDocument()
  })
})

describe('applications page', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.listApplicationsPage.mockResolvedValue({
      items: [{
        ...application,
        name: 'Example',
        deploymentSummary: {
          targetCount: 1,
          desiredReplicas: 2,
          readyReplicas: 2,
          status: 'ready',
          targets: [{ id: 'prod', stage: 'prod', desiredReplicas: 2, readyReplicas: 2, status: 'ready' }],
        },
      }],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })
  })

  it('opens an application from its row and keeps one visible action per row', async () => {
    const user = userEvent.setup()
    renderApplicationsPage()

    const row = await screen.findByRole('row', { name: 'Open application Example' })
    expect(within(row).getByRole('link', { name: 'Example' })).toHaveAttribute('href', '/projects/project-1/apps/app-1')
    expect(within(row).getByRole('link', { name: 'Deploy' })).toHaveAttribute('href', '/projects/project-1/apps/app-1?tab=deployments')
    expect(within(row).getByRole('button', { name: 'More actions for Example' })).toBeVisible()
    expect(within(row).queryByText('Open details')).not.toBeInTheDocument()
    expect(mocks.listApplicationsPage).toHaveBeenCalledWith('project-1', expect.objectContaining({ includeRuntime: true }))

    await user.click(within(row).getByText('example'))
    expect(await screen.findByText('Application detail route')).toBeInTheDocument()
  })
})

function renderApplicationsPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/projects/project-1']}>
        <Routes>
          <Route element={<ApplicationsPage projectId="project-1" />} path="/projects/:projectId" />
          <Route element={<div>Application detail route</div>} path="/projects/:projectId/apps/:applicationId" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}
