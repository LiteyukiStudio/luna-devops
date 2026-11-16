import type { DeploymentTargetEditorSection, DeploymentTargetFormValues } from './deployment-target-form'
import type { DeploymentTarget, RepositoryBinding } from '@/api'
import { useEffect, useRef, useState } from 'react'
import { useFormContext, useFormState, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/common/status-badge'
import { cn } from '@/lib/utils'
import { advancedItemConfiguredCount, visibleAdvancedItems } from './advanced/advanced-items'
import { deploymentTargetSectionAnchor, preferredScrollBehavior, scrollToDeploymentTargetSection, sectionErrorCount } from './deployment-target-editor-utils'
import { deploymentTargetSectionOrder } from './deployment-target-form'

/** 分区导航：显示当前摘要和错误数；桌面端固定在左侧，窄屏为横向滚动条。 */
export function DeploymentTargetEditorNav({ onNavigate, repositoryBindings, target }: {
  /** 跳转前通知编辑器，例如先展开默认折叠的高级分区。 */
  onNavigate: (section: DeploymentTargetEditorSection) => void
  repositoryBindings: RepositoryBinding[]
  target: DeploymentTarget | null
}) {
  const { t } = useTranslation()
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const { errors } = useFormState({ control })
  const activeSection = useActiveSection()
  const activeLinkRef = useRef<HTMLAnchorElement | null>(null)
  useEffect(() => {
    activeLinkRef.current?.scrollIntoView?.({ behavior: preferredScrollBehavior(), block: 'nearest', inline: 'nearest' })
  }, [activeSection])
  const values = useWatch({ control })
  const isImageSource = values.sourceType === 'image'
  const titles: Record<DeploymentTargetEditorSection, string> = {
    basics: t('deploymentTargetEditor.sections.basics'),
    source: isImageSource ? t('deploymentTargetEditor.sections.sourceImage') : t('deploymentTargetEditor.sections.sourceRepository'),
    runtime: t('deploymentTargetEditor.sections.runtime'),
    advanced: t('deploymentTargetEditor.sections.advanced'),
  }
  const binding = repositoryBindings.find(item => item.id === values.repositoryBindingId)
  const ports = values.servicePorts ?? []
  const advancedConfigured = visibleAdvancedItems(values.sourceType).reduce((count, item) => count + advancedItemConfiguredCount(item, values, target), 0)
  const sourceSummary = isImageSource
    ? (values.imageRef?.trim() || t('apps.image'))
    : (binding ? `${binding.owner}/${binding.repo}` : t('apps.repository'))
  const workloadLabel = values.workloadType === 'StatefulSet'
    ? t('deploymentsPage.kubernetesValues.StatefulSet')
    : t('deploymentsPage.kubernetesValues.Deployment')
  const summaries: Record<DeploymentTargetEditorSection, string> = {
    basics: [values.name?.trim() || t('deploymentTargetEditor.unnamed'), workloadLabel].join(' · '),
    source: [
      sourceSummary,
      values.autoDeploy ? t('deploymentTargetEditor.summaryAutoDeployOn') : t('deploymentTargetEditor.summaryAutoDeployOff'),
    ].join(' · '),
    runtime: [
      values.autoScalingEnabled
        ? t('deploymentTargetEditor.summaryAutoScaling', { min: values.autoScalingMinReplicas, max: values.autoScalingMaxReplicas })
        : t('deploymentTargetEditor.summaryReplicas', { count: Number(values.replicas) || 0 }),
      `${values.cpuRequest ?? ''} / ${values.memoryRequest ?? ''}`,
      t('deploymentTargetEditor.summaryPorts', { count: ports.length }),
    ].join(' · '),
    advanced: advancedConfigured > 0
      ? t('deploymentTargetEditor.configuredCount', { count: advancedConfigured })
      : t('deploymentTargetEditor.advancedDefault'),
  }

  return (
    <nav aria-label={t('deploymentTargetEditor.navLabel')} className="min-w-0 rounded-container border border-border bg-surface p-2 shadow-surface-sm">
      <ol className="flex gap-1 overflow-x-auto @2xl/workspace:grid @2xl/workspace:overflow-visible">
        {deploymentTargetSectionOrder.map((section) => {
          const errorCount = sectionErrorCount(errors, section, values.sourceType)
          const active = activeSection === section
          return (
            <li key={section} className="shrink-0">
              <a
                ref={active ? activeLinkRef : undefined}
                aria-current={active ? 'location' : undefined}
                className={cn(
                  'grid min-w-36 gap-0.5 rounded-control px-3 py-2 text-left outline-none transition-colors duration-fast hover:bg-surface-subtle focus-visible:ring-2 focus-visible:ring-ring @2xl/workspace:min-w-0',
                  active && 'bg-primary-subtle',
                )}
                href={`#${deploymentTargetSectionAnchor(section)}`}
                onClick={(event) => {
                  event.preventDefault()
                  onNavigate(section)
                  // 双 rAF 等待折叠的高级分区完成状态更新与 DOM 布局后再平滑滚动。
                  window.requestAnimationFrame(() => window.requestAnimationFrame(() => scrollToDeploymentTargetSection(section)))
                }}
              >
                <span className="flex min-w-0 items-center justify-between gap-2">
                  <span className={cn('truncate text-sm', active ? 'font-semibold text-primary-text' : 'font-medium text-foreground')}>{titles[section]}</span>
                  {errorCount > 0 && <StatusBadge tone="danger">{t('deploymentTargetEditor.errorCount', { count: errorCount })}</StatusBadge>}
                </span>
                <span className="hidden truncate text-xs text-muted-foreground @2xl/workspace:block">{summaries[section]}</span>
              </a>
            </li>
          )
        })}
      </ol>
    </nav>
  )
}

/** 观察各分区进入视口的位置，用于导航高亮；同步监听动态展开挂载或折叠属性变化的分区。 */
function useActiveSection() {
  const [active, setActive] = useState<DeploymentTargetEditorSection>('basics')
  useEffect(() => {
    if (typeof IntersectionObserver === 'undefined')
      return
    const observed = new Set<HTMLElement>()
    const observer = new IntersectionObserver((entries) => {
      const visible = entries
        .filter(entry => entry.isIntersecting)
        .sort((left, right) => left.boundingClientRect.top - right.boundingClientRect.top)[0]
      const section = deploymentTargetSectionOrder.find(item => deploymentTargetSectionAnchor(item) === visible?.target.id)
      if (section)
        setActive(section)
    }, { rootMargin: '0px 0px -65% 0px' })

    const syncObservedElements = () => {
      for (const section of deploymentTargetSectionOrder) {
        const element = document.getElementById(deploymentTargetSectionAnchor(section))
        if (element && !observed.has(element)) {
          observed.add(element)
          observer.observe(element)
        }
      }
    }
    syncObservedElements()

    const mutationObserver = typeof MutationObserver !== 'undefined'
      ? new MutationObserver(syncObservedElements)
      : null
    mutationObserver?.observe(document.body, {
      attributeFilter: ['data-state', 'hidden', 'id'],
      attributes: true,
      childList: true,
      subtree: true,
    })

    return () => {
      mutationObserver?.disconnect()
      observer.disconnect()
    }
  }, [])
  return active
}
