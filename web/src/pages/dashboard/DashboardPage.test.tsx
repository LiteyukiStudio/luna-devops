import type { Application, DashboardOverview } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { DashboardPage } from './DashboardPage'

const mocks = vi.hoisted(() => ({
  getDashboard: vi.fn(),
  listApplicationsPage: vi.fn(),
  role: 'user',
}))

vi.mock('@/app/session-context', () => ({
  useSession: () => ({ user: { id: 'usr_1', role: mocks.role } }),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getDashboard: mocks.getDashboard,
      listApplicationsPage: mocks.listApplicationsPage,
    },
  }
})

describe('dashboard page', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    mocks.role = 'user'
    await i18next.changeLanguage('en-US')
    mocks.getDashboard.mockResolvedValue(dashboardOverviewFixture())
    mocks.listApplicationsPage.mockResolvedValue({
      items: [applicationFixture()],
      page: 1,
      pageSize: 20,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })
  })

  it('puts failures that need action first with a direct link to the build', async () => {
    renderPage()

    const attention = await screen.findByText('Needs attention')
    const section = attention.closest('[data-slot="surface"]') as HTMLElement
    expect(within(section).getByText('2 consecutive')).toBeVisible()
    expect(within(section).getByRole('link', { name: 'View build' })).toHaveAttribute('href', '/projects/prj_1/apps/app_1?tab=builds&visibility=related')
    expect(screen.queryByText('Active builds')).not.toBeInTheDocument()
    expect(screen.queryByText('Healthy clusters')).not.toBeInTheDocument()
  })

  it('lists recent applications with live status from the existing application API', async () => {
    const user = userEvent.setup()
    renderPage()

    const table = await screen.findByRole('table')
    expect(await within(table).findByRole('link', { name: 'API' })).toHaveAttribute('href', '/projects/prj_1/apps/app_1')
    expect(within(table).getByText('Example project')).toBeVisible()
    expect(within(table).getByText('Running')).toBeVisible()
    expect(within(table).getByText('2/2')).toBeVisible()
    expect(mocks.listApplicationsPage).toHaveBeenCalledWith('prj_1', expect.objectContaining({ includeRuntime: true, page: 1 }))
    expect(mocks.getDashboard).toHaveBeenCalledTimes(1)
    expect(mocks.getDashboard).toHaveBeenCalledWith('related')

    await user.click(within(table).getByRole('row', { name: 'Open application API' }))
    expect(await screen.findByText('Application detail route')).toBeInTheDocument()
  })

  it('offers one primary page action and keeps platform readiness for platform admins only', async () => {
    renderPage()

    expect(await screen.findByRole('link', { name: 'Start from a template' })).toHaveAttribute('href', '/app-templates')
    expect(screen.queryByText('Platform readiness')).not.toBeInTheDocument()
    expect(screen.getByText('Frequent project spaces')).toBeVisible()
  })

  it('shows platform readiness to platform admins', async () => {
    mocks.role = 'platform_admin'
    renderPage()

    expect(await screen.findByText('Platform readiness')).toBeVisible()
    expect(screen.getByText('Partially available')).toBeVisible()
    expect(screen.getByRole('link', { name: 'Registries' })).toHaveAttribute('href', '/registries?visibility=related')
  })

  it('keeps empty dashboard sections compact and actionable', async () => {
    const fixture = dashboardOverviewFixture()
    fixture.projects = []
    fixture.activities = []
    fixture.attention = []
    mocks.getDashboard.mockResolvedValue(fixture)

    renderPage()

    expect(await screen.findByText('No activity yet')).toBeInTheDocument()
    expect(screen.getByText('No applications yet')).toBeVisible()
    expect(screen.getByText('Create or join a project space to continue work from here.')).toBeInTheDocument()
    expect(screen.queryByText('Needs attention')).not.toBeInTheDocument()
    expect(mocks.listApplicationsPage).not.toHaveBeenCalled()
  })

  it('keeps the dashboard related when a legacy all range remains in the URL', async () => {
    renderPage(['/dashboard?visibility=all'])

    await waitFor(() => expect(mocks.getDashboard).toHaveBeenCalledWith('related'))
    expect(screen.queryByRole('combobox', { name: 'View range' })).not.toBeInTheDocument()
    expect(await screen.findByRole('link', { name: 'View all events' })).toHaveAttribute('href', '/events?visibility=related')
  })
})

function renderPage(initialEntries = ['/dashboard']) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={initialEntries}>
        <Routes>
          <Route element={<DashboardPage />} path="/dashboard" />
          <Route element={<div>Application detail route</div>} path="/projects/:projectId/apps/:applicationId" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function applicationFixture(): Application {
  return {
    id: 'app_1',
    projectId: 'prj_1',
    identifier: 'api',
    name: 'API',
    icon: 'box',
    deleteStatus: 'active',
    deleteMessage: '',
    createdAt: '2026-07-01T00:00:00Z',
    updatedAt: '2026-07-01T00:00:00Z',
    deploymentSummary: {
      targetCount: 1,
      desiredReplicas: 2,
      readyReplicas: 2,
      status: 'ready',
      targets: [{ id: 'dpt_1', stage: 'prod', desiredReplicas: 2, readyReplicas: 2, status: 'ready' }],
    },
  }
}

function dashboardOverviewFixture(): DashboardOverview {
  return {
    generatedAt: '2026-07-16T12:00:00Z',
    summary: {
      projects: 1,
      applications: 1,
      activeBuilds: 1,
      activeReleases: 1,
      attentionItems: 1,
      healthyClusters: 1,
      totalClusters: 2,
    },
    projects: [{
      id: 'prj_1',
      name: 'Example project',
      identifier: 'example',
      description: 'Example project space',
      pinned: true,
      applicationCount: 1,
      latestActivity: {
        id: 'evt_1',
        type: 'build.failed',
        category: 'build',
        severity: 'error',
        status: 'failed',
        message: 'build failed',
        project: { id: 'prj_1', name: 'Example project', identifier: 'example' },
        application: { id: 'app_1', name: 'API', identifier: 'api' },
        resourceType: 'build_run',
        resourceId: 'bldr_1',
        links: {},
        occurredAt: '2026-07-16T11:59:00Z',
      },
    }],
    attention: [{
      key: 'build:prj_1:app_1',
      category: 'build',
      severity: 'error',
      occurrences: 2,
      latest: {
        id: 'evt_1',
        type: 'build.failed',
        category: 'build',
        severity: 'error',
        status: 'failed',
        message: 'build failed',
        project: { id: 'prj_1', name: 'Example project', identifier: 'example' },
        application: { id: 'app_1', name: 'API', identifier: 'api' },
        resourceType: 'build_run',
        resourceId: 'bldr_1',
        links: {},
        occurredAt: '2026-07-16T11:59:00Z',
      },
    }],
    activities: [{
      id: 'evt_1',
      type: 'build.failed',
      category: 'build',
      severity: 'error',
      status: 'failed',
      message: 'build failed',
      project: { id: 'prj_1', name: 'Example project', identifier: 'example' },
      application: { id: 'app_1', name: 'API', identifier: 'api' },
      resourceType: 'build_run',
      resourceId: 'bldr_1',
      links: {},
      occurredAt: '2026-07-16T11:59:00Z',
    }],
    readiness: {
      clusters: { status: 'degraded', available: 1, total: 2 },
      registries: { status: 'available', available: 1, total: 1 },
    },
  }
}
