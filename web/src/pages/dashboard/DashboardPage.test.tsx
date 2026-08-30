import type { DashboardOverview } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { DashboardPage } from './DashboardPage'

const mocks = vi.hoisted(() => ({
  getDashboard: vi.fn(),
  session: { user: { role: 'user' } },
}))

vi.mock('@/app/session-context', () => ({
  useSession: () => mocks.session,
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getDashboard: mocks.getDashboard,
    },
  }
})

describe('dashboard page', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    mocks.session.user.role = 'user'
    await i18next.changeLanguage('en-US')
    mocks.getDashboard.mockResolvedValue(dashboardOverviewFixture())
  })

  it('renders the task-oriented overview from one dashboard query', async () => {
    renderPage()

    expect(await screen.findByText('Frequent project spaces')).toBeInTheDocument()
    expect(screen.getByText('Active builds')).toBeInTheDocument()
    expect(screen.getByText('Active releases')).toBeInTheDocument()
    expect(screen.getByText('2 consecutive')).toBeInTheDocument()
    expect(screen.getByText('Recent activity')).toBeInTheDocument()
    expect(screen.getByText('Partially available')).toBeInTheDocument()
    expect(screen.getByText('Attention').closest('[data-slot="alert"]')).toHaveClass('bg-surface-raised')
    expect(screen.getByText('Active builds').closest('[data-slot="metric-item"]')).toHaveAttribute('data-surface', 'neutral')
    const overview = screen.getByText('Active builds').closest('[data-slot="dashboard-overview"]')
    expect(overview).toContainElement(screen.getByText('Recent activity'))
    expect(overview).toContainElement(screen.getByText('Platform readiness'))
    expect(mocks.getDashboard).toHaveBeenCalledTimes(1)
    expect(mocks.getDashboard).toHaveBeenCalledWith('related')
  })

  it('keeps empty dashboard sections compact and actionable', async () => {
    const fixture = dashboardOverviewFixture()
    fixture.projects = []
    fixture.activities = []
    mocks.getDashboard.mockResolvedValue(fixture)

    renderPage()

    expect(await screen.findByText('No activity yet')).toBeInTheDocument()
    expect(screen.getByText('Build, release, and gateway activity will appear here.')).toBeInTheDocument()
    expect(screen.getByText('Create or join a project space to continue work from here.')).toBeInTheDocument()
  })

  it('loads an explicit all range for administrators and preserves it across dashboard links', async () => {
    const user = userEvent.setup()
    mocks.session.user.role = 'platform_admin'

    renderPage(['/dashboard?visibility=all'])

    await waitFor(() => expect(mocks.getDashboard).toHaveBeenCalledWith('all'))
    expect(await screen.findByRole('combobox', { name: 'View range' })).toHaveValue('all')
    expect(screen.getByRole('link', { name: 'View all events' })).toHaveAttribute('href', '/events?visibility=all')
    expect(screen.getByRole('link', { name: /^Registries/ })).toHaveAttribute('href', '/registries?visibility=all')

    await user.selectOptions(screen.getByRole('combobox', { name: 'View range' }), 'related')
    await waitFor(() => expect(mocks.getDashboard).toHaveBeenCalledWith('related'))
  })
})

function renderPage(initialEntries = ['/dashboard']) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={initialEntries}>
        <DashboardPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
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
