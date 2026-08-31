import type { Application, Project, RuntimeCluster } from '@/api'
import { KeyRound } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/common/empty-state'
import { Section } from '@/components/common/section'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { KubeconfigCreateDialog } from './kubeconfig-create-dialog'

export function ProjectKubectlAccessPanel({
  applications,
  featureEnabled,
  project,
  runtimeClusters,
}: {
  applications: Application[]
  featureEnabled: boolean
  project?: Project
  runtimeClusters: RuntimeCluster[]
}) {
  const { t } = useTranslation()
  const [dialogOpen, setDialogOpen] = useState(false)
  const activeClusters = useMemo(
    () => runtimeClusters.filter(cluster => (cluster.deleteStatus ?? 'active') === 'active'),
    [runtimeClusters],
  )

  if (!featureEnabled) {
    return (
      <Section description={t('kubectlAccess.featureDisabledDescription')} title={t('kubectlAccess.projectTitle')} variant="bordered">
        <EmptyState description={t('kubectlAccess.featureDisabledDescription')} title={t('kubectlAccess.featureDisabledTitle')} variant="plain" />
      </Section>
    )
  }

  return (
    <>
      <Section
        description={t('kubectlAccess.projectDescription')}
        title={t('kubectlAccess.projectTitle')}
        tools={(
          <Button disabled={activeClusters.length === 0} onClick={() => setDialogOpen(true)}>
            <KeyRound className="size-4" />
            {t('kubectlAccess.createButton')}
          </Button>
        )}
        variant="bordered"
      >
        <div className="grid gap-4">
          <div className="flex flex-wrap gap-2">
            <StatusBadge>{t('kubectlAccess.namespaceLabel', { namespace: project?.kubernetesNamespace ?? '-' })}</StatusBadge>
            <StatusBadge>{t('kubectlAccess.activeClusterCount', { count: activeClusters.length })}</StatusBadge>
          </div>
          {activeClusters.length === 0
            ? (
                <EmptyState
                  description={t('kubectlAccess.noActiveClustersDescription')}
                  title={t('kubectlAccess.noActiveClustersTitle')}
                  variant="plain"
                />
              )
            : (
                <div className="grid gap-3 md:grid-cols-2">
                  {activeClusters.map(cluster => (
                    <div key={cluster.id} className="grid gap-2 rounded-container border border-border bg-surface-inset p-4">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <p className="truncate font-medium">{cluster.name}</p>
                          <p className="text-sm text-muted-foreground">{cluster.gatewayDomainSuffixes?.join(', ') || cluster.gatewayRootDomain}</p>
                        </div>
                        <StatusBadge>{cluster.kubeGatewayEnabled ? t('kubectlAccess.gatewayExpectedEnabled') : t('kubectlAccess.gatewayExpectedDisabled') }</StatusBadge>
                      </div>
                    </div>
                  ))}
                </div>
              )}
        </div>
      </Section>
      <KubeconfigCreateDialog
        applications={applications}
        open={dialogOpen}
        project={project}
        runtimeClusters={activeClusters}
        onOpenChange={setDialogOpen}
      />
    </>
  )
}
