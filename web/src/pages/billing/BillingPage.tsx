import type { ReactNode } from 'react'
import type { BillingDeploymentSpend, BillingLedgerEntry, BillingUsageRecord, Project, User } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import type { StatusTone } from '@/components/common/status-tone'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Coins, CreditCard, Plus, TrendingDown, WalletCards } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ContentTabs } from '@/components/common/content-tabs'
import { DataList } from '@/components/common/data-list'
import { FormField as Field } from '@/components/common/form-field'
import { ProjectSpaceMultiSelect } from '@/components/common/project-space-select'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { TabsContent } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { formatBillingNumber, useBillingDisplay } from '@/lib/billing-display'
import { cn } from '@/lib/utils'

const PAGE_SIZE = 10
const BILLING_PROJECT_SCOPE_CACHE_KEY = 'liteyuki.billing.projectScope'

export function BillingPage() {
  const { i18n, t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const billingDisplay = useBillingDisplay(i18n.language)
  const [activeTab, setActiveTab] = useState('deployment-spend')
  const [selectedProjectIds, setSelectedProjectIds] = useState<string[]>(readCachedBillingProjectScope)
  const [deploymentSpendPage, setDeploymentSpendPage] = useState(1)
  const [ledgerPage, setLedgerPage] = useState(1)
  const [usagePage, setUsagePage] = useState(1)
  const [transactionOpen, setTransactionOpen] = useState(false)
  const [transactionUserId, setTransactionUserId] = useState('')
  const [transactionType, setTransactionType] = useState<'credit' | 'adjustment'>('credit')
  const [transactionAmount, setTransactionAmount] = useState('')
  const [transactionDescription, setTransactionDescription] = useState('')
  const canManageBilling = user?.role === 'platform_admin'

  const projectsQuery = useQuery({
    queryKey: ['billing', 'projects', canManageBilling],
    queryFn: () => api.listProjectsPage({ page: 1, pageSize: 100, scope: canManageBilling ? 'all' : 'related', sortBy: 'lastUsedAt', sortOrder: 'desc' }),
  })
  const projectItems = useMemo(() => projectsQuery.data?.items ?? [], [projectsQuery.data?.items])
  const projectMap = useMemo(() => new Map(projectItems.map(project => [project.id, project])), [projectItems])
  const projectIds = selectedProjectIds.length > 0 ? selectedProjectIds : undefined
  const usersQuery = useQuery({
    enabled: canManageBilling,
    queryKey: ['billing', 'users'],
    queryFn: () => api.listUsers({ page: 1, pageSize: 100, sortBy: 'email', sortOrder: 'asc' }),
  })
  const userItems = useMemo(() => usersQuery.data?.items ?? [], [usersQuery.data?.items])

  const accountSummaryQuery = useQuery({
    queryKey: ['billing', 'summary', 'account'],
    queryFn: () => api.getBillingSummary(),
  })
  const scopedSummaryQuery = useQuery({
    queryKey: ['billing', 'summary', 'scope', selectedProjectIds],
    queryFn: () => api.getBillingSummary(projectIds),
  })
  const deploymentSpendQuery = useQuery({
    queryKey: ['billing', 'deployment-spend', selectedProjectIds, deploymentSpendPage],
    queryFn: () => api.listBillingDeploymentSpend({
      page: deploymentSpendPage,
      pageSize: PAGE_SIZE,
      projectIds,
      sortBy: 'amountCredits',
      sortOrder: 'desc',
    }),
  })
  const ledgerQuery = useQuery({
    queryKey: ['billing', 'ledger', selectedProjectIds, ledgerPage],
    queryFn: () => api.listBillingLedgerEntries({
      page: ledgerPage,
      pageSize: PAGE_SIZE,
      projectIds,
      sortBy: 'createdAt',
      sortOrder: 'desc',
    }),
  })
  const usageQuery = useQuery({
    queryKey: ['billing', 'usage', selectedProjectIds, usagePage],
    queryFn: () => api.listBillingUsageRecords({
      page: usagePage,
      pageSize: PAGE_SIZE,
      projectIds,
      sortBy: 'createdAt',
      sortOrder: 'desc',
    }),
  })
  const createTransaction = useMutation({
    mutationFn: () => api.createBillingWalletTransaction({
      amountCredits: transactionAmount,
      type: transactionType,
      description: transactionDescription,
      userId: transactionUserId,
    }),
    onSuccess: () => {
      toast.success(t('billingPage.walletTransactionCreated'))
      setTransactionOpen(false)
      setTransactionAmount('')
      setTransactionDescription('')
      queryClient.invalidateQueries({ queryKey: ['billing'] })
    },
    onError: error => toast.error(error.message),
  })

  function handleProjectFilterChange(projectIds: string[]) {
    setSelectedProjectIds(projectIds)
    writeCachedBillingProjectScope(projectIds)
    setDeploymentSpendPage(1)
    setLedgerPage(1)
    setUsagePage(1)
  }

  const accountSummary = accountSummaryQuery.data
  const scopedSummary = scopedSummaryQuery.data
  const balanceStatus = normalizeBalanceStatus(accountSummary?.balanceStatus)

  const billingScopeTools = (
    <>
      <div className="w-full sm:w-80">
        <ProjectSpaceMultiSelect
          disabled={projectsQuery.isLoading}
          projects={projectItems}
          value={selectedProjectIds}
          onChange={handleProjectFilterChange}
        />
      </div>
      {selectedProjectIds.length > 0 && (
        <Button className="h-10 rounded-lg" type="button" variant="outline" onClick={() => handleProjectFilterChange([])}>
          {t('billingPage.clearProjectFilter')}
        </Button>
      )}
      {canManageBilling && (
        <Button
          className="h-10 rounded-lg"
          disabled={usersQuery.isLoading || userItems.length === 0}
          type="button"
          onClick={() => {
            setTransactionUserId(userItems[0]?.id ?? '')
            setTransactionOpen(true)
          }}
        >
          <Plus size={16} />
          {t('billingPage.createWalletTransaction')}
        </Button>
      )}
    </>
  )

  const deploymentSpendColumns = useMemo<DataListColumn<BillingDeploymentSpend>[]>(() => [
    {
      key: 'project',
      header: t('billingPage.project'),
      className: 'min-w-56',
      render: item => <ProjectCell project={projectMap.get(item.projectId)} fallbackName={item.projectName} fallbackSlug={item.projectSlug} unknownLabel={t('billingPage.unknownProject')} />,
    },
    {
      key: 'application',
      header: t('billingPage.application'),
      className: 'min-w-52',
      render: item => <ApplicationCell item={item} unassignedLabel={t('billingPage.unassignedApplication')} />,
    },
    {
      key: 'deploymentTarget',
      header: t('billingPage.deploymentTarget'),
      className: 'min-w-48',
      render: item => <DeploymentTargetCell item={item} unassignedLabel={t('billingPage.unassignedDeploymentTarget')} />,
    },
    {
      key: 'amount',
      header: t('billingPage.amount'),
      className: 'w-40',
      render: item => <SpendAmount value={item.amountCredits} billingDisplay={billingDisplay} strong />,
    },
    {
      key: 'build',
      header: t('billingPage.buildSpend'),
      className: 'w-32',
      render: item => <SpendAmount value={item.buildCredits} billingDisplay={billingDisplay} />,
    },
    {
      key: 'runtime',
      header: t('billingPage.runtimeSpend'),
      className: 'w-32',
      render: item => <SpendAmount value={item.runtimeCredits} billingDisplay={billingDisplay} />,
    },
    {
      key: 'storage',
      header: t('billingPage.storageSpend'),
      className: 'w-32',
      render: item => <SpendAmount value={item.storageCredits} billingDisplay={billingDisplay} />,
    },
    {
      key: 'gateway',
      header: t('billingPage.gatewaySpend'),
      className: 'w-32',
      render: item => <SpendAmount value={item.gatewayCredits} billingDisplay={billingDisplay} />,
    },
    {
      key: 'other',
      header: t('billingPage.otherSpend'),
      className: 'w-32',
      render: item => <SpendAmount value={item.otherCredits} billingDisplay={billingDisplay} />,
    },
  ], [billingDisplay, projectMap, t])

  const ledgerColumns = useMemo<DataListColumn<BillingLedgerEntry>[]>(() => [
    {
      key: 'project',
      header: t('billingPage.project'),
      className: 'min-w-56',
      render: item => item.projectId
        ? <ProjectCell project={projectMap.get(item.projectId)} unknownLabel={t('billingPage.unknownProject')} />
        : <span className="text-sm text-muted-foreground">{t('billingPage.accountTransaction')}</span>,
    },
    {
      key: 'application',
      header: t('billingPage.application'),
      className: 'min-w-52',
      render: item => item.projectId
        ? <ApplicationCell item={item} unassignedLabel={t('billingPage.unassignedApplication')} />
        : <span className="text-sm text-muted-foreground">-</span>,
    },
    {
      key: 'type',
      header: t('billingPage.type'),
      className: 'w-28',
      render: item => (
        <StatusBadge tone={item.type === 'debit' ? 'danger' : item.type === 'credit' ? 'success' : 'neutral'}>
          {t(`billingPage.types.${item.type}`, { defaultValue: item.type })}
        </StatusBadge>
      ),
    },
    {
      key: 'amount',
      header: t('billingPage.amount'),
      className: 'w-40',
      render: item => (
        <span className={cn('font-medium tabular-nums', amountToneClass(item.amountCredits))}>
          {billingDisplay.formatSignedAmountWithUnit(item.amountCredits)}
        </span>
      ),
    },
    {
      key: 'balance',
      header: t('billingPage.balanceAfter'),
      className: 'w-40',
      render: item => <span className="tabular-nums">{billingDisplay.formatAmountWithUnit(item.balanceAfterCredits)}</span>,
    },
    {
      key: 'reason',
      header: t('billingPage.reason'),
      className: 'min-w-40',
      render: item => t(`billingPage.reasons.${item.reason}`, { defaultValue: item.reason || '-' }),
    },
    {
      key: 'resource',
      header: t('billingPage.resource'),
      className: 'min-w-56',
      render: item => <ResourceCell resourceId={item.resourceId} resourceType={item.resourceType} />,
    },
    {
      key: 'time',
      header: t('billingPage.time'),
      className: 'w-44',
      render: item => formatSmartDateTime(item.createdAt, t),
    },
  ], [billingDisplay, projectMap, t])

  const usageColumns = useMemo<DataListColumn<BillingUsageRecord>[]>(() => [
    {
      key: 'project',
      header: t('billingPage.project'),
      className: 'min-w-56',
      render: item => <ProjectCell project={projectMap.get(item.projectId)} unknownLabel={t('billingPage.unknownProject')} />,
    },
    {
      key: 'application',
      header: t('billingPage.application'),
      className: 'min-w-52',
      render: item => <ApplicationCell item={item} unassignedLabel={t('billingPage.unassignedApplication')} />,
    },
    {
      key: 'meter',
      header: t('billingPage.meter'),
      className: 'min-w-40',
      render: item => t(`billingPage.meters.${item.meter}`, { defaultValue: item.meter }),
    },
    {
      key: 'status',
      header: t('common.status'),
      className: 'w-28',
      render: item => <StatusValueBadge value={item.status} />,
    },
    {
      key: 'quantity',
      header: t('billingPage.quantity'),
      className: 'w-36',
      render: item => <span className="tabular-nums">{formatQuantity(item.quantity, item.unit, i18n.language)}</span>,
    },
    {
      key: 'amount',
      header: t('billingPage.amount'),
      className: 'w-40',
      render: item => <span className="font-medium tabular-nums text-destructive">{billingDisplay.formatAmountWithUnit(item.amountCredits)}</span>,
    },
    {
      key: 'resource',
      header: t('billingPage.resource'),
      className: 'min-w-56',
      render: item => <ResourceCell resourceId={item.resourceId} resourceType={item.resourceType} />,
    },
    {
      key: 'time',
      header: t('billingPage.time'),
      className: 'w-44',
      render: item => formatSmartDateTime(item.createdAt, t),
    },
  ], [billingDisplay, i18n.language, projectMap, t])

  return (
    <div className="grid min-w-0 gap-5">
      {accountSummary && balanceStatus !== 'ok' && (
        <Card className={cn(
          'flex min-w-0 items-start gap-3 rounded-lg border p-4',
          balanceStatus === 'insufficient'
            ? 'border-destructive/30 bg-destructive/5 text-destructive'
            : 'border-amber-300/60 bg-amber-50 text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200',
        )}
        >
          <AlertTriangle className="mt-0.5 size-5 shrink-0" />
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <p className="font-medium">{t(`billingPage.balanceStatuses.${balanceStatus}`)}</p>
              <StatusBadge tone={balanceStatus === 'insufficient' ? 'danger' : 'warning'}>
                {billingDisplay.formatAmountWithUnit(accountSummary.availableCredits)}
              </StatusBadge>
            </div>
            <p className="mt-1 text-sm opacity-80">
              {t('billingPage.balanceWarningDescription', {
                pending: billingDisplay.formatAmountWithUnit(accountSummary.pendingSpend),
                threshold: billingDisplay.formatAmountWithUnit(accountSummary.lowBalanceLimit),
              })}
            </p>
          </div>
        </Card>
      )}

      <div className="grid gap-3 md:grid-cols-4">
        <MetricCard
          fiatValue={canManageBilling ? billingDisplay.formatFiatAmount(accountSummary?.balanceCredits) : ''}
          icon={<Coins className="size-5" />}
          label={t('billingPage.balance')}
          loading={accountSummaryQuery.isLoading}
          value={billingDisplay.formatAmountWithUnit(accountSummary?.balanceCredits)}
        />
        <MetricCard
          fiatValue={canManageBilling ? billingDisplay.formatFiatAmount(accountSummary?.todaySpend) : ''}
          icon={<TrendingDown className="size-5" />}
          label={t('billingPage.todaySpend')}
          loading={accountSummaryQuery.isLoading}
          value={billingDisplay.formatAmountWithUnit(accountSummary?.todaySpend)}
        />
        <MetricCard
          fiatValue={canManageBilling ? billingDisplay.formatFiatAmount(accountSummary?.monthSpend) : ''}
          icon={<CreditCard className="size-5" />}
          label={t('billingPage.monthSpend')}
          loading={accountSummaryQuery.isLoading}
          value={billingDisplay.formatAmountWithUnit(accountSummary?.monthSpend)}
        />
        <MetricCard
          fiatValue={canManageBilling ? billingDisplay.formatFiatAmount(accountSummary?.pendingSpend) : ''}
          icon={<WalletCards className="size-5" />}
          label={t('billingPage.pendingSpend')}
          loading={accountSummaryQuery.isLoading}
          value={billingDisplay.formatAmountWithUnit(accountSummary?.pendingSpend)}
        />
      </div>

      <Card className="rounded-lg p-5">
        <div className="flex min-w-0 flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <h3 className="text-base font-semibold text-foreground">{t('billingPage.monthlyCategoriesTitle')}</h3>
            <p className="text-sm text-muted-foreground">{t('billingPage.monthlyCategoriesDescription')}</p>
          </div>
          <StatusBadge tone={balanceStatusTone(balanceStatus)}>
            {t(`billingPage.balanceStatuses.${balanceStatus}`)}
          </StatusBadge>
        </div>
        <div className="mt-4 grid gap-3 md:grid-cols-3 xl:grid-cols-6">
          {(scopedSummary?.monthlyCategories?.length ?? 0) > 0
            ? (scopedSummary?.monthlyCategories ?? []).map(category => (
                <div key={category.category} className="min-w-0 rounded-md border border-border bg-muted/20 p-3">
                  <p className="truncate text-xs text-muted-foreground">
                    {t(`billingPage.categories.${category.category}`, { defaultValue: category.category })}
                  </p>
                  <p className="mt-1 truncate text-lg font-semibold tabular-nums text-foreground">
                    {billingDisplay.formatAmountWithUnit(category.amountCredits)}
                  </p>
                </div>
              ))
            : (
                <p className="text-sm text-muted-foreground md:col-span-3 xl:col-span-6">
                  {scopedSummaryQuery.isLoading ? t('common.loading') : t('billingPage.emptyMonthlyCategories')}
                </p>
              )}
        </div>
      </Card>

      <ContentTabs
        tabs={[
          { label: t('billingPage.deploymentSpendTitle'), value: 'deployment-spend' },
          { label: t('billingPage.ledgerTitle'), value: 'ledger' },
          { label: t('billingPage.usageTitle'), value: 'usage' },
        ]}
        tools={billingScopeTools}
        value={activeTab}
        onValueChange={setActiveTab}
      >
        <TabsContent value="deployment-spend">
          <DataList
            columns={deploymentSpendColumns}
            emptyDescription={t('billingPage.emptyDeploymentSpendDescription')}
            emptyTitle={t('billingPage.emptyDeploymentSpendTitle')}
            items={deploymentSpendQuery.data?.items ?? []}
            pagination={{
              page: deploymentSpendQuery.data?.page ?? deploymentSpendPage,
              pageInfoLabel: t('billingPage.deploymentSpendPageInfo', {
                page: deploymentSpendQuery.data?.page ?? deploymentSpendPage,
                total: deploymentSpendQuery.data?.total ?? 0,
                totalPages: deploymentSpendQuery.data?.totalPages ?? 1,
              }),
              pageSize: deploymentSpendQuery.data?.pageSize ?? PAGE_SIZE,
              total: deploymentSpendQuery.data?.total ?? 0,
              totalPages: deploymentSpendQuery.data?.totalPages ?? 1,
              onPageChange: setDeploymentSpendPage,
            }}
            rowKey={item => `${item.projectId}:${item.applicationId || 'unassigned'}:${item.deploymentTargetId || 'unassigned'}`}
          />
        </TabsContent>
        <TabsContent value="ledger">
          <DataList
            columns={ledgerColumns}
            emptyDescription={t('billingPage.emptyLedgerDescription')}
            emptyTitle={t('billingPage.emptyLedgerTitle')}
            items={ledgerQuery.data?.items ?? []}
            pagination={{
              page: ledgerQuery.data?.page ?? ledgerPage,
              pageInfoLabel: t('billingPage.ledgerPageInfo', {
                page: ledgerQuery.data?.page ?? ledgerPage,
                total: ledgerQuery.data?.total ?? 0,
                totalPages: ledgerQuery.data?.totalPages ?? 1,
              }),
              pageSize: ledgerQuery.data?.pageSize ?? PAGE_SIZE,
              total: ledgerQuery.data?.total ?? 0,
              totalPages: ledgerQuery.data?.totalPages ?? 1,
              onPageChange: setLedgerPage,
            }}
            rowKey={item => item.id}
          />
        </TabsContent>
        <TabsContent value="usage">
          <DataList
            columns={usageColumns}
            emptyDescription={t('billingPage.emptyUsageDescription')}
            emptyTitle={t('billingPage.emptyUsageTitle')}
            items={usageQuery.data?.items ?? []}
            pagination={{
              page: usageQuery.data?.page ?? usagePage,
              pageInfoLabel: t('billingPage.usagePageInfo', {
                page: usageQuery.data?.page ?? usagePage,
                total: usageQuery.data?.total ?? 0,
                totalPages: usageQuery.data?.totalPages ?? 1,
              }),
              pageSize: usageQuery.data?.pageSize ?? PAGE_SIZE,
              total: usageQuery.data?.total ?? 0,
              totalPages: usageQuery.data?.totalPages ?? 1,
              onPageChange: setUsagePage,
            }}
            rowKey={item => item.id}
          />
        </TabsContent>
      </ContentTabs>

      <Dialog open={transactionOpen} onOpenChange={setTransactionOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('billingPage.walletTransactionTitle')}</DialogTitle>
            <DialogDescription>{t('billingPage.walletTransactionDescription')}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-2">
            <Field label={t('billingPage.user')}>
              <Select value={transactionUserId} onValueChange={setTransactionUserId}>
                <SelectTrigger>
                  <SelectValue placeholder={t('billingPage.selectUser')} />
                </SelectTrigger>
                <SelectContent>
                  {userItems.map(item => (
                    <SelectItem key={item.id} value={item.id}>
                      {userLabel(item)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field hint={t('billingPage.walletTransactionTypeHint')} label={t('billingPage.walletTransactionType')}>
              <Select value={transactionType} onValueChange={value => setTransactionType(value as 'credit' | 'adjustment')}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="credit">{t('billingPage.walletTransactionTypes.credit')}</SelectItem>
                  <SelectItem value="adjustment">{t('billingPage.walletTransactionTypes.adjustment')}</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field hint={t('billingPage.walletTransactionAmountHint', { unit: billingDisplay.currencyUnit })} label={t('billingPage.amount')}>
              <Input
                inputMode="decimal"
                placeholder={t('billingPage.walletTransactionAmountPlaceholder')}
                value={transactionAmount}
                onChange={event => setTransactionAmount(event.target.value)}
              />
            </Field>
            <Field label={t('billingPage.descriptionLabel')}>
              <Textarea
                className="min-h-24"
                placeholder={t('billingPage.walletTransactionDescriptionPlaceholder')}
                value={transactionDescription}
                onChange={event => setTransactionDescription(event.target.value)}
              />
            </Field>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setTransactionOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              disabled={createTransaction.isPending || !transactionUserId || !transactionAmount.trim()}
              type="button"
              onClick={() => createTransaction.mutate()}
            >
              {t('common.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function userLabel(user: User) {
  return `${user.name || user.email} · ${user.email}`
}

function MetricCard({ fiatValue, icon, label, loading, value }: { fiatValue?: string, icon: ReactNode, label: string, loading: boolean, value: string }) {
  return (
    <Card className="flex min-w-0 items-center gap-4 rounded-lg p-5">
      <div className="flex size-11 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
        {icon}
      </div>
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-2">
          <p className="shrink-0 text-sm text-muted-foreground">{label}</p>
          {!loading && fiatValue && (
            <span className="min-w-0 truncate text-xs tabular-nums text-muted-foreground/80">
              {fiatValue}
            </span>
          )}
        </div>
        <p className="mt-1 truncate text-2xl font-semibold tabular-nums">
          {loading ? '-' : value}
        </p>
      </div>
    </Card>
  )
}

function ProjectCell({
  fallbackName,
  fallbackSlug,
  project,
  unknownLabel,
}: {
  fallbackName?: string
  fallbackSlug?: string
  project?: Project
  unknownLabel: string
}) {
  const name = project?.name || fallbackName || unknownLabel
  const slug = project?.slug || fallbackSlug || '-'
  return (
    <span className="block min-w-0">
      <span className="block truncate font-medium">{name}</span>
      <span className="block truncate text-xs text-muted-foreground">{slug}</span>
    </span>
  )
}

type BillingApplicationRef = Pick<BillingDeploymentSpend, 'applicationName' | 'applicationSlug'> & {
  applicationId?: string
}

function ApplicationCell({ item, unassignedLabel }: { item: BillingApplicationRef, unassignedLabel: string }) {
  return (
    <span className="block min-w-0">
      <span className="block truncate font-medium">{item.applicationName || unassignedLabel}</span>
      <span className="block truncate text-xs text-muted-foreground">{item.applicationSlug || '-'}</span>
    </span>
  )
}

type BillingDeploymentTargetRef = Pick<BillingDeploymentSpend, 'deploymentTargetName' | 'deploymentTargetStage'> & {
  deploymentTargetId?: string
}

function DeploymentTargetCell({ item, unassignedLabel }: { item: BillingDeploymentTargetRef, unassignedLabel: string }) {
  return (
    <span className="block min-w-0">
      <span className="block truncate font-medium">{item.deploymentTargetName || unassignedLabel}</span>
      <span className="block truncate text-xs text-muted-foreground">{item.deploymentTargetStage || item.deploymentTargetId || '-'}</span>
    </span>
  )
}

function SpendAmount({
  billingDisplay,
  strong = false,
  value,
}: {
  billingDisplay: ReturnType<typeof useBillingDisplay>
  strong?: boolean
  value: string
}) {
  return (
    <span className={cn('tabular-nums text-foreground', strong && 'font-semibold')}>
      {billingDisplay.formatAmountWithUnit(value)}
    </span>
  )
}

function ResourceCell({ resourceId, resourceType }: { resourceId: string, resourceType: string }) {
  return (
    <span className="block min-w-0">
      <span className="block truncate font-medium">{resourceType || '-'}</span>
      <span className="block truncate text-xs text-muted-foreground">{resourceId || '-'}</span>
    </span>
  )
}

function formatQuantity(value: string, unit: string, locale: string) {
  const formatted = formatBillingNumber(value, locale)
  return unit ? `${formatted} ${unit}` : formatted
}

function amountToneClass(value: string) {
  const numeric = Number.parseFloat(value)
  if (numeric < 0)
    return 'text-destructive'
  if (numeric > 0)
    return 'text-emerald-600 dark:text-emerald-400'
  return 'text-muted-foreground'
}

type BalanceStatus = 'ok' | 'low' | 'insufficient'

function normalizeBalanceStatus(status: string | undefined): BalanceStatus {
  if (status === 'low' || status === 'insufficient')
    return status
  return 'ok'
}

function balanceStatusTone(status: BalanceStatus): StatusTone {
  if (status === 'insufficient')
    return 'danger'
  if (status === 'low')
    return 'warning'
  return 'success'
}

function readCachedBillingProjectScope() {
  if (typeof window === 'undefined')
    return []
  try {
    const cached = window.sessionStorage.getItem(BILLING_PROJECT_SCOPE_CACHE_KEY)
    if (!cached)
      return []
    const parsed = JSON.parse(cached)
    if (!Array.isArray(parsed))
      return []
    return parsed.map(item => String(item).trim()).filter(Boolean)
  }
  catch {
    return []
  }
}

function writeCachedBillingProjectScope(projectIds: string[]) {
  if (typeof window === 'undefined')
    return
  try {
    const normalized = projectIds.map(item => item.trim()).filter(Boolean)
    if (normalized.length === 0) {
      window.sessionStorage.removeItem(BILLING_PROJECT_SCOPE_CACHE_KEY)
      return
    }
    window.sessionStorage.setItem(BILLING_PROJECT_SCOPE_CACHE_KEY, JSON.stringify(normalized))
  }
  catch {
    // Ignore storage errors so private mode or quota issues do not break billing.
  }
}
