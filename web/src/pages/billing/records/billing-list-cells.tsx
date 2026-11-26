import type { BillingDeploymentSpend, Project } from '@/api'
import type { useBillingDisplay } from '@/lib/billing-display'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

export type BillingDisplay = ReturnType<typeof useBillingDisplay>

export interface BillingListPage<T> {
  items: T[]
  page: number
  pageSize: number
  total: number
  totalPages: number
}

export interface BillingListError {
  description?: string
  onRetry?: () => void
  retrying?: boolean
  title: string
}

export function ProjectCell({
  fallbackName,
  fallbackIdentifier,
  loading = false,
  project,
  unknownLabel,
}: {
  fallbackName?: string
  fallbackIdentifier?: string
  loading?: boolean
  project?: Project
  unknownLabel: string
}) {
  const resolvedName = project?.name || fallbackName
  if (!resolvedName && loading) {
    return <Skeleton className="h-4 w-24" />
  }
  const name = resolvedName || unknownLabel
  const identifier = project?.identifier || fallbackIdentifier || '-'
  return (
    <span className="block min-w-0">
      <span className="block truncate font-medium">{name}</span>
      <span className="block truncate text-xs text-muted-foreground">{identifier}</span>
    </span>
  )
}

type BillingApplicationRef = Pick<BillingDeploymentSpend, 'applicationName' | 'applicationIdentifier'> & {
  applicationId?: string
}

export function ApplicationCell({ item, unassignedLabel }: { item: BillingApplicationRef, unassignedLabel: string }) {
  return (
    <span className="block min-w-0">
      <span className="block truncate font-medium">{item.applicationName || unassignedLabel}</span>
      <span className="block truncate text-xs text-muted-foreground">{item.applicationIdentifier || '-'}</span>
    </span>
  )
}

export function DeploymentTargetCell({ item, projectStorageHint, projectStorageLabel, stageLabel, unassignedLabel }: {
  item: Pick<BillingDeploymentSpend, 'deploymentTargetId' | 'deploymentTargetName' | 'deploymentTargetStage' | 'spendGroup'>
  projectStorageHint: string
  projectStorageLabel: string
  stageLabel?: string
  unassignedLabel: string
}) {
  if (item.spendGroup === 'project_storage') {
    return (
      <span className="block min-w-0">
        <span className="block truncate font-medium">{projectStorageLabel}</span>
        <span className="block truncate text-xs text-muted-foreground">{projectStorageHint}</span>
      </span>
    )
  }
  const resolvedStageText = item.deploymentTargetStage
    ? (stageLabel && stageLabel !== item.deploymentTargetStage
        ? `${stageLabel} · ${item.deploymentTargetStage}`
        : item.deploymentTargetStage)
    : (item.deploymentTargetId || '-')
  return (
    <span className="block min-w-0">
      <span className="block truncate font-medium">{item.deploymentTargetName || unassignedLabel}</span>
      <span className="block truncate text-xs text-muted-foreground">{resolvedStageText}</span>
    </span>
  )
}

export function SpendAmount({
  billingDisplay,
  strong = false,
  value,
}: {
  billingDisplay: BillingDisplay
  strong?: boolean
  value: string
}) {
  return (
    <span className={cn('tabular-nums text-foreground', strong && 'font-semibold')}>
      {billingDisplay.formatAmountWithUnit(value)}
    </span>
  )
}

export function ResourceCell({ resourceId, resourceType }: { resourceId: string, resourceType: string }) {
  return (
    <span className="block min-w-0">
      <span className="block truncate font-medium">{resourceType || '-'}</span>
      <span className="block truncate text-xs text-muted-foreground">{resourceId || '-'}</span>
    </span>
  )
}
