import type { TFunction } from 'i18next'
import type { ReactNode } from 'react'
import type { DeploymentTargetAdvancedItem, DeploymentTargetFieldName, DeploymentTargetFormValues } from '../deployment-target-form'
import type { FormSnapshot } from './advanced-items'
import type { DeploymentTarget, RepositoryBinding } from '@/api'
import { ChevronDown } from 'lucide-react'
import { useState } from 'react'
import { Controller, useFormContext, useFormState, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { RuntimeConfigFilesEditor } from '@/components/common/runtime-config-files-editor'
import { StatusBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { NativeSelect } from '@/components/ui/native-select'
import { cn } from '@/lib/utils'
import { JsonField } from '../deployment-target-editor-fields'
import { deploymentTargetSectionAnchor, fieldErrorCount, useFieldError } from '../deployment-target-editor-utils'
import { changedFieldCount, deploymentTargetAdvancedItemFields, deploymentTargetSectionFields } from '../deployment-target-form'
import { ServiceAdvancedFields } from '../runtime/service-ports-fields'
import { BuildAdvancedFields } from '../source/source-build-fields'
import { advancedItemConfiguredCount, visibleAdvancedItems } from './advanced-items'
import { ContainerFields } from './container-fields'
import { HookBindingsFields } from './hook-bindings-fields'
import { AuxContainersFields, SecuritySchedulingFields } from './security-scheduling-fields'

/**
 * 高级（Kubernetes）分区：默认折叠，每项一行摘要。
 * 已有非默认值的项默认展开；任一字段校验失败时分区和对应项都会强制展开，错误不会被折叠隐藏。
 */
export function AdvancedSection({ buildEnvironmentAvailable, onOpenChange, open, projectId, repositoryBindings, target }: {
  buildEnvironmentAvailable: boolean
  onOpenChange: (open: boolean) => void
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
  const errorCount = fieldErrorCount(errors, deploymentTargetSectionFields.advanced)
  const expanded = open || errorCount > 0
  const anchor = deploymentTargetSectionAnchor('advanced')

  return (
    <section aria-labelledby={`${anchor}-title`} className="scroll-mt-24" id={anchor}>
      <Collapsible open={expanded} onOpenChange={onOpenChange}>
        <Surface variant="bordered">
          <div className="grid gap-1 px-6 py-5">
            <h2 className="text-base font-semibold text-foreground" id={`${anchor}-title`}>
              <CollapsibleTrigger className="flex w-full min-w-0 items-center gap-3 rounded-control text-left outline-none focus-visible:ring-2 focus-visible:ring-ring">
                <span className="min-w-0 flex-1 truncate">{t('deploymentTargetEditor.sections.advanced')}</span>
                <ItemState configured={configured} errorCount={errorCount} />
                <Chevron open={expanded} />
              </CollapsibleTrigger>
            </h2>
            <p className="text-sm text-muted-foreground">{t('deploymentTargetEditor.sections.advancedDescription')}</p>
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
                  {item === 'build' && <BuildAdvancedFields buildEnvironmentAvailable={buildEnvironmentAvailable} repositoryBindings={repositoryBindings} />}
                  {item === 'workload' && <WorkloadFields />}
                  {item === 'container' && <ContainerFields />}
                  {item === 'service' && <ServiceAdvancedFields />}
                  {item === 'files' && <RuntimeFilesFields target={target} />}
                  {item === 'hooks' && <HookBindingsFields projectId={projectId} />}
                  {item === 'webConsole' && <WebConsoleField />}
                  {item === 'securityScheduling' && <SecuritySchedulingFields />}
                  {item === 'auxContainers' && <AuxContainersFields />}
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
  const open = expanded || errorCount > 0
  return (
    <li>
      <Collapsible open={open} onOpenChange={setExpanded}>
        <h3>
          <CollapsibleTrigger className="flex w-full min-w-0 flex-wrap items-center gap-x-3 gap-y-1 px-6 py-3 text-left outline-none transition-colors duration-fast hover:bg-surface-subtle focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring motion-reduce:transition-none">
            <span className="w-52 shrink-0 text-sm font-medium text-foreground">{title}</span>
            <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">{summary}</span>
            <ItemState configured={configured} errorCount={errorCount} />
            <Chevron open={open} />
          </CollapsibleTrigger>
        </h3>
        <CollapsibleContent className="grid min-w-0 gap-4 px-6 pt-1 pb-5">
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
    workload: t('deploymentTargetEditor.advancedItems.workload'),
    container: t('deploymentTargetEditor.containerTitle'),
    service: t('deploymentTargetEditor.serviceAdvanced'),
    files: t('deploymentTargetEditor.advancedItems.files'),
    hooks: t('deploymentTargetEditor.advancedItems.hooks'),
    webConsole: t('deploymentsPage.webConsole'),
    securityScheduling: t('deploymentTargetEditor.advancedItems.securityScheduling'),
    auxContainers: t('deploymentsPage.kubernetesAdvancedMultiContainer'),
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
  if (item === 'webConsole')
    return values.webConsoleEnabled === 'disabled' ? t('deploymentsPage.webConsoleDisableForTarget') : t('deploymentsPage.webConsoleInheritProject')
  const labels = advancedFieldLabels(t)
  const configuredLabels = deploymentTargetAdvancedItemFields[item]
    .filter(field => field !== 'workloadType' && changedFieldCount(values, [field]) > 0)
    .map(field => labels[field] ?? field)
  if (item === 'files' && target?.secretFilesSet && !values.secretFiles)
    configuredLabels.push(t('deploymentsPage.runtimeSecretFiles'))
  if (item === 'workload') {
    const workload = values.workloadType === 'StatefulSet' ? t('deploymentsPage.kubernetesValues.StatefulSet') : t('deploymentsPage.kubernetesValues.Deployment')
    return [workload, ...configuredLabels].join(' · ')
  }
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
    autoScalingBehavior: t('deploymentsPage.autoScalingBehavior'),
    imagePullPolicy: t('deploymentsPage.imagePullPolicy'),
    containerCommand: t('deploymentsPage.containerCommand'),
    containerArgs: t('deploymentsPage.containerArgs'),
    lifecycle: t('deploymentsPage.lifecycle'),
    readinessProbe: t('deploymentsPage.readinessProbe'),
    livenessProbe: t('deploymentsPage.livenessProbe'),
    startupProbe: t('deploymentsPage.startupProbe'),
    serviceSessionAffinity: t('deploymentsPage.serviceSessionAffinity'),
    serviceAnnotations: t('deploymentsPage.serviceAnnotations'),
    configFiles: t('deploymentsPage.runtimeConfigFiles'),
    secretFiles: t('deploymentsPage.runtimeSecretFiles'),
    runAsUser: t('deploymentsPage.runAsUser'),
    runAsGroup: t('deploymentsPage.runAsGroup'),
    fsGroup: t('deploymentsPage.fsGroup'),
    fsGroupChangePolicy: t('deploymentsPage.fsGroupChangePolicy'),
    readOnlyRootFilesystem: t('deploymentsPage.readOnlyRootFilesystem'),
    capabilityDrop: t('deploymentsPage.capabilityDrop'),
    nodeSelector: t('deploymentsPage.nodeSelector'),
    tolerations: t('deploymentsPage.tolerations'),
    affinity: t('deploymentsPage.affinity'),
    topologySpreadConstraints: t('deploymentsPage.topologySpreadConstraints'),
    priorityClassName: t('deploymentsPage.priorityClassName'),
    initContainers: t('deploymentsPage.initContainers'),
    sidecarContainers: t('deploymentsPage.sidecarContainers'),
  }
}

function WorkloadFields() {
  const { t } = useTranslation()
  const { control, register } = useFormContext<DeploymentTargetFormValues>()
  const autoScalingEnabled = useWatch({ control, name: 'autoScalingEnabled' })
  return (
    <>
      <div className="grid gap-4 @xl/workspace:grid-cols-2">
        <FormField hint={t('deploymentsPage.workloadTypeHint')} label={t('deploymentsPage.workloadType')}>
          <NativeSelect {...register('workloadType')}>
            <option value="Deployment">{t('deploymentsPage.kubernetesValues.Deployment')}</option>
            <option value="StatefulSet">{t('deploymentsPage.kubernetesValues.StatefulSet')}</option>
          </NativeSelect>
        </FormField>
      </div>
      {autoScalingEnabled
        ? <JsonField hint={t('deploymentsPage.autoScalingBehaviorHint')} label={t('deploymentsPage.autoScalingBehavior')} name="autoScalingBehavior" placeholder={t('deploymentsPage.autoScalingBehaviorPlaceholder')} />
        : <p className="text-sm text-muted-foreground">{t('deploymentTargetEditor.autoScalingBehaviorUnavailable')}</p>}
    </>
  )
}

function RuntimeFilesFields({ target }: { target: DeploymentTarget | null }) {
  const { t } = useTranslation()
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const configFilesError = useFieldError('configFiles')
  const secretFilesError = useFieldError('secretFiles')
  return (
    <div className="grid gap-4 @2xl/workspace:grid-cols-2">
      <FormField error={configFilesError} hint={t('deploymentsPage.runtimeConfigFilesHint')} label={t('deploymentsPage.runtimeConfigFiles')}>
        <Controller
          control={control}
          name="configFiles"
          render={({ field }) => <RuntimeConfigFilesEditor initialValue={field.value} onChange={field.onChange} />}
        />
      </FormField>
      <FormField
        error={secretFilesError}
        hint={target?.secretFilesSet ? t('deploymentsPage.runtimeSecretFilesConfigured') : t('deploymentsPage.runtimeSecretFilesHint')}
        label={t('deploymentsPage.runtimeSecretFiles')}
      >
        <Controller
          control={control}
          name="secretFiles"
          render={({ field }) => (
            <RuntimeConfigFilesEditor
              configuredPlaceholder={target?.secretFilesSet ? t('common.secretSetPlaceholder') : undefined}
              initialValue={field.value}
              onChange={field.onChange}
            />
          )}
        />
      </FormField>
    </div>
  )
}

function WebConsoleField() {
  const { t } = useTranslation()
  const { register } = useFormContext<DeploymentTargetFormValues>()
  return (
    <div className="grid gap-4 @xl/workspace:grid-cols-2">
      <FormField hint={t('deploymentsPage.webConsoleOverrideHint')} label={t('deploymentsPage.webConsoleOverride')}>
        <NativeSelect {...register('webConsoleEnabled')}>
          <option value="inherit">{t('deploymentsPage.webConsoleInheritProject')}</option>
          <option value="disabled">{t('deploymentsPage.webConsoleDisableForTarget')}</option>
        </NativeSelect>
      </FormField>
    </div>
  )
}
