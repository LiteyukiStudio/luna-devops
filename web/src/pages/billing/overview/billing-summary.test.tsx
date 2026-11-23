import type { BillingDisplay } from '../records/billing-list-cells'
import type { BillingSpendTimeseries, BillingSummary as BillingSummaryData, GatewayTrafficStatus } from '@/api'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { BillingBalanceWarning, BillingSummary } from './billing-summary'

const billingDisplay: BillingDisplay = {
  buildMinuteCost: () => 0,
  creditsPerFiatUnit: 1,
  currencyUnit: 'Credits',
  fiatCurrencyUnit: 'USD',
  formatAmount: value => `${value ?? ''}`,
  formatAmountWithUnit: (value: string | number | undefined) => `C:${value ?? ''}`,
  formatFiatAmount: (value: string | number | undefined) => `F:${value ?? ''}`,
  formatSignedAmountWithUnit: value => `C:${value ?? ''}`,
  runtimeHourCostRange: () => ({ ceiling: 0, floor: 0 }),
}

const gatewayTrafficStatus: GatewayTrafficStatus = {
  available: false,
  componentId: '',
  installableTemplateId: '',
  installed: false,
  lastError: '',
  observationCode: 'not_installed',
  status: 'not-configured',
}

beforeAll(async () => {
  await i18next.changeLanguage('en-US')
})

describe('billing summary', () => {
  it('uses shared metrics and semantic surfaces while preserving gateway guidance and spend chart controls', async () => {
    const user = userEvent.setup()
    const onChartTypeChange = vi.fn()
    const onGranularityChange = vi.fn()
    const onActiveCategoryChange = vi.fn()
    const onSelectBucket = vi.fn()

    const { container } = render(
      <BillingSummary
        accountLoading={false}
        accountSummary={summary({ balanceCredits: '100' })}
        billingDisplay={billingDisplay}
        canManageBilling
        chartType="bar"
        gatewayTrafficStatus={gatewayTrafficStatus}
        gatewayTrafficStatusLoaded
        granularity="hour"
        locale="en-US"
        scopedFetching={false}
        scopedLoading={false}
        scopedSummary={summary({ periodCategories: [{ amountCredits: '12', category: 'build' }, { amountCredits: '3', category: 'gateway' }] })}
        timeseries={timeseriesFixture()}
        timeZone="UTC"
        onActiveCategoryChange={onActiveCategoryChange}
        onChartTypeChange={onChartTypeChange}
        onGranularityChange={onGranularityChange}
        onSelectBucket={onSelectBucket}
      />,
    )

    expect(container.querySelectorAll('[data-slot="metric-group"]')).toHaveLength(1)
    expect(container.querySelectorAll('[data-slot="metric-item"]')).toHaveLength(4)
    expect(screen.getByText('C:100')).toBeVisible()
    expect(screen.getByText('F:100')).toBeVisible()

    const distributionSection = screen.getByText('Spend distribution').closest('[data-slot="surface"]')
    expect(distributionSection).toHaveAttribute('data-variant', 'bordered')
    expect(screen.getByText('Not deployed')).toBeVisible()
    expect(screen.getByRole('link', { name: 'Configure Traefik metrics' })).toHaveAttribute('target', '_blank')

    const chart = screen.getByRole('group', { name: 'Spend over time chart' })
    expect(chart).toHaveAttribute('data-billing-chart-type', 'bar')
    expect(chart).toHaveAttribute('data-billing-granularity', 'hour')

    await user.click(screen.getByRole('button', { name: 'Area' }))
    expect(onChartTypeChange).toHaveBeenCalledWith('area')

    await user.click(screen.getByRole('button', { name: 'Daily' }))
    expect(onGranularityChange).toHaveBeenCalledWith('day')

    await user.click(screen.getByRole('button', { name: /Build/i }))
    expect(onActiveCategoryChange).toHaveBeenCalledWith('build')

    // Clicking the peak chip selects the peak bucket for drill-down
    await user.click(screen.getByRole('button', { name: /Peak/i }))
    expect(onSelectBucket).toHaveBeenCalledTimes(1)
  })

  it('keeps settled gateway category visible alongside probe guidance when historical gateway charges exist', () => {
    render(
      <BillingSummary
        accountLoading={false}
        accountSummary={summary()}
        billingDisplay={billingDisplay}
        canManageBilling
        gatewayTrafficStatus={gatewayTrafficStatus}
        gatewayTrafficStatusLoaded
        scopedFetching={false}
        scopedLoading={false}
        scopedSummary={summary()}
        timeseries={timeseriesFixture({
          categories: [
            { amountCredits: '9', category: 'runtime' },
            { amountCredits: '3', category: 'gateway' },
          ],
        })}
      />,
    )

    expect(screen.getByRole('button', { name: /Gateway.*C:3/i })).toBeVisible()
    expect(screen.getByText('Not deployed')).toBeVisible()
  })

  it('disables hourly granularity when the selected period exceeds the hourly bucket limit', () => {
    render(
      <BillingSummary
        accountLoading={false}
        accountSummary={summary()}
        billingDisplay={billingDisplay}
        billingPeriod={{ endDate: '2026-12-31', preset: 'thisYear', startDate: '2026-01-01' }}
        canManageBilling
        gatewayTrafficStatusLoaded={false}
        granularity="month"
        scopedFetching={false}
        scopedLoading={false}
        scopedSummary={summary()}
        timeseries={timeseriesFixture({ granularity: 'month' })}
      />,
    )

    expect(screen.getByRole('button', { name: 'Hourly' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Daily' })).not.toBeDisabled()
    expect(screen.getByRole('button', { name: 'Monthly' })).not.toBeDisabled()
  })

  it('maps low and insufficient balances to warning and destructive alerts', () => {
    const { rerender } = render(
      <BillingBalanceWarning accountSummary={summary({ balanceStatus: 'low' })} billingDisplay={billingDisplay} />,
    )

    expect(screen.getByRole('alert')).toHaveClass('bg-warning-subtle')
    expect(screen.getByText('C:80')).toBeVisible()

    rerender(
      <BillingBalanceWarning accountSummary={summary({ balanceStatus: 'insufficient' })} billingDisplay={billingDisplay} />,
    )
    expect(screen.getByRole('alert')).toHaveClass('bg-danger/5')
  })

  it('labels the platform wallet total without an account balance status', () => {
    const platform = summary({ balanceCredits: '900', balanceScope: 'platform_total', balanceStatus: undefined })
    render(
      <>
        <BillingBalanceWarning accountSummary={platform} billingDisplay={billingDisplay} />
        <BillingSummary
          accountLoading={false}
          accountSummary={platform}
          billingDisplay={billingDisplay}
          canManageBilling
          gatewayTrafficStatusLoaded={false}
          scopedFetching={false}
          scopedLoading={false}
          scopedSummary={summary({ periodSpend: '0' })}
        />
      </>,
    )

    expect(screen.getByText('Platform wallet total')).toBeVisible()
    expect(screen.queryByText('Balance')).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByText('Balance healthy')).toBeNull()
  })

  it('renders skeletons instead of dashes or empty spend copy while loading', () => {
    const { container } = render(
      <BillingSummary
        accountLoading
        billingDisplay={billingDisplay}
        canManageBilling
        gatewayTrafficStatusLoaded={false}
        scopedFetching
        scopedLoading
        timeseriesFetching
        timeseriesLoading
      />,
    )

    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThanOrEqual(8)
    expect(screen.queryByText('-')).toBeNull()
    expect(screen.queryByText('C:')).toBeNull()
    expect(screen.queryByText(i18next.t('billingPage.emptyPeriodCategories'))).toBeNull()
  })

  it('renders retryable error states instead of zero credits on summary or timeseries failure', async () => {
    const user = userEvent.setup()
    const onRetryAccount = vi.fn()
    const onRetryScoped = vi.fn()
    const onRetryTimeseries = vi.fn()

    render(
      <BillingSummary
        accountError
        accountLoading={false}
        billingDisplay={billingDisplay}
        canManageBilling
        gatewayTrafficStatusLoaded={false}
        scopedError
        scopedFetching={false}
        scopedLoading={false}
        timeseriesError
        onRetryAccount={onRetryAccount}
        onRetryScoped={onRetryScoped}
        onRetryTimeseries={onRetryTimeseries}
      />,
    )

    expect(screen.queryByText('C:')).toBeNull()
    expect(screen.queryByText(i18next.t('billingPage.emptyPeriodCategories'))).toBeNull()
    await user.click(screen.getByRole('button', { name: i18next.t('common.retry') }))
    expect(onRetryAccount).toHaveBeenCalledTimes(1)
    expect(onRetryScoped).toHaveBeenCalledTimes(1)
    expect(onRetryTimeseries).toHaveBeenCalledTimes(1)
  })

  it('keeps skeletons instead of emptyPeriodCategories while gatewayTrafficStatusLoading is true', () => {
    const { container } = render(
      <BillingSummary
        accountLoading={false}
        accountSummary={summary({ periodSpend: '0' })}
        billingDisplay={billingDisplay}
        canManageBilling
        gatewayTrafficStatusLoaded={false}
        gatewayTrafficStatusLoading
        scopedFetching={false}
        scopedLoading={false}
        scopedSummary={summary({ periodCategories: [], periodSpend: '0' })}
      />,
    )

    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThanOrEqual(4)
    expect(screen.queryByText(i18next.t('billingPage.emptyPeriodCategories'))).toBeNull()
  })
})

function summary(overrides: Partial<BillingSummaryData> = {}): BillingSummaryData {
  return {
    availableCredits: '80',
    balanceCredits: '100',
    balanceScope: 'account',
    balanceStatus: 'ok',
    lowBalanceLimit: '20',
    monthSpend: '15',
    monthlyCategories: [],
    pendingSpend: '4',
    periodCategories: [],
    periodSpend: '12',
    todaySpend: '2',
    ...overrides,
  }
}

function timeseriesFixture(overrides: Partial<BillingSpendTimeseries> = {}): BillingSpendTimeseries {
  return {
    buckets: [
      {
        bucketStart: '2026-11-22T22:00:00Z',
        categories: [{ amountCredits: '4', category: 'runtime' }],
        periodEnd: '2026-11-22T23:00:00Z',
        periodStart: '2026-11-22T22:00:00Z',
        totalCredits: '4',
      },
      {
        bucketStart: '2026-11-22T23:00:00Z',
        categories: [{ amountCredits: '8', category: 'build' }],
        periodEnd: '2026-11-23T00:00:00Z',
        periodStart: '2026-11-22T23:00:00Z',
        totalCredits: '8',
      },
    ],
    categories: [
      { amountCredits: '8', category: 'build' },
      { amountCredits: '4', category: 'runtime' },
    ],
    granularity: 'hour',
    periodEnd: '2026-11-23T00:00:00Z',
    periodStart: '2026-11-22T22:00:00Z',
    timeZone: 'UTC',
    totalCredits: '12',
    ...overrides,
  }
}
