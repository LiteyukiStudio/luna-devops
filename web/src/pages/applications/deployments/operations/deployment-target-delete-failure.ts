import type { TFunction } from 'i18next'
import type { DeploymentTarget } from '@/api'

/** 部署配置删除失败原因按稳定 deleteCode 本地化；未带原因码的历史记录才回退到诊断原文。 */
export function deploymentTargetDeleteFailureMessage(t: TFunction, target: Pick<DeploymentTarget, 'deleteCode' | 'deleteMessage'>): string {
  switch (target.deleteCode) {
    case 'service_binding_in_use':
      return t('errors.service_binding_in_use')
    case 'deployment_target.delete_cleanup_failed':
      return t('errors.deployment_target.delete_cleanup_failed')
    case 'deployment_target.delete_finalize_failed':
      return t('errors.deployment_target.delete_finalize_failed')
    case 'deployment_target.delete_enqueue_failed':
      return t('errors.deployment_target.delete_enqueue_failed')
    case '':
      return target.deleteMessage
  }
}
