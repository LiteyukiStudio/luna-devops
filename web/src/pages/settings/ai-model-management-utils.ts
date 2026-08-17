import type { AIModelConfig } from '@/api'
import i18next from 'i18next'
import { z } from 'zod'
import { ApiError } from '@/api'

const price = () => z.string().trim().regex(/^\d+(?:\.\d{1,8})?$/, { message: i18next.t('settings.ai.models.priceInvalid') })

export const aiModelFormSchema = z.object({
  name: z.string().trim().min(1, { message: i18next.t('settings.ai.models.nameRequired') }),
  maxContextTokens: z.number().int().min(4096).max(2097152),
  maxOutputTokens: z.number().int().min(256).max(262144),
  inputCreditsPerMillion: price(),
  outputCreditsPerMillion: price(),
  cachedInputCreditsPerMillion: price(),
  cachedOutputCreditsPerMillion: price(),
  enabled: z.boolean(),
}).refine(value => value.maxOutputTokens < value.maxContextTokens, {
  path: ['maxOutputTokens'],
  message: i18next.t('settings.ai.models.outputLimitInvalid'),
})

export type ModelFormValues = z.infer<typeof aiModelFormSchema>

export const emptyModel: ModelFormValues = {
  name: '',
  maxContextTokens: 524288,
  maxOutputTokens: 65536,
  inputCreditsPerMillion: '0',
  outputCreditsPerMillion: '0',
  cachedInputCreditsPerMillion: '0',
  cachedOutputCreditsPerMillion: '0',
  enabled: true,
}

export function modelSaveErrorMessage(error: unknown, fallback: string, translate: (key: string) => string) {
  if (!(error instanceof ApiError))
    return fallback
  switch (error.code) {
    case 'ai.model_name_conflict':
      return translate('settings.ai.models.errors.nameConflict')
    case 'ai.last_model_cannot_be_disabled':
      return translate('settings.ai.models.errors.lastEnabled')
    case 'ai.model_not_found':
      return translate('settings.ai.models.errors.notFound')
    case 'ai.model_context_limit_invalid':
      return translate('settings.ai.models.errors.contextLimit')
    case 'ai.model_output_limit_invalid':
      return translate('settings.ai.models.errors.outputLimit')
    default:
      return fallback
  }
}

export function modelFormValues(model: AIModelConfig): ModelFormValues {
  return {
    name: model.name,
    maxContextTokens: model.maxContextTokens,
    maxOutputTokens: model.maxOutputTokens,
    inputCreditsPerMillion: model.inputCreditsPerMillion,
    outputCreditsPerMillion: model.outputCreditsPerMillion,
    cachedInputCreditsPerMillion: model.cachedInputCreditsPerMillion,
    cachedOutputCreditsPerMillion: model.cachedOutputCreditsPerMillion,
    enabled: model.enabled,
  }
}
