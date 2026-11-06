import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ProjectWorkspacePage } from './ProjectWorkspacePage'

const mocks = vi.hoisted(() => ({
  getProject: vi.fn(),
  getProjectOverview: vi.fn(),
  getProjectTopology: vi.fn(),
  listApplications: vi.fn(),
  listBuildRunsPage: vi.fn(),
  listBuildVariableSets: vi.fn(),
  listGatewayRoutes: vi.fn(),
  listPlatformEvents: vi.fn(),
  listProjectMembers: vi.fn(),
  listProjectRuntimeConfigSets: vi.fn(),
  listReleases: vi.fn(),
}))
const hookPageMocks = vi.hoisted(() => ({ openCreateDialog: vi.fn() }))

vi.mock('@/app/session-context', () => ({
  useSession: () => ({ user: { id: 'usr_1', permissions: [], role: 'user' } }),
}))

vi.mock('@/pages/projects/delivery-config/ProjectBuildVariableSetsPage', () => ({
  ProjectBuildVariableSetsPage: () => <div>Build variable sets list</div>,
}))
vi.mock('@/pages/projects/delivery-config/ProjectRuntimeConfigSetsPage', () => ({
  ProjectRuntimeConfigSetsPage: () => <div>Runtime config sets list</div>,
}))
vi.mock('@/pages/projects/delivery-config/ProjectHooksPage', async () => {
  const { useImperativeHandle } = await import('react')
  return {
    ProjectHooksPage: ({ ref }: { ref?: React.Ref<{ openCreateDialog: () => void }> }) => {
      useImperativeHandle(ref, () => ({ openCreateDialog: hookPageMocks.openCreateDialog }))
      return <div>Hooks list</div>
    },
  }
})

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      ...mocks,
    },
  }
})

const emptyPage = { items: [], page: 1, pageSize: 5, sortBy: 'createdAt', sortOrder: 'desc', total: 0, totalPages: 0 }

describe('project workspace overview', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.getProject.mockResolvedValue({ id: 'prj_1', identifier: 'project', name: 'Project', description: '' })
    mocks.getProjectOverview.mockResolvedValue({
      activeReleases: 3,
      applications: 120,
      failedReleases: 2,
      failedReleaseWindowDays: 7,
      accessRoutes: { status: 'degraded', ready: 4, total: 5, observationCode: 'project_overview.access_routes_not_ready', observedAt: '2026-11-06T00:00:00Z' },
    })
    mocks.getProjectTopology.mockResolvedValue({ nodes: [], edges: [] })
    mocks.listApplications.mockResolvedValue([{ id: 'app_1', name: 'API' }])
    mocks.listBuildRunsPage.mockResolvedValue(emptyPage)
    mocks.listBuildVariableSets.mockResolvedValue([])
    mocks.listPlatformEvents.mockResolvedValue(emptyPage)
    mocks.listProjectMembers.mockResolvedValue([])
    mocks.listProjectRuntimeConfigSets.mockResolvedValue([])
  })

  it('shows server-side project counts and the failed release window instead of counting a truncated page', async () => {
    renderPage()

    // 页面切换带淡入动画，负载较高时需等待动画结束再断言可见性。
    await waitFor(() => expect(screen.getByText('120')).toBeVisible())
    expect(screen.getByText('3')).toBeVisible()
    expect(screen.getByText('2 releases failed in the last 7 days')).toBeVisible()
    expect(mocks.getProjectOverview).toHaveBeenCalledWith('prj_1')
    expect(mocks.listReleases).not.toHaveBeenCalled()
  })

  it('lands on the overview when no section is given', async () => {
    renderPage('/projects/prj_1')

    expect(await screen.findByText('120')).toBeVisible()
    expect(mocks.getProjectOverview).toHaveBeenCalledWith('prj_1')
  })

  it('redirects the former hooks section to the shared config hooks tab', async () => {
    const user = userEvent.setup()
    renderPage('/projects/prj_1?tab=hooks')

    expect(await screen.findByText('Hooks list')).toBeInTheDocument()
    expect(screen.getByTestId('location')).toHaveTextContent('/projects/prj_1?tab=shared-config&config=hooks')
    expect(screen.getByRole('tab', { name: 'Hooks' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getAllByRole('tab').map(tab => tab.textContent)).toEqual(['Build variables', 'Runtime config', 'Hooks'])

    await user.click(screen.getByRole('button', { name: i18next.t('projectHooks.createTitle') }))
    expect(hookPageMocks.openCreateDialog).toHaveBeenCalledTimes(1)
  })

  it('switches shared config tabs through the URL', async () => {
    const user = userEvent.setup()
    renderPage('/projects/prj_1?tab=build-variables')

    expect(await screen.findByText('Build variable sets list')).toBeInTheDocument()
    expect(screen.getByTestId('location')).toHaveTextContent(/^\/projects\/prj_1\?tab=shared-config$/)

    await user.click(screen.getByRole('tab', { name: 'Runtime config' }))
    expect(await screen.findByText('Runtime config sets list')).toBeInTheDocument()
    expect(screen.getByTestId('location')).toHaveTextContent('/projects/prj_1?tab=shared-config&config=runtime-configs')
  })

  it('shows the server-observed access route readiness instead of counting a route page', async () => {
    renderPage()

    await waitFor(() => expect(screen.getByText('4/5')).toBeVisible())
    expect(mocks.listGatewayRoutes).not.toHaveBeenCalled()
  })

  it('reports unavailable gateway observation instead of zero ready routes', async () => {
    mocks.getProjectOverview.mockResolvedValue({
      activeReleases: 0,
      applications: 1,
      failedReleases: 0,
      failedReleaseWindowDays: 7,
      accessRoutes: { status: 'unavailable', total: 3, observationCode: 'project_overview.access_routes_unavailable', observedAt: '2026-11-06T00:00:00Z' },
    })
    renderPage()

    await waitFor(() => expect(screen.getByText(i18next.t('projectSpaces.accessHealthUnavailable'))).toBeVisible())
    expect(screen.queryByText('0/3')).not.toBeInTheDocument()
  })

  it('offers topology management only when the server grants project:manage', async () => {
    mocks.getProject.mockResolvedValue({ id: 'prj_1', identifier: 'project', name: 'Project', description: '', currentUserRole: 'admin', currentUserActions: ['project:read', 'project:manage'] })
    const { unmount } = renderPage('/projects/prj_1?tab=topology')
    expect(await screen.findByRole('button', { name: i18next.t('projectTopology.addRelation') })).toBeInTheDocument()
    unmount()

    mocks.getProject.mockResolvedValue({ id: 'prj_1', identifier: 'project', name: 'Project', description: '', currentUserRole: 'admin', currentUserActions: ['project:read', 'project:write'] })
    renderPage('/projects/prj_1?tab=topology')
    expect(await screen.findByText(i18next.t('projectTopology.emptyReadOnly'))).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: i18next.t('projectTopology.addRelation') })).not.toBeInTheDocument()
  })

  it('offers billing owner transfer only to the billing owner with the owner-only action', async () => {
    mocks.getProject.mockResolvedValue({ id: 'prj_1', identifier: 'project', name: 'Project', description: '', billingOwnerUserId: 'usr_1', currentUserActions: ['project:read', 'project:owner_only'] })
    const { unmount } = renderPage()
    expect(await screen.findByRole('button', { name: i18next.t('projectSpaces.billingOwnerTransfer.trigger') })).toBeInTheDocument()
    unmount()

    mocks.getProject.mockResolvedValue({ id: 'prj_1', identifier: 'project', name: 'Project', description: '', billingOwnerUserId: 'usr_1', currentUserRole: 'owner', currentUserActions: ['project:read'] })
    renderPage()
    expect(await screen.findByText('120')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: i18next.t('projectSpaces.billingOwnerTransfer.trigger') })).not.toBeInTheDocument()
  })
})

function LocationProbe() {
  const location = useLocation()
  return <div data-testid="location">{`${location.pathname}${location.search}`}</div>
}

function renderPage(initialEntry = '/projects/prj_1?tab=overview') {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <MemoryRouter initialEntries={[initialEntry]}>
          <Routes>
            <Route
              element={(
                <>
                  <ProjectWorkspacePage />
                  <LocationProbe />
                </>
              )}
              path="/projects/:projectId"
            />
          </Routes>
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  )
}
