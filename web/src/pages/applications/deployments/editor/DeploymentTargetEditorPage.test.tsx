import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { DeploymentTargetEditorPage } from './DeploymentTargetEditorPage'

vi.mock('@/app/session-context', () => ({
  useSession: () => ({ user: { role: 'user' } }),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getApplication: vi.fn(),
      getGitRepositoryBuildOptions: vi.fn(),
      getProject: vi.fn(),
      getRegistryImageTemplateDefault: vi.fn(),
      listBillingRateRules: vi.fn(),
      listBuildTemplates: vi.fn(),
      listDeploymentTargets: vi.fn(),
      listProjectHooks: vi.fn(),
      listProjectRuntimeConfigSets: vi.fn(),
      listRegistries: vi.fn(),
      listReleases: vi.fn(),
      listRepositoryBindings: vi.fn(),
      listRuntimeClusters: vi.fn(),
      observeRuntimeClusterPressure: vi.fn(),
    },
  }
})

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter initialEntries={['/projects/prj_1/apps/app_1/deployments/new']}>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <Routes>
            <Route path="/projects/:projectId/apps/:applicationId/deployments/new" element={<DeploymentTargetEditorPage />} />
          </Routes>
        </TooltipProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('deployment target editor page', () => {
  beforeAll(() => {
    Element.prototype.scrollIntoView = vi.fn()
  })

  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
    vi.clearAllMocks()
    vi.mocked(api.getApplication).mockResolvedValue({ id: 'app_1', identifier: 'web', name: 'Web' } as never)
    vi.mocked(api.getProject).mockResolvedValue({ currentUserRole: 'owner', id: 'prj_1', identifier: 'demo', name: 'Demo' } as never)
    vi.mocked(api.listRegistries).mockResolvedValue([{ credentialSet: true, endpoint: 'https://registry.example.com', id: 'reg_1', isDefault: true, name: 'Registry', namespace: 'team', provider: 'harbor' }] as never)
    vi.mocked(api.listRepositoryBindings).mockResolvedValue([{ applicationId: 'app_1', defaultBranch: 'main', gitAccountId: 'acc_1', id: 'rb_1', owner: 'team', repo: 'demo' }] as never)
    vi.mocked(api.listDeploymentTargets).mockResolvedValue([])
    vi.mocked(api.listReleases).mockResolvedValue([])
    vi.mocked(api.listRuntimeClusters).mockResolvedValue([{ id: 'clu_1', isDefault: true, name: 'Cluster' }] as never)
    vi.mocked(api.getGitRepositoryBuildOptions).mockResolvedValue({ directories: ['.'], dockerfiles: ['Dockerfile'], exposedPorts: { Dockerfile: [3000] } } as never)
    vi.mocked(api.getRegistryImageTemplateDefault).mockResolvedValue({ targetImageRef: 'team/demo-web-dev:latest', targetRepository: 'team/demo-web-dev', targetTag: 'latest' })
    vi.mocked(api.listBillingRateRules).mockResolvedValue([])
    vi.mocked(api.listBuildTemplates).mockResolvedValue([])
    vi.mocked(api.listProjectHooks).mockResolvedValue([])
    vi.mocked(api.listProjectRuntimeConfigSets).mockResolvedValue([])
    vi.mocked(api.observeRuntimeClusterPressure).mockResolvedValue([])
  })

  it('keeps the new editor mounted when sections refetch the prefetched queries', async () => {
    renderPage()

    expect(await screen.findByDisplayValue('team/demo-web-dev:latest')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Dockerfile')).toBeInTheDocument()
    // 来源分区挂载后的后台重取曾让页面在骨架屏与编辑器之间无限切换。
    await new Promise(resolve => setTimeout(resolve, 200))
    expect(screen.getByDisplayValue('team/demo-web-dev:latest')).toBeInTheDocument()
    expect(vi.mocked(api.getRegistryImageTemplateDefault).mock.calls.length).toBeLessThanOrEqual(2)
    expect(vi.mocked(api.getGitRepositoryBuildOptions).mock.calls.length).toBeLessThanOrEqual(2)
  })
})
