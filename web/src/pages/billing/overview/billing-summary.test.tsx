import type { BillingDisplay } from '../records/billing-list-cells'
import type { BillingSummary as BillingSummaryData, GatewayTrafficStatus } from '@/api'
import { render, screen } from '@testing-library/react'
import { beforeAll, describe, expect, it } from 'vitest'
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
  runtimeHourCost: () => 0,
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
  it('uses shared metrics and semantic surfaces while preserving gateway guidance', () => {
    const { container } = render(
      <BillingSummary
        accountLoading={false}
        accountSummary={summary({ balanceCredits: '100' })}
        billingDisplay={billingDisplay}
        canManageBilling
        gatewayTrafficStatus={gatewayTrafficStatus}
        gatewayTrafficStatusLoaded
        scopedFetching={false}
        scopedLoading={false}
        scopedSummary={summary({ periodCategories: [{ amountCredits: '12', category: 'build' }, { amountCredits: '3', category: 'gateway' }] })}
      />,
    )

    expect(container.querySelectorAll('[data-slot="metric-group"]')).toHaveLength(1)
    expect(container.querySelectorAll('[data-slot="metric-item"]')).toHaveLength(4)
    expect(screen.getByText('C:100')).toBeVisible()
    expect(screen.getByText('F:100')).toBeVisible()

    const categorySection = screen.getByText('Period spend by category').closest('[data-slot="surface"]')
    expect(categorySection).toHaveAttribute('data-variant', 'bordered')
    expect(screen.getByText('Not deployed')).toBeVisible()
    expect(screen.getByRole('link', { name: 'Configure Traefik metrics' })).toHaveAttribute('target', '_blank')
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
})

function summary(overrides: Partial<BillingSummaryData> = {}): BillingSummaryData {
  return {
    availableCredits: '80',
    balanceCredits: '100',
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
