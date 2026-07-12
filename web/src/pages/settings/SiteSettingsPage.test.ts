import type { ConfigDefinition } from '@/api'
import { beforeAll, describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { configDefinitionText } from './config-definition-text'

const baseDefinition: ConfigDefinition = {
  default: '',
  key: 'missing.definition',
  public: false,
  type: 'string',
}

describe('config definition translations', () => {
  beforeAll(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('renders an explicit i18n key when it exists', () => {
    expect(configDefinitionText({
      ...baseDefinition,
      label: 'fallback.label.token',
      labelKey: 'settings.saveConfig',
    }, 'label', i18next.t)).toBe(i18next.t('settings.saveConfig'))
  })

  it('renders the conventional config-definition i18n key', () => {
    expect(configDefinitionText({
      ...baseDefinition,
      key: 'site.title',
      labelKey: 'missing.requested.key',
    }, 'label', i18next.t)).toBe(i18next.t('settings.configDefinitions.site.title.label'))
  })

  it('falls back to backend definition text when no i18n key exists', () => {
    expect(configDefinitionText({
      ...baseDefinition,
      description: 'fallback.description.token',
      descriptionKey: 'missing.requested.key',
    }, 'description', i18next.t)).toBe('fallback.description.token')
  })
})
