import { useTranslation } from 'react-i18next'
import { ContentTabs } from '@/components/common/content-tabs'
import { TabsContent } from '@/components/ui/tabs'
import { useBillingDisplay } from '@/lib/billing-display'
import { BillingDeploymentSpendList } from './billing-deployment-spend-list'
import { BillingLedgerList } from './billing-ledger-list'
import { BillingPriceList } from './billing-price-list'
import { BillingScopeToolbar } from './billing-scope-toolbar'
import { BillingBalanceWarning, BillingSummary } from './billing-summary'
import { BillingUsageList } from './billing-usage-list'
import { BillingWalletTransactionDialog } from './billing-wallet-transaction-dialog'
import { useBillingPage } from './use-billing-page'

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
        projectsLoading={billing.projectsQuery.isLoading}
        selectedProjectIds={billing.selectedProjectIds}
        userItems={billing.userItems}
        usersLoading={billing.usersQuery.isLoading}
        visibleProjectItems={billing.visibleProjectItems}
        onBillingUserChange={billing.handleBillingUserChange}
        onCreateTransaction={() => {
          billing.setTransactionUserId(billing.userItems[0]?.id ?? '')
          billing.setTransactionOpen(true)
        }}
        onPeriodChange={billing.handlePeriodChange}
        onProjectFilterChange={billing.handleProjectFilterChange}
      />

      <BillingSummary
        accountLoading={billing.accountSummaryQuery.isLoading}
        accountSummary={accountSummary}
        billingDisplay={billingDisplay}
        canManageBilling={billing.canManageBilling}
        gatewayTrafficStatus={billing.gatewayTrafficStatusQuery.data}
        gatewayTrafficStatusLoaded={billing.gatewayTrafficStatusQuery.isSuccess}
        scopedFetching={billing.scopedSummaryQuery.isFetching}
        scopedLoading={billing.scopedSummaryQuery.isLoading}
        scopedSummary={billing.scopedSummaryQuery.data}
      />

      <ContentTabs
        tabs={[
          { label: t('billingPage.deploymentSpendTitle'), value: 'deployment-spend' },
          { label: t('billingPage.ledgerTitle'), value: 'ledger' },
          { label: t('billingPage.usageTitle'), value: 'usage' },
          { label: t('billingPage.priceTableTitle'), value: 'prices' },
        ]}
        value={billing.activeTab}
        onValueChange={billing.setActiveTab}
      >
        <TabsContent value="deployment-spend">
          <BillingDeploymentSpendList
            billingDisplay={billingDisplay}
            data={billing.deploymentSpendQuery.data}
            page={billing.deploymentSpendPage}
            projectMap={billing.projectMap}
            onPageChange={billing.setDeploymentSpendPage}
          />
        </TabsContent>
        <TabsContent value="ledger">
          <BillingLedgerList
            billingDisplay={billingDisplay}
            data={billing.ledgerQuery.data}
            page={billing.ledgerPage}
            projectMap={billing.projectMap}
            onPageChange={billing.setLedgerPage}
          />
        </TabsContent>
        <TabsContent value="usage">
          <BillingUsageList
            billingDisplay={billingDisplay}
            data={billing.usageQuery.data}
            locale={i18n.language}
            page={billing.usagePage}
            projectMap={billing.projectMap}
            onPageChange={billing.setUsagePage}
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
        usersLoading={billing.usersQuery.isLoading}
        onAmountChange={billing.setTransactionAmount}
        onConfirm={() => billing.createTransaction.mutate()}
        onDescriptionChange={billing.setTransactionDescription}
        onOpenChange={billing.setTransactionOpen}
        onTypeChange={billing.setTransactionType}
        onUserIdChange={billing.setTransactionUserId}
      />
    </div>
  )
}
