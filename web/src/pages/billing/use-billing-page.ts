import type { BillingPeriodSelection } from './billing-page-utils'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { isPlatformAdmin } from '@/lib/roles'
import { BILLING_PAGE_SIZE, billingPeriodToQuery, periodSelectionForPreset, readCachedBillingProjectScope, writeCachedBillingProjectScope } from './billing-page-utils'

export function useBillingPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('deployment-spend')
  const [selectedProjectIds, setSelectedProjectIds] = useState<string[]>(readCachedBillingProjectScope)
  const [billingPeriod, setBillingPeriod] = useState<BillingPeriodSelection>(() => periodSelectionForPreset('thisMonth'))
  const [deploymentSpendPage, setDeploymentSpendPage] = useState(1)
  const [ledgerPage, setLedgerPage] = useState(1)
  const [usagePage, setUsagePage] = useState(1)
  const [selectedBillingUserId, setSelectedBillingUserId] = useState<string | null>(null)
  const [transactionOpen, setTransactionOpen] = useState(false)
  const [transactionUserId, setTransactionUserId] = useState('')
  const [transactionType, setTransactionType] = useState<'credit' | 'adjustment'>('credit')
  const [transactionAmount, setTransactionAmount] = useState('')
  const [transactionDescription, setTransactionDescription] = useState('')
  const canManageBilling = isPlatformAdmin(user?.role)
  const billingUserScopeId = selectedBillingUserId ?? (canManageBilling ? user?.id ?? '' : '')

  const projectsQuery = useQuery({
    queryKey: ['billing', 'projects', canManageBilling],
    queryFn: () => api.listProjectsPage({ page: 1, pageSize: 100, scope: canManageBilling ? 'all' : 'related', sortBy: 'lastUsedAt', sortOrder: 'desc' }),
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
  const billingUserQuery = canManageBilling && billingUserScopeId ? billingUserScopeId : undefined
  const accountSummaryParams = canManageBilling && !billingUserQuery
    ? { ...billingPeriodQuery, userId: billingUserQuery }
    : { ...billingPeriodQuery, accountScope: 'current' as const, userId: billingUserQuery }
  const usersQuery = useQuery({
    enabled: canManageBilling,
    queryKey: ['billing', 'users'],
    queryFn: () => api.listUsers({ page: 1, pageSize: 100, sortBy: 'email', sortOrder: 'asc' }),
  })
  const userItems = useMemo(() => usersQuery.data?.items ?? [], [usersQuery.data?.items])

  const accountSummaryQuery = useQuery({
    queryKey: ['billing', 'summary', 'account', billingPeriodQuery, billingUserQuery, canManageBilling],
    queryFn: () => api.getBillingSummary(undefined, accountSummaryParams),
  })
  const scopedSummaryQuery = useQuery({
    queryKey: ['billing', 'summary', 'scope', selectedProjectIds, billingPeriodQuery, billingUserQuery],
    queryFn: () => api.getBillingSummary(projectIds, { ...billingPeriodQuery, userId: billingUserQuery }),
  })
  const deploymentSpendQuery = useQuery({
    queryKey: ['billing', 'deployment-spend', selectedProjectIds, billingPeriodQuery, billingUserQuery, deploymentSpendPage],
    queryFn: () => api.listBillingDeploymentSpend({
      page: deploymentSpendPage,
      pageSize: BILLING_PAGE_SIZE,
      ...billingPeriodQuery,
      projectIds,
      userId: billingUserQuery,
      sortBy: 'amountCredits',
      sortOrder: 'desc',
    }),
  })
  const ledgerQuery = useQuery({
    queryKey: ['billing', 'ledger', selectedProjectIds, billingPeriodQuery, billingUserQuery, ledgerPage],
    queryFn: () => api.listBillingLedgerEntries({
      page: ledgerPage,
      pageSize: BILLING_PAGE_SIZE,
      ...billingPeriodQuery,
      projectIds,
      userId: billingUserQuery,
      sortBy: 'createdAt',
      sortOrder: 'desc',
    }),
  })
  const usageQuery = useQuery({
    queryKey: ['billing', 'usage', selectedProjectIds, billingPeriodQuery, billingUserQuery, usagePage],
    queryFn: () => api.listBillingUsageRecords({
      page: usagePage,
      pageSize: BILLING_PAGE_SIZE,
      ...billingPeriodQuery,
      projectIds,
      userId: billingUserQuery,
      sortBy: 'createdAt',
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
    setUsagePage(1)
  }

  function handleProjectFilterChange(projectIds: string[]) {
    setSelectedProjectIds(projectIds)
    writeCachedBillingProjectScope(projectIds)
    resetBillingPages()
  }

  function handleBillingUserChange(userId: string) {
    setSelectedBillingUserId(userId)
    handleProjectFilterChange([])
  }

  function handlePeriodChange(period: BillingPeriodSelection) {
    setBillingPeriod(period)
    resetBillingPages()
  }

  return {
    accountSummaryQuery,
    activeTab,
    billingPeriod,
    billingUserScopeId,
    canManageBilling,
    createTransaction,
    deploymentSpendPage,
    deploymentSpendQuery,
    gatewayTrafficStatusQuery,
    handleBillingUserChange,
    handlePeriodChange,
    handleProjectFilterChange,
    ledgerPage,
    ledgerQuery,
    projectMap,
    projectsQuery,
    scopedSummaryQuery,
    selectedProjectIds,
    setActiveTab,
    setDeploymentSpendPage,
    setLedgerPage,
    setTransactionAmount,
    setTransactionDescription,
    setTransactionOpen,
    setTransactionType,
    setTransactionUserId,
    setUsagePage,
    transactionAmount,
    transactionDescription,
    transactionOpen,
    transactionType,
    transactionUserId,
    usagePage,
    usageQuery,
    userItems,
    usersQuery,
    visibleProjectItems,
  }
}
