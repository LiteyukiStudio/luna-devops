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
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { aiSettingsPayload, aiSettingsSchema } from './ai-assistant-settings'

type FormValues = AISettingsFormValues

const defaults: FormValues = {
  enabled: false,
  providerType: '',
  baseUrl: '',
  apiKey: '',
  defaultModel: '',
  fallbackModel: '',
  modelPricing: '[]',
  accessMode: 'admins',
  userIds: '',
  projectIds: '',
  userConcurrentRuns: 2,
  userDailyTokens: 200000,
  projectConcurrentRuns: 5,
  runMaxToolCalls: 20,
  platformDailyCostHard: 0,
  platformDailyCostSoft: 0,
  conversationDays: 90,
  runEventDays: 30,
  checkpointDays: 7,
}

function numberValue(values: Record<string, string>, key: string, fallback: number) {
  const value = Number(values[key])
  return Number.isFinite(value) ? value : fallback
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
      providerType: values['ai.provider.type'] === 'openai-compatible' ? 'openai-compatible' : '',
      baseUrl: values['ai.provider.base_url'] ?? '',
      apiKey: '',
      defaultModel: values['ai.provider.default_model'] ?? '',
      fallbackModel: values['ai.provider.fallback_model'] ?? '',
      modelPricing: values['ai.provider.model_pricing'] ?? '[]',
      accessMode: ['admins', 'all_authenticated', 'allowlist'].includes(values['ai.access.mode']) ? values['ai.access.mode'] as FormValues['accessMode'] : 'admins',
      userIds: parseList(values['ai.access.user_ids']),
      projectIds: parseList(values['ai.access.project_ids']),
      userConcurrentRuns: numberValue(values, 'ai.quota.user_concurrent_runs', 2),
      userDailyTokens: numberValue(values, 'ai.quota.user_daily_tokens', 200000),
      projectConcurrentRuns: numberValue(values, 'ai.quota.project_concurrent_runs', 5),
      runMaxToolCalls: numberValue(values, 'ai.quota.run_max_tool_calls', 20),
      platformDailyCostHard: numberValue(values, 'ai.quota.platform_daily_cost_hard', 0),
      platformDailyCostSoft: numberValue(values, 'ai.quota.platform_daily_cost_soft', 0),
      conversationDays: numberValue(values, 'ai.retention.conversation_days', 90),
      runEventDays: numberValue(values, 'ai.retention.run_event_days', 30),
      checkpointDays: numberValue(values, 'ai.retention.checkpoint_days', 7),
    })
  }, [configs.data, form])

  const save = useMutation({
    mutationFn: (values: FormValues) => api.updateConfigs(aiSettingsPayload(values)),
    onSuccess: (values) => {
      queryClient.setQueryData(['configs'], values)
      void queryClient.invalidateQueries({ queryKey: ['ai', 'capabilities'] })
      toast.success(t('settings.ai.saved'))
      form.setValue('apiKey', '')
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('settings.ai.saveFailed')),
  })

  const errors = form.formState.errors
  return (
    <form className="max-w-3xl" onSubmit={form.handleSubmit(values => save.mutate(values))}>
      <Surface className="grid gap-5 rounded-xl p-6" variant="bordered">
        <CheckboxField description={t('settings.ai.enabledHint')} {...form.register('enabled')}>{t('settings.ai.enabled')}</CheckboxField>
        <div className="grid gap-4 md:grid-cols-2">
          <Field error={errors.providerType?.message} label={t('settings.ai.providerType')} required>
            <NativeSelect {...form.register('providerType')}>
              <option value="">{t('settings.ai.notConfigured')}</option>
              <option value="openai-compatible">{t('settings.ai.openAICompatible')}</option>
            </NativeSelect>
          </Field>
          <Field error={errors.defaultModel?.message} label={t('settings.ai.defaultModel')} required>
            <Input {...form.register('defaultModel')} />
          </Field>
          <Field error={errors.fallbackModel?.message} label={t('settings.ai.fallbackModel')}>
            <Input {...form.register('fallbackModel')} />
          </Field>
        </div>
        <Field error={errors.baseUrl?.message} hint={t('settings.ai.baseUrlHint')} label={t('settings.ai.baseUrl')} required>
          <Input autoComplete="url" placeholder="https://api.example.com/v1" {...form.register('baseUrl')} />
        </Field>
        <Field hint={t('settings.ai.apiKeyHint')} label={t('settings.ai.apiKey')}>
          <Input autoComplete="new-password" placeholder={t('settings.ai.secretUnchanged')} type="password" {...form.register('apiKey')} />
        </Field>
        <Field error={errors.modelPricing?.message} hint={t('settings.ai.modelPricingHint')} label={t('settings.ai.modelPricing')}>
          <Textarea className="min-h-24 font-mono text-xs" {...form.register('modelPricing')} />
        </Field>
        <Field label={t('settings.ai.accessMode')} required>
          <NativeSelect {...form.register('accessMode')}>
            <option value="admins">{t('settings.ai.accessAdmins')}</option>
            <option value="allowlist">{t('settings.ai.accessAllowlist')}</option>
            <option value="all_authenticated">{t('settings.ai.accessAll')}</option>
          </NativeSelect>
        </Field>
        {form.watch('accessMode') === 'allowlist' && (
          <div className="grid gap-4 md:grid-cols-2">
            <Field hint={t('settings.ai.idsHint')} label={t('settings.ai.userIds')}><Textarea {...form.register('userIds')} /></Field>
            <Field hint={t('settings.ai.idsHint')} label={t('settings.ai.projectIds')}><Textarea {...form.register('projectIds')} /></Field>
          </div>
        )}
        <div className="grid gap-4 md:grid-cols-2">
          <NumberField form={form} keyName="userConcurrentRuns" label={t('settings.ai.userConcurrentRuns')} />
          <NumberField form={form} keyName="userDailyTokens" label={t('settings.ai.userDailyTokens')} />
          <NumberField form={form} keyName="projectConcurrentRuns" label={t('settings.ai.projectConcurrentRuns')} />
          <NumberField form={form} keyName="runMaxToolCalls" label={t('settings.ai.runMaxToolCalls')} />
          <NumberField form={form} keyName="platformDailyCostHard" label={t('settings.ai.platformDailyCostHard')} />
          <NumberField form={form} keyName="platformDailyCostSoft" label={t('settings.ai.platformDailyCostSoft')} />
        </div>
        <div className="grid gap-4 md:grid-cols-3">
          <NumberField form={form} keyName="conversationDays" label={t('settings.ai.conversationDays')} />
          <NumberField form={form} keyName="runEventDays" label={t('settings.ai.runEventDays')} />
          <NumberField form={form} keyName="checkpointDays" label={t('settings.ai.checkpointDays')} />
        </div>
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

function parseList(value?: string) {
  if (!value)
    return ''
  try {
    const parsed = JSON.parse(value)
    return Array.isArray(parsed) ? parsed.join('\n') : ''
  }
  catch {
    return ''
  }
}

function NumberField({ form, keyName, label }: { form: ReturnType<typeof useForm<FormValues>>, keyName: keyof Pick<FormValues, 'userConcurrentRuns' | 'userDailyTokens' | 'projectConcurrentRuns' | 'runMaxToolCalls' | 'platformDailyCostHard' | 'platformDailyCostSoft' | 'conversationDays' | 'runEventDays' | 'checkpointDays'>, label: string }) {
  return (
    <Field error={form.formState.errors[keyName]?.message} label={label} required>
      <Input type="number" {...form.register(keyName, { valueAsNumber: true })} />
    </Field>
  )
}
