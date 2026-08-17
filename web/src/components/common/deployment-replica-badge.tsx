import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/common/status-badge'
import { statusToneFor } from '@/components/common/status-tone'

export function DeploymentReplicaBadge({
  availableReplicas,
  deployed = true,
  desiredReplicas,
  label,
  labelKeyPrefix = 'deploymentsPage.runtimeStatuses',
  readyReplicas,
  status,
}: {
  availableReplicas?: number
  deployed?: boolean
  desiredReplicas: number
  label?: ReactNode
  labelKeyPrefix?: string
  readyReplicas: number
  status: string
}) {
  const { t } = useTranslation()
  if (!deployed || status === 'not-deployed')
    return <StatusBadge tone="warning">{t('deploymentsPage.notDeployed')}</StatusBadge>

  const normalizedStatus = status.trim().toLowerCase() || 'unknown'
  const hasReplicaObservation = !['checking', 'disabled', 'not-configured', 'not-found', 'unavailable', 'unknown'].includes(normalizedStatus)
  let effectiveStatus = normalizedStatus
  if (normalizedStatus === 'scaled-to-zero' || (normalizedStatus === 'ready' && desiredReplicas === 0))
    effectiveStatus = 'scaled-to-zero'
  else if (normalizedStatus === 'ready' && (readyReplicas < desiredReplicas || (availableReplicas !== undefined && availableReplicas < desiredReplicas)))
    effectiveStatus = 'progressing'
  const statusLabel = label ?? t(`${labelKeyPrefix}.${effectiveStatus}`, { defaultValue: effectiveStatus })
  return (
    <StatusBadge className="gap-1" tone={statusToneFor(effectiveStatus)}>
      {statusLabel}
      {hasReplicaObservation && (
        <>
          <span aria-hidden="true">·</span>
          <span className="tabular-nums">
            {readyReplicas}
            /
            {desiredReplicas}
          </span>
        </>
      )}
    </StatusBadge>
  )
}
