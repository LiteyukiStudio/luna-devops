import type { BillingDisplay } from '../records/billing-list-cells'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { BillingScopeToolbar } from '../overview/billing-scope-toolbar'
import { BillingWalletTransactionDialog } from './billing-wallet-transaction-dialog'

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

beforeAll(async () => {
  await i18next.changeLanguage('en-US')
})

describe('billing user error and retry states', () => {
  it('surfaces retry button in BillingScopeToolbar when users fail to load', async () => {
    const user = userEvent.setup()
    const onRetryUsers = vi.fn()

    render(
      <BillingScopeToolbar
        billingPeriod={{ preset: 'thisMonth', startDate: '2026-11-01', endDate: '2026-11-30' }}
        billingUserScopeId="__all_users__"
        canManageBilling
        projectsLoading={false}
        selectedProjectIds={[]}
        userItems={[]}
        usersError
        usersLoading={false}
        visibleProjectItems={[]}
        onBillingUserChange={vi.fn()}
        onCreateTransaction={vi.fn()}
        onPeriodChange={vi.fn()}
        onProjectFilterChange={vi.fn()}
        onRetryUsers={onRetryUsers}
      />,
    )

    await user.click(screen.getByRole('button', { name: i18next.t('common.retry') }))
    expect(onRetryUsers).toHaveBeenCalledTimes(1)
  })

  it('binds fieldControl id onto UserSelect and renders retryable ErrorState in BillingWalletTransactionDialog', async () => {
    const user = userEvent.setup()
    const onRetryUsers = vi.fn()

    render(
      <TooltipProvider>
        <BillingWalletTransactionDialog
          amount="10"
          billingDisplay={billingDisplay}
          description=""
          isPending={false}
          open
          type="credit"
          userId=""
          userItems={[]}
          usersError
          usersLoading={false}
          onAmountChange={vi.fn()}
          onConfirm={vi.fn()}
          onDescriptionChange={vi.fn()}
          onOpenChange={vi.fn()}
          onRetryUsers={onRetryUsers}
          onTypeChange={vi.fn()}
          onUserIdChange={vi.fn()}
        />
      </TooltipProvider>,
    )

    const userSelect = screen.getByRole('button', { name: i18next.t('billingPage.user') })
    expect(userSelect).toHaveAttribute('id')
    expect(screen.getByText(i18next.t('errors.request.failed'))).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: i18next.t('common.retry') }))
    expect(onRetryUsers).toHaveBeenCalledTimes(1)
  })
})
