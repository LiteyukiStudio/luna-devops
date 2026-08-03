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
import { ProgressiveSection } from '@/components/common/progressive-section'
import { Surface } from '@/components/common/surface'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { aiSettingsPayload, aiSettingsSchema } from './ai-assistant-settings'

type FormValues = AISettingsFormValues

const defaults: FormValues = {
  enabled: false,
  baseUrl: '',
  apiKey: '',
  apiKeyConfigured: false,
  model: '',
  webProxyEnabled: false,
  webProxyPool: '',
  webProxyPoolConfigured: false,
  providerTimeoutSeconds: 30,
  runTimeoutSeconds: 300,
  agentConcurrentRuns: 2,
  observabilityEnabled: false,
  prometheusUrl: '',
  prometheusToken: '',
  prometheusTokenConfigured: false,
  lokiUrl: '',
  lokiTenantId: '',
  lokiToken: '',
  lokiTokenConfigured: false,
  tempoUrl: '',
  tempoTenantId: '',
  tempoToken: '',
  tempoTokenConfigured: false,
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
      webProxyEnabled: values['ai.web.proxy_enabled'] === 'true',
      webProxyPool: '',
      webProxyPoolConfigured: values['ai.web.proxy_pool'] === 'true',
      providerTimeoutSeconds: Number(values['ai.runtime.provider_timeout_seconds'] ?? 30),
      runTimeoutSeconds: Number(values['ai.runtime.run_timeout_seconds'] ?? 300),
      agentConcurrentRuns: Number(values['ai.runtime.agent_concurrent_runs'] ?? 2),
      observabilityEnabled: values['ai.observability.enabled'] === 'true',
      prometheusUrl: values['ai.observability.prometheus_url'] ?? '',
      prometheusToken: '',
      prometheusTokenConfigured: values['ai.observability.prometheus_token'] === 'true',
      lokiUrl: values['ai.observability.loki_url'] ?? '',
      lokiTenantId: values['ai.observability.loki_tenant_id'] ?? '',
      lokiToken: '',
      lokiTokenConfigured: values['ai.observability.loki_token'] === 'true',
      tempoUrl: values['ai.observability.tempo_url'] ?? '',
      tempoTenantId: values['ai.observability.tempo_tenant_id'] ?? '',
      tempoToken: '',
      tempoTokenConfigured: values['ai.observability.tempo_token'] === 'true',
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
      form.setValue('webProxyPool', '')
      form.setValue('webProxyPoolConfigured', values['ai.web.proxy_pool'] === 'true')
      for (const source of ['prometheus', 'loki', 'tempo'] as const) {
        form.setValue(`${source}Token`, '')
        form.setValue(`${source}TokenConfigured`, values[`ai.observability.${source}_token`] === 'true')
      }
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('settings.ai.saveFailed')),
  })

  const errors = form.formState.errors
  const providerTimeoutSeconds = form.watch('providerTimeoutSeconds')
  const runTimeoutSeconds = form.watch('runTimeoutSeconds')
  const agentConcurrentRuns = form.watch('agentConcurrentRuns')
  const webProxyEnabled = form.watch('webProxyEnabled')
  const observabilityEnabled = form.watch('observabilityEnabled')
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
        <ProgressiveSection
          description={t('settings.ai.runtimeDescription')}
          storageKey="luna-settings-ai-runtime-open"
          summary={t('settings.ai.runtimeSummary', {
            providerTimeout: providerTimeoutSeconds,
            runTimeout: runTimeoutSeconds,
            concurrency: agentConcurrentRuns,
          })}
          title={t('settings.ai.runtimeTitle')}
        >
          <div className="grid gap-4 md:grid-cols-3">
            <Field error={errors.providerTimeoutSeconds?.message} hint={t('settings.ai.providerTimeoutHint')} label={t('settings.ai.providerTimeout')}>
              <Input max={120} min={1} step={1} type="number" {...form.register('providerTimeoutSeconds', { valueAsNumber: true })} />
            </Field>
            <Field error={errors.runTimeoutSeconds?.message} hint={t('settings.ai.runTimeoutHint')} label={t('settings.ai.runTimeout')}>
              <Input max={900} min={30} step={1} type="number" {...form.register('runTimeoutSeconds', { valueAsNumber: true })} />
            </Field>
            <Field error={errors.agentConcurrentRuns?.message} hint={t('settings.ai.agentConcurrentRunsHint')} label={t('settings.ai.agentConcurrentRuns')}>
              <Input max={10} min={1} step={1} type="number" {...form.register('agentConcurrentRuns', { valueAsNumber: true })} />
            </Field>
          </div>
        </ProgressiveSection>
        <ProgressiveSection
          description={t('settings.ai.observabilityDescription')}
          storageKey="luna-settings-ai-observability-open"
          summary={observabilityEnabled ? t('settings.ai.observabilitySummaryEnabled') : t('settings.ai.observabilitySummaryDisabled')}
          title={t('settings.ai.observabilityTitle')}
        >
          <CheckboxField description={t('settings.ai.observabilityEnabledHint')} {...form.register('observabilityEnabled')}>{t('settings.ai.observabilityEnabled')}</CheckboxField>
          <ObservabilitySourceFields form={form} source="prometheus" />
          <ObservabilitySourceFields form={form} source="loki" tenant />
          <ObservabilitySourceFields form={form} source="tempo" tenant />
        </ProgressiveSection>
        <ProgressiveSection
          description={t('settings.ai.webProxyDescription')}
          storageKey="luna-settings-ai-web-proxy-open"
          summary={webProxyEnabled ? t('settings.ai.webProxySummaryEnabled') : t('settings.ai.webProxySummaryDirect')}
          title={t('settings.ai.webProxyTitle')}
        >
          <div className="grid gap-4">
            <CheckboxField description={t('settings.ai.webProxyEnabledHint')} {...form.register('webProxyEnabled')}>{t('settings.ai.webProxyEnabled')}</CheckboxField>
            <Field error={errors.webProxyPool?.message} hint={t('settings.ai.webProxyPoolHint')} label={t('settings.ai.webProxyPool')}>
              <Textarea
                autoComplete="off"
                className="min-h-28 font-mono text-sm"
                placeholder={form.getValues('webProxyPoolConfigured') ? t('settings.ai.secretUnchanged') : 'http://user:password@proxy.example.com:888'}
                {...form.register('webProxyPool')}
              />
            </Field>
          </div>
        </ProgressiveSection>
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

function ObservabilitySourceFields({ form, source, tenant = false }: { form: ReturnType<typeof useForm<FormValues>>, source: 'prometheus' | 'loki' | 'tempo', tenant?: boolean }) {
  const { t } = useTranslation()
  const urlField = `${source}Url` as const
  const tokenField = `${source}Token` as const
  const configuredField = `${source}TokenConfigured` as const
  const tenantField = `${source}TenantId` as 'lokiTenantId' | 'tempoTenantId'
  const errors = form.formState.errors
  return (
    <div className="grid gap-4 rounded-lg bg-surface-subtle p-4">
      <p className="text-sm font-medium">{t(`settings.ai.observabilitySources.${source}`)}</p>
      <div className="grid gap-4 md:grid-cols-2">
        <Field error={errors[urlField]?.message} hint={t('settings.ai.observabilityUrlHint')} label={t('settings.ai.observabilityUrl')} required={form.watch('observabilityEnabled')}>
          <Input autoComplete="url" placeholder={`http://${source}:${source === 'prometheus' ? '9090' : source === 'loki' ? '3100' : '3200'}`} {...form.register(urlField)} />
        </Field>
        {tenant && <Field label={t('settings.ai.observabilityTenantId')}><Input autoComplete="off" {...form.register(tenantField)} /></Field>}
        <Field hint={t('settings.ai.observabilityTokenHint')} label={t('settings.ai.observabilityToken')}>
          <Input autoComplete="new-password" placeholder={form.getValues(configuredField) ? t('settings.ai.secretUnchanged') : ''} type="password" {...form.register(tokenField)} />
        </Field>
      </div>
    </div>
  )
}
