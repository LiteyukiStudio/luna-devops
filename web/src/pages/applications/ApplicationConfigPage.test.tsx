import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Link, MemoryRouter, Route, Routes, useParams } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ApplicationConfigPage } from './ApplicationConfigPage'

const mocks = vi.hoisted(() => ({
  getApplication: vi.fn(),
  getApplicationRuntime: vi.fn(),
  getProject: vi.fn(),
  listBuildJobs: vi.fn(),
  listBuildRuns: vi.fn(),
  listDeploymentTargets: vi.fn(),
  listGatewayRoutes: vi.fn(),
  listRegistries: vi.fn(),
  listReleases: vi.fn(),
  listRepositoryBindings: vi.fn(),
}))

vi.mock('@/app/session-context', () => ({
  useSession: () => ({ user: { id: 'usr_1', role: 'platform_admin' } }),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getApplication: mocks.getApplication,
      getApplicationRuntime: mocks.getApplicationRuntime,
      getProject: mocks.getProject,
      listBuildJobs: mocks.listBuildJobs,
      listBuildRuns: mocks.listBuildRuns,
      listDeploymentTargets: mocks.listDeploymentTargets,
      listGatewayRoutes: mocks.listGatewayRoutes,
      listRegistries: mocks.listRegistries,
      listReleases: mocks.listReleases,
      listRepositoryBindings: mocks.listRepositoryBindings,
    },
  }
})

vi.mock('@/pages/applications/builds/application-builds-panel', () => ({
  ApplicationBuildsPanel: () => <div>Build history panel</div>,
}))

vi.mock('@/pages/repositories/RepositoryBindingsPage', () => ({
  RepositoryBindingsPage: () => <div>Repository bindings list</div>,
}))

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

const applicationFixture = {
  id: 'app_1',
  projectId: 'prj_1',
  identifier: 'example',
  name: 'Example',
  icon: 'box',
  deleteStatus: 'active',
  deleteMessage: '',
  createdAt: '2026-10-06T00:00:00Z',
  updatedAt: '2026-10-06T00:00:00Z',
}

const productionTarget = {
  id: 'dpt_1',
  applicationId: 'app_1',
  name: 'Production',
  stage: 'prod',
  enabled: true,
  status: 'ready',
  clusterId: '',
  effectiveClusterId: 'cluster_1',
  desiredReplicas: 2,
  readyReplicas: 2,
  availableReplicas: 2,
  sourceType: 'image',
  imageRef: 'registry.example.test/app:v1',
  dataVolumes: [],
}

describe('application config page', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    window.history.replaceState(null, '', '/')
    await i18next.changeLanguage('en-US')
    mocks.getApplication.mockResolvedValue(applicationFixture)
    mocks.getApplicationRuntime.mockResolvedValue({
      ...applicationFixture,
      deploymentSummary: { targetCount: 0, desiredReplicas: 0, readyReplicas: 0, status: 'not-deployed', targets: [] },
    })
    mocks.getProject.mockResolvedValue({ id: 'prj_1', identifier: 'project', name: 'Project' })
    mocks.listBuildJobs.mockResolvedValue([])
    mocks.listBuildRuns.mockResolvedValue([])
    mocks.listDeploymentTargets.mockResolvedValue([])
    mocks.listGatewayRoutes.mockResolvedValue([])
    mocks.listRegistries.mockResolvedValue([])
    mocks.listReleases.mockResolvedValue([])
    mocks.listRepositoryBindings.mockResolvedValue([])
  })

  it('loads every dependency needed by the builds tab after a direct refresh', async () => {
    renderPage()

    expect(await screen.findByText('Build history panel')).toBeVisible()
    await waitFor(() => {
      expect(mocks.listRepositoryBindings).toHaveBeenCalledWith('prj_1', 'app_1')
      expect(mocks.listRegistries).toHaveBeenCalledWith('prj_1')
    })
  })

  it('summarises status, access and version in the fact strip', async () => {
    mocks.listDeploymentTargets.mockResolvedValue([productionTarget])
    mocks.getApplicationRuntime.mockResolvedValue({
      ...applicationFixture,
      currentRelease: { id: 'rel_1', deploymentTargetId: 'dpt_1', imageRef: 'registry.example.test/app:v1', deployedAt: '2026-10-06T00:05:00Z' },
      deploymentSummary: {
        targetCount: 1,
        desiredReplicas: 2,
        readyReplicas: 2,
        status: 'ready',
        targets: [{ id: 'dpt_1', stage: 'prod', enabled: true, deployed: true, desiredReplicas: 2, readyReplicas: 2, status: 'ready' }],
      },
    })
    mocks.listReleases.mockResolvedValue([{
      id: 'rel_1',
      applicationId: 'app_1',
      deploymentTargetId: 'dpt_1',
      imageRef: 'registry.example.test/app:v1',
      status: 'succeeded',
      createdAt: '2026-10-06T00:00:00Z',
      revision: 1,
      type: 'deploy',
    }])
    mocks.listGatewayRoutes.mockResolvedValue([{
      id: 'route_1',
      applicationId: 'app_1',
      deploymentTargetId: 'dpt_1',
      accessUrl: 'https://app.example.com',
      host: 'app.example.com',
      enabled: true,
      status: 'ready',
      deleteStatus: 'active',
      tlsMode: 'http-challenge',
      path: '/',
    }])
    renderPage('/projects/prj_1/apps/app_1?tab=topology')

    const facts = await screen.findByLabelText('Application key facts')
    expect(await within(facts).findByText('2/2')).toBeVisible()
    expect(within(facts).getByRole('link', { name: /app\.example\.com/ })).toHaveAttribute('href', 'https://app.example.com')
    expect(within(facts).getByText('registry.example.test/app:v1')).toBeVisible()
    expect(within(facts).getByText('No builds yet')).toBeVisible()
    expect(within(facts).queryByText('Not ready')).not.toBeInTheDocument()
    expect(mocks.getApplicationRuntime).toHaveBeenCalledWith('prj_1', 'app_1')
  })

  it('keeps the onboarding guide consistent with the backend verdict and the primary access route', async () => {
    mocks.listDeploymentTargets.mockResolvedValue([productionTarget])
    mocks.getApplicationRuntime.mockResolvedValue({
      ...applicationFixture,
      currentRelease: { id: 'rel_1', deploymentTargetId: 'dpt_1', imageRef: 'registry.example.test/app:v1', deployedAt: '2026-10-06T00:05:00Z' },
      deploymentSummary: {
        targetCount: 1,
        desiredReplicas: 0,
        readyReplicas: 0,
        status: 'not-configured',
        targets: [{ id: 'dpt_1', stage: 'prod', enabled: true, deployed: true, desiredReplicas: 0, readyReplicas: 0, status: 'not-configured' }],
      },
    })
    mocks.listReleases.mockResolvedValue([{ id: 'rel_1', applicationId: 'app_1', deploymentTargetId: 'dpt_1', imageRef: 'registry.example.test/app:v1', status: 'succeeded', createdAt: '2026-10-06T00:00:00Z', revision: 1, type: 'deploy' }])
    mocks.listGatewayRoutes.mockResolvedValue([{
      id: 'route_1',
      applicationId: 'app_1',
      deploymentTargetId: 'dpt_1',
      accessUrl: 'https://app.example.com',
      host: 'app.example.com',
      enabled: true,
      status: 'progressing',
      deleteStatus: 'active',
      tlsMode: 'http-challenge',
      path: '/',
    }])
    renderPage('/projects/prj_1/apps/app_1?tab=overview')

    const facts = await screen.findByLabelText('Application key facts')
    expect(await within(facts).findByText('Not ready')).toBeVisible()
    expect(within(facts).getByText('No cluster')).toBeVisible()
    const guide = (await screen.findByText('Deploy online')).closest('[data-slot="surface"]') as HTMLElement
    const releaseStep = within(guide).getByText('Release runtime').closest('li') as HTMLElement
    expect(within(releaseStep).getByText('No cluster')).toBeVisible()
    const accessStep = within(guide).getByText('Expose access').closest('li') as HTMLElement
    expect(within(accessStep).queryByText('Ready')).not.toBeInTheDocument()
  })

  it('orders tabs along the delivery path while keeping existing tab keys', async () => {
    renderPage('/projects/prj_1/apps/app_1?tab=gateway')

    expect(await screen.findByRole('tab', { name: 'Access routes' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getAllByRole('tab').map(tab => tab.textContent)).toEqual(['Overview', 'Builds', 'Deployments', 'Access routes', 'Topology', 'Settings'])
  })

  it('keeps settings as a read-only list of bound repositories without a second binding entry', async () => {
    renderPage('/projects/prj_1/apps/app_1?tab=repositories')

    expect(await screen.findByText('Repository bindings list')).toBeVisible()
    expect(screen.getByRole('tab', { name: 'Settings' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.queryByRole('button', { name: /add repository|bind code repository/i })).not.toBeInTheDocument()
  })

  it('sends the onboarding source step to the deploy config source instead of settings', async () => {
    const user = userEvent.setup()
    mocks.listDeploymentTargets.mockResolvedValue([{ ...productionTarget, sourceType: 'repository', imageRef: '', repositoryBindingId: '' }])
    renderPage('/projects/prj_1/apps/app_1')

    await user.click(await screen.findByRole('button', { name: 'Bind code repository' }))
    expect(await screen.findByText('deploy config editor dpt_1')).toBeVisible()
  })

  it('drops the duplicated deployment summary from the overview and links to the deployments tab', async () => {
    const user = userEvent.setup()
    mocks.listDeploymentTargets.mockResolvedValue([productionTarget])
    renderPage('/projects/prj_1/apps/app_1')

    expect(await screen.findByText(i18next.t('apps.recentActivity'))).toBeVisible()
    expect(screen.queryByText('Production')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'View deployments' }))
    expect(screen.getByRole('tab', { name: 'Deployments' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.queryByRole('button', { name: 'View deployments' })).not.toBeInTheDocument()
  })

  it('derives the active tab from the URL so in-route ?tab links switch tabs', async () => {
    const user = userEvent.setup()
    renderPage('/projects/prj_1/apps/app_1?tab=topology')

    expect(await screen.findByRole('tab', { name: 'Topology' })).toHaveAttribute('aria-selected', 'true')
    await user.click(screen.getByRole('link', { name: 'jump to gateway' }))
    expect(screen.getByRole('tab', { name: 'Access routes' })).toHaveAttribute('aria-selected', 'true')
    expect(window.location.hash).toBe('')
  })

  it('reports overview load failures instead of guiding the user to create resources', async () => {
    const user = userEvent.setup()
    mocks.listDeploymentTargets.mockRejectedValue(new Error('cluster unreachable'))
    renderPage('/projects/prj_1/apps/app_1')

    expect(await screen.findByText('Failed to load application overview')).toBeVisible()
    expect(screen.queryByText(i18next.t('apps.deployGuideTitle'))).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: i18next.t('apps.deployGuideCreateTarget') })).not.toBeInTheDocument()

    mocks.listDeploymentTargets.mockResolvedValue([])
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText(i18next.t('apps.deployGuideTitle'))).toBeVisible()
    expect(screen.queryByText('Failed to load application overview')).not.toBeInTheDocument()
  })

  it('explains why creating a release is unavailable and links to the missing prerequisite', async () => {
    const user = userEvent.setup()
    renderPage('/projects/prj_1/apps/app_1?tab=topology')

    expect(await screen.findByRole('button', { name: 'Create release' })).toBeDisabled()
    await user.click(await screen.findByRole('button', { name: 'Why a release cannot be created' }))
    const popover = await screen.findByRole('dialog')
    expect(within(popover).getByText('Creating a release needs a deploy config first.')).toBeVisible()
    await user.click(within(popover).getByRole('button', { name: 'New deploy config' }))
    expect(await screen.findByText('deploy config editor')).toBeVisible()
  })
})

function EditorRouteProbe() {
  const { targetId } = useParams()
  return <p>{`deploy config editor ${targetId}`}</p>
}

function renderPage(initialEntry = '/projects/prj_1/apps/app_1?tab=builds') {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <MemoryRouter initialEntries={[initialEntry]}>
          <Routes>
            <Route
              element={(
                <>
                  <Link to="/projects/prj_1/apps/app_1?tab=gateway">jump to gateway</Link>
                  <ApplicationConfigPage />
                </>
              )}
              path="/projects/:projectId/apps/:applicationId"
            />
            <Route element={<p>deploy config editor</p>} path="/projects/:projectId/apps/:applicationId/deployments/new" />
            <Route element={<EditorRouteProbe />} path="/projects/:projectId/apps/:applicationId/deployments/:targetId/edit" />
          </Routes>
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  )
}
