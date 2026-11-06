import type { DeploymentRuntimeStatus } from './application-deployment-runtime-utils'
import type { DeploymentTarget } from '@/api'
import { useTranslation } from 'react-i18next'
import { StatusDot } from '@/components/common/status-dot'
import { isInProgressStatus } from '@/components/common/status-tone'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { formatTargetRuntimeSize } from '@/pages/applications/deployments/application-deployments-panel-utils'
import { effectiveReplicaState, replicaStateTone } from './application-deployment-runtime-utils'

/** 规格是普通数据，用文字展示，不做成状态标签。 */
export function DeploymentRuntimeSpec({ target }: { target: DeploymentTarget }) {
  const { t } = useTranslation()
  return <span className="text-sm tabular-nums text-muted-foreground">{formatTargetRuntimeSize(target, t)}</span>
}

/** 部署配置或应用的运行状态：圆点 + 状态文字 + 就绪/期望副本数。 */
export function ReplicaStatusDot({ availableReplicas, deployed = true, desiredReplicas, readyReplicas, status }: {
  availableReplicas?: number
  deployed?: boolean
  desiredReplicas: number
  readyReplicas: number
  status: string
}) {
  const { t } = useTranslation()
  const state = effectiveReplicaState({ availableReplicas, deployed, desiredReplicas, readyReplicas, status })
  const label = state.value === 'not-deployed'
    ? t('deploymentsPage.notDeployed')
    : t(`deploymentsPage.runtimeStatuses.${state.value}`, { defaultValue: state.value })
  return (
    <StatusDot
      detail={state.showReplicas ? `${readyReplicas}/${desiredReplicas}` : undefined}
      label={label}
      pulse={isInProgressStatus(state.value)}
      tone={replicaStateTone(state.value)}
    />
  )
}

export function DeploymentRuntimeStatusDot({ availableReplicas, deployed, desiredReplicas, readyReplicas, status }: { availableReplicas: number, deployed: boolean, desiredReplicas: number, readyReplicas: number, status: DeploymentRuntimeStatus }) {
  const { t } = useTranslation()
  const detail = status.summary.trim() || t(`deploymentsPage.runtimeStatusDetails.${status.value}`, { defaultValue: '' })
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex max-w-full rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" tabIndex={0}>
          <ReplicaStatusDot availableReplicas={availableReplicas} deployed={deployed} desiredReplicas={desiredReplicas} readyReplicas={readyReplicas} status={status.value} />
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
