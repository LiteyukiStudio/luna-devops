import type { RuntimeCluster } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ClusterEgressExceptionsDialog } from './cluster-egress-exceptions-dialog'

const mocks = vi.hoisted(() => ({
  approveRuntimeClusterNetworkShare: vi.fn(),
  createRuntimeClusterEgressException: vi.fn(),
  deleteRuntimeClusterEgressException: vi.fn(),
  getProjectTopology: vi.fn(),
  listProjectsPage: vi.fn(),
  listRuntimeClusterEgressExceptions: vi.fn(),
  listRuntimeClusterNetworkShareApprovals: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return { ...actual, api: { ...actual.api, ...mocks } }
})

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const cluster = { id: 'clu_dc', name: 'Data center' } as RuntimeCluster
const page = (items: unknown[]) => ({ items, page: 1, pageSize: 20, sortBy: 'createdAt', sortOrder: 'desc', total: items.length, totalPages: 1 })

function renderDialog() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <ClusterEgressExceptionsDialog cluster={cluster} onOpenChange={vi.fn()} />
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

describe('cluster egress exceptions dialog', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.getProjectTopology.mockResolvedValue({ generatedAt: '', nodes: [], edges: [], warnings: [] })
    mocks.listProjectsPage.mockResolvedValue(page([{ id: 'prj_pay', name: 'Payments', identifier: 'payments' }]))
    mocks.listRuntimeClusterNetworkShareApprovals.mockResolvedValue(page([]))
  })

  it('approves cross-project shares from project spaces holding exceptions', async () => {
    mocks.listRuntimeClusterEgressExceptions.mockResolvedValue(page([{
      id: 'nex_db',
      runtimeClusterId: 'clu_dc',
      name: 'Billing MySQL',
      cidr: '192.168.31.10/32',
      ports: '3306',
      protocol: 'tcp',
      reason: 'Billing',
      grants: [{ projectId: 'prj_pay', projectName: 'Payments', deploymentTargetIds: [], includeBuilds: false, providedShares: 1 }],
      createdBy: 'usr',
      createdAt: '',
      updatedAt: '',
    }]))
    mocks.listRuntimeClusterNetworkShareApprovals.mockResolvedValue(page([{
      id: 'nss_pay',
      providerProjectId: 'prj_pay',
      providerProjectName: 'Payments',
      deploymentTargetId: 'dplt_api',
      deploymentTargetName: 'prod',
      applicationName: 'pay-api',
      consumerProjectId: 'prj_web',
      consumerProjectName: 'Storefront',
      address: 'pay-api.payments.svc',
      status: 'pending_approval',
      approvalRequired: true,
      acceptedAt: '',
      approvedAt: null,
      createdAt: '',
    }]))
    mocks.approveRuntimeClusterNetworkShare.mockResolvedValue({})
    renderDialog()
    expect(await screen.findByText('Shared services: 1')).toBeInTheDocument()
    expect(await screen.findByText('Storefront')).toBeInTheDocument()
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Approve' }))
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Approve' }))
    await waitFor(() => expect(mocks.approveRuntimeClusterNetworkShare).toHaveBeenCalledWith('clu_dc', 'nss_pay'))
  })

  it('lists exceptions with their granted project spaces', async () => {
    mocks.listRuntimeClusterEgressExceptions.mockResolvedValue(page([{
      id: 'nex_db',
      runtimeClusterId: 'clu_dc',
      name: 'Billing MySQL',
      cidr: '192.168.31.10/32',
      ports: '3306',
      protocol: 'tcp',
      reason: 'Billing',
      grants: [{ projectId: 'prj_pay', projectName: 'Payments', deploymentTargetIds: [], includeBuilds: false }],
      createdBy: 'usr',
      createdAt: '',
      updatedAt: '',
    }]))
    renderDialog()
    expect(await screen.findByText('Billing MySQL')).toBeInTheDocument()
    expect(screen.getByText('Payments')).toBeInTheDocument()
  })

  it('creates an exception with a reason and a project grant', async () => {
    mocks.listRuntimeClusterEgressExceptions.mockResolvedValue(page([]))
    mocks.createRuntimeClusterEgressException.mockResolvedValue({})
    renderDialog()
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Add exception' }))
    await user.type(screen.getByRole('textbox', { name: /^Name/ }), 'Billing MySQL')
    await user.type(screen.getByRole('textbox', { name: /^Target CIDR/ }), '192.168.31.10/32')
    await user.type(screen.getByRole('textbox', { name: /^Ports/ }), '3306')
    await user.type(screen.getByRole('textbox', { name: /^Approval reason/ }), 'Billing database in the data center')
    await user.click(screen.getByRole('button', { name: 'Grant a project space' }))
    await user.click(screen.getByRole('button', { name: 'Project space' }))
    await user.click(await screen.findByRole('button', { name: /Payments/ }))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(mocks.createRuntimeClusterEgressException).toHaveBeenCalledWith('clu_dc', {
      name: 'Billing MySQL',
      cidr: '192.168.31.10/32',
      ports: '3306',
      protocol: 'tcp',
      reason: 'Billing database in the data center',
      grants: [{ projectId: 'prj_pay', deploymentTargetIds: [], includeBuilds: false }],
    }))
  })

  it('clears narrowed targets when the project changes and hides projects used by other rows', async () => {
    mocks.listRuntimeClusterEgressExceptions.mockResolvedValue(page([]))
    mocks.createRuntimeClusterEgressException.mockResolvedValue({})
    mocks.listProjectsPage.mockResolvedValue(page([
      { id: 'prj_pay', name: 'Payments', identifier: 'payments' },
      { id: 'prj_ops', name: 'Operations', identifier: 'ops' },
    ]))
    mocks.getProjectTopology.mockResolvedValue({
      generatedAt: '',
      nodes: [{ id: 'app_api', kind: 'application', name: 'api', status: 'ready', deploymentTargets: [{ id: 'dt_api', name: 'prod', stage: 'prod', clusterId: 'clu_dc', enabled: true }] }],
      edges: [],
      warnings: [],
    })
    renderDialog()
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Add exception' }))
    await user.type(screen.getByRole('textbox', { name: /^Name/ }), 'Billing MySQL')
    await user.type(screen.getByRole('textbox', { name: /^Target CIDR/ }), '192.168.31.10/32')
    await user.type(screen.getByRole('textbox', { name: /^Approval reason/ }), 'Billing database')
    await user.click(screen.getByRole('button', { name: 'Grant a project space' }))
    await user.click(screen.getByRole('button', { name: 'Project space' }))
    await user.click(await screen.findByRole('button', { name: /Payments/ }))
    await user.click(screen.getByRole('button', { name: 'Deployment targets' }))
    await user.click(await screen.findByRole('button', { name: /api \/ prod/ }))
    await user.keyboard('{Escape}')
    await user.click(screen.getByRole('button', { name: 'Project space' }))
    await user.click(await screen.findByRole('button', { name: /Operations/ }))

    await user.click(screen.getByRole('button', { name: 'Grant a project space' }))
    const selects = screen.getAllByRole('button', { name: 'Project space' })
    await user.click(selects[1])
    expect(await screen.findByRole('button', { name: /Payments/ })).toBeInTheDocument()
    expect(screen.queryAllByRole('button', { name: /Operations/ }).filter(element => element !== selects[0])).toHaveLength(0)
    await user.keyboard('{Escape}')
    await user.click(screen.getAllByRole('button', { name: 'Remove grant' })[1])

    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(mocks.createRuntimeClusterEgressException).toHaveBeenCalledWith('clu_dc', expect.objectContaining({
      grants: [{ projectId: 'prj_ops', deploymentTargetIds: [], includeBuilds: false }],
    })))
  })
})
