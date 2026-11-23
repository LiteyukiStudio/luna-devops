import { useTranslation } from 'react-i18next'
import { ContentTabs } from '@/components/common/content-tabs'
import { TabsContent } from '@/components/ui/tabs'
import { useBillingDisplay } from '@/lib/billing-display'
import { BillingScopeToolbar } from './overview/billing-scope-toolbar'
import { BillingBalanceWarning, BillingSummary } from './overview/billing-summary'
import { useBillingPage } from './overview/use-billing-page'
import { BillingPriceList } from './pricing/billing-price-list'
import { BillingDeploymentSpendList } from './records/billing-deployment-spend-list'
import { BillingLedgerList } from './records/billing-ledger-list'
import { BillingWalletTransactionDialog } from './wallet/billing-wallet-transaction-dialog'

export function BillingPage() {
  const { i18n, t } = useTranslation()
  const billingDisplay = useBillingDisplay(i18n.language)
  const billing = useBillingPage()
  const accountSummary = billing.accountSummaryQuery.data

  return (
    <div className="grid min-w-0 gap-5">
      <BillingBalanceWarning accountSummary={accountSummary} billingDisplay={billingDisplay} />

      <BillingScopeToolbar
        billingPeriod={billing.billingPeriod}
        billingUserScopeId={billing.billingUserScopeId}
        canManageBilling={billing.canManageBilling}
        projectsError={billing.projectsQuery.isError}
        projectsLoading={billing.projectsQuery.isPending}
        selectedProjectIds={billing.selectedProjectIds}
        userItems={billing.userItems}
        usersError={billing.canManageBilling && billing.usersQuery.isError}
        usersLoading={billing.canManageBilling && billing.usersQuery.isPending}
        usersRetrying={billing.usersQuery.isFetching}
        visibleProjectItems={billing.visibleProjectItems}
        onBillingUserChange={billing.handleBillingUserChange}
        onCreateTransaction={() => {
          billing.setTransactionUserId(billing.userItems[0]?.id ?? '')
          billing.setTransactionOpen(true)
        }}
        onPeriodChange={billing.handlePeriodChange}
        onProjectFilterChange={billing.handleProjectFilterChange}
        onRetryUsers={() => void billing.usersQuery.refetch()}
      />

      <BillingSummary
        accountError={billing.accountSummaryQuery.isError}
        accountLoading={billing.accountSummaryQuery.isPending}
        accountRetrying={billing.accountSummaryQuery.isFetching}
        accountSummary={accountSummary}
        activeCategory={billing.activeCategory}
        billingDisplay={billingDisplay}
        billingPeriod={billing.billingPeriod}
        canManageBilling={billing.canManageBilling}
        chartType={billing.chartType}
        focusedBucket={billing.focusedBucket}
        gatewayTrafficStatus={billing.gatewayTrafficStatusQuery.data}
        gatewayTrafficStatusLoaded={billing.gatewayTrafficStatusQuery.isSuccess}
        gatewayTrafficStatusLoading={billing.gatewayTrafficStatusQuery.isLoading}
        granularity={billing.effectiveGranularity}
        locale={i18n.language}
        scopedError={billing.scopedSummaryQuery.isError}
        scopedFetching={billing.scopedSummaryQuery.isFetching}
        scopedLoading={billing.scopedSummaryQuery.isPending}
        scopedRetrying={billing.scopedSummaryQuery.isFetching}
        scopedSummary={billing.scopedSummaryQuery.data}
        timeseries={billing.spendTimeseriesQuery.data}
        timeseriesError={billing.spendTimeseriesQuery.isError}
        timeseriesErrorMessage={billing.spendTimeseriesQuery.error instanceof Error ? billing.spendTimeseriesQuery.error.message : undefined}
        timeseriesFetching={billing.spendTimeseriesQuery.isFetching}
        timeseriesLoading={billing.spendTimeseriesQuery.isPending}
        timeseriesRetrying={billing.spendTimeseriesQuery.isFetching}
        timeZone={billing.timeZone}
        onActiveCategoryChange={billing.setActiveCategory}
        onChartTypeChange={billing.setChartType}
        onClearFocusedBucket={billing.handleClearFocusedBucket}
        onGranularityChange={billing.handleGranularityChange}
        onRetryAccount={() => void billing.accountSummaryQuery.refetch()}
        onRetryScoped={() => void billing.scopedSummaryQuery.refetch()}
        onRetryTimeseries={() => void billing.spendTimeseriesQuery.refetch()}
        onSelectBucket={billing.handleSelectBucket}
      />

      <ContentTabs
        tabs={[
          { label: t('billingPage.deploymentSpendTitle'), value: 'deployment-spend' },
          { label: t('billingPage.ledgerTitle'), value: 'ledger' },
          { label: t('billingPage.priceTableTitle'), value: 'prices' },
        ]}
        value={billing.activeTab}
        onValueChange={billing.setActiveTab}
      >
        <TabsContent value="deployment-spend">
          <BillingDeploymentSpendList
            billingDisplay={billingDisplay}
            data={billing.deploymentSpendQuery.data}
            error={billing.deploymentSpendQuery.isError
              ? {
                  onRetry: () => void billing.deploymentSpendQuery.refetch(),
                  retrying: billing.deploymentSpendQuery.isFetching,
                  title: t('errors.request.failed'),
                }
              : undefined}
            loading={billing.deploymentSpendQuery.isPending}
            page={billing.deploymentSpendPage}
            projectMap={billing.projectMap}
            projectsLoading={billing.projectsQuery.isPending}
            onPageChange={billing.setDeploymentSpendPage}
          />
        </TabsContent>
        <TabsContent value="ledger">
          <BillingLedgerList
            billingDisplay={billingDisplay}
            categoryFilter={billing.ledgerCategoryFilter}
            data={billing.ledgerQuery.data}
            error={billing.ledgerQuery.isError
              ? {
                  onRetry: () => void billing.ledgerQuery.refetch(),
                  retrying: billing.ledgerQuery.isFetching,
                  title: t('errors.request.failed'),
                }
              : undefined}
            loading={billing.ledgerQuery.isPending}
            locale={i18n.language}
            page={billing.ledgerPage}
            projectMap={billing.projectMap}
            projectsLoading={billing.projectsQuery.isPending}
            typeFilter={billing.ledgerTypeFilter}
            onCategoryFilterChange={billing.handleLedgerCategoryFilterChange}
            onClearFilters={billing.handleClearLedgerFilters}
            onPageChange={billing.setLedgerPage}
            onTypeFilterChange={billing.handleLedgerTypeFilterChange}
          />
        </TabsContent>
        <TabsContent value="prices">
          <BillingPriceList billingDisplay={billingDisplay} />
        </TabsContent>
      </ContentTabs>

      <BillingWalletTransactionDialog
        amount={billing.transactionAmount}
        billingDisplay={billingDisplay}
        description={billing.transactionDescription}
        isPending={billing.createTransaction.isPending}
        open={billing.transactionOpen}
        type={billing.transactionType}
        userId={billing.transactionUserId}
        userItems={billing.userItems}
        usersError={billing.canManageBilling && billing.usersQuery.isError}
        usersLoading={billing.canManageBilling && billing.usersQuery.isPending}
        usersRetrying={billing.usersQuery.isFetching}
        onAmountChange={billing.setTransactionAmount}
        onConfirm={() => billing.createTransaction.mutate()}
        onDescriptionChange={billing.setTransactionDescription}
        onOpenChange={billing.setTransactionOpen}
        onRetryUsers={() => void billing.usersQuery.refetch()}
        onTypeChange={billing.setTransactionType}
        onUserIdChange={billing.setTransactionUserId}
      />
    </div>
  )
}
