import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { ApplicationConfigPage } from './ApplicationConfigPage'

const mocks = vi.hoisted(() => ({
  getApplication: vi.fn(),
  getProject: vi.fn(),
  listBuildJobs: vi.fn(),
  listBuildRuns: vi.fn(),
  listDeploymentTargets: vi.fn(),
  listRegistries: vi.fn(),
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
      getProject: mocks.getProject,
      listBuildJobs: mocks.listBuildJobs,
      listBuildRuns: mocks.listBuildRuns,
      listDeploymentTargets: mocks.listDeploymentTargets,
      listRegistries: mocks.listRegistries,
      listRepositoryBindings: mocks.listRepositoryBindings,
    },
  }
})

vi.mock('@/pages/applications/builds/application-builds-panel', () => ({
  ApplicationBuildsPanel: () => <div>Build history panel</div>,
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
    mocks.listRegistries.mockResolvedValue([])
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
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/projects/prj_1/apps/app_1?tab=builds']}>
        <Routes>
          <Route element={<ApplicationConfigPage />} path="/projects/:projectId/apps/:applicationId" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}
