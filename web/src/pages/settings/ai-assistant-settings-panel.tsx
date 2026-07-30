import type { AISettingsFormValues } from './ai-assistant-settings'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { CheckboxField } from '@/components/common/checkbox-field'
import { FormActions } from '@/components/common/form-actions'
import { FormField as Field } from '@/components/common/form-field'
import { Surface } from '@/components/common/surface'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { aiSettingsPayload, aiSettingsSchema } from './ai-assistant-settings'

type FormValues = AISettingsFormValues

const defaults: FormValues = {
  enabled: false,
  baseUrl: '',
  apiKey: '',
  apiKeyConfigured: false,
  model: '',
}

export function AIAssistantSettingsPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const configs = useQuery({ queryKey: ['configs'], queryFn: api.getConfigs })
  const form = useForm<FormValues>({ resolver: zodResolver(aiSettingsSchema), mode: 'onChange', defaultValues: defaults })

  useEffect(() => {
    if (!configs.data)
      return
    const values = configs.data
    form.reset({
      enabled: values['ai.assistant.enabled'] === 'true',
      baseUrl: values['ai.provider.base_url'] ?? '',
      apiKey: '',
      apiKeyConfigured: values['ai.provider.api_key'] === 'true',
      model: values['ai.provider.default_model'] ?? '',
    })
  }, [configs.data, form])

  const save = useMutation({
    mutationFn: (values: FormValues) => api.updateConfigs(aiSettingsPayload(values)),
    onSuccess: (values) => {
      queryClient.setQueryData(['configs'], values)
      void queryClient.invalidateQueries({ queryKey: ['ai', 'capabilities'] })
      toast.success(t('settings.ai.saved'))
      form.setValue('apiKey', '')
      form.setValue('apiKeyConfigured', values['ai.provider.api_key'] === 'true')
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('settings.ai.saveFailed')),
  })

  const errors = form.formState.errors
  return (
    <form className="max-w-3xl" onSubmit={form.handleSubmit(values => save.mutate(values))}>
      <Surface className="grid gap-5 rounded-xl p-6" variant="bordered">
        <CheckboxField description={t('settings.ai.enabledHint')} {...form.register('enabled')}>{t('settings.ai.enabled')}</CheckboxField>
        <div className="grid gap-4 md:grid-cols-2">
          <Field error={errors.model?.message} label={t('settings.ai.model')} required>
            <Input placeholder="deepseek-v4-pro" {...form.register('model')} />
          </Field>
          <Field error={errors.baseUrl?.message} hint={t('settings.ai.baseUrlHint')} label={t('settings.ai.baseUrl')} required>
            <Input autoComplete="url" placeholder="https://api.example.com/v1" {...form.register('baseUrl')} />
          </Field>
        </div>
        <Field error={errors.apiKey?.message} hint={t('settings.ai.apiKeyHint')} label={t('settings.ai.apiKey')} required>
          <Input autoComplete="new-password" placeholder={form.getValues('apiKeyConfigured') ? t('settings.ai.secretUnchanged') : 'sk-…'} type="password" {...form.register('apiKey')} />
        </Field>
        <p className="text-sm leading-6 text-muted-foreground">{t('settings.ai.securitySummary')}</p>
      </Surface>
      <FormActions className="mt-4" separated={false}>
        <Button disabled={!form.formState.isDirty || !form.formState.isValid || save.isPending} type="submit">
          <Save className="size-4" />
          {t('settings.saveConfig')}
        </Button>
      </FormActions>
    </form>
  )
}
