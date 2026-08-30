import type { UseFormReturn } from 'react-hook-form'
import type { ArtifactRegistry, BuildTemplate, DeploymentTargetPayload, RepositoryBinding } from '@/api'
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FormField as Field } from '@/components/common/form-field'
import { TargetImageRefInput } from '@/components/common/target-image-ref-input'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { registryOptionLabel } from '@/pages/applications/application-config-utils'
import { applyDockerfileBuildDefaults } from '@/pages/applications/deployments/application-deployments-panel-utils'
import { BuildEnvironmentFields } from '@/pages/applications/deployments/editor/runtime/application-deployment-resource-fields'
import { ApplicationBuildTemplateFields } from './application-build-template-fields'

interface ApplicationDeploymentSourceFieldsProps {
  registries: ArtifactRegistry[]
  repositoryBindings: RepositoryBinding[]
  sourceType: DeploymentTargetPayload['sourceType']
  targetForm: UseFormReturn<DeploymentTargetPayload>
  onBindRepository: () => void
}

export function ApplicationDeploymentSourceFields({
  onBindRepository,
  registries,
  repositoryBindings,
  sourceType,
  targetForm,
}: ApplicationDeploymentSourceFieldsProps) {
  const { t } = useTranslation()

  return (
    <>
      <div className="grid gap-3 md:col-span-2 md:grid-cols-2">
        <Field hint={t('apps.sourceTypeHint')} label={t('apps.sourceType')} required>
          <Select {...targetForm.register('sourceType', { required: true })}>
            <option value="repository">{t('apps.repository')}</option>
            <option value="image">{t('apps.image')}</option>
          </Select>
        </Field>
        {sourceType === 'repository' && (
          <Field label={t('apps.repository')} required>
            <div className="flex flex-col gap-2 sm:flex-row">
              <Select containerClassName="min-w-0 flex-1" {...targetForm.register('repositoryBindingId', { required: sourceType === 'repository' })}>
                <option value="">{t('common.select')}</option>
                {repositoryBindings.map(binding => (
                  <option key={binding.id} value={binding.id}>
                    {binding.owner}
                    /
                    {binding.repo}
                  </option>
                ))}
              </Select>
              <Button className="shrink-0" type="button" variant="secondary" onClick={onBindRepository}>
                <Plus className="size-4" />
                {t('deploymentsPage.bindRepositoryInTarget')}
              </Button>
            </div>
          </Field>
        )}
        {sourceType === 'repository' && (
          <Field label={t('buildsPage.targetRegistry')} required>
            <Select {...targetForm.register('targetRegistryId', { required: sourceType === 'repository' })}>
              <option value="">{t('common.select')}</option>
              {registries.map(registry => <option key={registry.id} value={registry.id}>{registryOptionLabel(registry)}</option>)}
            </Select>
          </Field>
        )}
        {sourceType === 'image' && (
          <Field hint={t('apps.imageReferenceHint')} label={t('apps.imageReference')} required>
            <Input {...targetForm.register('imageRef', { required: sourceType === 'image' })} placeholder={t('apps.imageReferencePlaceholder')} />
          </Field>
        )}
      </div>
    </>
  )
}

interface ApplicationDeploymentBuildSettingsFieldsProps {
  buildContextSuggestions: string[]
  buildTemplates: BuildTemplate[]
  buildMinutePriceText: string
  buildTimeoutMinutes: number
  dockerfileExposedPorts: Record<string, number[]>
  dockerfileSuggestions: string[]
  recommendedTemplateIds: string[]
  sourceType: DeploymentTargetPayload['sourceType']
  targetForm: UseFormReturn<DeploymentTargetPayload>
  targetImagePrefix: string
  targetOptionsError: boolean
  targetOptionsFetching: boolean
}

export function ApplicationDeploymentBuildSettingsFields({
  buildContextSuggestions,
  buildTemplates,
  buildMinutePriceText,
  buildTimeoutMinutes,
  dockerfileExposedPorts,
  dockerfileSuggestions,
  recommendedTemplateIds,
  sourceType,
  targetForm,
  targetImagePrefix,
  targetOptionsError,
  targetOptionsFetching,
}: ApplicationDeploymentBuildSettingsFieldsProps) {
  const { t } = useTranslation()
  const buildDirectorySuggestions = buildContextSuggestions.filter(option => option !== '.')
  const buildDefinitionMode = targetForm.watch('buildDefinitionMode') || 'repository_dockerfile'
  const dockerfilePathField = targetForm.register('dockerfilePath', { required: sourceType === 'repository' })

  if (sourceType !== 'repository')
    return null

  return (
    <div className="grid gap-4">
      <div className="grid gap-3 md:grid-cols-2">
        <ApplicationBuildTemplateFields
          dockerfileSuggestions={dockerfileSuggestions}
          form={targetForm}
          recommendedTemplateIds={recommendedTemplateIds}
          templates={buildTemplates}
        />
        {buildDefinitionMode === 'repository_dockerfile' && (
          <Field hint={t('buildsPage.dockerfileLookupHint')} label={t('buildsPage.dockerfilePath')} required>
            <Input
              {...dockerfilePathField}
              list="deployment-target-dockerfile-options"
              placeholder={t('deploymentsPage.dockerfilePathPlaceholder')}
              onChange={(event) => {
                dockerfilePathField.onChange(event)
                applyDockerfileBuildDefaults(targetForm, event.target.value, buildContextSuggestions, dockerfileExposedPorts)
              }}
            />
            <datalist id="deployment-target-dockerfile-options">
              {dockerfileSuggestions.map(option => <option key={option} value={option} />)}
            </datalist>
            {targetOptionsFetching && <p className="mt-1 text-xs text-muted-foreground">{t('apps.detectingRepository')}</p>}
            {targetOptionsError && <p className="mt-1 text-xs text-destructive">{t('deploymentsPage.buildOptionsLoadFailed')}</p>}
          </Field>
        )}
        <Field hint={t('buildsPage.buildContextLookupHint')} label={t('buildsPage.buildContext')} required>
          <Input {...targetForm.register('buildContext', { required: true })} list="deployment-target-build-context-options" placeholder={t('deploymentsPage.buildContextPlaceholder')} />
          <datalist id="deployment-target-build-context-options">
            {buildContextSuggestions.map(option => <option key={option} value={option} />)}
          </datalist>
        </Field>
        <Field hint={t('buildsPage.buildDirectoryHint')} label={t('buildsPage.buildDirectory')}>
          <Input {...targetForm.register('buildDirectory')} list="deployment-target-build-directory-options" placeholder={t('buildsPage.buildDirectoryPlaceholder')} />
          <datalist id="deployment-target-build-directory-options">
            {buildDirectorySuggestions.map(option => <option key={option} value={option} />)}
          </datalist>
        </Field>
        <Field hint={t('buildsPage.buildArgsHint')} label={t('buildsPage.buildArgs')}>
          <Textarea className="min-h-24 font-mono" {...targetForm.register('buildArgs')} placeholder={t('buildsPage.buildArgsPlaceholder')} />
        </Field>
        <Field hint={t('buildsPage.targetImageRefHint')} label={t('buildsPage.targetImageRef')} required>
          <TargetImageRefInput
            placeholder={t('buildsPage.targetImageRefPlaceholder')}
            prefix={targetImagePrefix}
            register={targetForm.register('targetImageRef', { required: true })}
          />
        </Field>
      </div>
      <BuildEnvironmentFields buildTimeoutMinutes={buildTimeoutMinutes} form={targetForm} priceText={buildMinutePriceText} />
    </div>
  )
}
