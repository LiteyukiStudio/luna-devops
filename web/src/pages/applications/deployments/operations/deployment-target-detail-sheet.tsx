import type { DeploymentTargetRow } from './application-deployment-targets-list'
import type { Release } from '@/api'
import { Gauge, Network, Package } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { DescriptionListItem } from '@/components/common/description-list-item'
import { Section } from '@/components/common/section'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { formatReleaseTime } from '@/pages/applications/application-config-utils'
import { AccessUrlLink } from '@/pages/applications/gateway/access-url-link'
import { gatewayRouteAccessUrl } from '@/pages/applications/gateway/gateway-route-url'
import { DeploymentRuntimeSpec, DeploymentRuntimeStatusIndicator } from './application-deployment-runtime'
import { DeploymentTargetMetricsCards } from './deployment-target-metrics-cards'
import {
  DeploymentTargetRuntimeEventsSection,
  DeploymentTargetRuntimePodsSection,
  RuntimeEventsWarningBanner,
} from './deployment-target-runtime-diagnostics'
import { useDeploymentTargetRuntimeDiagnostics } from './use-deployment-target-runtime-diagnostics'

type DetailTab = 'overview' | 'events'

export function DeploymentTargetDetailSheet({
  applicationId,
  item,
  onOpenChange,
  onOpenPodConsole,
  onViewPodLogs,
  open,
  projectId,
}: {
  applicationId: string
  item?: DeploymentTargetRow
  onOpenChange: (open: boolean) => void
  onOpenPodConsole?: (release: Release, pod: string) => void
  onViewPodLogs?: (release: Release, pod: string, previous: boolean) => void
  open: boolean
  projectId: string
}) {
  const { t } = useTranslation()
  const target = item?.target
  const namespace = item?.internalEndpoint?.namespace || '-'
  const stage = target ? t(`deploymentsPage.stageLabels.${target.stage}`, { defaultValue: target.stage }) : '-'
  const cluster = item?.runtimeStatus.clusterName || '-'

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="flex h-full w-[min(96vw,48rem)] max-w-none flex-col gap-0 overflow-hidden p-0 sm:max-w-none" side="right">
        {item && target
          ? (
              <DeploymentTargetDetailBody
                key={target.id}
                applicationId={applicationId}
                cluster={cluster}
                item={item}
                namespace={namespace}
                open={open}
                projectId={projectId}
                stage={stage}
                onOpenPodConsole={onOpenPodConsole}
                onViewPodLogs={onViewPodLogs}
              />
            )
          : (
              <SheetHeader className="shrink-0 border-b border-border p-6 pr-14">
                <SheetTitle>{t('deploymentsPage.deploymentDetails')}</SheetTitle>
                <SheetDescription>{`${stage} · ${cluster} · ${namespace}`}</SheetDescription>
              </SheetHeader>
            )}
      </SheetContent>
    </Sheet>
  )
}

function DeploymentTargetDetailBody({
  applicationId,
  cluster,
  item,
  namespace,
  open,
  projectId,
  stage,
  onOpenPodConsole,
  onViewPodLogs,
}: {
  applicationId: string
  cluster: string
  item: DeploymentTargetRow
  namespace: string
  open: boolean
  projectId: string
  stage: string
  onOpenPodConsole?: (release: Release, pod: string) => void
  onViewPodLogs?: (release: Release, pod: string, previous: boolean) => void
}) {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState<DetailTab>('overview')
  const [warningOnly, setWarningOnly] = useState(false)

  const target = item.target
  const release = item.release
  const projectRoutes = (item.routes ?? []).filter(route => route.enabled && gatewayRouteAccessUrl(route))
  const releaseMessage = release?.status === 'succeeded' ? '' : release?.message?.trim()

  const diagnostics = useDeploymentTargetRuntimeDiagnostics({
    applicationId,
    deployed: item.deployed,
    enabled: target.enabled,
    open,
    projectId,
    release,
    targetId: target.id,
  })

  const badgeCount = diagnostics.warningCount > 0 ? diagnostics.warningCount : diagnostics.events.length

  const handleViewWarningEvents = () => {
    setWarningOnly(true)
    setActiveTab('events')
  }

  return (
    <Tabs
      className="flex min-h-0 flex-1 flex-col gap-0"
      value={activeTab}
      onValueChange={value => setActiveTab(value as DetailTab)}
    >
      <div className="shrink-0 border-b border-border">
        <SheetHeader className="p-6 pb-3 pr-14">
          <SheetTitle>{target.name}</SheetTitle>
          <SheetDescription>{`${stage} · ${cluster} · ${namespace}`}</SheetDescription>
        </SheetHeader>
        <TabsList className="w-full border-b-0 px-6">
          <TabsTrigger value="overview">{t('deploymentsPage.overviewTab')}</TabsTrigger>
          <TabsTrigger value="events">
            <span>{t('deploymentsPage.eventsTab')}</span>
            {diagnostics.active && badgeCount > 0 && (
              <span
                className={cn(
                  'inline-flex min-w-5 items-center justify-center rounded-full border px-1.5 py-0.5 text-[11px] font-medium leading-none tabular-nums',
                  diagnostics.warningCount > 0
                    ? 'border-warning-border bg-warning-subtle text-warning'
                    : 'border-border bg-surface-subtle text-muted-foreground',
                )}
              >
                {badgeCount}
              </span>
            )}
          </TabsTrigger>
        </TabsList>
      </div>

      <TabsContent className="min-h-0 flex-1 overflow-y-auto p-6" value="overview">
        <div className="grid gap-4">
          <div className="flex flex-wrap items-center gap-x-5 gap-y-2 rounded-control border border-border bg-surface-subtle/50 px-3.5 py-2.5 text-xs">
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground">{t('deploymentsPage.runtimeStatus')}</span>
              <DeploymentRuntimeStatusIndicator
                availableReplicas={target.availableReplicas}
                deployed={item.deployed}
                desiredReplicas={target.desiredReplicas}
                readyReplicas={target.readyReplicas}
                status={item.runtimeStatus}
              />
            </div>
            {release && (
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground">{t('deploymentsPage.latestReleaseStatus')}</span>
                <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={release.status} />
              </div>
            )}
            {!target.enabled && <StatusValueBadge value="disabled" />}
            <div className="ml-auto">
              <DeploymentRuntimeSpec target={target} />
            </div>
          </div>

          <RuntimeEventsWarningBanner
            latestReason={diagnostics.latestWarningReason}
            warningCount={diagnostics.warningCount}
            onViewEvents={handleViewWarningEvents}
          />

          <DeploymentTargetRuntimePodsSection
            diagnostics={diagnostics}
            release={release}
            webConsoleEnabled={item.webConsoleEnabled}
            onOpenPodConsole={onOpenPodConsole}
            onViewPodLogs={onViewPodLogs}
          />

          <Section
            description={t('deploymentsPage.runtimeResourcesDescription')}
            icon={<Gauge className="size-4" aria-hidden="true" />}
            title={t('deploymentsPage.runtimeResources')}
            variant="bordered"
          >
            <div className="grid gap-4">
              <DeploymentTargetMetricsCards
                applicationId={applicationId}
                enabled={target.enabled && Boolean(release)}
                projectId={projectId}
                targetId={target.id}
              />
              <dl className="grid grid-cols-2 gap-4 border-t border-border pt-4 sm:grid-cols-4 [&_[data-slot=description-list-label]]:font-medium">
                <DescriptionListItem
                  label={t('deploymentsPage.replicas')}
                  value={target.status === 'unavailable' ? '-' : `${target.readyReplicas} / ${target.desiredReplicas}`}
                />
                <DescriptionListItem
                  label={t('deploymentsPage.workloadType')}
                  value={target.workloadType || '-'}
                />
                <DescriptionListItem
                  label={t('deploymentsPage.autoScalingEnabled')}
                  value={<StatusValueBadge value={target.autoScalingEnabled ? 'enabled' : 'disabled'} />}
                />
                <DescriptionListItem
                  label={t('deploymentsPage.servicePorts')}
                  value={formatServicePorts(target.servicePorts) || '-'}
                />
              </dl>
            </div>
          </Section>

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
                              <AccessUrlLink url={gatewayRouteAccessUrl(route)} />
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
      </TabsContent>

      <TabsContent className="min-h-0 flex-1 overflow-y-auto p-6" value="events">
        <DeploymentTargetRuntimeEventsSection
          diagnostics={diagnostics}
          warningOnly={warningOnly}
          onToggleWarningOnly={() => setWarningOnly(value => !value)}
        />
      </TabsContent>
    </Tabs>
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
