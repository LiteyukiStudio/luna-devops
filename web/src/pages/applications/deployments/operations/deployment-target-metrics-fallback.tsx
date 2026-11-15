import type { MetricsStreamState } from './use-deployment-target-metrics'
import { useTranslation } from 'react-i18next'
import { StatusValueBadge } from '@/components/common/status-badge'

export function MetricsStreamFallback({ state }: { state: Exclude<MetricsStreamState, { kind: 'ready' }> }) {
  const { t } = useTranslation()
  if (state.kind === 'disabled')
    return <span className="text-muted-foreground">-</span>
  if (state.kind === 'connecting')
    return <span className="text-xs text-muted-foreground">{t('deploymentsPage.metricsConnecting')}</span>
  return <StatusValueBadge value={state.status} />
}
