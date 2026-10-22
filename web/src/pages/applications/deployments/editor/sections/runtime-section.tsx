import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { DeploymentTarget, Release, RuntimeCluster } from '@/api'
import { Controller, useFormContext, useFormState, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { AdvancedOptions, EditorGroup, EditorSection, JsonField } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'
import { changedFieldCount } from '../deployment-target-form'
import { RuntimeDataVolumesEditor } from '../runtime/application-deployment-data-volumes-editor'
import { RuntimeEnvironmentFields } from '../runtime/runtime-environment-fields'
import { RuntimeScaleFields } from '../runtime/runtime-scale-fields'
import { ServicePortsFields } from '../runtime/service-ports-fields'

const containerFields = ['imagePullPolicy', 'containerCommand', 'containerArgs', 'lifecycle', 'readinessProbe', 'livenessProbe', 'startupProbe'] as const

export function RuntimeSection({ applicationId, canManageRuntimeSecrets, deploymentTargets, projectId, releases, runtimeClusters, target }: {
  applicationId: string
  canManageRuntimeSecrets: boolean
  deploymentTargets: DeploymentTarget[]
  projectId: string
  releases: Release[]
  runtimeClusters: RuntimeCluster[]
  target: DeploymentTarget | null
}) {
  const { t } = useTranslation()

  return (
    <EditorSection description={t('deploymentTargetEditor.sections.runtimeDescription')} section="runtime" title={t('deploymentTargetEditor.sections.runtime')}>
      <RuntimeScaleFields />
      <ServicePortsFields />
      <RuntimeEnvironmentFields
        applicationId={applicationId}
        canManageRuntimeSecrets={canManageRuntimeSecrets}
        deploymentTargets={deploymentTargets}
        projectId={projectId}
        releases={releases}
        target={target}
      />
      <RuntimeStorageFields projectId={projectId} runtimeClusters={runtimeClusters} />
      <ContainerFields />
    </EditorSection>
  )
}

function RuntimeStorageFields({ projectId, runtimeClusters }: { projectId: string, runtimeClusters: RuntimeCluster[] }) {
  const { t } = useTranslation()
  const { control, register } = useFormContext<DeploymentTargetFormValues>()
  const clusterId = useWatch({ control, name: 'clusterId' })
  const { errors } = useFormState({ control, name: 'dataVolumes' })
  const effectiveClusterId = clusterId || runtimeClusters.find(cluster => cluster.isDefault)?.id || runtimeClusters[0]?.id || ''
  const rowErrors = Array.isArray(errors.dataVolumes)
    ? errors.dataVolumes.map(error => typeof error?.message === 'string' ? error.message : undefined)
    : []

  return (
    <EditorGroup description={t('deploymentsPage.runtimeDataDescription')} title={t('deploymentTargetEditor.storageTitle')}>
      <div className="grid gap-4 md:grid-cols-2">
        <FormField hint={t('deploymentsPage.workloadTypeHint')} label={t('deploymentsPage.workloadType')}>
          <NativeSelect {...register('workloadType')}>
            <option value="Deployment">{t('deploymentsPage.kubernetesValues.Deployment')}</option>
            <option value="StatefulSet">{t('deploymentsPage.kubernetesValues.StatefulSet')}</option>
          </NativeSelect>
        </FormField>
      </div>
      <div className="grid gap-2">
        <h4 className="text-sm font-medium text-foreground">{t('deploymentsPage.dataVolumes')}</h4>
        <p className="text-xs leading-5 text-muted-foreground">{t('deploymentsPage.dataVolumesHint')}</p>
        <Controller
          control={control}
          name="dataVolumes"
          render={({ field }) => (
            <RuntimeDataVolumesEditor
              clusterId={effectiveClusterId}
              projectId={projectId}
              rowErrors={rowErrors}
              rows={field.value}
              runtimeClusters={runtimeClusters}
              onChange={field.onChange}
            />
          )}
        />
      </div>
    </EditorGroup>
  )
}

function ContainerFields() {
  const { t } = useTranslation()
  const { control, register } = useFormContext<DeploymentTargetFormValues>()
  const values = useWatch({ control, name: [...containerFields] })
  const commandError = useFieldError('containerCommand')
  const argsError = useFieldError('containerArgs')
  const configuredCount = changedFieldCount(Object.fromEntries(containerFields.map((field, index) => [field, values[index]])), containerFields)

  return (
    <EditorGroup description={t('deploymentTargetEditor.containerDescription')} title={t('deploymentTargetEditor.containerTitle')}>
      <AdvancedOptions configuredCount={configuredCount} fields={containerFields} title={t('deploymentTargetEditor.containerAdvanced')}>
        <div className="grid gap-4 md:grid-cols-2">
          <FormField hint={t('deploymentsPage.imagePullPolicyHint')} label={t('deploymentsPage.imagePullPolicy')}>
            <NativeSelect {...register('imagePullPolicy')}>
              <option value="">{t('deploymentsPage.kubernetesDefault')}</option>
              <option value="IfNotPresent">{t('deploymentsPage.kubernetesValues.IfNotPresent')}</option>
              <option value="Always">{t('deploymentsPage.kubernetesValues.Always')}</option>
              <option value="Never">{t('deploymentsPage.kubernetesValues.Never')}</option>
            </NativeSelect>
          </FormField>
          <div className="hidden md:block" />
          <FormField error={commandError} hint={t('deploymentsPage.containerCommandHint')} label={t('deploymentsPage.containerCommand')}>
            <Textarea {...register('containerCommand')} className="min-h-20 font-mono text-sm" placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.containerCommandPlaceholder') })} spellCheck={false} />
          </FormField>
          <FormField error={argsError} hint={t('deploymentsPage.containerArgsHint')} label={t('deploymentsPage.containerArgs')}>
            <Textarea {...register('containerArgs')} className="min-h-20 font-mono text-sm" placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.containerArgsPlaceholder') })} spellCheck={false} />
          </FormField>
        </div>
        <JsonField hint={t('deploymentsPage.lifecycleHint')} label={t('deploymentsPage.lifecycle')} name="lifecycle" placeholder={t('deploymentsPage.lifecyclePlaceholder')} />
        <JsonField hint={t('deploymentsPage.readinessProbeHint')} label={t('deploymentsPage.readinessProbe')} name="readinessProbe" placeholder={t('deploymentsPage.probePlaceholder')} />
        <JsonField hint={t('deploymentsPage.livenessProbeHint')} label={t('deploymentsPage.livenessProbe')} name="livenessProbe" placeholder={t('deploymentsPage.probePlaceholder')} />
        <JsonField hint={t('deploymentsPage.startupProbeHint')} label={t('deploymentsPage.startupProbe')} name="startupProbe" placeholder={t('deploymentsPage.probePlaceholder')} />
      </AdvancedOptions>
    </EditorGroup>
  )
}
