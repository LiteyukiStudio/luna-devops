import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { ArtifactRegistry, BuildConcurrencyPolicy, DeploymentTargetConfigInferenceOutput, RepositoryBinding } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, RotateCw, Sparkles } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { Controller, useFormContext, useFormState, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { FormField } from '@/components/common/form-field'
import { HelpTip } from '@/components/common/help-tip'
import { KeyValueRowsEditor } from '@/components/common/key-value-rows-editor'
import { TargetImageRefInput } from '@/components/common/target-image-ref-input'
import { CpuQuotaInput, MemoryQuotaInput } from '@/components/common/unit-input'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { RadioGroup } from '@/components/ui/radio-group'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { EMPTY_SELECT_VALUE, fromSelectValue, toSelectValue } from '@/components/ui/select-value'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { useBillingDisplay } from '@/lib/billing-display'
import { registryInputPrefix, registryOptionLabel } from '@/pages/applications/application-config-utils'
import { ChoiceCard, EditorGroup } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'
import { buildConcurrencyPolicies, deploymentTargetAdvancedItemFields } from '../deployment-target-form'
import { dockerfileBuildDefaults, repositoryBuildOptionsQuery } from '../deployment-target-queries'
import { AiInferConfigDialog } from './ai-infer-config-dialog'
import { buildAiInferDraftPlan } from './ai-infer-config-utils'
import { ApplicationBuildTemplateFields } from './application-build-template-fields'
import { RepositoryBindingField } from './repository-binding-field'

const repositoryOnlyFields = [
  'repositoryBindingId',
  'targetRegistryId',
  'targetImageRef',
  'buildTemplateId',
  'buildTemplateValues',
  'dockerfilePath',
  'buildContext',
  ...deploymentTargetAdvancedItemFields.build,
] as const

/** 来源与构建 / 镜像与发布分区的主体内容。 */
export function SourceFields({
  applicationId,
  editing,
  followsImageTemplate,
  onExpandAdvanced,
  onTargetImageRefManualChange,
  projectId,
  registries,
  repositoryBindings,
  resolvedTemplateImageRef,
}: {
  applicationId: string
  editing: boolean
  followsImageTemplate: boolean
  onExpandAdvanced?: () => void
  onTargetImageRefManualChange: () => void
  projectId: string
  registries: ArtifactRegistry[]
  repositoryBindings: RepositoryBinding[]
  resolvedTemplateImageRef?: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { clearErrors, control, getValues, register, setValue, trigger } = useFormContext<DeploymentTargetFormValues>()
  const { isSubmitted } = useFormState({ control })
  const [sourceType, repositoryBindingId, registryId, buildDefinitionMode, buildConcurrencyPolicy] = useWatch({
    control,
    name: ['sourceType', 'repositoryBindingId', 'targetRegistryId', 'buildDefinitionMode', 'buildConcurrencyPolicy'],
  })
  const repository = sourceType === 'repository'
  const binding = repositoryBindings.find(item => item.id === repositoryBindingId)
  const buildOptionsQuery = repositoryBuildOptionsQuery(binding)
  const buildOptionsEnabled = repository && Boolean(buildOptionsQuery.enabled)
  const buildOptions = useQuery({ ...buildOptionsQuery, enabled: buildOptionsEnabled })
  const buildTemplates = useQuery({ queryKey: ['build-templates'], queryFn: () => api.listBuildTemplates(), enabled: repository, staleTime: 5 * 60 * 1000 })
  const registry = registries.find(item => item.id === registryId)
  const dockerfiles = buildOptions.data?.dockerfiles ?? []
  const directories = buildOptions.data?.directories ?? []
  const exposedPorts = buildOptions.data?.exposedPorts ?? {}
  const registryError = useFieldError('targetRegistryId')
  const imageRefError = useFieldError('imageRef')
  const targetImageRefError = useFieldError('targetImageRef')
  const dockerfileError = useFieldError('dockerfilePath')
  const buildContextError = useFieldError('buildContext')
  const inferAbortRef = useRef<AbortController | null>(null)
  const [inferredPreview, setInferredPreview] = useState<DeploymentTargetConfigInferenceOutput | null>(null)

  useEffect(() => () => inferAbortRef.current?.abort(), [])

  const inferConfig = useMutation({
    mutationFn: async (targetBinding: RepositoryBinding) => {
      inferAbortRef.current?.abort()
      const controller = new AbortController()
      inferAbortRef.current = controller
      const current = getValues()
      const rawContext = current.buildContext.trim()
      const subpath = rawContext && rawContext !== '.' ? rawContext : undefined
      const result = await api.inferDeploymentTargetConfig(
        projectId,
        applicationId,
        {
          repositoryBindingId: targetBinding.id,
          ref: targetBinding.defaultBranch || undefined,
          subpath,
          stage: current.stage || undefined,
        },
        controller.signal,
      )
      return { appId: applicationId, bindingId: targetBinding.id, prjId: projectId, result }
    },
    onSuccess: ({ appId, bindingId, prjId, result }) => {
      if (
        prjId !== projectId
        || appId !== applicationId
        || getValues('sourceType') !== 'repository'
        || getValues('repositoryBindingId') !== bindingId
      ) {
        return
      }
      setInferredPreview(result)
    },
    onError: (error) => {
      if (error instanceof Error && error.name === 'AbortError')
        return
      toast.error(error instanceof Error ? error.message : t('errors.request.failed'))
    },
  })

  const cancelInFlightInference = () => {
    inferAbortRef.current?.abort()
    inferAbortRef.current = null
    inferConfig.reset()
    setInferredPreview(null)
  }

  const inferPlan = inferredPreview
    ? buildAiInferDraftPlan(getValues(), inferredPreview, t)
    : null

  const applyInferredDraft = () => {
    if (!inferPlan)
      return
    const entries = Object.entries(inferPlan.patch) as Array<[keyof DeploymentTargetFormValues, DeploymentTargetFormValues[keyof DeploymentTargetFormValues]]>
    for (const [key, value] of entries) {
      if (value !== undefined) {
        setValue(key, value as never, {
          shouldDirty: true,
          shouldTouch: key !== 'environmentVariables',
          shouldValidate: false,
        })
      }
    }
    if (isSubmitted)
      void trigger()
    if (inferPlan.needsAdvancedExpand)
      onExpandAdvanced?.()
    setInferredPreview(null)
    toast.success(t('deploymentTargetEditor.aiInfer.appliedToast'))
  }

  const policyOptions: Record<BuildConcurrencyPolicy, { hint: string, label: string }> = {
    queue: { label: t('deploymentTargetEditor.buildConcurrencyPolicies.queue'), hint: t('deploymentTargetEditor.buildConcurrencyPolicyHints.queue') },
    cancel_in_progress: { label: t('deploymentTargetEditor.buildConcurrencyPolicies.cancel_in_progress'), hint: t('deploymentTargetEditor.buildConcurrencyPolicyHints.cancel_in_progress') },
    parallel: { label: t('deploymentTargetEditor.buildConcurrencyPolicies.parallel'), hint: t('deploymentTargetEditor.buildConcurrencyPolicyHints.parallel') },
  }

  const applyDockerfile = (dockerfilePath: string, nextDirectories: string[], nextPorts: Record<string, number[]>, options: { includePath: boolean }) => {
    const defaults = dockerfileBuildDefaults(dockerfilePath, nextDirectories, nextPorts)
    const update = { shouldDirty: true, shouldValidate: true }
    if (options.includePath && defaults.dockerfilePath !== undefined)
      setValue('dockerfilePath', defaults.dockerfilePath, update)
    if (defaults.buildContext !== undefined)
      setValue('buildContext', defaults.buildContext, update)
    if (defaults.buildDirectory !== undefined)
      setValue('buildDirectory', defaults.buildDirectory, update)
    if (defaults.servicePorts)
      setValue('servicePorts', defaults.servicePorts, update)
  }

  const changeRepository = async (nextBinding: RepositoryBinding | undefined) => {
    cancelInFlightInference()
    const currentDockerfile = getValues('dockerfilePath').trim()
    if (editing || !nextBinding || (currentDockerfile && currentDockerfile !== 'Dockerfile'))
      return
    const options = await queryClient.fetchQuery(repositoryBuildOptionsQuery(nextBinding)).catch(() => undefined)
    const detected = options?.dockerfiles?.[0]
    if (detected)
      applyDockerfile(detected, options.directories ?? [], options.exposedPorts ?? {}, { includePath: true })
  }

  return (
    <div className="grid min-w-0 gap-4">
      <Controller
        control={control}
        name="sourceType"
        render={({ field }) => (
          <RadioGroup
            aria-label={t('apps.sourceType')}
            className="grid gap-2 @lg/workspace:grid-cols-2"
            value={field.value}
            onValueChange={(value) => {
              const nextSource = value === 'image' ? 'image' : 'repository'
              cancelInFlightInference()
              field.onChange(nextSource)
              if (nextSource === 'image') {
                clearErrors([...repositoryOnlyFields])
                if (isSubmitted)
                  void trigger(['imageRef'])
              }
              else {
                clearErrors(['imageRef'])
                if (isSubmitted)
                  void trigger([...repositoryOnlyFields])
              }
            }}
          >
            <ChoiceCard description={t('deploymentTargetEditor.sourceRepositoryDescription')} id="deployment-target-source-repository" label={t('apps.repository')} value="repository" />
            <ChoiceCard description={t('deploymentTargetEditor.sourceImageDescription')} id="deployment-target-source-image" label={t('apps.image')} value="image" />
          </RadioGroup>
        )}
      />

      {repository
        ? (
            <>
              <div className="grid gap-4 @xl/workspace:grid-cols-2">
                <div className="@xl/workspace:col-span-2">
                  <RepositoryBindingField
                    applicationId={applicationId}
                    projectId={projectId}
                    repositoryBindings={repositoryBindings}
                    onRepositoryChange={nextBinding => void changeRepository(nextBinding)}
                  />
                </div>
                <FormField error={registryError} label={t('buildsPage.targetRegistry')} required>
                  {fieldControl => (
                    <Controller
                      control={control}
                      name="targetRegistryId"
                      render={({ field }) => (
                        <Select value={field.value || undefined} onValueChange={field.onChange}>
                          <SelectTrigger {...fieldControl} ref={field.ref} className="w-full" onBlur={field.onBlur}>
                            <SelectValue placeholder={registries.length === 0 ? t('common.noOptions') : t('common.select')} />
                          </SelectTrigger>
                          <SelectContent>
                            {registries.length === 0
                              ? <SelectItem disabled value="__empty__">{t('common.noOptions')}</SelectItem>
                              : registries.map(item => <SelectItem key={item.id} value={item.id}>{registryOptionLabel(item)}</SelectItem>)}
                          </SelectContent>
                        </Select>
                      )}
                    />
                  )}
                </FormField>
                <FormField error={targetImageRefError} hint={t('buildsPage.targetImageRefHint')} label={t('buildsPage.targetImageRef')} required>
                  {fieldControl => (
                    <Controller
                      control={control}
                      name="targetImageRef"
                      render={({ field }) => (
                        <TargetImageRefInput
                          {...fieldControl}
                          name={field.name}
                          placeholder={t('buildsPage.targetImageRefPlaceholder')}
                          prefix={registry ? registryInputPrefix(registry) : ''}
                          ref={field.ref}
                          value={followsImageTemplate ? (resolvedTemplateImageRef || field.value) : field.value}
                          onBlur={field.onBlur}
                          onChange={(event) => {
                            onTargetImageRefManualChange()
                            field.onChange(event.target.value)
                          }}
                        />
                      )}
                    />
                  )}
                </FormField>
              </div>

              {buildOptionsEnabled && buildOptions.isLoading && (
                <Skeleton className="h-8 w-full rounded-control" />
              )}
              {buildOptionsEnabled && buildOptions.isError && (
                <div className="flex flex-wrap items-center justify-between gap-2 rounded-control border border-border bg-surface-subtle px-3 py-2 text-xs text-muted-foreground">
                  <span>{t('deploymentsPage.buildOptionsLoadFailed')}</span>
                  <Button disabled={buildOptions.isFetching} size="sm" type="button" variant="outline" onClick={() => void buildOptions.refetch()}>
                    <RotateCw className="size-3.5" />
                    {t('common.retry')}
                  </Button>
                </div>
              )}

              <EditorGroup
                title={t('deploymentTargetEditor.buildTitle')}
                tools={(
                  <Button
                    disabled={!binding || inferConfig.isPending}
                    size="sm"
                    title={!binding ? t('deploymentTargetEditor.aiInfer.disabledHint') : undefined}
                    type="button"
                    variant="outline"
                    onClick={() => {
                      if (binding)
                        inferConfig.mutate(binding)
                    }}
                  >
                    {inferConfig.isPending ? <Loader2 className="size-3.5 animate-spin" /> : <Sparkles className="size-3.5" />}
                    {inferConfig.isPending ? t('deploymentTargetEditor.aiInfer.loading') : t('deploymentTargetEditor.aiInfer.button')}
                  </Button>
                )}
              >
                <div className="grid gap-4 @xl/workspace:grid-cols-2">
                  <ApplicationBuildTemplateFields
                    dockerfileSuggestions={dockerfiles}
                    recommendedTemplateIds={buildOptions.data?.recommendedTemplateIds ?? []}
                    showNoDockerfileNotice={Boolean(binding) && buildOptions.isSuccess && dockerfiles.length === 0}
                    templates={buildTemplates.data ?? []}
                    templatesError={buildTemplates.isError}
                    templatesLoading={buildTemplates.isPending}
                    templatesRetrying={buildTemplates.isFetching}
                    onRetryTemplates={() => void buildTemplates.refetch()}
                  />
                  {buildDefinitionMode === 'repository_dockerfile' && (
                    <FormField error={dockerfileError} hint={t('buildsPage.dockerfileLookupHint')} label={t('buildsPage.dockerfilePath')} required>
                      {fieldControl => (
                        <div className="grid gap-1">
                          <Input
                            {...register('dockerfilePath', { onChange: event => applyDockerfile(event.target.value, directories, exposedPorts, { includePath: false }) })}
                            {...fieldControl}
                            className="font-mono"
                            list="deployment-target-dockerfile-options"
                            placeholder={buildOptionsEnabled && buildOptions.isLoading ? t('common.loading') : t('deploymentsPage.dockerfilePathPlaceholder')}
                          />
                          <datalist id="deployment-target-dockerfile-options">
                            {dockerfiles.map(option => <option key={option} value={option} />)}
                          </datalist>
                          {buildOptionsEnabled && buildOptions.isFetching && <p className="text-xs text-muted-foreground">{t('apps.detectingRepository')}</p>}
                        </div>
                      )}
                    </FormField>
                  )}
                  <FormField error={buildContextError} hint={t('buildsPage.buildContextLookupHint')} label={t('buildsPage.buildContext')} required>
                    {fieldControl => (
                      <div>
                        <Input {...register('buildContext')} {...fieldControl} className="font-mono" list="deployment-target-build-context-options" placeholder={t('deploymentsPage.buildContextPlaceholder')} />
                        <datalist id="deployment-target-build-context-options">
                          {directories.map(option => <option key={option} value={option} />)}
                        </datalist>
                      </div>
                    )}
                  </FormField>
                </div>
              </EditorGroup>

              <EditorGroup title={t('deploymentTargetEditor.releaseTriggerTitle')}>
                <div className="grid gap-4 @xl/workspace:grid-cols-2">
                  <FormField hint={policyOptions[buildConcurrencyPolicy].hint} label={t('deploymentTargetEditor.buildConcurrencyPolicy')}>
                    {fieldControl => (
                      <Controller
                        control={control}
                        name="buildConcurrencyPolicy"
                        render={({ field }) => (
                          <Select value={field.value} onValueChange={field.onChange}>
                            <SelectTrigger {...fieldControl} ref={field.ref} className="w-full" onBlur={field.onBlur}>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {buildConcurrencyPolicies.map(policy => <SelectItem key={policy} value={policy}>{policyOptions[policy].label}</SelectItem>)}
                            </SelectContent>
                          </Select>
                        )}
                      />
                    )}
                  </FormField>
                  <ImagePullPolicyField />
                  <FormField hint={t('deploymentsPage.branchPatternHint')} label={t('deploymentsPage.branchPattern')}>
                    <Input {...register('branchPattern')} className="font-mono" placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.branchPatternPlaceholder') })} />
                  </FormField>
                  <FormField hint={t('deploymentsPage.tagPatternHint')} label={t('deploymentsPage.tagPattern')}>
                    <Input {...register('tagPattern')} className="font-mono" placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.tagPatternPlaceholder') })} />
                  </FormField>
                </div>
              </EditorGroup>
            </>
          )
        : (
            <div className="grid gap-4 @xl/workspace:grid-cols-2">
              <FormField error={imageRefError} hint={t('apps.imageReferenceHint')} label={t('apps.imageReference')} required>
                <Input {...register('imageRef')} autoComplete="off" className="font-mono" placeholder={t('apps.imageReferencePlaceholder')} />
              </FormField>
              <ImagePullPolicyField />
            </div>
          )}
      <AiInferConfigDialog
        inferred={inferredPreview}
        open={Boolean(inferredPreview)}
        plan={inferPlan}
        onApply={applyInferredDraft}
        onOpenChange={(open) => {
          if (!open)
            setInferredPreview(null)
        }}
      />
    </div>
  )
}

function ImagePullPolicyField() {
  const { t } = useTranslation()
  const { control } = useFormContext<DeploymentTargetFormValues>()
  return (
    <FormField hint={t('deploymentsPage.imagePullPolicyHint')} label={t('deploymentsPage.imagePullPolicy')}>
      {fieldControl => (
        <Controller
          control={control}
          name="imagePullPolicy"
          render={({ field }) => (
            <Select
              value={toSelectValue(field.value)}
              onValueChange={next => field.onChange(fromSelectValue(next))}
            >
              <SelectTrigger {...fieldControl} ref={field.ref} className="w-full" onBlur={field.onBlur}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={EMPTY_SELECT_VALUE}>{t('deploymentsPage.kubernetesDefault')}</SelectItem>
                <SelectItem value="IfNotPresent">{t('deploymentsPage.kubernetesValues.IfNotPresent')}</SelectItem>
                <SelectItem value="Always">{t('deploymentsPage.kubernetesValues.Always')}</SelectItem>
                <SelectItem value="Never">{t('deploymentsPage.kubernetesValues.Never')}</SelectItem>
              </SelectContent>
            </Select>
          )}
        />
      )}
    </FormField>
  )
}

/** 高级分区中的构建选项：构建目录、参数、构建规格、超时和部署级构建变量。 */
export function BuildAdvancedFields({
  buildEnvironmentAvailable,
  buildEnvironmentRetrying,
  onRetryBuildEnvironment,
  repositoryBindings,
}: {
  buildEnvironmentAvailable: boolean
  buildEnvironmentRetrying?: boolean
  onRetryBuildEnvironment?: () => void
  repositoryBindings: RepositoryBinding[]
}) {
  const { i18n, t } = useTranslation()
  const billing = useBillingDisplay(i18n.language)
  const { control, register } = useFormContext<DeploymentTargetFormValues>()
  const [repositoryBindingId, buildCpuRequest, buildMemoryRequest] = useWatch({ control, name: ['repositoryBindingId', 'buildCpuRequest', 'buildMemoryRequest'] })
  const buildOptionsQuery = repositoryBuildOptionsQuery(repositoryBindings.find(item => item.id === repositoryBindingId))
  const buildOptions = useQuery(buildOptionsQuery)
  const directories = buildOptions.data?.directories ?? []
  const buildCpuError = useFieldError('buildCpuRequest')
  const buildMemoryError = useFieldError('buildMemoryRequest')
  const buildTimeoutError = useFieldError('buildTimeoutMinutes')

  return (
    <>
      <div className="grid gap-4 @xl/workspace:grid-cols-3">
        <FormField error={buildCpuError} label={t('deploymentsPage.buildCpuRequest')} required>
          {fieldControl => (
            <Controller
              control={control}
              name="buildCpuRequest"
              render={({ field }) => <CpuQuotaInput inputProps={fieldControl} value={field.value} onChange={field.onChange} />}
            />
          )}
        </FormField>
        <FormField error={buildMemoryError} label={t('deploymentsPage.buildMemoryRequest')} required>
          {fieldControl => (
            <Controller
              control={control}
              name="buildMemoryRequest"
              render={({ field }) => <MemoryQuotaInput inputProps={fieldControl} value={field.value} onChange={field.onChange} />}
            />
          )}
        </FormField>
        <FormField error={buildTimeoutError} hint={t('deploymentsPage.buildTimeoutHint')} label={t('deploymentsPage.buildTimeoutMinutes')} required>
          <Input {...register('buildTimeoutMinutes', { valueAsNumber: true })} className="font-mono tabular-nums" inputMode="numeric" max={1440} min={1} type="number" />
        </FormField>
      </div>
      <p className="text-xs text-muted-foreground">
        {t('deploymentsPage.buildEstimatedPrice', { price: billing.formatAmountWithUnit(billing.buildMinuteCost(buildCpuRequest, buildMemoryRequest)) })}
      </p>
      <div className="grid gap-4 @xl/workspace:grid-cols-2">
        <FormField hint={t('buildsPage.buildDirectoryHint')} label={t('buildsPage.buildDirectory')}>
          {fieldControl => (
            <div className="grid gap-1">
              <Input {...register('buildDirectory')} {...fieldControl} className="font-mono" list="deployment-target-build-directory-options" placeholder={t('buildsPage.buildDirectoryPlaceholder')} />
              <datalist id="deployment-target-build-directory-options">
                {directories.filter(option => option !== '.').map(option => <option key={option} value={option} />)}
              </datalist>
              {buildOptions.isError && (
                <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                  <span>{t('deploymentsPage.buildOptionsLoadFailed')}</span>
                  <Button disabled={buildOptions.isFetching} size="sm" type="button" variant="ghost" onClick={() => void buildOptions.refetch()}>
                    <RotateCw className="size-3.5" />
                    {t('common.retry')}
                  </Button>
                </div>
              )}
            </div>
          )}
        </FormField>
        <FormField hint={t('buildsPage.buildArgsHint')} label={t('buildsPage.buildArgs')}>
          <Textarea {...register('buildArgs')} className="min-h-20 font-mono text-sm" placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('buildsPage.buildArgsPlaceholder') })} />
        </FormField>
      </div>
      <div className="grid gap-3">
        <h4 className="inline-flex items-center gap-1.5 text-sm font-medium">
          {t('deploymentsPage.deploymentBuildEnvironment')}
          <HelpTip label={t('deploymentsPage.deploymentBuildEnvironment')}>{t('deploymentsPage.deploymentBuildEnvironmentDescription')}</HelpTip>
        </h4>
        {buildEnvironmentAvailable
          ? (
              <div className="grid gap-3 @2xl/workspace:grid-cols-2">
                <Controller
                  control={control}
                  name="buildVariables"
                  render={({ field }) => (
                    <KeyValueRowsEditor rows={field.value} title={t('buildsPage.variables')} valuePlaceholder={t('buildsPage.variableValuePlaceholder')} onChange={field.onChange} />
                  )}
                />
                <Controller
                  control={control}
                  name="buildSecrets"
                  render={({ field }) => (
                    <KeyValueRowsEditor secret rows={field.value} title={t('buildsPage.secrets')} valuePlaceholder={t('buildsPage.secretValuePlaceholder')} onChange={field.onChange} />
                  )}
                />
              </div>
            )
          : (
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-control border border-border bg-surface-subtle px-3 py-2 text-sm text-muted-foreground">
                <span>{t('buildsPage.buildEnvironmentLoadFailed')}</span>
                {onRetryBuildEnvironment && (
                  <Button disabled={buildEnvironmentRetrying} size="sm" type="button" variant="outline" onClick={onRetryBuildEnvironment}>
                    <RotateCw className="size-3.5" />
                    {t('common.retry')}
                  </Button>
                )}
              </div>
            )}
      </div>
    </>
  )
}
