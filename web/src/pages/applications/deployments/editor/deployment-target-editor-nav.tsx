import type { DeploymentTargetEditorSection, DeploymentTargetFormValues } from './deployment-target-form'
import type { RepositoryBinding, RuntimeCluster } from '@/api'
import { useEffect, useState } from 'react'
import { useFormContext, useFormState, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/common/status-badge'
import { cn } from '@/lib/utils'
import { deploymentTargetSectionAnchor, scrollToDeploymentTargetSection, sectionErrorCount } from './deployment-target-editor-utils'
import { deploymentTargetSectionOrder } from './deployment-target-form'

/** 分区导航：显示当前摘要和错误数；桌面端固定在左侧，窄屏为横向滚动条。 */
export function DeploymentTargetEditorNav({ repositoryBindings, runtimeClusters }: {
  repositoryBindings: RepositoryBinding[]
  runtimeClusters: RuntimeCluster[]
}) {
  const { t } = useTranslation()
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const { errors } = useFormState({ control })
  const activeSection = useActiveSection()
  const values = useWatch({ control })
  const titles: Record<DeploymentTargetEditorSection, string> = {
    basics: t('deploymentTargetEditor.sections.basics'),
    source: t('deploymentTargetEditor.sections.source'),
    runtime: t('deploymentTargetEditor.sections.runtime'),
    release: t('deploymentTargetEditor.sections.release'),
    hooks: t('deploymentTargetEditor.sections.hooks'),
    security: t('deploymentTargetEditor.sections.security'),
  }
  const cluster = runtimeClusters.find(item => item.id === values.clusterId)
  const binding = repositoryBindings.find(item => item.id === values.repositoryBindingId)
  const ports = values.servicePorts ?? []
  const summaries: Record<DeploymentTargetEditorSection, string> = {
    basics: [values.name?.trim() || t('deploymentTargetEditor.unnamed'), cluster?.name ?? t('deploymentTargetEditor.platformDefaultCluster')].join(' · '),
    source: values.sourceType === 'image'
      ? (values.imageRef?.trim() || t('apps.image'))
      : (binding ? `${binding.owner}/${binding.repo}` : t('apps.repository')),
    runtime: [
      values.autoScalingEnabled
        ? t('deploymentTargetEditor.summaryAutoScaling', { min: values.autoScalingMinReplicas, max: values.autoScalingMaxReplicas })
        : t('deploymentTargetEditor.summaryReplicas', { count: Number(values.replicas) || 0 }),
      `${values.cpuRequest ?? ''} / ${values.memoryRequest ?? ''}`,
      t('deploymentTargetEditor.summaryPorts', { count: ports.length }),
    ].join(' · '),
    release: values.autoDeploy ? t('deploymentTargetEditor.summaryAutoDeployOn') : t('deploymentTargetEditor.summaryAutoDeployOff'),
    hooks: values.buildHooksEnabled
      ? t('deploymentsPage.progressiveHooksSummary', { count: values.buildHookBindings?.length ?? 0 })
      : t('deploymentsPage.progressiveHooksDisabledSummary'),
    security: values.webConsoleEnabled === 'disabled' ? t('deploymentsPage.webConsoleDisableForTarget') : t('deploymentsPage.webConsoleInheritProject'),
  }

  return (
    <nav aria-label={t('deploymentTargetEditor.navLabel')} className="min-w-0">
      <ol className="flex gap-1 overflow-x-auto pb-1 lg:grid lg:overflow-visible lg:pb-0">
        {deploymentTargetSectionOrder.map((section) => {
          const errorCount = sectionErrorCount(errors, section)
          const active = activeSection === section
          return (
            <li key={section} className="shrink-0">
              <a
                aria-current={active ? 'location' : undefined}
                className={cn(
                  'grid min-w-36 gap-0.5 rounded-control px-3 py-2 text-left outline-none transition-colors duration-fast hover:bg-surface-subtle focus-visible:ring-2 focus-visible:ring-ring lg:min-w-0',
                  active && 'bg-primary-subtle',
                )}
                href={`#${deploymentTargetSectionAnchor(section)}`}
                onClick={(event) => {
                  event.preventDefault()
                  scrollToDeploymentTargetSection(section)
                }}
              >
                <span className="flex min-w-0 items-center justify-between gap-2">
                  <span className={cn('truncate text-sm', active ? 'font-semibold text-primary-text' : 'font-medium text-foreground')}>{titles[section]}</span>
                  {errorCount > 0 && <StatusBadge tone="danger">{t('deploymentTargetEditor.errorCount', { count: errorCount })}</StatusBadge>}
                </span>
                <span className="hidden truncate text-xs text-muted-foreground lg:block">{summaries[section]}</span>
              </a>
            </li>
          )
        })}
      </ol>
    </nav>
  )
}

/** 观察各分区进入视口的位置，用于导航高亮；只同步外部 DOM 状态。 */
function useActiveSection() {
  const [active, setActive] = useState<DeploymentTargetEditorSection>('basics')
  useEffect(() => {
    const elements = deploymentTargetSectionOrder
      .map(section => document.getElementById(deploymentTargetSectionAnchor(section)))
      .filter((element): element is HTMLElement => Boolean(element))
    if (elements.length === 0 || typeof IntersectionObserver === 'undefined')
      return
    const observer = new IntersectionObserver((entries) => {
      const visible = entries
        .filter(entry => entry.isIntersecting)
        .sort((left, right) => left.boundingClientRect.top - right.boundingClientRect.top)[0]
      const section = deploymentTargetSectionOrder.find(item => deploymentTargetSectionAnchor(item) === visible?.target.id)
      if (section)
        setActive(section)
    }, { rootMargin: '0px 0px -70% 0px' })
    for (const element of elements)
      observer.observe(element)
    return () => observer.disconnect()
  }, [])
  return active
}
