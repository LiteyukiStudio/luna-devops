import type { BillingChartType, BillingFocusedBucket, BillingLedgerCategoryFilter, BillingLedgerTypeFilter, BillingPeriodSelection } from './billing-page-utils'
import type { BillingSpendGranularity, BillingSpendTimeseriesBucket } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { isPlatformAdmin } from '@/lib/roles'
import {
  BILLING_PAGE_SIZE,
  billingPeriodToQuery,
  browserTimeZone,
  defaultGranularityForPeriod,
  isGranularityAllowedForPeriod,
  periodSelectionForPreset,
  readCachedBillingProjectScope,
  writeCachedBillingProjectScope,
} from './billing-page-utils'

export function useBillingPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('deployment-spend')
  const [selectedProjectIds, setSelectedProjectIds] = useState<string[]>(readCachedBillingProjectScope)
  const [billingPeriod, setBillingPeriod] = useState<BillingPeriodSelection>(() => periodSelectionForPreset('thisMonth'))
  const [granularity, setGranularity] = useState<BillingSpendGranularity>(() => defaultGranularityForPeriod(periodSelectionForPreset('thisMonth')))
  const [chartType, setChartType] = useState<BillingChartType>('bar')
  const [activeCategory, setActiveCategory] = useState<string>('all')
  const [focusedBucket, setFocusedBucket] = useState<BillingFocusedBucket | null>(null)
  const [deploymentSpendPage, setDeploymentSpendPage] = useState(1)
  const [ledgerPage, setLedgerPage] = useState(1)
  const [ledgerTypeFilter, setLedgerTypeFilter] = useState<BillingLedgerTypeFilter>('all')
  const [ledgerCategoryFilter, setLedgerCategoryFilter] = useState<BillingLedgerCategoryFilter>('all')
  const [selectedBillingUserId, setSelectedBillingUserId] = useState<string | null>(null)
  const [transactionOpen, setTransactionOpen] = useState(false)
  const [transactionUserId, setTransactionUserId] = useState('')
  const [transactionType, setTransactionType] = useState<'credit' | 'adjustment'>('credit')
  const [transactionAmount, setTransactionAmount] = useState('')
  const [transactionDescription, setTransactionDescription] = useState('')
  const canManageBilling = isPlatformAdmin(user?.role)
  const billingUserScopeId = selectedBillingUserId ?? (canManageBilling ? user?.id ?? '' : '')
  const effectiveGranularity = isGranularityAllowedForPeriod(granularity, billingPeriod)
    ? granularity
    : defaultGranularityForPeriod(billingPeriod)

  const projectsQuery = useQuery({
    queryKey: ['billing', 'projects', canManageBilling],
    queryFn: () => api.listProjectsPage({ page: 1, pageSize: 100, visibility: canManageBilling ? 'all' : 'related', sortBy: 'lastUsedAt', sortOrder: 'desc' }),
  })
  const projectItems = useMemo(() => projectsQuery.data?.items ?? [], [projectsQuery.data?.items])
  const visibleProjectItems = useMemo(() => {
    if (!canManageBilling || !billingUserScopeId)
      return projectItems
    return projectItems.filter(project => project.billingOwnerUserId === billingUserScopeId)
  }, [billingUserScopeId, canManageBilling, projectItems])
  const projectMap = useMemo(() => new Map(projectItems.map(project => [project.id, project])), [projectItems])
  const projectIds = selectedProjectIds.length > 0 ? selectedProjectIds : undefined
  const billingPeriodQuery = useMemo(() => billingPeriodToQuery(billingPeriod), [billingPeriod])
  const detailPeriodQuery = useMemo(
    () => focusedBucket
      ? { periodEnd: focusedBucket.periodEnd, periodStart: focusedBucket.periodStart }
      : billingPeriodQuery,
    [billingPeriodQuery, focusedBucket],
  )
  const billingUserQuery = canManageBilling && billingUserScopeId ? billingUserScopeId : undefined
  // Today and this month follow the viewer's local calendar, not the server's.
  const timeZone = browserTimeZone()
  const accountSummaryParams = canManageBilling && !billingUserQuery
    ? { ...billingPeriodQuery, timeZone, userId: billingUserQuery }
    : { ...billingPeriodQuery, accountScope: 'current' as const, timeZone, userId: billingUserQuery }
  const usersQuery = useQuery({
    enabled: canManageBilling,
    queryKey: ['billing', 'users'],
    queryFn: () => api.listUsers({ page: 1, pageSize: 100, sortBy: 'email', sortOrder: 'asc' }),
  })
  const userItems = useMemo(() => usersQuery.data?.items ?? [], [usersQuery.data?.items])

  const accountSummaryQuery = useQuery({
    queryKey: ['billing', 'summary', 'account', billingPeriodQuery, billingUserQuery, canManageBilling, timeZone],
    queryFn: () => api.getBillingSummary(undefined, accountSummaryParams),
  })
  const scopedSummaryQuery = useQuery({
    queryKey: ['billing', 'summary', 'scope', selectedProjectIds, billingPeriodQuery, billingUserQuery, timeZone],
    queryFn: () => api.getBillingSummary(projectIds, { ...billingPeriodQuery, timeZone, userId: billingUserQuery }),
  })
  const spendTimeseriesQuery = useQuery({
    queryKey: ['billing', 'spend-timeseries', selectedProjectIds, billingPeriodQuery, effectiveGranularity, billingUserQuery, timeZone],
    queryFn: ({ signal }) => api.getBillingSpendTimeseries(
      projectIds,
      { ...billingPeriodQuery, granularity: effectiveGranularity, timeZone, userId: billingUserQuery },
      signal,
    ),
  })
  const deploymentSpendQuery = useQuery({
    queryKey: ['billing', 'deployment-spend', selectedProjectIds, detailPeriodQuery, billingUserQuery, deploymentSpendPage],
    queryFn: () => api.listBillingDeploymentSpend({
      page: deploymentSpendPage,
      pageSize: BILLING_PAGE_SIZE,
      ...detailPeriodQuery,
      projectIds,
      userId: billingUserQuery,
      sortBy: 'amountCredits',
      sortOrder: 'desc',
    }),
  })
  const ledgerQuery = useQuery({
    queryKey: ['billing', 'ledger', selectedProjectIds, detailPeriodQuery, billingUserQuery, ledgerTypeFilter, ledgerCategoryFilter, ledgerPage],
    queryFn: () => api.listBillingLedgerEntries({
      page: ledgerPage,
      pageSize: BILLING_PAGE_SIZE,
      ...detailPeriodQuery,
      category: ledgerCategoryFilter === 'all' ? undefined : ledgerCategoryFilter,
      projectIds,
      type: ledgerTypeFilter === 'all' ? undefined : ledgerTypeFilter,
      userId: billingUserQuery,
      sortBy: 'occurredAt',
      sortOrder: 'desc',
    }),
  })
  const gatewayTrafficStatusQuery = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['billing', 'gateway-traffic-status'],
    queryFn: api.getGatewayTrafficStatus,
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

  function resetBillingPages() {
    setDeploymentSpendPage(1)
    setLedgerPage(1)
  }

  function handleLedgerTypeFilterChange(nextType: BillingLedgerTypeFilter) {
    setLedgerTypeFilter(nextType)
    setLedgerPage(1)
  }

  function handleLedgerCategoryFilterChange(nextCategory: BillingLedgerCategoryFilter) {
    setLedgerCategoryFilter(nextCategory)
    setLedgerPage(1)
  }

  function handleClearLedgerFilters() {
    setLedgerTypeFilter('all')
    setLedgerCategoryFilter('all')
    setLedgerPage(1)
  }

  function handleProjectFilterChange(projectIds: string[]) {
    setSelectedProjectIds(projectIds)
    writeCachedBillingProjectScope(projectIds)
    setFocusedBucket(null)
    resetBillingPages()
  }

  function handleBillingUserChange(userId: string) {
    setSelectedBillingUserId(userId)
    handleProjectFilterChange([])
  }

  function handlePeriodChange(period: BillingPeriodSelection) {
    setBillingPeriod(period)
    setGranularity(defaultGranularityForPeriod(period))
    setFocusedBucket(null)
    resetBillingPages()
  }

  function handleGranularityChange(nextGranularity: BillingSpendGranularity) {
    if (!isGranularityAllowedForPeriod(nextGranularity, billingPeriod))
      return
    setGranularity(nextGranularity)
    setFocusedBucket(null)
    resetBillingPages()
  }

  function handleSelectBucket(bucket: BillingSpendTimeseriesBucket) {
    const isDeselecting = focusedBucket?.bucketStart === bucket.bucketStart
    setFocusedBucket(isDeselecting
      ? null
      : {
          bucketStart: bucket.bucketStart,
          periodEnd: bucket.periodEnd,
          periodStart: bucket.periodStart,
        })
    if (!isDeselecting && activeTab === 'prices')
      setActiveTab('ledger')
    resetBillingPages()
  }

  function handleClearFocusedBucket() {
    setFocusedBucket(null)
    resetBillingPages()
  }

  return {
    accountSummaryQuery,
    activeCategory,
    activeTab,
    billingPeriod,
    billingUserScopeId,
    canManageBilling,
    chartType,
    createTransaction,
    deploymentSpendPage,
    deploymentSpendQuery,
    effectiveGranularity,
    focusedBucket,
    gatewayTrafficStatusQuery,
    handleBillingUserChange,
    handleClearFocusedBucket,
    handleClearLedgerFilters,
    handleGranularityChange,
    handleLedgerCategoryFilterChange,
    handleLedgerTypeFilterChange,
    handlePeriodChange,
    handleProjectFilterChange,
    handleSelectBucket,
    ledgerCategoryFilter,
    ledgerPage,
    ledgerQuery,
    ledgerTypeFilter,
    projectMap,
    projectsQuery,
    scopedSummaryQuery,
    selectedProjectIds,
    setActiveCategory,
    setActiveTab,
    setChartType,
    setDeploymentSpendPage,
    setLedgerPage,
    setTransactionAmount,
    setTransactionDescription,
    setTransactionOpen,
    setTransactionType,
    setTransactionUserId,
    spendTimeseriesQuery,
    timeZone,
    transactionAmount,
    transactionDescription,
    transactionOpen,
    transactionType,
    transactionUserId,
    userItems,
    usersQuery,
    visibleProjectItems,
  }
}
