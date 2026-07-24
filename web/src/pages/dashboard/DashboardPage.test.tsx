import type { DashboardOverview } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { DashboardPage } from './DashboardPage'

const mocks = vi.hoisted(() => ({
  getDashboard: vi.fn(),
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
    expect(screen.getByText('Attention').closest('[data-slot="notice"]')).toHaveAttribute('data-variant', 'neutral')
    expect(screen.getByText('Active builds').closest('[data-slot="metric-item"]')).toHaveAttribute('data-surface', 'neutral')
    const overview = screen.getByText('Active builds').closest('[data-slot="dashboard-overview"]')
    expect(overview).toContainElement(screen.getByText('Recent activity'))
    expect(overview).toContainElement(screen.getByText('Platform readiness'))
    expect(mocks.getDashboard).toHaveBeenCalledTimes(1)
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
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
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
