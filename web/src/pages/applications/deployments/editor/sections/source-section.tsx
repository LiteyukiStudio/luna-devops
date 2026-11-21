import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { ArtifactRegistry, RepositoryBinding } from '@/api'
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { EditorSection, SwitchField } from '../deployment-target-editor-fields'
import { SourceFields } from '../source/source-build-fields'

/** 来源与构建 / 镜像与发布：整合来源类型、仓库或镜像、构建定义、拉取策略与自动触发规则。 */
export function SourceSection({
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
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const sourceType = useWatch({ control, name: 'sourceType' })
  const repository = sourceType !== 'image'

  return (
    <EditorSection
      section="source"
      title={repository ? t('deploymentTargetEditor.sections.sourceRepository') : t('deploymentTargetEditor.sections.sourceImage')}
      tools={(
        <SwitchField
          inline
          description={t('deploymentTargetEditor.autoDeployDescription')}
          label={t('deploymentsPage.autoDeploy')}
          name="autoDeploy"
        />
      )}
    >
      <SourceFields
        applicationId={applicationId}
        editing={editing}
        followsImageTemplate={followsImageTemplate}
        projectId={projectId}
        registries={registries}
        repositoryBindings={repositoryBindings}
        resolvedTemplateImageRef={resolvedTemplateImageRef}
        onExpandAdvanced={onExpandAdvanced}
        onTargetImageRefManualChange={onTargetImageRefManualChange}
      />
    </EditorSection>
  )
}
