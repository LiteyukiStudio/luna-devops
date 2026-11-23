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
  listBillingDeploymentSpend: vi.fn(),
  listBillingLedgerEntries: vi.fn(),
  listBillingRateRules: vi.fn(),
  listBillingUsageRecords: vi.fn(),
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
      listBillingDeploymentSpend: mocks.listBillingDeploymentSpend,
      listBillingLedgerEntries: mocks.listBillingLedgerEntries,
      listBillingRateRules: mocks.listBillingRateRules,
      listBillingUsageRecords: mocks.listBillingUsageRecords,
      listProjectsPage: mocks.listProjectsPage,
      listUsers: mocks.listUsers,
    },
  }
})

function emptyPage() {
  return {
    items: [],
    page: 1,
    pageSize: 10,
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
    mocks.listBillingUsageRecords.mockResolvedValue(emptyPage())
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
})
