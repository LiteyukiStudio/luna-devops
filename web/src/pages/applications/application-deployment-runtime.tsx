import type { DeploymentRuntimeStatus } from './application-deployment-runtime-utils'
import type { DeploymentTarget } from '@/api'
import { useTranslation } from 'react-i18next'
import { DeploymentReplicaBadge } from '@/components/common/deployment-replica-badge'
import { StatusBadge } from '@/components/common/status-badge'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { formatTargetRuntimeSize } from './application-deployments-panel-utils'

export function DeploymentRuntimeSpecBadge({ target }: { target: DeploymentTarget }) {
  const { t } = useTranslation()
  return <StatusBadge tone="neutral">{formatTargetRuntimeSize(target, t)}</StatusBadge>
}

export function DeploymentRuntimeStatusBadge({ availableReplicas, deployed, desiredReplicas, readyReplicas, status }: { availableReplicas: number, deployed: boolean, desiredReplicas: number, readyReplicas: number, status: DeploymentRuntimeStatus }) {
  const { t } = useTranslation()
  const detail = status.summary.trim() || t(`deploymentsPage.runtimeStatusDetails.${status.value}`, { defaultValue: '' })
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">
          <DeploymentReplicaBadge availableReplicas={availableReplicas} deployed={deployed} desiredReplicas={desiredReplicas} readyReplicas={readyReplicas} status={status.value} />
        </span>
      </TooltipTrigger>
      <TooltipContent className="grid max-w-96 gap-1 leading-5" side="top">
        {status.clusterName && <span>{t('deploymentsPage.runtimeStatusCluster', { cluster: status.clusterName })}</span>}
        {status.podCount > 0 && <span>{t('deploymentsPage.runtimePodCount', { count: status.podCount })}</span>}
        {detail && <span className="break-words">{detail}</span>}
      </TooltipContent>
    </Tooltip>
  )
}
