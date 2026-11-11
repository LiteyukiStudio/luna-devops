import type { NetworkServiceShare } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ProjectNetworkSharesCard } from './project-network-shares-card'

const mocks = vi.hoisted(() => ({
  listProjectNetworkShares: vi.fn(),
  createProjectNetworkShare: vi.fn(),
  acceptProjectNetworkShare: vi.fn(),
  deleteProjectNetworkShare: vi.fn(),
  getProjectTopology: vi.fn(),
  getProjectNetworkPolicy: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return { ...actual, api: { ...actual.api, ...mocks } }
})

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const share: NetworkServiceShare = {
  id: 'nss_auth',
  providerProjectId: 'prj_auth',
  providerProjectName: 'Auth',
  deploymentTargetId: 'dplt_auth',
  deploymentTargetName: 'prod',
  applicationName: 'auth-api',
  runtimeClusterId: 'clu_a',
  runtimeClusterName: 'Cluster A',
  consumerProjectId: 'prj_demo',
  consumerProjectName: 'Demo',
  address: 'auth-api.prj-auth.svc',
  status: 'pending_acceptance',
  approvalRequired: false,
  acceptedAt: null,
  approvedAt: null,
  createdAt: '2026-11-11T00:00:00Z',
}

function page(items: NetworkServiceShare[]) {
  return { items, page: 1, pageSize: 10, sortBy: 'createdAt', sortOrder: 'desc', total: items.length, totalPages: items.length > 0 ? 1 : 0 }
}

function renderCard(canManage: boolean) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><TooltipProvider><ProjectNetworkSharesCard canManage={canManage} projectId="prj_demo" /></TooltipProvider></QueryClientProvider>)
}

describe('project network shares card', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.listProjectNetworkShares.mockImplementation((_projectId: string, direction: string) => Promise.resolve(page(direction === 'consumed' ? [share] : [])))
    mocks.getProjectNetworkPolicy.mockResolvedValue({ ingressMode: 'isolated', egressMode: 'internet', egressAllowlist: '', egressExceptions: [] })
    mocks.getProjectTopology.mockResolvedValue({ nodes: [{ id: 'app_db', name: 'postgres', deploymentTargets: [
      { id: 'dplt_db', name: 'prod', stage: 'prod', clusterId: 'clu_a', deleteStatus: 'active' },
      { id: 'dplt_old', name: 'legacy', stage: 'legacy', clusterId: 'clu_a', deleteStatus: 'deleting' },
    ] }], edges: [] })
  })

  it('accepts a service shared with the project and shows its in-cluster address', async () => {
    mocks.acceptProjectNetworkShare.mockResolvedValue({ ...share, status: 'active', acceptedAt: '2026-11-11T01:00:00Z' })
    renderCard(true)
    expect(await screen.findByText('auth-api.prj-auth.svc')).toBeInTheDocument()
    expect(screen.getByText('Awaiting acceptance')).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Accept' }))
    await waitFor(() => expect(mocks.acceptProjectNetworkShare).toHaveBeenCalledWith('prj_demo', 'nss_auth'))
  })

  it('shares a deployment target with another project space by identifier', async () => {
    mocks.createProjectNetworkShare.mockResolvedValue({ ...share, id: 'nss_db', approvalRequired: false })
    renderCard(true)
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Share service' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Deployment target' }))
    expect(screen.queryByRole('button', { name: /postgres \/ legacy/ })).not.toBeInTheDocument()
    await user.click(await screen.findByRole('button', { name: /postgres \/ prod/ }))
    await user.type(within(dialog).getByRole('textbox', { name: /^Consumer project space identifier/ }), 'web-team')
    await user.click(within(dialog).getByRole('button', { name: 'Share service' }))
    await waitFor(() => expect(mocks.createProjectNetworkShare).toHaveBeenCalledWith('prj_demo', { deploymentTargetId: 'dplt_db', consumerProjectIdentifier: 'web-team' }))
  })

  it('warns only about egress exceptions on the selected target cluster', async () => {
    mocks.getProjectNetworkPolicy.mockResolvedValue({
      ingressMode: 'isolated',
      egressMode: 'internet',
      egressAllowlist: '',
      egressExceptions: [{ id: 'nex_b', runtimeClusterId: 'clu_b', name: 'idc', cidr: '10.0.0.0/24', ports: '', protocol: 'tcp', reason: 'r', deploymentTargetIds: [], includeBuilds: false }],
    })
    renderCard(true)
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Share service' }))
    // 未选部署配置时按全部例外提示。
    expect(await screen.findByText(/egress exceptions granted to this project space: 1/i)).toBeInTheDocument()
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Deployment target' }))
    await user.click(await screen.findByRole('button', { name: /postgres \/ prod/ }))
    await waitFor(() => expect(screen.queryByText(/egress exceptions granted to this project space/i)).not.toBeInTheDocument())
  })

  it('hides management actions without project management permission', async () => {
    renderCard(false)
    expect(await screen.findByText('auth-api.prj-auth.svc')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Accept' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Share service' })).not.toBeInTheDocument()
  })
})
