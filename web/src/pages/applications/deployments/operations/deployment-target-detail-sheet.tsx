import type { DeploymentTargetRow } from './application-deployment-targets-list'
import { ExternalLink, Gauge, Network, Package } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { DescriptionListItem } from '@/components/common/description-list-item'
import { Section } from '@/components/common/section'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { formatReleaseTime } from '@/pages/applications/application-config-utils'
import { DeploymentRuntimeSpecBadge, DeploymentRuntimeStatusBadge } from './application-deployment-runtime'
import { DeploymentTargetMetricsCell } from './application-deployment-target-metrics-cell'

export function DeploymentTargetDetailSheet({ applicationId, item, onOpenChange, open, projectId }: {
  applicationId: string
  item?: DeploymentTargetRow
  onOpenChange: (open: boolean) => void
  open: boolean
  projectId: string
}) {
  const { t } = useTranslation()
  const target = item?.target
  const release = item?.release
  const namespace = item?.internalEndpoint?.namespace || '-'
  const stage = target ? t(`deploymentsPage.stageLabels.${target.stage}`, { defaultValue: target.stage }) : '-'
  const cluster = item?.runtimeStatus.clusterName || '-'
  const projectRoutes = (item?.routes ?? []).filter(route => route.enabled && route.accessUrl.trim())
  const releaseMessage = release?.status === 'succeeded' ? '' : release?.message?.trim()

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-[min(96vw,44rem)] max-w-none gap-0 overflow-y-auto p-0 sm:max-w-none" side="right">
        <SheetHeader className="border-b border-border p-6 pr-14">
          <SheetTitle>{target?.name ?? t('deploymentsPage.deploymentDetails')}</SheetTitle>
          <SheetDescription>{`${stage} · ${cluster} · ${namespace}`}</SheetDescription>
        </SheetHeader>
        {item && target && (
          <div className="grid gap-4 p-6">
            <div className="flex flex-wrap items-center gap-2">
              <StatusBadge tone="neutral">{stage}</StatusBadge>
              <StatusValueBadge value={target.enabled ? 'enabled' : 'disabled'} />
              <DeploymentRuntimeStatusBadge
                availableReplicas={target.availableReplicas}
                deployed={Boolean(release)}
                desiredReplicas={target.desiredReplicas}
                readyReplicas={target.readyReplicas}
                status={item.runtimeStatus}
              />
              {release
                ? <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={release.status} />
                : <StatusBadge tone="warning">{t('deploymentsPage.notDeployed')}</StatusBadge>}
              <DeploymentRuntimeSpecBadge target={target} />
            </div>

            <Section
              description={t('deploymentsPage.accessAddressesDescription')}
              icon={<Network className="size-4" aria-hidden="true" />}
              title={t('deploymentsPage.accessAddresses')}
              variant="bordered"
            >
              <dl className="grid gap-4 sm:grid-cols-2 [&_[data-slot=description-list-label]]:font-medium">
                <DescriptionListItem
                  label={t('deploymentsPage.namespaceLocalAddress')}
                  value={<CopyableValue value={item.internalEndpoint?.serviceName} />}
                />
                <DescriptionListItem
                  label={t('deploymentsPage.crossNamespaceAddress')}
                  value={<CopyableValue value={item.internalEndpoint?.fqdn} />}
                />
                <DescriptionListItem
                  className="sm:col-span-2"
                  label={t('deploymentsPage.projectAddresses')}
                  value={(
                    projectRoutes.length > 0
                      ? (
                          <div className="divide-y divide-separator-subtle">
                            {projectRoutes.map(route => (
                              <div key={route.id} className="flex min-w-0 items-center justify-between gap-3 py-2 first:pt-0 last:pb-0">
                                <a className="inline-flex min-w-0 items-center gap-1 font-mono text-xs text-primary-text hover:underline" href={route.accessUrl} rel="noreferrer" target="_blank">
                                  <span className="truncate">{route.accessUrl}</span>
                                  <ExternalLink className="size-3.5 shrink-0" aria-hidden="true" />
                                </a>
                                <StatusValueBadge labelKeyPrefix="gatewayRoutesPage.statuses" value={route.status} />
                              </div>
                            ))}
                          </div>
                        )
                      : <span className="text-muted-foreground">{t('deploymentsPage.projectAddressesEmpty')}</span>
                  )}
                />
              </dl>
            </Section>

            <Section
              description={t('deploymentsPage.runtimeResourcesDescription')}
              icon={<Gauge className="size-4" aria-hidden="true" />}
              title={t('deploymentsPage.runtimeResources')}
              variant="bordered"
            >
              <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 [&_[data-slot=description-list-label]]:font-medium">
                <DescriptionListItem
                  label={t('deploymentsPage.runtimeMetrics')}
                  value={(
                    <DeploymentTargetMetricsCell applicationId={applicationId} enabled={target.enabled && Boolean(release)} projectId={projectId} targetId={target.id} />
                  )}
                />
                <DescriptionListItem label={t('deploymentsPage.cpuRequest')} value={target.cpuRequest || '-'} />
                <DescriptionListItem label={t('deploymentsPage.memoryRequest')} value={target.memoryRequest || '-'} />
                <DescriptionListItem label={t('deploymentsPage.replicas')} value={target.status === 'unavailable' ? '-' : `${target.readyReplicas} / ${target.desiredReplicas}`} />
                <DescriptionListItem label={t('deploymentsPage.workloadType')} value={target.workloadType || '-'} />
                <DescriptionListItem label={t('deploymentsPage.autoScalingEnabled')} value={<StatusValueBadge value={target.autoScalingEnabled ? 'enabled' : 'disabled'} />} />
                <DescriptionListItem label={t('deploymentsPage.servicePorts')} value={formatServicePorts(target.servicePorts)} />
              </dl>
            </Section>

            <Section
              description={t('deploymentsPage.releaseInformationDescription')}
              icon={<Package className="size-4" aria-hidden="true" />}
              title={t('deploymentsPage.releaseInformation')}
              variant="bordered"
            >
              <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 [&_[data-slot=description-list-label]]:font-medium">
                <DescriptionListItem className="sm:col-span-2 lg:col-span-3" label={t('deploymentsPage.imageSummary')} value={<CopyableValue value={release?.imageRef} />} />
                <DescriptionListItem label={t('deploymentsPage.revision')} value={release ? `#${release.revision}` : '-'} />
                <DescriptionListItem label={t('deploymentsPage.releaseTime')} value={release ? formatReleaseTime(release, t) : '-'} />
                <DescriptionListItem label={t('deploymentsPage.imagePullPolicy')} value={target.imagePullPolicy || 'IfNotPresent'} />
                <DescriptionListItem label={t('deploymentsPage.kubernetesResourceName')} value={<CopyableValue value={target.kubernetesName} />} />
                <DescriptionListItem label={t('deploymentsPage.namespace')} value={namespace} />
                <DescriptionListItem label={t('deploymentsPage.runtimeData')} value={target.dataVolumes.length > 0 ? t('deploymentsPage.dataVolumeCount', { count: target.dataVolumes.length }) : t('common.disabled')} />
                {releaseMessage && (
                  <DescriptionListItem
                    className="sm:col-span-2 lg:col-span-3"
                    label={t('deploymentsPage.rolloutMessage')}
                    value={(
                      <CopyableHoverText className="whitespace-pre-wrap break-words text-sm" truncate={false} value={releaseMessage} />
                    )}
                  />
                )}
              </dl>
            </Section>
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}

function CopyableValue({ value }: { value?: string }) {
  if (!value?.trim())
    return <span className="text-muted-foreground">-</span>
  return <CopyableHoverText className="max-w-full font-mono text-xs" value={value} />
}

function formatServicePorts(ports: Array<{ name: string, port: number }>) {
  return ports.map(port => `${port.name || 'http'}:${port.port}`).join(' · ')
}
