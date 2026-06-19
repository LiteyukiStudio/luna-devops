import type { ReactNode } from 'react'
import type { BillingLedgerEntry, BillingUsageRecord, Project } from '@/api/client'
import type { DataListColumn } from '@/components/common/data-list'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Coins, CreditCard, Plus, TrendingDown } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api/client'
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
import { cn } from '@/lib/utils'

const PAGE_SIZE = 10

export function BillingPage() {
  const { i18n, t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('ledger')
  const [selectedProjectIds, setSelectedProjectIds] = useState<string[]>([])
  const [ledgerPage, setLedgerPage] = useState(1)
  const [usagePage, setUsagePage] = useState(1)
  const [transactionOpen, setTransactionOpen] = useState(false)
  const [transactionProjectId, setTransactionProjectId] = useState('')
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

  const summaryQuery = useQuery({
    queryKey: ['billing', 'summary', selectedProjectIds],
    queryFn: () => api.getBillingSummary(projectIds),
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
      projectId: transactionProjectId,
      amountCredits: transactionAmount,
      type: transactionType,
      description: transactionDescription,
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
    setLedgerPage(1)
    setUsagePage(1)
  }

  const scopeLabel = selectedProjectIds.length > 0
    ? t('billingPage.selectedProjects', { count: selectedProjectIds.length })
    : t('billingPage.allRelatedProjects')

  const ledgerColumns = useMemo<DataListColumn<BillingLedgerEntry>[]>(() => [
    {
      key: 'project',
      header: t('billingPage.project'),
      className: 'min-w-56',
      render: item => <ProjectCell project={projectMap.get(item.projectId)} unknownLabel={t('billingPage.unknownProject')} />,
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
          {formatSignedCredits(item.amountCredits, i18n.language)}
        </span>
      ),
    },
    {
      key: 'balance',
      header: t('billingPage.balanceAfter'),
      className: 'w-40',
      render: item => <span className="tabular-nums">{formatCredits(item.balanceAfterCredits, i18n.language)}</span>,
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
  ], [i18n.language, projectMap, t])

  const usageColumns = useMemo<DataListColumn<BillingUsageRecord>[]>(() => [
    {
      key: 'project',
      header: t('billingPage.project'),
      className: 'min-w-56',
      render: item => <ProjectCell project={projectMap.get(item.projectId)} unknownLabel={t('billingPage.unknownProject')} />,
    },
    {
      key: 'meter',
      header: t('billingPage.meter'),
      className: 'min-w-40',
      render: item => t(`billingPage.meters.${item.meter}`, { defaultValue: item.meter }),
    },
    {
      key: 'status',
      header: t('clustersPage.status'),
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
      render: item => <span className="font-medium tabular-nums text-destructive">{formatCredits(item.amountCredits, i18n.language)}</span>,
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
  ], [i18n.language, projectMap, t])

  return (
    <div className="grid min-w-0 gap-5">
      <div className="grid gap-3 md:grid-cols-3">
        <MetricCard
          icon={<Coins className="size-5" />}
          label={t('billingPage.balance')}
          loading={summaryQuery.isLoading}
          value={formatCredits(summaryQuery.data?.balanceCredits, i18n.language)}
        />
        <MetricCard
          icon={<TrendingDown className="size-5" />}
          label={t('billingPage.todaySpend')}
          loading={summaryQuery.isLoading}
          value={formatCredits(summaryQuery.data?.todaySpend, i18n.language)}
        />
        <MetricCard
          icon={<CreditCard className="size-5" />}
          label={t('billingPage.monthSpend')}
          loading={summaryQuery.isLoading}
          value={formatCredits(summaryQuery.data?.monthSpend, i18n.language)}
        />
      </div>

      <ContentTabs
        tabs={[
          { label: t('billingPage.ledgerTitle'), value: 'ledger' },
          { label: t('billingPage.usageTitle'), value: 'usage' },
        ]}
        tools={(
          <div className="flex w-full min-w-0 flex-col gap-2 sm:w-auto sm:flex-row sm:items-center">
            <div className="w-full sm:w-80">
              <ProjectSpaceMultiSelect
                disabled={projectsQuery.isLoading}
                projects={projectItems}
                value={selectedProjectIds}
                onChange={handleProjectFilterChange}
              />
            </div>
            {selectedProjectIds.length > 0 && (
              <Button className="h-11 rounded-2xl" type="button" variant="outline" onClick={() => handleProjectFilterChange([])}>
                {t('billingPage.clearProjectFilter')}
              </Button>
            )}
            {canManageBilling && (
              <Button
                className="h-11 rounded-2xl"
                type="button"
                onClick={() => {
                  setTransactionProjectId(selectedProjectIds[0] ?? projectItems[0]?.id ?? '')
                  setTransactionOpen(true)
                }}
              >
                <Plus size={16} />
                {t('billingPage.createWalletTransaction')}
              </Button>
            )}
          </div>
        )}
        value={activeTab}
        onValueChange={setActiveTab}
      >
        <p className="text-sm text-muted-foreground">
          {t('billingPage.filters')}
          {' · '}
          {scopeLabel}
          {' · '}
          {t('billingPage.projectScopeHint')}
        </p>
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
            <Field label={t('billingPage.project')}>
              <Select value={transactionProjectId} onValueChange={setTransactionProjectId}>
                <SelectTrigger>
                  <SelectValue placeholder={t('billingPage.selectProject')} />
                </SelectTrigger>
                <SelectContent>
                  {projectItems.map(project => (
                    <SelectItem key={project.id} value={project.id}>
                      {project.name}
                      {' · '}
                      {project.slug}
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
            <Field hint={t('billingPage.walletTransactionAmountHint')} label={t('billingPage.amount')}>
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
              disabled={createTransaction.isPending || !transactionProjectId || !transactionAmount.trim()}
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

function MetricCard({ icon, label, loading, value }: { icon: ReactNode, label: string, loading: boolean, value: string }) {
  const { t } = useTranslation()
  return (
    <Card className="flex min-w-0 items-center gap-4 rounded-2xl p-5">
      <div className="flex size-11 shrink-0 items-center justify-center rounded-2xl bg-primary/10 text-primary">
        {icon}
      </div>
      <div className="min-w-0">
        <p className="text-sm text-muted-foreground">{label}</p>
        <p className="mt-1 truncate text-2xl font-semibold tabular-nums">
          {loading ? '-' : value}
          <span className="ml-2 text-sm font-normal text-muted-foreground">{t('billingPage.creditsUnit')}</span>
        </p>
      </div>
    </Card>
  )
}

function ProjectCell({ project, unknownLabel }: { project?: Project, unknownLabel: string }) {
  return (
    <span className="block min-w-0">
      <span className="block truncate font-medium">{project?.name ?? unknownLabel}</span>
      <span className="block truncate text-xs text-muted-foreground">{project?.slug ?? '-'}</span>
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

function formatCredits(value: string | undefined, locale: string) {
  const numeric = Number.parseFloat(value ?? '0')
  if (!Number.isFinite(numeric))
    return '0'
  return numeric.toLocaleString(locale, { maximumFractionDigits: 4, minimumFractionDigits: 0 })
}

function formatSignedCredits(value: string, locale: string) {
  const numeric = Number.parseFloat(value)
  if (!Number.isFinite(numeric))
    return value
  const formatted = Math.abs(numeric).toLocaleString(locale, { maximumFractionDigits: 4, minimumFractionDigits: 0 })
  if (numeric > 0)
    return `+${formatted}`
  if (numeric < 0)
    return `-${formatted}`
  return formatted
}

function formatQuantity(value: string, unit: string, locale: string) {
  const formatted = formatCredits(value, locale)
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
