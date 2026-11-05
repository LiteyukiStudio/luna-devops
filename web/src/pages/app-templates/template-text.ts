import type { TFunction } from 'i18next'
import type { AppTemplate } from '@/api'

export function templateCategoryLabel(category: string, t: TFunction) {
  return t(`appTemplatesPage.categories.${category}`, { defaultValue: category })
}

export function templateDescription(template: Pick<AppTemplate, 'description' | 'id'>, t: TFunction) {
  return t(`appTemplatesPage.templates.${template.id}.description`, { defaultValue: template.description || t('common.noDescription') })
}
