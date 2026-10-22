import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { DeploymentTarget, Release } from '@/api'
import { Controller, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/common/form-field'
import { RuntimeConfigFilesEditor } from '@/components/common/runtime-config-files-editor'
import { Textarea } from '@/components/ui/textarea'
import { EditorGroup } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'
import { ApplicationDeploymentRuntimeSecretsEditor } from './application-deployment-runtime-secrets-editor'
import { RuntimeConfigSetsField } from './runtime-config-sets-field'

/** 环境与配置：公共配置 → 环境变量 → 密钥变量 → 配置文件 → 密钥文件，按“先共享、后覆盖、再敏感”的顺序排列。 */
export function RuntimeEnvironmentFields({ applicationId, canManageRuntimeSecrets, deploymentTargets, projectId, releases, target }: {
  applicationId: string
  canManageRuntimeSecrets: boolean
  deploymentTargets: DeploymentTarget[]
  projectId: string
  releases: Release[]
  target: DeploymentTarget | null
}) {
  const { t } = useTranslation()
  const { control, register } = useFormContext<DeploymentTargetFormValues>()
  const environmentError = useFieldError('environmentVariablesText')
  const configFilesError = useFieldError('configFiles')
  const secretFilesError = useFieldError('secretFiles')

  return (
    <EditorGroup description={t('deploymentTargetEditor.environmentDescription')} title={t('deploymentTargetEditor.environmentTitle')}>
      <RuntimeConfigSetsField canManageSecrets={canManageRuntimeSecrets} deploymentTargets={deploymentTargets} projectId={projectId} releases={releases} />
      <FormField error={environmentError} hint={t('deploymentsPage.runtimeEnvVarsHint')} label={t('deploymentsPage.runtimeEnvVars')}>
        <Textarea
          {...register('environmentVariablesText')}
          className="min-h-28 font-mono text-sm"
          placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: t('deploymentsPage.runtimeEnvVarsPlaceholder') })}
          spellCheck={false}
        />
      </FormField>
      <ApplicationDeploymentRuntimeSecretsEditor
        applicationId={applicationId}
        canManage={canManageRuntimeSecrets}
        projectId={projectId}
        targetId={target?.id}
      />
      <div className="grid gap-4 lg:grid-cols-2">
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
    </EditorGroup>
  )
}
