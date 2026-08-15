import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/common/status-badge'
import { statusToneFor } from '@/components/common/status-tone'

export function DeploymentReplicaBadge({
  deployed = true,
  desiredReplicas,
  label,
  labelKeyPrefix = 'deploymentsPage.runtimeStatuses',
  readyReplicas,
  status,
}: {
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

  const normalizedStatus = status.trim()
  const statusLabel = label ?? t(`${labelKeyPrefix}.${normalizedStatus}`, { defaultValue: normalizedStatus })
  return (
    <StatusBadge className="gap-1" tone={statusToneFor(normalizedStatus)}>
      {statusLabel}
      <span aria-hidden="true">·</span>
      <span className="tabular-nums">
        {readyReplicas}
        /
        {desiredReplicas}
      </span>
    </StatusBadge>
  )
}
