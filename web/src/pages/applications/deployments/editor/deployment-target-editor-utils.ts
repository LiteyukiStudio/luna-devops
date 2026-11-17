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

/**
 * 仅在横向滚动容器内部水平对齐目标子项，严禁调用 Element.scrollIntoView()，
 * 避免窄屏下顶部导航滚出主视口后被级联拉回导致 `<main>` 纵向回弹死循环。
 */
export function scrollChildIntoHorizontalContainer(
  container: HTMLElement | null,
  child: HTMLElement | null,
  behavior: ScrollBehavior = preferredScrollBehavior(),
) {
  if (!container || !child)
    return
  const containerRect = container.getBoundingClientRect()
  const childRect = child.getBoundingClientRect()
  const containerWidth = container.clientWidth || containerRect.width
  const childWidth = child.offsetWidth || childRect.width
  if (containerWidth <= 0 || container.scrollWidth <= containerWidth)
    return

  const childLeft = containerRect.width > 0 || childRect.left !== containerRect.left
    ? container.scrollLeft + (childRect.left - containerRect.left)
    : child.offsetLeft - container.offsetLeft
  const maxScrollLeft = Math.max(0, container.scrollWidth - containerWidth)
  const centeredLeft = childLeft - (containerWidth - childWidth) / 2
  const nextLeft = Math.min(maxScrollLeft, Math.max(0, Math.round(centeredLeft)))
  if (Math.abs(nextLeft - container.scrollLeft) < 1)
    return

  if (typeof container.scrollTo === 'function')
    container.scrollTo({ behavior, left: nextLeft })
  else
    container.scrollLeft = nextLeft
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
