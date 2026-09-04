import { describe, expect, it } from 'vitest'
import i18next, { loadTranslationBundles } from './index'

describe('translation bundles', () => {
  it('keeps public shell translations in the initial locale', async () => {
    await i18next.changeLanguage('zh-CN')
    expect(i18next.t('accountMenu')).toBe('账号菜单')
    expect(i18next.t('common.codeEditorFallback')).toContain('高级编辑器暂时不可用')
  })

  it('loads a feature bundle on demand', async () => {
    await i18next.changeLanguage('zh-CN')
    await loadTranslationBundles(['billingPage'])
    expect(i18next.t('billingPage.balance')).toBe('余额')
  })
})
