import type { Project, RuntimeCluster } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ClusterEgressExceptionsDialog } from './cluster-egress-exceptions-dialog'

const mocks = vi.hoisted(() => ({
  createRuntimeClusterEgressException: vi.fn(),
  deleteRuntimeClusterEgressException: vi.fn(),
  getProjectTopology: vi.fn(),
  listRuntimeClusterEgressExceptions: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return { ...actual, api: { ...actual.api, ...mocks } }
})

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const cluster = { id: 'clu_dc', name: 'Data center' } as RuntimeCluster
const projects = [{ id: 'prj_pay', name: 'Payments', identifier: 'payments' }] as Project[]
const page = (items: unknown[]) => ({ items, page: 1, pageSize: 20, sortBy: 'createdAt', sortOrder: 'desc', total: items.length, totalPages: 1 })

function renderDialog() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <ClusterEgressExceptionsDialog cluster={cluster} projects={projects} onOpenChange={vi.fn()} />
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

describe('cluster egress exceptions dialog', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.getProjectTopology.mockResolvedValue({ generatedAt: '', nodes: [], edges: [], warnings: [] })
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
      grants: [{ projectId: 'prj_pay', deploymentTargetIds: [], includeBuilds: false }],
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
})
