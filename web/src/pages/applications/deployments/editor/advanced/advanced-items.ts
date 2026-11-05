import type { DeepPartial } from 'react-hook-form'
import type { DeploymentTargetAdvancedItem, DeploymentTargetFormValues } from '../deployment-target-form'
import type { DeploymentTarget } from '@/api'
import { changedFieldCount, deploymentTargetAdvancedItemFields, deploymentTargetAdvancedItemOrder } from '../deployment-target-form'

export type FormSnapshot = DeepPartial<DeploymentTargetFormValues>

/** 高级项的“已设置”数量；已保存的密钥文件在表单里为空值，单独计入。 */
export function advancedItemConfiguredCount(item: DeploymentTargetAdvancedItem, values: FormSnapshot, target: DeploymentTarget | null) {
  const count = changedFieldCount(values, deploymentTargetAdvancedItemFields[item])
  return item === 'files' && target?.secretFilesSet && !values.secretFiles ? count + 1 : count
}

/** 当前来源下可见的高级项：已有镜像来源不经过构建，构建选项保留原值但不展示。 */
export function visibleAdvancedItems(sourceType: string | undefined) {
  return deploymentTargetAdvancedItemOrder.filter(item => item !== 'build' || sourceType !== 'image')
}
