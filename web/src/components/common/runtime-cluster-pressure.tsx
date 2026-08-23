import type { RuntimeClusterPressure, RuntimeClusterPressureResource } from '@/api'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/common/status-badge'
import { UsageRing } from '@/components/common/usage-ring'
import { runtimeClusterPressureTone } from '@/lib/runtime-cluster-pressure'

export function RuntimeClusterPressureBadge({ loading = false, pressure }: { loading?: boolean, pressure?: RuntimeClusterPressure }) {
  const { t } = useTranslation()
  if (loading && !pressure) {
    return (
      <StatusBadge className="shrink-0" tone="warning">
        {t('clustersPage.pressureLevels.checking')}
      </StatusBadge>
    )
  }
  const level = pressure?.pressureLevel ?? 'unavailable'
  const labels = {
    idle: t('clustersPage.pressureLevels.idle'),
    light: t('clustersPage.pressureLevels.light'),
    moderate: t('clustersPage.pressureLevels.moderate'),
    heavy: t('clustersPage.pressureLevels.heavy'),
    full: t('clustersPage.pressureLevels.full'),
    unavailable: t('clustersPage.pressureLevels.unavailable'),
  }
  return (
    <StatusBadge className="shrink-0" tone={runtimeClusterPressureTone(level)}>
      {labels[level]}
    </StatusBadge>
  )
}

export function RuntimeClusterPressureRings({ loading = false, pressure }: { loading?: boolean, pressure?: RuntimeClusterPressure }) {
  const { t } = useTranslation()
  if (!pressure?.details)
    return <RuntimeClusterPressureBadge loading={loading} pressure={pressure} />

  return (
    <div className="flex items-center gap-3">
      <PressureRing baseTone="primary" label="CPU" resource={pressure.details.cpu} unit="cpu" />
      <PressureRing baseTone="info" label={t('clustersPage.memoryShort')} resource={pressure.details.memory} unit="memory" />
    </div>
  )
}

function PressureRing({ baseTone, label, resource, unit }: {
  baseTone: 'info' | 'primary'
  label: string
  resource: RuntimeClusterPressureResource
  unit: 'cpu' | 'memory'
}) {
  const { t } = useTranslation()
  const requestPercent = Math.round(resource.requestPercent)
  const requests = unit === 'cpu' ? formatCPU(resource.requests) : formatBytes(resource.requests)
  const allocatable = unit === 'cpu' ? formatCPU(resource.allocatable) : formatBytes(resource.allocatable)
  const usage = resource.usage === undefined
    ? t('clustersPage.metricsUnavailable')
    : unit === 'cpu' ? formatCPU(resource.usage) : formatBytes(resource.usage)
  const usagePercent = resource.usagePercent === undefined ? undefined : Math.round(resource.usagePercent)
  const tooltip = t('clustersPage.pressureDetail', {
    allocatable,
    requestPercent,
    requests,
    usage,
    usagePercent: usagePercent === undefined ? t('clustersPage.metricsUnavailable') : `${usagePercent}%`,
  })
  const ariaLabel = `${label}: ${tooltip}`
  const requestPercentLabel = `${requestPercent}%`

  return (
    <div className="flex items-center gap-1.5 whitespace-nowrap">
      <span className="text-xs text-muted-foreground">{label}</span>
      <UsageRing ariaLabel={ariaLabel} baseTone={baseTone} ratio={resource.requestPercent / 100} tooltip={ariaLabel} />
      <span className="text-xs tabular-nums">{requestPercentLabel}</span>
    </div>
  )
}

function formatCPU(milli: number) {
  if (milli < 1000)
    return `${milli}m`
  return `${Number((milli / 1000).toFixed(1))} CPU`
}

function formatBytes(bytes: number) {
  const gibibytes = bytes / (1024 ** 3)
  if (gibibytes >= 1)
    return `${Number(gibibytes.toFixed(1))} GiB`
  return `${Math.round(bytes / (1024 ** 2))} MiB`
}
