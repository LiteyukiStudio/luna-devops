import type { BillingSpendTimeseries, BillingSummary } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { BillingPage } from './BillingPage'

const mocks = vi.hoisted(() => ({
  getBillingSpendTimeseries: vi.fn(),
  getBillingSummary: vi.fn(),
  getGatewayTrafficStatus: vi.fn(),
  listApplications: vi.fn(),
  listBillingDeploymentSpend: vi.fn(),
  listBillingLedgerEntries: vi.fn(),
  listBillingRateRules: vi.fn(),
  listProjectsPage: vi.fn(),
  listUsers: vi.fn(),
}))

vi.mock('@/app/session-context', () => ({
  useSession: () => ({ user: { id: 'usr_1', role: 'user' } }),
}))

vi.mock('@/app/public-config-context', () => ({
  usePublicConfig: () => ({
    'billing.creditsDisplayName': 'Credits',
    'billing.creditsPerFiatUnit': '10',
    'billing.fiatCurrencyUnit': 'CNY',
  }),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getBillingSpendTimeseries: mocks.getBillingSpendTimeseries,
      getBillingSummary: mocks.getBillingSummary,
      getGatewayTrafficStatus: mocks.getGatewayTrafficStatus,
      listApplications: mocks.listApplications,
      listBillingDeploymentSpend: mocks.listBillingDeploymentSpend,
      listBillingLedgerEntries: mocks.listBillingLedgerEntries,
      listBillingRateRules: mocks.listBillingRateRules,
      listProjectsPage: mocks.listProjectsPage,
      listUsers: mocks.listUsers,
    },
  }
})

function emptyPage() {
  return {
    items: [],
    page: 1,
    pageSize: 20,
    sortBy: 'amountCredits',
    sortOrder: 'desc' as const,
    total: 0,
    totalPages: 0,
  }
}

function summaryFixture(): BillingSummary {
  return {
    availableCredits: '100',
    balanceCredits: '100',
    balanceScope: 'account',
    balanceStatus: 'ok',
    lowBalanceLimit: '20',
    monthSpend: '18.5',
    monthlyCategories: [{ amountCredits: '18.5', category: 'ai' }],
    pendingSpend: '0',
    periodCategories: [{ amountCredits: '18.5', category: 'ai' }],
    periodSpend: '18.5',
    todaySpend: '6.25',
  }
}

function timeseriesFixture(granularity: 'hour' | 'day' | 'month' = 'day'): BillingSpendTimeseries {
  return {
    buckets: [
      {
        bucketStart: '2026-11-22T00:00:00Z',
        categories: [{ amountCredits: '12.25', category: 'ai' }],
        periodEnd: '2026-11-23T00:00:00Z',
        periodStart: '2026-11-22T00:00:00Z',
        totalCredits: '12.25',
      },
      {
        bucketStart: '2026-11-23T00:00:00Z',
        categories: [{ amountCredits: '6.25', category: 'ai' }],
        periodEnd: '2026-11-24T00:00:00Z',
        periodStart: '2026-11-23T00:00:00Z',
        totalCredits: '6.25',
      },
    ],
    categories: [{ amountCredits: '18.5', category: 'ai' }],
    granularity,
    periodEnd: '2026-11-24T00:00:00Z',
    periodStart: '2026-11-01T00:00:00Z',
    timeZone: 'UTC',
    totalCredits: '18.5',
  }
}

describe('billing page spend chart integration', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    window.sessionStorage.clear()
    await i18next.changeLanguage('en-US')
    mocks.listProjectsPage.mockResolvedValue({ ...emptyPage(), items: [] })
    mocks.listApplications.mockResolvedValue([])
    mocks.listUsers.mockResolvedValue({ ...emptyPage(), items: [] })
    mocks.listBillingRateRules.mockResolvedValue([])
    mocks.getBillingSummary.mockResolvedValue(summaryFixture())
    mocks.getBillingSpendTimeseries.mockImplementation((_projects, params) =>
      Promise.resolve(timeseriesFixture(params?.granularity ?? 'day')),
    )
    mocks.getGatewayTrafficStatus.mockResolvedValue({
      available: true,
      componentId: 'gateway-traffic-probe',
      installableTemplateId: '',
      installed: true,
      lastError: '',
      observationCode: 'ok',
      status: 'ready',
    })
    mocks.listBillingDeploymentSpend.mockResolvedValue(emptyPage())
    mocks.listBillingLedgerEntries.mockResolvedValue(emptyPage())
  })

  it('switches granularity and chart view and drills down into a focused time bucket', async () => {
    const user = userEvent.setup()
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <BillingPage />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    const chart = await screen.findByRole('group', { name: 'Spend over time chart' })
    expect(chart).toHaveAttribute('data-billing-chart-type', 'bar')
    expect(chart).toHaveAttribute('data-billing-granularity', 'day')

    await user.click(screen.getByRole('button', { name: 'Area' }))
    expect(chart).toHaveAttribute('data-billing-chart-type', 'area')

    await user.click(screen.getByRole('button', { name: 'Hourly' }))
    await waitFor(() => {
      expect(mocks.getBillingSpendTimeseries).toHaveBeenCalledWith(
        undefined,
        expect.objectContaining({ granularity: 'hour' }),
        expect.any(AbortSignal),
      )
    })
    const hourlyChart = await screen.findByRole('group', { name: 'Spend over time chart' })
    expect(hourlyChart).toHaveAttribute('data-billing-granularity', 'hour')

    // Keyboard navigation onto the second (non-peak) bucket and pressing Enter filters records to that bucket
    hourlyChart.focus()
    await user.keyboard('{ArrowRight}{ArrowRight}{Enter}')
    await waitFor(() => {
      expect(mocks.listBillingDeploymentSpend).toHaveBeenCalledWith(
        expect.objectContaining({
          periodEnd: '2026-11-24T00:00:00Z',
          periodStart: '2026-11-23T00:00:00Z',
        }),
      )
    })
    expect(screen.getByText(/Filtered to:/i)).toBeVisible()

    // Switch to Price table tab, then deselect the focused bucket; tab must remain on Price table
    await user.click(screen.getByRole('tab', { name: 'Price table' }))
    hourlyChart.focus()
    await user.keyboard('{ArrowRight}{ArrowRight}{Enter}')
    expect(screen.queryByText(/Filtered to:/i)).toBeNull()
    expect(screen.getByRole('tab', { name: 'Price table' })).toHaveAttribute('data-state', 'active')
  })

  it('displays inline usage quantities in unified billing ledger and filters by type and category', async () => {
    const user = userEvent.setup()
    mocks.listBillingLedgerEntries.mockImplementation((params) => {
      if (params?.category === 'gateway')
        return Promise.resolve(emptyPage())
      return Promise.resolve({
        items: [
          {
            amountCredits: '-6.25',
            applicationId: '',
            applicationIdentifier: '',
            applicationName: '',
            balanceAfterCredits: '93.75',
            createdAt: '2026-11-23T01:00:00Z',
            createdBy: 'system',
            description: 'AI model token usage',
            id: 'bled_ai_1',
            meter: 'ai.completion_tokens_1m',
            occurredAt: '2026-11-23T00:59:00Z',
            projectId: '',
            reason: 'ai.usage',
            resourceId: 'aiuse_1',
            resourceType: 'ai_model_request',
            type: 'debit',
            usageQuantity: '1.5',
            usageRecordId: 'busg_ai_1',
            usageUnit: 'million_tokens',
            userId: 'usr_1',
          },
          {
            amountCredits: '100',
            applicationId: '',
            applicationIdentifier: '',
            applicationName: '',
            balanceAfterCredits: '100',
            createdAt: '2026-11-22T00:00:00Z',
            createdBy: 'usr_admin',
            description: 'Initial recharge',
            id: 'bled_credit_1',
            meter: '',
            occurredAt: '2026-11-22T00:00:00Z',
            projectId: '',
            reason: 'billing.recharge',
            resourceId: 'usr_1',
            resourceType: 'user_wallet',
            type: 'credit',
            usageRecordId: '',
            userId: 'usr_1',
          },
        ],
        page: 1,
        pageSize: 20,
        sortBy: 'occurredAt',
        sortOrder: 'desc' as const,
        total: 2,
        totalPages: 1,
      })
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <BillingPage />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByRole('tab', { name: 'Billing ledger' })).toBeVisible()
    expect(screen.queryByRole('tab', { name: 'Usage records' })).toBeNull()

    await user.click(screen.getByRole('tab', { name: 'Billing ledger' }))
    expect((await screen.findAllByText('1.5 1M tokens')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('AI completion tokens').length).toBeGreaterThan(0)

    await user.click(screen.getByRole('combobox', { name: 'Filter by category' }))
    await user.click(await screen.findByRole('option', { name: 'Gateway' }))
    await waitFor(() => {
      expect(mocks.listBillingLedgerEntries).toHaveBeenCalledWith(
        expect.objectContaining({ category: 'gateway', page: 1, pageSize: 20 }),
      )
    })
    expect(await screen.findByText('No matching billing ledger entries')).toBeVisible()

    // Clearing filters from the filtered empty state restores the full ledger list
    const clearButtons = screen.getAllByRole('button', { name: 'Clear filters' })
    await user.click(clearButtons[0])
    await waitFor(() => {
      expect(mocks.listBillingLedgerEntries).toHaveBeenLastCalledWith(
        expect.objectContaining({ category: undefined, page: 1, type: undefined }),
      )
    })
    expect((await screen.findAllByText('1.5 1M tokens')).length).toBeGreaterThan(0)
  })

  it('supports project workspace mode with stage and application filters, all-project toggle, and wallet privacy', async () => {
    const user = userEvent.setup()
    mocks.listProjectsPage.mockResolvedValue({
      ...emptyPage(),
      items: [
        { billingOwnerUserId: 'usr_owner', description: '', id: 'prj_demo', identifier: 'demo', maxConcurrentBuilds: 2, name: 'Demo Space', webConsoleEnabled: true },
        { billingOwnerUserId: 'usr_other', description: '', id: 'prj_two', identifier: 'two', maxConcurrentBuilds: 2, name: 'Second Space', webConsoleEnabled: true },
      ],
      total: 2,
      totalPages: 1,
    })
    mocks.listApplications.mockResolvedValue([
      { createdAt: '', description: '', id: 'app_blog', identifier: 'blog', name: 'Blog Frontend', projectId: 'prj_demo', updatedAt: '' },
      { createdAt: '', description: '', id: 'app_api', identifier: 'api', name: 'Blog API', projectId: 'prj_demo', updatedAt: '' },
    ])
    mocks.getBillingSummary.mockResolvedValue({
      ...summaryFixture(),
      availableCredits: '0',
      balanceCredits: '0',
      balanceScope: 'project',
      balanceStatus: undefined,
      lowBalanceLimit: '0',
      monthSpend: '42',
    })
    mocks.listBillingDeploymentSpend.mockResolvedValue({
      ...emptyPage(),
      items: [
        {
          amountCredits: '12',
          applicationId: 'app_blog',
          applicationIdentifier: 'blog',
          applicationName: 'Blog Frontend',
          buildCredits: '2',
          deploymentTargetId: 'dplt_prod',
          deploymentTargetName: 'prod-web',
          deploymentTargetStage: 'prod',
          gatewayCredits: '1',
          otherCredits: '0',
          projectId: 'prj_demo',
          projectIdentifier: 'demo',
          projectName: 'Demo Space',
          runtimeCredits: '9',
          spendGroup: 'deployment_target',
          storageCredits: '0',
        },
      ],
      total: 1,
      totalPages: 1,
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/projects/prj_demo?tab=billing']}>
          <BillingPage projectId="prj_demo" />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByText('This month spend')).toBeVisible()
    expect(screen.queryByText('Balance')).toBeNull()
    expect((await screen.findAllByText('Production · prod')).length).toBeGreaterThan(0)

    expect(mocks.getBillingSummary).toHaveBeenCalledTimes(1)
    expect(mocks.getBillingSummary).toHaveBeenCalledWith(
      ['prj_demo'],
      expect.objectContaining({ scopeMode: 'project' }),
    )

    // Filter by application and stage
    await user.click(screen.getByRole('combobox', { name: 'Filter by application' }))
    await user.click(await screen.findByRole('option', { name: 'Blog Frontend' }))
    await user.click(screen.getByRole('combobox', { name: 'Filter by stage' }))
    await user.click(await screen.findByRole('option', { name: 'Production' }))

    await waitFor(() => {
      expect(mocks.listBillingDeploymentSpend).toHaveBeenLastCalledWith(
        expect.objectContaining({
          applicationId: 'app_blog',
          projectIds: ['prj_demo'],
          scopeMode: 'project',
          stage: 'prod',
        }),
      )
    })

    // Switch to all accessible project spaces and reset back to current project
    await user.click(screen.getByRole('button', { name: 'View all project spaces' }))
    await waitFor(() => {
      expect(mocks.getBillingSummary).toHaveBeenLastCalledWith(
        undefined,
        expect.objectContaining({ scopeMode: 'project' }),
      )
    })
    expect(screen.getByRole('button', { name: 'Select project spaces' })).toHaveTextContent('All project spaces')
    expect(window.sessionStorage.getItem('luna.billing.projectScope')).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Current project space only' }))
    await waitFor(() => {
      expect(mocks.getBillingSummary).toHaveBeenLastCalledWith(
        ['prj_demo'],
        expect.objectContaining({ scopeMode: 'project' }),
      )
    })

    // Ledger tab omits private wallet Balance after column and type filter, and supports application filter
    await user.click(screen.getByRole('tab', { name: 'Billing ledger' }))
    expect(screen.queryByText('Balance after')).toBeNull()
    expect(screen.queryByRole('combobox', { name: 'Filter by entry type' })).toBeNull()
    await user.click(screen.getByRole('combobox', { name: 'Filter by application' }))
    await user.click(await screen.findByRole('option', { name: 'Blog API' }))
    await waitFor(() => {
      expect(mocks.listBillingLedgerEntries).toHaveBeenLastCalledWith(
        expect.objectContaining({
          applicationId: 'app_api',
          projectIds: ['prj_demo'],
          scopeMode: 'project',
        }),
      )
    })
  })

  it('prunes non-owned cached project IDs in wallet mode and preserves chart when scoped summary fails', async () => {
    window.sessionStorage.setItem('luna.billing.projectScope', JSON.stringify(['prj_not_owned', 'prj_owned']))
    mocks.listProjectsPage.mockResolvedValue({
      ...emptyPage(),
      items: [
        { billingOwnerUserId: 'usr_1', description: '', id: 'prj_owned', identifier: 'owned', maxConcurrentBuilds: 2, name: 'Owned Space', webConsoleEnabled: true },
        { billingOwnerUserId: 'usr_other', description: '', id: 'prj_not_owned', identifier: 'other', maxConcurrentBuilds: 2, name: 'Team Space', webConsoleEnabled: true },
      ],
      total: 2,
      totalPages: 1,
    })
    mocks.getBillingSummary.mockImplementation((projectIds) => {
      if (projectIds)
        return Promise.reject(new Error('Summary unavailable'))
      return Promise.resolve(summaryFixture())
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <BillingPage />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    // Non-owned cached project ID is gated before initial requests fire, persisted pruned in sessionStorage, and chart stays visible despite scoped summary failure
    await waitFor(() => {
      expect(mocks.getBillingSpendTimeseries).toHaveBeenCalledWith(
        ['prj_owned'],
        expect.any(Object),
        expect.any(AbortSignal),
      )
    })
    expect(mocks.getBillingSpendTimeseries).not.toHaveBeenCalledWith(
      expect.arrayContaining(['prj_not_owned']),
      expect.anything(),
      expect.anything(),
    )
    expect(mocks.getBillingSummary).not.toHaveBeenCalledWith(
      expect.arrayContaining(['prj_not_owned']),
      expect.anything(),
    )
    expect(window.sessionStorage.getItem('luna.billing.projectScope')).toBe(JSON.stringify(['prj_owned']))
    expect(await screen.findByRole('group', { name: 'Spend over time chart' })).toBeVisible()
    expect(screen.getByText('Summary unavailable')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible()
  })
})
