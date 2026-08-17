import type { DeploymentRuntimeStatus, InternalServiceEndpointValue } from './application-deployment-runtime-utils'
import { useTranslation } from 'react-i18next'
import { DeploymentReplicaBadge } from '@/components/common/deployment-replica-badge'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

export function InternalServiceEndpoint({ endpoint, onCopy }: { endpoint?: InternalServiceEndpointValue, onCopy: (value?: string) => void }) {
  const { t } = useTranslation()
  if (!endpoint)
    return <span className="text-sm text-muted-foreground">-</span>

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button className="grid min-w-0 max-w-64 gap-0.5 text-left transition hover:text-primary-text" type="button" onClick={() => onCopy(endpoint.fqdn)}>
          <span className="truncate font-mono text-xs">{endpoint.serviceName}</span>
          <span className="truncate text-xs text-muted-foreground">{endpoint.fqdn}</span>
        </button>
      </TooltipTrigger>
      <TooltipContent className="grid max-w-96 gap-1 break-all leading-5" side="top">
        <span>{t('deploymentsPage.internalEndpointHint')}</span>
        <span className="font-mono">{endpoint.fqdn}</span>
      </TooltipContent>
    </Tooltip>
  )
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
