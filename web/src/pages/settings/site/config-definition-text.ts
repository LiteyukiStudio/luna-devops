import type { TFunction } from 'i18next'
import type { ConfigDefinition } from '@/api'
import i18next from '@/i18n'

export function configDefinitionText(
  definition: ConfigDefinition,
  kind: 'label' | 'description',
  t: TFunction,
) {
  const requestedKey = kind === 'label' ? definition.labelKey : definition.descriptionKey
  const conventionalKey = `settings.configDefinitions.${definition.key}.${kind}`
  const fallback = (kind === 'label' ? definition.label : definition.description) || (kind === 'label' ? definition.key : '')
  const key = requestedKey && i18next.exists(requestedKey) ? requestedKey : conventionalKey
  return t(key, { defaultValue: fallback })
}
