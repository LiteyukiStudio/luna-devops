import type { TFunction } from 'i18next'
import type { Application } from '@/api'

/** 删除失败原因按稳定 deleteCode 本地化；未带原因码的历史记录才回退到诊断原文。 */
export function applicationDeleteFailureMessage(t: TFunction, application: Pick<Application, 'deleteCode' | 'deleteMessage'>): string {
  switch (application.deleteCode) {
    case 'service_binding_in_use':
      return t('errors.service_binding_in_use')
    case 'application.delete_cleanup_failed':
      return t('errors.application.delete_cleanup_failed')
    case 'application.delete_finalize_failed':
      return t('errors.application.delete_finalize_failed')
    case 'application.delete_enqueue_failed':
      return t('errors.application.delete_enqueue_failed')
    case '':
      return application.deleteMessage
  }
}
