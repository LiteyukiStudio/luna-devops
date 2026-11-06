import { beforeAll, describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { deploymentTargetDeleteFailureMessage } from './deployment-target-delete-failure'

describe('deployment target delete failure message', () => {
  beforeAll(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('localizes the stable delete code instead of showing the diagnostic message', () => {
    const t = i18next.t.bind(i18next)
    expect(deploymentTargetDeleteFailureMessage(t, { deleteCode: 'service_binding_in_use', deleteMessage: 'deployment target is referenced by active service bindings: 1' }))
      .toBe(i18next.t('errors.service_binding_in_use'))
    expect(deploymentTargetDeleteFailureMessage(t, { deleteCode: 'deployment_target.delete_cleanup_failed', deleteMessage: 'list services: timeout' }))
      .toBe(i18next.t('errors.deployment_target.delete_cleanup_failed'))
    expect(deploymentTargetDeleteFailureMessage(t, { deleteCode: 'deployment_target.delete_enqueue_failed', deleteMessage: 'enqueue failed' }))
      .toBe(i18next.t('errors.deployment_target.delete_enqueue_failed'))
    expect(deploymentTargetDeleteFailureMessage(t, { deleteCode: '', deleteMessage: 'legacy failure' })).toBe('legacy failure')
  })
})
