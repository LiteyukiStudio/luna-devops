import type { RuntimeCluster } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ClustersPage } from './ClustersPage'

const mocks = vi.hoisted(() => ({
  listProjects: vi.fn(),
  listRuntimeClusterResourcesPage: vi.fn(),
  listRuntimeClusters: vi.fn(),
  listRuntimeClustersPage: vi.fn(),
  observeRuntimeClusterPressure: vi.fn(),
  session: { user: { id: 'usr_admin', role: 'platform_admin' } },
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
      listProjects: mocks.listProjects,
      listRuntimeClusterResourcesPage: mocks.listRuntimeClusterResourcesPage,
      listRuntimeClusters: mocks.listRuntimeClusters,
      listRuntimeClustersPage: mocks.listRuntimeClustersPage,
      observeRuntimeClusterPressure: mocks.observeRuntimeClusterPressure,
    },
  }
})

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

const cluster: RuntimeCluster = {
  id: 'clu_test',
  name: 'Test cluster',
  endpoint: 'https://cluster.example.test',
  scope: 'global',
  ownerRef: '',
  projectIds: [],
  kubeconfigSet: true,
  isDefault: true,
  maxConcurrentBuilds: 4,
  cpuRequestPercent: 10,
  memoryRequestPercent: 25,
  cpuLimitPercent: 100,
  memoryLimitPercent: 100,
  gatewayDomainSuffixes: ['apps.example.test'],
  gatewayPublicScheme: 'https',
  gatewayPublicPort: 443,
  gatewayControllerType: 'generic',
  gatewayClassName: '',
  gatewayName: '',
  gatewayNamespace: '',
  gatewayHttpListenerName: '',
  gatewayHttpListenerPort: 80,
  gatewayHttpsListenerName: '',
  gatewayHttpsListenerPort: 443,
  gatewayTlsSecretName: '',
  gatewayTlsSecretNamespace: '',
  gatewayCertIssuerKind: 'ClusterIssuer',
  gatewayCertIssuerName: '',
  gatewayCertificateNamespace: '',
  gatewayWildcardCertEnabled: false,
  gatewayWildcardCertDomain: '',
  gatewayWildcardCertSecretName: '',
  gatewayExternalTLSMode: 'none',
  gatewayForwardedHeadersMode: 'preserve',
  gatewayTrustedProxyCIDRs: '',
  gatewayDefaultRequestHeaders: '',
  gatewayDefaultResponseHeaders: '',
  status: 'ready',
  deleteStatus: 'active',
  createdBy: 'usr_admin',
  createdAt: '2026-09-11T00:00:00Z',
}

describe('clusters page visibility', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.listProjects.mockResolvedValue([])
    mocks.listRuntimeClusters.mockResolvedValue([cluster])
    mocks.listRuntimeClustersPage.mockResolvedValue(page([cluster]))
    mocks.listRuntimeClusterResourcesPage.mockResolvedValue(page([]))
    mocks.observeRuntimeClusterPressure.mockResolvedValue([])
  })

  it('uses the full platform view for administrators without exposing a range selector', async () => {
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => {
      expect(mocks.listProjects).toHaveBeenCalledWith('all')
      expect(mocks.listRuntimeClusters).toHaveBeenCalledWith(undefined, 'all')
      expect(mocks.listRuntimeClustersPage).toHaveBeenCalledWith(expect.objectContaining({ visibility: 'all' }))
    })
    expect(screen.queryByRole('combobox', { name: 'View range' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'Workloads' }))
    await waitFor(() => expect(mocks.listRuntimeClusterResourcesPage).toHaveBeenCalledWith(
      cluster.id,
      expect.objectContaining({ resourceCategory: 'workloads', visibility: 'all' }),
    ))
  })
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/clusters?visibility=related']}>
        <TooltipProvider>
          <ClustersPage />
        </TooltipProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function page<T>(items: T[]) {
  return {
    items,
    page: 1,
    pageSize: 10,
    sortBy: 'createdAt',
    sortOrder: 'desc' as const,
    total: items.length,
    totalPages: items.length > 0 ? 1 : 0,
  }
}
