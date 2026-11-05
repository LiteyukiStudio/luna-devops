import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { ArtifactRegistry, RepositoryBinding } from '@/api'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Box, GitBranch } from 'lucide-react'
import { Controller, useFormContext, useFormState, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { FormField } from '@/components/common/form-field'
import { KeyValueRowsEditor } from '@/components/common/key-value-rows-editor'
import { TargetImageRefInput } from '@/components/common/target-image-ref-input'
import { UnitInput } from '@/components/common/unit-input'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { RadioGroup } from '@/components/ui/radio-group'
import { Textarea } from '@/components/ui/textarea'
import { useBillingDisplay } from '@/lib/billing-display'
import { registryInputPrefix, registryOptionLabel } from '@/pages/applications/application-config-utils'
import { ChoiceCard, EditorGroup } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'
import { deploymentTargetAdvancedItemFields, deploymentTargetSectionFields } from '../deployment-target-form'
import { dockerfileBuildDefaults, repositoryBuildOptionsQuery, targetImageTemplateQuery } from '../deployment-target-queries'
import { cpuUnits, memoryUnits } from '../resource-units'
import { ApplicationBuildTemplateFields } from './application-build-template-fields'
import { RepositoryBindingField } from './repository-binding-field'

// 来源类型决定这些字段是否必填。
const sourceDependentFields = [...deploymentTargetSectionFields.basics, ...deploymentTargetAdvancedItemFields.build]

/** 来源与构建方式：基本信息分区中的常用部分；构建目录、参数、规格与变量收在高级分区。 */
export function SourceFields({ applicationId, editing, projectId, registries, repositoryBindings }: {
  applicationId: string
  editing: boolean
  projectId: string
  registries: ArtifactRegistry[]
  repositoryBindings: RepositoryBinding[]
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { control, getValues, register, setValue, trigger } = useFormContext<DeploymentTargetFormValues>()
  const { dirtyFields, isSubmitted } = useFormState({ control, name: 'targetImageRef' })
  const [sourceType, repositoryBindingId, registryId, buildDefinitionMode, stage, targetName] = useWatch({
    control,
    name: ['sourceType', 'repositoryBindingId', 'targetRegistryId', 'buildDefinitionMode', 'stage', 'name'],
  })
  const repository = sourceType === 'repository'
  const binding = repositoryBindings.find(item => item.id === repositoryBindingId)
  const buildOptionsQuery = repositoryBuildOptionsQuery(binding)
  const buildOptions = useQuery({ ...buildOptionsQuery, enabled: repository && Boolean(buildOptionsQuery.enabled) })
  const buildTemplates = useQuery({ queryKey: ['build-templates'], queryFn: () => api.listBuildTemplates(), enabled: repository, staleTime: 5 * 60 * 1000 })
  // 新建时目标镜像跟随镜像站模板，直到用户手动修改；展示值在渲染中派生，提交时同样以模板为准。
  const followsImageTemplate = !editing && repository && !dirtyFields.targetImageRef
  const imageTemplate = useQuery({
    ...targetImageTemplateQuery({ applicationId, projectId, registryId, stage, targetName }),
    enabled: followsImageTemplate && Boolean(registryId),
  })
  const registry = registries.find(item => item.id === registryId)
  const dockerfiles = buildOptions.data?.dockerfiles ?? []
  const directories = buildOptions.data?.directories ?? []
  const exposedPorts = buildOptions.data?.exposedPorts ?? {}
  const registryError = useFieldError('targetRegistryId')
  const imageRefError = useFieldError('imageRef')
  const targetImageRefError = useFieldError('targetImageRef')
  const dockerfileError = useFieldError('dockerfilePath')
  const buildContextError = useFieldError('buildContext')

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
    const currentDockerfile = getValues('dockerfilePath').trim()
    if (editing || !nextBinding || (currentDockerfile && currentDockerfile !== 'Dockerfile'))
      return
    const options = await queryClient.fetchQuery(repositoryBuildOptionsQuery(nextBinding)).catch(() => undefined)
    const detected = options?.dockerfiles?.[0]
    if (detected)
      applyDockerfile(detected, options.directories ?? [], options.exposedPorts ?? {}, { includePath: true })
  }

  return (
    <>
      <EditorGroup description={t('apps.sourceTypeHint')} title={t('apps.sourceType')}>
        <Controller
          control={control}
          name="sourceType"
          render={({ field }) => (
            <RadioGroup
              aria-label={t('apps.sourceType')}
              className="grid gap-2 sm:grid-cols-2"
              value={field.value}
              onValueChange={(value) => {
                field.onChange(value === 'image' ? 'image' : 'repository')
                // 来源类型决定哪些字段必填；提交过后切换来源要立即刷新该分区的校验结果。
                if (isSubmitted)
                  void trigger(sourceDependentFields)
              }}
            >
              <ChoiceCard description={t('deploymentTargetEditor.sourceRepositoryDescription')} icon={<GitBranch className="size-4" />} id="deployment-target-source-repository" label={t('apps.repository')} value="repository" />
              <ChoiceCard description={t('deploymentTargetEditor.sourceImageDescription')} icon={<Box className="size-4" />} id="deployment-target-source-image" label={t('apps.image')} value="image" />
            </RadioGroup>
          )}
        />
        {repository
          ? (
              <div className="grid gap-4 md:grid-cols-2">
                <div className="md:col-span-2">
                  <RepositoryBindingField
                    applicationId={applicationId}
                    projectId={projectId}
                    repositoryBindings={repositoryBindings}
                    onRepositoryChange={nextBinding => void changeRepository(nextBinding)}
                  />
                </div>
                <FormField error={registryError} label={t('buildsPage.targetRegistry')} required>
                  <NativeSelect {...register('targetRegistryId')}>
                    <option value="">{t('common.select')}</option>
                    {registries.map(item => <option key={item.id} value={item.id}>{registryOptionLabel(item)}</option>)}
                  </NativeSelect>
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
                          value={followsImageTemplate ? (imageTemplate.data?.targetImageRef || field.value) : field.value}
                          onBlur={field.onBlur}
                          onChange={field.onChange}
                        />
                      )}
                    />
                  )}
                </FormField>
              </div>
            )
          : (
              <FormField error={imageRefError} hint={t('apps.imageReferenceHint')} label={t('apps.imageReference')} required>
                <Input {...register('imageRef')} autoComplete="off" placeholder={t('apps.imageReferencePlaceholder')} />
              </FormField>
            )}
        {repository && buildOptions.isError && (
          <p className="text-xs text-muted-foreground">{t('deploymentsPage.buildOptionsLoadFailed')}</p>
        )}
      </EditorGroup>

      {repository && (
        <EditorGroup description={t('deploymentTargetEditor.buildDescription')} title={t('deploymentTargetEditor.buildTitle')}>
          <div className="grid gap-4 md:grid-cols-2">
            <ApplicationBuildTemplateFields
              dockerfileSuggestions={dockerfiles}
              recommendedTemplateIds={buildOptions.data?.recommendedTemplateIds ?? []}
              templates={buildTemplates.data ?? []}
            />
            {buildDefinitionMode === 'repository_dockerfile' && (
              <FormField error={dockerfileError} hint={t('buildsPage.dockerfileLookupHint')} label={t('buildsPage.dockerfilePath')} required>
                {fieldControl => (
                  <div className="grid gap-1">
                    <Input
                      {...register('dockerfilePath', { onChange: event => applyDockerfile(event.target.value, directories, exposedPorts, { includePath: false }) })}
                      {...fieldControl}
                      list="deployment-target-dockerfile-options"
                      placeholder={t('deploymentsPage.dockerfilePathPlaceholder')}
                    />
                    <datalist id="deployment-target-dockerfile-options">
                      {dockerfiles.map(option => <option key={option} value={option} />)}
                    </datalist>
                    {buildOptions.isFetching && <p className="text-xs text-muted-foreground">{t('apps.detectingRepository')}</p>}
                  </div>
                )}
              </FormField>
            )}
            <FormField error={buildContextError} hint={t('buildsPage.buildContextLookupHint')} label={t('buildsPage.buildContext')} required>
              {fieldControl => (
                <div>
                  <Input {...register('buildContext')} {...fieldControl} list="deployment-target-build-context-options" placeholder={t('deploymentsPage.buildContextPlaceholder')} />
                  <datalist id="deployment-target-build-context-options">
                    {directories.map(option => <option key={option} value={option} />)}
                  </datalist>
                </div>
              )}
            </FormField>
          </div>
        </EditorGroup>
      )}
    </>
  )
}

/** 高级分区中的构建选项：构建目录、参数、构建规格、超时和部署级构建变量。 */
export function BuildAdvancedFields({ buildEnvironmentAvailable, repositoryBindings }: {
  buildEnvironmentAvailable: boolean
  repositoryBindings: RepositoryBinding[]
}) {
  const { i18n, t } = useTranslation()
  const billing = useBillingDisplay(i18n.language)
  const { control, register } = useFormContext<DeploymentTargetFormValues>()
  const [repositoryBindingId, buildCpuRequest, buildMemoryRequest] = useWatch({ control, name: ['repositoryBindingId', 'buildCpuRequest', 'buildMemoryRequest'] })
  const buildOptionsQuery = repositoryBuildOptionsQuery(repositoryBindings.find(item => item.id === repositoryBindingId))
  const buildOptions = useQuery(buildOptionsQuery)
  const directories = buildOptions.data?.directories ?? []
  const buildTimeoutError = useFieldError('buildTimeoutMinutes')

  return (
    <>
      <div className="grid gap-4 md:grid-cols-2">
        <FormField hint={t('buildsPage.buildDirectoryHint')} label={t('buildsPage.buildDirectory')}>
          {fieldControl => (
            <div>
              <Input {...register('buildDirectory')} {...fieldControl} list="deployment-target-build-directory-options" placeholder={t('buildsPage.buildDirectoryPlaceholder')} />
              <datalist id="deployment-target-build-directory-options">
                {directories.filter(option => option !== '.').map(option => <option key={option} value={option} />)}
              </datalist>
            </div>
          )}
        </FormField>
        <FormField hint={t('buildsPage.buildArgsHint')} label={t('buildsPage.buildArgs')}>
          <Textarea {...register('buildArgs')} className="min-h-20 font-mono text-sm" placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('buildsPage.buildArgsPlaceholder') })} />
        </FormField>
      </div>
      <div className="grid gap-4 md:grid-cols-3">
        <FormField label={t('deploymentsPage.buildCpuRequest')} required>
          {fieldControl => (
            <Controller
              control={control}
              name="buildCpuRequest"
              render={({ field }) => <UnitInput unitSelectLabel={t('deploymentTargetEditor.cpuUnitLabel')} units={cpuUnits(t)} inputProps={fieldControl} value={field.value} onChange={field.onChange} />}
            />
          )}
        </FormField>
        <FormField label={t('deploymentsPage.buildMemoryRequest')} required>
          {fieldControl => (
            <Controller
              control={control}
              name="buildMemoryRequest"
              render={({ field }) => <UnitInput unitSelectLabel={t('deploymentTargetEditor.memoryUnitLabel')} units={memoryUnits} inputProps={fieldControl} value={field.value} onChange={field.onChange} />}
            />
          )}
        </FormField>
        <FormField error={buildTimeoutError} hint={t('deploymentsPage.buildTimeoutHint')} label={t('deploymentsPage.buildTimeoutMinutes')} required>
          <Input {...register('buildTimeoutMinutes', { valueAsNumber: true })} inputMode="numeric" max={1440} min={1} type="number" />
        </FormField>
      </div>
      <p className="text-xs text-muted-foreground">
        {t('deploymentsPage.buildEstimatedPrice', { price: billing.formatAmountWithUnit(billing.buildMinuteCost(buildCpuRequest, buildMemoryRequest)) })}
      </p>
      <div className="grid gap-3">
        <div>
          <h4 className="text-sm font-medium">{t('deploymentsPage.deploymentBuildEnvironment')}</h4>
          <p className="mt-1 text-xs text-muted-foreground">{t('deploymentsPage.deploymentBuildEnvironmentDescription')}</p>
        </div>
        {buildEnvironmentAvailable
          ? (
              <div className="grid gap-3 lg:grid-cols-2">
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
          : <p className="text-sm text-muted-foreground">{t('buildsPage.buildEnvironmentLoadFailed')}</p>}
      </div>
    </>
  )
}
