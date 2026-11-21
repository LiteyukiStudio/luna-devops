import type { TFunction } from 'i18next'
import type { ReactNode } from 'react'
import type { DeploymentTargetAdvancedItem, DeploymentTargetFieldName, DeploymentTargetFormValues } from '../deployment-target-form'
import type { FormSnapshot } from './advanced-items'
import type { DeploymentTarget, RepositoryBinding } from '@/api'
import { ChevronDown, RotateCcw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Controller, useFormContext, useFormState, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { RuntimeConfigFilesEditor } from '@/components/common/runtime-config-files-editor'
import { StatusBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'
import { deploymentTargetSectionAnchor, fieldErrorCount, sectionErrorCount, useFieldError } from '../deployment-target-editor-utils'
import { changedFieldCount, deploymentTargetAdvancedItemFields } from '../deployment-target-form'
import { ServiceAdvancedFields } from '../runtime/service-ports-fields'
import { BuildAdvancedFields } from '../source/source-build-fields'
import { advancedItemConfiguredCount, visibleAdvancedItems } from './advanced-items'
import { ContainerFields, ProbesFields } from './container-fields'
import { HookBindingsFields } from './hook-bindings-fields'
import { SecuritySchedulingFields } from './security-scheduling-fields'

/**
 * 高级选项分区：默认折叠 6+1 子项，每项一行摘要。
 * 已有非默认值的项默认展开；任一可见字段校验失败时分区和对应子项强制展开，错误不会被折叠隐藏。
 */
export function AdvancedSection({
  buildEnvironmentAvailable,
  buildEnvironmentRetrying,
  onOpenChange,
  onRetryBuildEnvironment,
  open,
  projectId,
  repositoryBindings,
  target,
}: {
  buildEnvironmentAvailable: boolean
  buildEnvironmentRetrying?: boolean
  onOpenChange: (open: boolean) => void
  onRetryBuildEnvironment?: () => void
  open: boolean
  projectId: string
  repositoryBindings: RepositoryBinding[]
  target: DeploymentTarget | null
}) {
  const { i18n, t } = useTranslation()
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const { errors } = useFormState({ control })
  const values = useWatch({ control })
  const items = visibleAdvancedItems(values.sourceType)
  const configured = items.reduce((count, item) => count + advancedItemConfiguredCount(item, values, target), 0)
  const errorCount = sectionErrorCount(errors, 'advanced', values.sourceType)
  const expanded = open || errorCount > 0
  const anchor = deploymentTargetSectionAnchor('advanced')

  return (
    <section aria-labelledby={`${anchor}-title`} className="scroll-mt-24" id={anchor}>
      <Collapsible open={expanded} onOpenChange={onOpenChange}>
        <Surface variant="bordered">
          <div className="px-6 py-4">
            <h2 className="text-base font-semibold text-foreground" id={`${anchor}-title`}>
              <CollapsibleTrigger className="flex w-full min-w-0 items-center gap-3 rounded-control text-left outline-none focus-visible:ring-2 focus-visible:ring-ring">
                <span className="min-w-0 flex-1 truncate">{t('deploymentTargetEditor.sections.advanced')}</span>
                <ItemState configured={configured} errorCount={errorCount} />
                <Chevron open={expanded} />
              </CollapsibleTrigger>
            </h2>
          </div>
          <CollapsibleContent>
            <ul className="divide-y divide-border border-t border-border">
              {items.map(item => (
                <AdvancedItemRow
                  key={item}
                  configured={advancedItemConfiguredCount(item, values, target)}
                  errorCount={fieldErrorCount(errors, deploymentTargetAdvancedItemFields[item])}
                  summary={advancedItemSummary(item, values, target, t, i18n.language)}
                  title={advancedItemTitle(item, t)}
                >
                  {item === 'build' && (
                    <BuildAdvancedFields
                      buildEnvironmentAvailable={buildEnvironmentAvailable}
                      buildEnvironmentRetrying={buildEnvironmentRetrying}
                      repositoryBindings={repositoryBindings}
                      onRetryBuildEnvironment={onRetryBuildEnvironment}
                    />
                  )}
                  {item === 'service' && <ServiceAdvancedFields />}
                  {item === 'probes' && <ProbesFields />}
                  {item === 'container' && <ContainerFields />}
                  {item === 'files' && <RuntimeFilesFields target={target} />}
                  {item === 'securityScheduling' && <SecuritySchedulingFields />}
                  {item === 'hooks' && <HookBindingsFields projectId={projectId} />}
                </AdvancedItemRow>
              ))}
            </ul>
          </CollapsibleContent>
        </Surface>
      </Collapsible>
    </section>
  )
}

function AdvancedItemRow({ children, configured, errorCount, summary, title }: {
  children: ReactNode
  configured: number
  errorCount: number
  summary: string
  title: string
}) {
  const [expanded, setExpanded] = useState(configured > 0)
  const [prevConfigured, setPrevConfigured] = useState(configured)
  if (configured !== prevConfigured) {
    const wasEmpty = prevConfigured === 0
    setPrevConfigured(configured)
    if (wasEmpty && configured > 0)
      setExpanded(true)
  }
  const [prevErrorCount, setPrevErrorCount] = useState(errorCount)
  if (errorCount !== prevErrorCount) {
    setPrevErrorCount(errorCount)
    if (errorCount > 0)
      setExpanded(true)
  }
  const open = expanded || errorCount > 0
  return (
    <li>
      <Collapsible open={open} onOpenChange={setExpanded}>
        <h3>
          <CollapsibleTrigger className="flex w-full min-w-0 flex-wrap items-center gap-x-3 gap-y-1 px-6 py-3 text-left outline-none transition-colors duration-fast hover:bg-surface-subtle focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring motion-reduce:transition-none">
            <span className="min-w-0 flex-1 text-sm font-medium text-foreground @lg/workspace:w-56 @lg/workspace:flex-none">{title}</span>
            <span className="hidden min-w-0 flex-1 truncate text-xs text-muted-foreground @lg/workspace:inline">{summary}</span>
            <ItemState configured={configured} errorCount={errorCount} />
            <Chevron open={open} />
          </CollapsibleTrigger>
        </h3>
        <CollapsibleContent className="grid min-w-0 gap-4 border-t border-border/60 bg-surface-subtle/20 px-6 py-4">
          {children}
        </CollapsibleContent>
      </Collapsible>
    </li>
  )
}

function ItemState({ configured, errorCount }: { configured: number, errorCount: number }) {
  const { t } = useTranslation()
  return (
    <>
      {configured > 0 && <span className="shrink-0 text-xs font-normal text-muted-foreground">{t('deploymentTargetEditor.configuredCount', { count: configured })}</span>}
      {errorCount > 0 && <StatusBadge tone="danger">{t('deploymentTargetEditor.errorCount', { count: errorCount })}</StatusBadge>}
    </>
  )
}

function Chevron({ open }: { open: boolean }) {
  return <ChevronDown aria-hidden="true" className={cn('size-4 shrink-0 text-muted-foreground transition-transform duration-fast motion-reduce:transition-none', open && 'rotate-180')} />
}

function advancedItemTitle(item: DeploymentTargetAdvancedItem, t: TFunction) {
  const titles: Record<DeploymentTargetAdvancedItem, string> = {
    build: t('deploymentTargetEditor.buildAdvanced'),
    service: t('deploymentTargetEditor.serviceAdvanced'),
    probes: t('deploymentTargetEditor.advancedItems.probes'),
    container: t('deploymentTargetEditor.containerTitle'),
    files: t('deploymentTargetEditor.advancedItems.files'),
    securityScheduling: t('deploymentTargetEditor.advancedItems.securityScheduling'),
    hooks: t('deploymentTargetEditor.advancedItems.hooks'),
  }
  return titles[item]
}

/** 一行摘要：枚举类项显示当前取值，其余项列出已设置的字段名，全部默认时显示“使用默认值”。 */
function advancedItemSummary(item: DeploymentTargetAdvancedItem, values: FormSnapshot, target: DeploymentTarget | null, t: TFunction, language: string) {
  if (item === 'hooks') {
    return values.buildHooksEnabled
      ? t('deploymentsPage.progressiveHooksSummary', { count: values.buildHookBindings?.length ?? 0 })
      : t('deploymentsPage.progressiveHooksDisabledSummary')
  }
  if (item === 'files' && values.secretFiles?.trim() === '[]' && target?.secretFilesSet)
    return t('deploymentTargetEditor.secretFilesPendingClearSummary')

  const labels = advancedFieldLabels(t)
  const configuredLabels = deploymentTargetAdvancedItemFields[item]
    .filter(field => changedFieldCount(values, [field]) > 0)
    .map(field => labels[field] ?? field)
  if (item === 'files' && target?.secretFilesSet && !values.secretFiles)
    configuredLabels.push(t('deploymentsPage.runtimeSecretFiles'))

  return configuredLabels.length > 0
    ? new Intl.ListFormat(language, { style: 'short', type: 'conjunction' }).format(configuredLabels)
    : t('deploymentTargetEditor.advancedDefault')
}

function advancedFieldLabels(t: TFunction): Partial<Record<DeploymentTargetFieldName, string>> {
  return {
    buildDirectory: t('buildsPage.buildDirectory'),
    buildArgs: t('buildsPage.buildArgs'),
    buildCpuRequest: t('deploymentsPage.buildCpuRequest'),
    buildMemoryRequest: t('deploymentsPage.buildMemoryRequest'),
    buildTimeoutMinutes: t('deploymentsPage.buildTimeoutMinutes'),
    buildVariables: t('buildsPage.variables'),
    buildSecrets: t('buildsPage.secrets'),
    serviceSessionAffinity: t('deploymentsPage.serviceSessionAffinity'),
    internalOnly: t('deploymentsPage.internalOnly'),
    serviceAnnotations: t('deploymentsPage.serviceAnnotations'),
    startupProbe: t('deploymentsPage.startupProbe'),
    readinessProbe: t('deploymentsPage.readinessProbe'),
    livenessProbe: t('deploymentsPage.livenessProbe'),
    containerCommand: t('deploymentsPage.containerCommand'),
    containerArgs: t('deploymentsPage.containerArgs'),
    lifecycle: t('deploymentsPage.lifecycle'),
    webConsoleEnabled: t('deploymentsPage.webConsole'),
    initContainers: t('deploymentsPage.initContainers'),
    sidecarContainers: t('deploymentsPage.sidecarContainers'),
    configFiles: t('deploymentsPage.runtimeConfigFiles'),
    secretFiles: t('deploymentsPage.runtimeSecretFiles'),
    runAsUser: t('deploymentsPage.runAsUser'),
    runAsGroup: t('deploymentsPage.runAsGroup'),
    fsGroup: t('deploymentsPage.fsGroup'),
    fsGroupChangePolicy: t('deploymentsPage.fsGroupChangePolicy'),
    readOnlyRootFilesystem: t('deploymentsPage.readOnlyRootFilesystem'),
    capabilityDrop: t('deploymentsPage.capabilityDrop'),
    priorityClassName: t('deploymentsPage.priorityClassName'),
    nodeSelector: t('deploymentsPage.nodeSelector'),
    tolerations: t('deploymentsPage.tolerations'),
    affinity: t('deploymentsPage.affinity'),
    topologySpreadConstraints: t('deploymentsPage.topologySpreadConstraints'),
  }
}

function RuntimeFilesFields({ target }: { target: DeploymentTarget | null }) {
  const { t } = useTranslation()
  const { control, setValue } = useFormContext<DeploymentTargetFormValues>()
  const secretFilesValue = useWatch({ control, name: 'secretFiles' })
  const configFilesError = useFieldError('configFiles')
  const secretFilesError = useFieldError('secretFiles')
  const secretFilesClearing = Boolean(target?.secretFilesSet) && secretFilesValue.trim() === '[]'

  return (
    <div className="grid gap-4 @2xl/workspace:grid-cols-2">
      <FormField error={configFilesError} hint={t('deploymentsPage.runtimeConfigFilesHint')} label={t('deploymentsPage.runtimeConfigFiles')}>
        {(fieldControl, { labelId }) => (
          <Controller
            control={control}
            name="configFiles"
            render={({ field }) => (
              <RuntimeConfigFilesEditor
                ariaDescribedBy={fieldControl['aria-describedby']}
                ariaInvalid={Boolean(fieldControl['aria-invalid'])}
                ariaLabelledBy={labelId}
                initialValue={field.value}
                value={field.value}
                onChange={field.onChange}
              />
            )}
          />
        )}
      </FormField>
      <FormField
        error={secretFilesError}
        hint={target?.secretFilesSet ? t('deploymentsPage.runtimeSecretFilesConfigured') : t('deploymentsPage.runtimeSecretFilesHint')}
        label={t('deploymentsPage.runtimeSecretFiles')}
      >
        {(fieldControl, { labelId }) => (
          <div className="grid gap-2.5">
            {target?.secretFilesSet && (
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-control border border-border bg-surface-subtle px-3 py-2 text-xs">
                <span className={cn(secretFilesClearing ? 'font-medium text-danger' : 'text-muted-foreground')}>
                  {secretFilesClearing
                    ? t('deploymentTargetEditor.secretFilesPendingClearBanner')
                    : t('deploymentsPage.runtimeSecretFilesConfigured')}
                </span>
                <Button
                  size="sm"
                  type="button"
                  variant="outline"
                  onClick={() => setValue('secretFiles', secretFilesClearing ? '' : '[]', { shouldDirty: true, shouldValidate: true })}
                >
                  {secretFilesClearing ? <RotateCcw className="size-3.5" /> : <Trash2 className="size-3.5" />}
                  {secretFilesClearing ? t('deploymentTargetEditor.secretUndo') : t('deploymentTargetEditor.clearSecretFiles')}
                </Button>
              </div>
            )}
            {!secretFilesClearing && (
              <Controller
                control={control}
                name="secretFiles"
                render={({ field }) => (
                  <RuntimeConfigFilesEditor
                    ariaDescribedBy={fieldControl['aria-describedby']}
                    ariaInvalid={Boolean(fieldControl['aria-invalid'])}
                    ariaLabelledBy={labelId}
                    configuredPlaceholder={target?.secretFilesSet ? t('common.secretSetPlaceholder') : undefined}
                    initialValue={field.value}
                    value={field.value}
                    onChange={field.onChange}
                  />
                )}
              />
            )}
          </div>
        )}
      </FormField>
    </div>
  )
}
