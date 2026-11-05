import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useImperativeHandle } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ApplicationConfigPage } from './ApplicationConfigPage'

const mocks = vi.hoisted(() => ({
  getApplication: vi.fn(),
  getProject: vi.fn(),
  listBuildJobs: vi.fn(),
  listBuildRuns: vi.fn(),
  listDeploymentTargets: vi.fn(),
  listGatewayRoutes: vi.fn(),
  listRegistries: vi.fn(),
  listReleases: vi.fn(),
  listRepositoryBindings: vi.fn(),
  openRepositoryDialog: vi.fn(),
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
  RepositoryBindingsPage: ({ ref }: { ref?: React.Ref<{ openCreateDialog: () => void }> }) => {
    useImperativeHandle(ref, () => ({ openCreateDialog: mocks.openRepositoryDialog }))
    return <div>Repository bindings list</div>
  },
}))

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

describe('application config page', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    window.history.replaceState(null, '', '/')
    await i18next.changeLanguage('en-US')
    mocks.getApplication.mockResolvedValue({
      id: 'app_1',
      projectId: 'prj_1',
      identifier: 'example',
      name: 'Example',
      icon: 'box',
      deleteStatus: 'active',
      deleteMessage: '',
      createdAt: '2026-10-06T00:00:00Z',
      updatedAt: '2026-10-06T00:00:00Z',
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
    mocks.listDeploymentTargets.mockResolvedValue([{
      id: 'dpt_1',
      applicationId: 'app_1',
      name: 'Production',
      stage: 'prod',
      enabled: true,
      status: 'ready',
      desiredReplicas: 2,
      readyReplicas: 2,
      availableReplicas: 2,
      sourceType: 'image',
      imageRef: 'registry.example.test/app:v1',
    }])
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
  })

  it('merges repository bindings into settings and opens the binding dialog from there', async () => {
    const user = userEvent.setup()
    renderPage('/projects/prj_1/apps/app_1?tab=repositories')

    expect(await screen.findByText('Repository bindings list')).toBeVisible()
    expect(screen.queryByRole('tab', { name: 'Repository' })).not.toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Settings' })).toHaveAttribute('aria-selected', 'true')
    await user.click(screen.getByRole('button', { name: 'Add repository' }))
    expect(mocks.openRepositoryDialog).toHaveBeenCalledTimes(1)
  })
})

function renderPage(initialEntry = '/projects/prj_1/apps/app_1?tab=builds') {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <MemoryRouter initialEntries={[initialEntry]}>
          <Routes>
            <Route element={<ApplicationConfigPage />} path="/projects/:projectId/apps/:applicationId" />
          </Routes>
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  )
}
