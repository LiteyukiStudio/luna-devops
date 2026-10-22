import type { FieldPath, FieldValues } from 'react-hook-form'
import type { DeploymentTargetEditorSection, DeploymentTargetFormValues } from './deployment-target-form'
import { get, useFormState } from 'react-hook-form'
import { deploymentTargetSectionFields } from './deployment-target-form'

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

export function sectionErrorCount(errors: object, section: DeploymentTargetEditorSection) {
  return deploymentTargetSectionFields[section].filter(field => Boolean(get(errors, field))).length
}
