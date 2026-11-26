import type {
  BillingChartType,
  BillingDeploymentStageFilter,
  BillingFocusedBucket,
  BillingLedgerCategoryFilter,
  BillingLedgerTypeFilter,
  BillingPeriodSelection,
} from './billing-page-utils'
import type { BillingScopeMode, BillingSpendGranularity, BillingSpendTimeseriesBucket, Project } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { isPlatformAdmin } from '@/lib/roles'
import {
  normalizeProjectBillingTab,
  projectBillingTabParam,
  projectBillingTabs,
} from '@/pages/projects/overview/project-workspace-sections'
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

export function useBillingPage({ projectId }: { projectId?: string } = {}) {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const isProjectMode = Boolean(projectId)
  const scopeMode: BillingScopeMode | undefined = isProjectMode ? 'project' : undefined
  const [walletActiveTab, setWalletActiveTab] = useState('deployment-spend')
  const [selectedProjectIds, setSelectedProjectIds] = useState<string[]>(() => projectId ? [projectId] : readCachedBillingProjectScope())
  const [billingPeriod, setBillingPeriod] = useState<BillingPeriodSelection>(() => periodSelectionForPreset('thisMonth'))
  const [granularity, setGranularity] = useState<BillingSpendGranularity>(() => defaultGranularityForPeriod(periodSelectionForPreset('thisMonth')))
  const [chartType, setChartType] = useState<BillingChartType>('bar')
  const [activeCategory, setActiveCategory] = useState<string>('all')
  const [focusedBucket, setFocusedBucket] = useState<BillingFocusedBucket | null>(null)
  const [deploymentSpendPage, setDeploymentSpendPage] = useState(1)
  const [applicationFilter, setApplicationFilter] = useState('all')
  const [stageFilter, setStageFilter] = useState<BillingDeploymentStageFilter>('all')
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
  const activeTab = isProjectMode
    ? normalizeProjectBillingTab(searchParams.get(projectBillingTabParam))
    : walletActiveTab
  const setActiveTab = useCallback((nextTab: string) => {
    if (!isProjectMode) {
      setWalletActiveTab(nextTab)
      return
    }
    const normalized = normalizeProjectBillingTab(nextTab)
    setSearchParams((current) => {
      const next = new URLSearchParams(current)
      if (normalized === projectBillingTabs[0])
        next.delete(projectBillingTabParam)
      else
        next.set(projectBillingTabParam, normalized)
      return next
    }, { replace: true })
  }, [isProjectMode, setSearchParams])
  const effectiveGranularity = isGranularityAllowedForPeriod(granularity, billingPeriod)
    ? granularity
    : defaultGranularityForPeriod(billingPeriod)

  const projectsQuery = useQuery({
    queryKey: ['billing', 'projects', canManageBilling],
    queryFn: () => api.listProjectsPage({ page: 1, pageSize: 100, visibility: canManageBilling ? 'all' : 'related', sortBy: 'lastUsedAt', sortOrder: 'desc' }),
  })
  const cachedCurrentProject = projectId ? queryClient.getQueryData<Project>(['project', projectId]) : undefined
  const projectItems = useMemo(() => {
    const listed = projectsQuery.data?.items ?? []
    if (!cachedCurrentProject || listed.some(item => item.id === cachedCurrentProject.id))
      return listed
    return [cachedCurrentProject, ...listed]
  }, [cachedCurrentProject, projectsQuery.data?.items])
  const walletOwnerUserId = canManageBilling ? billingUserScopeId : (user?.id ?? '')
  const visibleProjectItems = useMemo(() => {
    if (isProjectMode || !walletOwnerUserId)
      return projectItems
    return projectItems.filter(project => project.billingOwnerUserId === walletOwnerUserId)
  }, [isProjectMode, projectItems, walletOwnerUserId])
  const waitingForWalletProjectPrune = !isProjectMode && !canManageBilling && selectedProjectIds.length > 0 && projectsQuery.isPending
  const effectiveSelectedProjectIds = useMemo(() => {
    if (isProjectMode || selectedProjectIds.length === 0 || canManageBilling)
      return selectedProjectIds
    if (!projectsQuery.isSuccess)
      return []
    const allowedIds = new Set(visibleProjectItems.map(project => project.id))
    return selectedProjectIds.filter(id => allowedIds.has(id))
  }, [canManageBilling, isProjectMode, projectsQuery.isSuccess, selectedProjectIds, visibleProjectItems])
  useEffect(() => {
    if (isProjectMode || canManageBilling || !projectsQuery.isSuccess)
      return
    if (effectiveSelectedProjectIds.length !== selectedProjectIds.length)
      writeCachedBillingProjectScope(effectiveSelectedProjectIds)
  }, [canManageBilling, effectiveSelectedProjectIds, isProjectMode, projectsQuery.isSuccess, selectedProjectIds.length])
  const projectMap = useMemo(() => new Map(projectItems.map(project => [project.id, project])), [projectItems])
  const singleSelectedProjectId = effectiveSelectedProjectIds.length === 1 ? effectiveSelectedProjectIds[0] : ''
  const applicationsQuery = useQuery({
    enabled: Boolean(singleSelectedProjectId) && !waitingForWalletProjectPrune,
    queryKey: ['applications', singleSelectedProjectId],
    queryFn: () => api.listApplications(singleSelectedProjectId),
  })
  const applicationItems = useMemo(() => applicationsQuery.data ?? [], [applicationsQuery.data])
  const effectiveApplicationFilter = useMemo(() => {
    if (!singleSelectedProjectId || applicationFilter === 'all')
      return 'all'
    if (applicationsQuery.data && !applicationsQuery.data.some(app => app.id === applicationFilter))
      return 'all'
    return applicationFilter
  }, [applicationFilter, applicationsQuery.data, singleSelectedProjectId])

  const projectIds = effectiveSelectedProjectIds.length > 0 ? effectiveSelectedProjectIds : undefined
  const billingPeriodQuery = useMemo(() => billingPeriodToQuery(billingPeriod), [billingPeriod])
  const detailPeriodQuery = useMemo(
    () => focusedBucket
      ? { periodEnd: focusedBucket.periodEnd, periodStart: focusedBucket.periodStart }
      : billingPeriodQuery,
    [billingPeriodQuery, focusedBucket],
  )
  const billingUserQuery = !isProjectMode && canManageBilling && billingUserScopeId ? billingUserScopeId : undefined
  // Today and this month follow the viewer's local calendar, not the server's.
  const timeZone = browserTimeZone()
  const accountSummaryParams = canManageBilling && !billingUserQuery
    ? { ...billingPeriodQuery, timeZone, userId: billingUserQuery }
    : { ...billingPeriodQuery, accountScope: 'current' as const, timeZone, userId: billingUserQuery }
  const usersQuery = useQuery({
    enabled: !isProjectMode && canManageBilling,
    queryKey: ['billing', 'users'],
    queryFn: () => api.listUsers({ page: 1, pageSize: 100, sortBy: 'email', sortOrder: 'asc' }),
  })
  const userItems = useMemo(() => usersQuery.data?.items ?? [], [usersQuery.data?.items])

  const accountSummaryQuery = useQuery({
    enabled: !isProjectMode,
    queryKey: ['billing', 'summary', 'account', billingPeriodQuery, billingUserQuery, canManageBilling, timeZone],
    queryFn: () => api.getBillingSummary(undefined, accountSummaryParams),
  })
  const scopedSummaryQuery = useQuery({
    enabled: !waitingForWalletProjectPrune,
    queryKey: ['billing', 'summary', 'scope', scopeMode ?? 'wallet', effectiveSelectedProjectIds, billingPeriodQuery, billingUserQuery, timeZone],
    queryFn: () => api.getBillingSummary(projectIds, { ...billingPeriodQuery, scopeMode, timeZone, userId: billingUserQuery }),
  })
  const spendTimeseriesQuery = useQuery({
    enabled: !waitingForWalletProjectPrune,
    queryKey: ['billing', 'spend-timeseries', scopeMode ?? 'wallet', effectiveSelectedProjectIds, billingPeriodQuery, effectiveGranularity, billingUserQuery, timeZone],
    queryFn: ({ signal }) => api.getBillingSpendTimeseries(
      projectIds,
      { ...billingPeriodQuery, granularity: effectiveGranularity, scopeMode, timeZone, userId: billingUserQuery },
      signal,
    ),
  })
  const deploymentSpendQuery = useQuery({
    enabled: !waitingForWalletProjectPrune,
    queryKey: ['billing', 'deployment-spend', scopeMode ?? 'wallet', effectiveSelectedProjectIds, effectiveApplicationFilter, stageFilter, detailPeriodQuery, billingUserQuery, deploymentSpendPage],
    queryFn: () => api.listBillingDeploymentSpend({
      page: deploymentSpendPage,
      pageSize: BILLING_PAGE_SIZE,
      ...detailPeriodQuery,
      applicationId: effectiveApplicationFilter === 'all' ? undefined : effectiveApplicationFilter,
      projectIds,
      scopeMode,
      stage: stageFilter === 'all' ? undefined : stageFilter,
      userId: billingUserQuery,
      sortBy: 'amountCredits',
      sortOrder: 'desc',
    }),
  })
  const ledgerQuery = useQuery({
    enabled: !waitingForWalletProjectPrune,
    queryKey: ['billing', 'ledger', scopeMode ?? 'wallet', effectiveSelectedProjectIds, effectiveApplicationFilter, detailPeriodQuery, billingUserQuery, ledgerTypeFilter, ledgerCategoryFilter, ledgerPage],
    queryFn: () => api.listBillingLedgerEntries({
      page: ledgerPage,
      pageSize: BILLING_PAGE_SIZE,
      ...detailPeriodQuery,
      applicationId: effectiveApplicationFilter === 'all' ? undefined : effectiveApplicationFilter,
      category: ledgerCategoryFilter === 'all' ? undefined : ledgerCategoryFilter,
      projectIds,
      scopeMode,
      type: isProjectMode || ledgerTypeFilter === 'all' ? undefined : ledgerTypeFilter,
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
    setApplicationFilter('all')
    setLedgerTypeFilter('all')
    setLedgerCategoryFilter('all')
    resetBillingPages()
  }

  function handleApplicationFilterChange(nextApplicationId: string) {
    setApplicationFilter(nextApplicationId)
    resetBillingPages()
  }

  function handleStageFilterChange(nextStage: BillingDeploymentStageFilter) {
    setStageFilter(nextStage)
    setDeploymentSpendPage(1)
  }

  function handleClearDeploymentSpendFilters() {
    setApplicationFilter('all')
    setStageFilter('all')
    resetBillingPages()
  }

  function handleProjectFilterChange(projectIds: string[]) {
    setSelectedProjectIds(projectIds)
    if (!isProjectMode)
      writeCachedBillingProjectScope(projectIds)
    setApplicationFilter('all')
    setFocusedBucket(null)
    resetBillingPages()
  }

  function handleResetToCurrentProject() {
    if (projectId)
      handleProjectFilterChange([projectId])
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
    applicationFilter: effectiveApplicationFilter,
    applicationItems,
    applicationsQuery,
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
    handleApplicationFilterChange,
    handleBillingUserChange,
    handleClearDeploymentSpendFilters,
    handleClearFocusedBucket,
    handleClearLedgerFilters,
    handleGranularityChange,
    handleLedgerCategoryFilterChange,
    handleLedgerTypeFilterChange,
    handlePeriodChange,
    handleProjectFilterChange,
    handleResetToCurrentProject,
    handleSelectBucket,
    handleStageFilterChange,
    isProjectMode,
    ledgerCategoryFilter,
    ledgerPage,
    ledgerQuery,
    ledgerTypeFilter,
    projectMap,
    projectsQuery,
    scopedSummaryQuery,
    selectedProjectIds: effectiveSelectedProjectIds,
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
    singleSelectedProjectId,
    spendTimeseriesQuery,
    stageFilter,
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
