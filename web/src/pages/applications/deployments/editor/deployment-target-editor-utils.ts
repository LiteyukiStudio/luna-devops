import type { FieldPath, FieldValues } from 'react-hook-form'
import type { DeploymentTargetEditorSection, DeploymentTargetFormValues } from './deployment-target-form'
import { get, useFormState } from 'react-hook-form'
import { visibleAdvancedItems } from './advanced/advanced-items'
import { deploymentTargetAdvancedItemFields, deploymentTargetSectionFields } from './deployment-target-form'

export function deploymentTargetSectionAnchor(section: DeploymentTargetEditorSection) {
  return `deployment-target-section-${section}`
}

/** 读取任意层级字段的错误文案；schema 中的文案已是本地化结果。 */
export function useFieldError<TValues extends FieldValues = DeploymentTargetFormValues>(name: FieldPath<TValues>) {
  const { errors } = useFormState<TValues>({ name })
  const error: unknown = get(errors, name)
  if (error && typeof error === 'object' && 'message' in error && typeof error.message === 'string')
    return error.message
  return undefined
}

export function preferredScrollBehavior(): ScrollBehavior {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches ? 'auto' : 'smooth'
}

export function scrollToDeploymentTargetSection(section: DeploymentTargetEditorSection) {
  document.getElementById(deploymentTargetSectionAnchor(section))?.scrollIntoView({ behavior: preferredScrollBehavior(), block: 'start' })
}

/** 递归统计叶子字段/动态行的真实错误数量，避免多行出错只计作 1 条。 */
export function countLeafErrors(value: unknown): number {
  if (!value || typeof value !== 'object')
    return 0
  if (Array.isArray(value))
    return value.reduce<number>((sum, item) => sum + countLeafErrors(item), 0)
  const record = value as Record<string, unknown>
  if (typeof record.message === 'string' || typeof record.type === 'string')
    return 1
  return Object.entries(record)
    .filter(([key]) => key !== 'ref')
    .reduce<number>((sum, [, child]) => sum + countLeafErrors(child), 0)
}

export function fieldErrorCount(errors: object, fields: readonly FieldPath<DeploymentTargetFormValues>[]) {
  return fields.reduce((sum, field) => sum + countLeafErrors(get(errors, field)), 0)
}

export function sectionErrorCount(errors: object, section: DeploymentTargetEditorSection, sourceType?: string) {
  if (section === 'advanced') {
    const visibleFields = visibleAdvancedItems(sourceType).flatMap(item => deploymentTargetAdvancedItemFields[item])
    return fieldErrorCount(errors, visibleFields)
  }
  return fieldErrorCount(errors, deploymentTargetSectionFields[section])
}
