import type { UseFormReturn } from 'react-hook-form'
import type { BuildTemplate, DeploymentRuntimeConfigRef, DeploymentTarget, DeploymentTargetPayload, ProjectHookConfig, ProjectRuntimeConfigSet, RuntimeCluster, RuntimeConfigRefMode } from '@/api'
import type { KeyValueRow } from '@/components/common/key-value-rows-editor'
import { Rocket, Save } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { KeyValueRowsEditor } from '@/components/common/key-value-rows-editor'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { RuntimeConfigFilesEditor } from '@/components/common/runtime-config-files-editor'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { RuntimeDataVolumesEditor } from './application-deployment-data-volumes-editor'
import { ApplicationDeploymentHooksEditor } from './application-deployment-hooks-editor'
import { KubernetesAdvancedFields } from './application-deployment-kubernetes-advanced-fields'
import { RuntimeResourceFields } from './application-deployment-resource-fields'
import { ServicePortsEditor } from './application-deployment-service-ports-editor'
import { ApplicationDeploymentBuildSettingsFields, ApplicationDeploymentSourceFields } from './application-deployment-source-fields'
import { deploymentTargetDefaults } from './application-deployments-panel-utils'
import { ApplicationRuntimeConfigSelector } from './application-runtime-config-selector'

export function ApplicationDeploymentTargetDialog({
  buildContextSuggestions,
  buildMinutePriceText,
  buildEnvironmentStatus,
  buildSecretRows,
  buildTemplates,
  buildTimeoutMinutes,
  defaultRuntimeCluster,
  dockerfileExposedPorts,
  dockerfileSuggestions,
  editingTarget,
  form,
  hooks,
  hooksError,
  hooksLoading,
  open,
  registries,
  repositoryBindings,
  recommendedTemplateIds,
  runtimeCostText,
  runtimeConfigRedeployableCount,
  runtimeConfigRedeployPending,
  runtimeConfigRestartAffectedCount,
  runtimeConfigSets,
  selectedHookBindings,
  selectedRuntimeConfigRefs,
  servicePorts,
  sourceType,
  targetBuildHooksEnabled,
  buildVariableRows,
  targetBuildOptionsError,
  targetBuildOptionsFetching,
  targetCanRedeploy,
  targetConfigFilesValid,
  targetDataRetentionEnabled,
  targetDataVolumes,
  targetHasRuntimeChanges,
  targetImagePrefix,
  targetRuntimeFilesValid,
  targetSecretFilesValid,
  runtimeClusters,
  savePending,
  summaries,
  onBindRepository,
  onChangeRuntimeConfigMode,
  onDismissRuntimeConfigRestart,
  onEditRuntimeConfigSet,
  onOpenChange,
  onRedeployRuntimeConfigTargets,
  onSave,
  onSetConfigFilesValid,
  onSetBuildSecretRows,
  onSetBuildVariableRows,
  onSetHookBindings,
  onSetSecretFilesValid,
  onToggleRuntimeConfigSet,
  onUpdateDataVolumes,
  onUpdateServicePorts,
}: {
  buildContextSuggestions: string[]
  buildMinutePriceText: string
  buildEnvironmentStatus: 'loading' | 'ready' | 'unavailable'
  buildSecretRows: KeyValueRow[]
  buildTemplates: BuildTemplate[]
  buildTimeoutMinutes: number
  defaultRuntimeCluster?: RuntimeCluster
  dockerfileExposedPorts: Record<string, number[]>
  dockerfileSuggestions: string[]
  editingTarget: DeploymentTarget | null
  form: UseFormReturn<DeploymentTargetPayload>
  hooks: ProjectHookConfig[]
  hooksError: boolean
  hooksLoading: boolean
  open: boolean
  registries: Parameters<typeof ApplicationDeploymentSourceFields>[0]['registries']
  repositoryBindings: Parameters<typeof ApplicationDeploymentSourceFields>[0]['repositoryBindings']
  recommendedTemplateIds: string[]
  runtimeCostText: string
  runtimeConfigRedeployableCount: number
  runtimeConfigRedeployPending: boolean
  runtimeConfigRestartAffectedCount: number
  runtimeConfigSets: ProjectRuntimeConfigSet[]
  selectedHookBindings: DeploymentTargetPayload['buildHookBindings']
  selectedRuntimeConfigRefs: DeploymentRuntimeConfigRef[]
  servicePorts: DeploymentTargetPayload['servicePorts']
  sourceType: DeploymentTargetPayload['sourceType']
  targetBuildHooksEnabled: boolean
  buildVariableRows: KeyValueRow[]
  targetBuildOptionsError: boolean
  targetBuildOptionsFetching: boolean
  targetCanRedeploy: boolean
  targetConfigFilesValid: boolean
  targetDataRetentionEnabled: boolean
  targetDataVolumes: Parameters<typeof RuntimeDataVolumesEditor>[0]['rows']
  targetHasRuntimeChanges: boolean
  targetImagePrefix: string
  targetRuntimeFilesValid: boolean
  targetSecretFilesValid: boolean
  runtimeClusters: RuntimeCluster[]
  savePending: boolean
  summaries: {
    basic: string
    build: string
    config: string
    data: string
    hooks: string
    kubernetesAdvanced: string
    policy: string
    runtime: string
  }
  onBindRepository: () => void
  onChangeRuntimeConfigMode: (setId: string, mode: RuntimeConfigRefMode) => void
  onDismissRuntimeConfigRestart: () => void
  onEditRuntimeConfigSet: (set?: ProjectRuntimeConfigSet) => void
  onOpenChange: (open: boolean) => void
  onRedeployRuntimeConfigTargets: () => void
  onSave: (values: DeploymentTargetPayload, redeploy: boolean) => void
  onSetConfigFilesValid: (valid: boolean) => void
  onSetBuildSecretRows: (rows: KeyValueRow[]) => void
  onSetBuildVariableRows: (rows: KeyValueRow[]) => void
  onSetHookBindings: (bindings: DeploymentTargetPayload['buildHookBindings']) => void
  onSetSecretFilesValid: (valid: boolean) => void
  onToggleRuntimeConfigSet: (setId: string, checked: boolean) => void
  onUpdateDataVolumes: (rows: Parameters<typeof RuntimeDataVolumesEditor>[0]['rows']) => void
  onUpdateServicePorts: (rows: DeploymentTargetPayload['servicePorts']) => void
}) {
  const { t } = useTranslation()

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        onOpenChange(nextOpen)
        if (!nextOpen)
          form.reset(deploymentTargetDefaults)
      }}
    >
      <DialogContent className="flex max-h-[90vh] max-w-4xl flex-col overflow-hidden p-0">
        <DialogHeader className="border-b border-border px-6 py-4">
          <DialogTitle>{editingTarget ? t('deploymentsPage.editDeploymentTarget') : t('deploymentsPage.createDeploymentTarget')}</DialogTitle>
          <DialogDescription>{t('deploymentsPage.deploymentTargetDialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="flex min-h-0 flex-1 flex-col" onSubmit={form.handleSubmit(values => onSave(values, false))}>
          <div className="grid gap-3 overflow-y-auto px-6 py-4 pb-6">
            <ProgressiveSection
              defaultOpen
              hint={t('deploymentsPage.progressiveBasicDescription')}
              storageKey="luna.deployments.targetDialog.basic"
              summary={summaries.basic}
              title={t('deploymentsPage.progressiveBasicTitle')}
            >
              <div className="grid gap-3 md:grid-cols-2">
                <Field hint={t('deploymentsPage.deploymentConfigNameHint')} label={t('common.name')} required>
                  <Input {...form.register('name', { required: true })} placeholder={t('deploymentsPage.deploymentConfigNamePattern')} />
                </Field>
                <Field label={t('deploymentsPage.stage')}>
                  <Select {...form.register('stage')} disabled={Boolean(editingTarget)}>
                    <option value="dev">{t('deploymentsPage.stageDev')}</option>
                    <option value="test">{t('deploymentsPage.stageTest')}</option>
                    <option value="staging">{t('deploymentsPage.stageStaging')}</option>
                    <option value="prod">{t('deploymentsPage.stageProd')}</option>
                  </Select>
                </Field>
                <Field hint={t('deploymentsPage.runtimeEnvironmentHint')} label={t('clustersPage.runtimeCluster')}>
                  <Select {...form.register('clusterId')}>
                    <option value="">{defaultRuntimeCluster ? t('deploymentsPage.clusterDefaultOption', { name: defaultRuntimeCluster.name }) : t('common.select')}</option>
                    {runtimeClusters.map(cluster => <option key={cluster.id} value={cluster.id}>{cluster.name}</option>)}
                  </Select>
                </Field>
                <Field label={t('common.status')}>
                  <Select {...form.register('enabled')}>
                    <option value="true">{t('common.enabled')}</option>
                    <option value="false">{t('common.disabled')}</option>
                  </Select>
                </Field>
                <ApplicationDeploymentSourceFields
                  registries={registries}
                  repositoryBindings={repositoryBindings}
                  sourceType={sourceType}
                  targetForm={form}
                  onBindRepository={onBindRepository}
                />
                <div className="grid gap-2 md:col-span-2">
                  <ServicePortsEditor ports={servicePorts} onChange={onUpdateServicePorts} />
                </div>
              </div>
            </ProgressiveSection>
            {sourceType === 'repository' && (
              <ProgressiveSection
                hint={t('deploymentsPage.progressiveBuildDescription')}
                storageKey="luna.deployments.targetDialog.build"
                summary={summaries.build}
                title={t('deploymentsPage.progressiveBuildTitle')}
              >
                <ApplicationDeploymentBuildSettingsFields
                  buildContextSuggestions={buildContextSuggestions}
                  buildMinutePriceText={buildMinutePriceText}
                  buildTemplates={buildTemplates}
                  buildTimeoutMinutes={buildTimeoutMinutes}
                  dockerfileExposedPorts={dockerfileExposedPorts}
                  dockerfileSuggestions={dockerfileSuggestions}
                  recommendedTemplateIds={recommendedTemplateIds}
                  sourceType={sourceType}
                  targetForm={form}
                  targetImagePrefix={targetImagePrefix}
                  targetOptionsError={targetBuildOptionsError}
                  targetOptionsFetching={targetBuildOptionsFetching}
                />
                <div className="grid gap-3 border-t border-border pt-3">
                  <div>
                    <h3 className="text-sm font-semibold">{t('deploymentsPage.deploymentBuildEnvironment')}</h3>
                    <p className="mt-1 text-sm text-muted-foreground">{t('deploymentsPage.deploymentBuildEnvironmentDescription')}</p>
                  </div>
                  {buildEnvironmentStatus === 'ready'
                    ? (
                        <>
                          <KeyValueRowsEditor
                            rows={buildVariableRows}
                            title={t('buildsPage.variables')}
                            valuePlaceholder={t('buildsPage.variableValuePlaceholder')}
                            onChange={onSetBuildVariableRows}
                          />
                          <KeyValueRowsEditor
                            secret
                            rows={buildSecretRows}
                            title={t('buildsPage.secrets')}
                            valuePlaceholder={t('buildsPage.secretValuePlaceholder')}
                            onChange={onSetBuildSecretRows}
                          />
                        </>
                      )
                    : <p className="text-sm text-muted-foreground">{t(buildEnvironmentStatus === 'loading' ? 'buildsPage.buildEnvironmentLoading' : 'buildsPage.buildEnvironmentLoadFailed')}</p>}
                </div>
              </ProgressiveSection>
            )}
            <ProgressiveSection
              hint={t('deploymentsPage.progressiveRuntimeConfigDescription')}
              storageKey="luna.deployments.targetDialog.runtime"
              summary={t('deploymentsPage.progressiveRuntimeConfigSummary', { config: summaries.config, runtime: summaries.runtime })}
              title={t('deploymentsPage.runtimeConfig')}
            >
              <RuntimeResourceFields form={form} priceText={runtimeCostText} />
              <div className="grid gap-4 border-t border-border pt-4">
                <ApplicationRuntimeConfigSelector
                  redeployableCount={runtimeConfigRedeployableCount}
                  redeployPending={runtimeConfigRedeployPending}
                  restartAffectedCount={runtimeConfigRestartAffectedCount}
                  selectedRefs={selectedRuntimeConfigRefs}
                  sets={runtimeConfigSets}
                  onCreate={() => onEditRuntimeConfigSet()}
                  onDismissRestart={onDismissRuntimeConfigRestart}
                  onEdit={onEditRuntimeConfigSet}
                  onModeChange={onChangeRuntimeConfigMode}
                  onRedeployAffected={onRedeployRuntimeConfigTargets}
                  onToggle={onToggleRuntimeConfigSet}
                />
              </div>
              <div className="grid gap-3 border-t border-border pt-4">
                <p className="text-sm font-medium text-foreground">{t('deploymentsPage.advancedRuntimeOverrides')}</p>
                <Field hint={t('deploymentsPage.runtimeEnvVarsHint')} label={t('deploymentsPage.runtimeEnvVars')}>
                  <textarea className="min-h-24 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none transition focus-visible:border-primary/60 focus-visible:ring-2 focus-visible:ring-primary/20" {...form.register('envVars')} placeholder={t('deploymentsPage.runtimeEnvVarsPlaceholder')} />
                </Field>
                <Field hint={t('deploymentsPage.runtimeConfigRefsHint')} label={t('deploymentsPage.runtimeConfigRefs')}>
                  <textarea className="min-h-24 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none transition focus-visible:border-primary/60 focus-visible:ring-2 focus-visible:ring-primary/20" {...form.register('configRefs')} placeholder={t('deploymentsPage.runtimeConfigRefsPlaceholder')} />
                </Field>
                <Field hint={t('deploymentsPage.runtimeConfigFilesHint')} label={t('deploymentsPage.runtimeConfigFiles')}>
                  <RuntimeConfigFilesEditor
                    key={`${editingTarget?.id ?? 'new'}-config-files`}
                    initialValue={form.getValues('configFiles') ?? ''}
                    onChange={value => form.setValue('configFiles', value, { shouldDirty: true, shouldValidate: true })}
                    onValidationChange={onSetConfigFilesValid}
                  />
                </Field>
                <Field hint={editingTarget?.secretRefsSet ? t('deploymentsPage.runtimeSecretRefsConfigured') : t('deploymentsPage.runtimeSecretRefsHint')} label={t('deploymentsPage.runtimeSecretRefs')}>
                  <textarea className="min-h-24 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none transition placeholder:text-muted-foreground focus-visible:border-primary/60 focus-visible:ring-2 focus-visible:ring-primary/20" {...form.register('secretRefs')} placeholder={editingTarget?.secretRefsSet ? t('common.secretSetPlaceholder') : t('deploymentsPage.runtimeSecretRefsPlaceholder')} />
                </Field>
                <Field hint={editingTarget?.secretFilesSet ? t('deploymentsPage.runtimeSecretFilesConfigured') : t('deploymentsPage.runtimeSecretFilesHint')} label={t('deploymentsPage.runtimeSecretFiles')}>
                  <RuntimeConfigFilesEditor
                    key={`${editingTarget?.id ?? 'new'}-secret-files`}
                    configuredPlaceholder={editingTarget?.secretFilesSet ? t('common.secretSetPlaceholder') : undefined}
                    initialValue={form.getValues('secretFiles') ?? ''}
                    onChange={value => form.setValue('secretFiles', value, { shouldDirty: true, shouldValidate: true })}
                    onValidationChange={onSetSecretFilesValid}
                  />
                </Field>
              </div>
            </ProgressiveSection>
            <ProgressiveSection
              hint={t('deploymentsPage.progressivePolicyDescription')}
              storageKey="luna.deployments.targetDialog.policy"
              summary={summaries.policy}
              title={t('deploymentsPage.progressivePolicyTitle')}
            >
              <div className="grid gap-3 md:grid-cols-2">
                <Field hint={t('deploymentsPage.branchPatternHint')} label={t('deploymentsPage.branchPattern')}>
                  <Input {...form.register('branchPattern')} placeholder={t('deploymentsPage.branchPatternPlaceholder')} />
                </Field>
                <Field hint={t('deploymentsPage.tagPatternHint')} label={t('deploymentsPage.tagPattern')}>
                  <Input {...form.register('tagPattern')} placeholder={t('deploymentsPage.tagPatternPlaceholder')} />
                </Field>
                <Field hint={t('apps.buildConcurrencyPolicyHint')} label={t('apps.buildConcurrencyPolicy')}>
                  <Select {...form.register('concurrencyPolicy')}>
                    <option value="queue">{t('apps.buildConcurrencyPolicies.queue')}</option>
                    <option value="parallel">{t('apps.buildConcurrencyPolicies.parallel')}</option>
                  </Select>
                </Field>
                <Field label={t('deploymentsPage.autoDeploy')}>
                  <Select {...form.register('autoDeploy')}>
                    <option value="false">{t('common.disabled')}</option>
                    <option value="true">{t('common.enabled')}</option>
                  </Select>
                </Field>
              </div>
            </ProgressiveSection>
            <ProgressiveSection
              hint={t('deploymentsPage.deploymentHooksDescription')}
              storageKey="luna.deployments.targetDialog.hooks"
              summary={summaries.hooks}
              title={t('deploymentsPage.deploymentHooks')}
            >
              <div className="grid gap-4">
                <CheckboxField description={t('deploymentsPage.deploymentHooksEnabledHint')} {...form.register('buildHooksEnabled')}>
                  {t('deploymentsPage.deploymentHooksEnabled')}
                </CheckboxField>
                {hooksError && (
                  <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                    {t('projectHooks.loadFailedDescription')}
                  </p>
                )}
                <ApplicationDeploymentHooksEditor
                  bindings={selectedHookBindings}
                  disabled={!targetBuildHooksEnabled || hooksLoading}
                  hooks={hooks}
                  onChange={onSetHookBindings}
                />
              </div>
            </ProgressiveSection>
            <ProgressiveSection
              hint={t('deploymentsPage.runtimeDataDescription')}
              storageKey="luna.deployments.targetDialog.data"
              summary={summaries.data}
              title={t('deploymentsPage.runtimeData')}
            >
              <div className="grid gap-3">
                <Field hint={t('deploymentsPage.dataRetentionHint')} label={t('deploymentsPage.dataRetention')}>
                  <Select {...form.register('dataRetentionEnabled')}>
                    <option value="false">{t('common.disabled')}</option>
                    <option value="true">{t('common.enabled')}</option>
                  </Select>
                </Field>
                {targetDataRetentionEnabled && (
                  <RuntimeDataVolumesEditor enabled={targetDataRetentionEnabled} rows={targetDataVolumes} onChange={onUpdateDataVolumes} />
                )}
              </div>
            </ProgressiveSection>
            <ProgressiveSection
              hint={t('deploymentsPage.progressiveKubernetesAdvancedDescription')}
              storageKey="luna.deployments.targetDialog.kubernetesAdvanced"
              summary={summaries.kubernetesAdvanced}
              title={t('deploymentsPage.progressiveKubernetesAdvancedTitle')}
            >
              <KubernetesAdvancedFields dataRetentionEnabled={targetDataRetentionEnabled} form={form} />
            </ProgressiveSection>
            {targetHasRuntimeChanges && (
              <div className="flex gap-3 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-amber-950 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-100">
                <Rocket className="mt-0.5 size-4 shrink-0" />
                <div className="grid gap-1 text-sm">
                  <p className="font-medium">{t('deploymentsPage.runtimeChangesNeedRedeployTitle')}</p>
                  <p className="text-amber-900/80 dark:text-amber-100/80">
                    {targetCanRedeploy ? t('deploymentsPage.runtimeChangesNeedRedeployDescription') : t('deploymentsPage.runtimeChangesNeedRedeployUnavailable')}
                  </p>
                </div>
              </div>
            )}
          </div>
          <DialogFooter className="shrink-0 border-t border-border bg-background px-6 py-4">
            {targetHasRuntimeChanges && (
              <Button
                disabled={!targetRuntimeFilesValid || !targetConfigFilesValid || !targetSecretFilesValid || !targetCanRedeploy || savePending}
                type="button"
                variant="secondary"
                onClick={form.handleSubmit(values => onSave(values, true))}
              >
                <Rocket className="size-4" />
                {t('deploymentsPage.saveAndRedeploy')}
              </Button>
            )}
            <Button disabled={!targetRuntimeFilesValid || !targetConfigFilesValid || !targetSecretFilesValid || savePending} type="submit">
              <Save className="size-4" />
              {t('common.save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
