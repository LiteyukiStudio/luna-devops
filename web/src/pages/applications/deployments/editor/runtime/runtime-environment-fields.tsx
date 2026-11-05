import type { DeploymentTarget, Release } from '@/api'
import { useTranslation } from 'react-i18next'
import { EditorGroup } from '../deployment-target-editor-fields'
import { RuntimeConfigSetsField } from './runtime-config-sets-field'
import { RuntimeEnvironmentTable } from './runtime-environment-table'

/** 环境变量：先引用项目空间公共配置，再用当前部署配置的变量覆盖同名项；配置文件与密钥文件在高级分区。 */
export function RuntimeEnvironmentFields({ canManageRuntimeSecrets, deploymentTargets, projectId, releases }: {
  canManageRuntimeSecrets: boolean
  deploymentTargets: DeploymentTarget[]
  projectId: string
  releases: Release[]
}) {
  const { t } = useTranslation()
  return (
    <EditorGroup description={t('deploymentTargetEditor.environmentDescription')} title={t('deploymentTargetEditor.environmentTitle')}>
      <RuntimeConfigSetsField canManageSecrets={canManageRuntimeSecrets} deploymentTargets={deploymentTargets} projectId={projectId} releases={releases} />
      <RuntimeEnvironmentTable />
    </EditorGroup>
  )
}
