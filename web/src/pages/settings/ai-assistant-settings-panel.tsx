import type { AISettingsFormValues } from './ai-assistant-settings'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { FlaskConical, LoaderCircle } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { CheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { PageChromeTools } from '@/components/common/page-chrome'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { StatusBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { aiSettingsPayload, aiSettingsSchema } from './ai-assistant-settings'
import { SettingsTabSaveButton } from './settings-tab-save-button'

type FormValues = AISettingsFormValues

const defaults: FormValues = {
  enabled: false,
  accessMode: 'all_authenticated',
  baseUrl: '',
  apiKey: '',
  apiKeyConfigured: false,
  model: '',
  webProxyEnabled: false,
  webProxyPool: '',
  webProxyPoolConfigured: false,
  providerTimeoutSeconds: 30,
  runTimeoutSeconds: 300,
  agentConcurrentRuns: 10,
  contextInputKTokens: 256,
  contextCompressionTriggerRatio: 0.8,
  contextCompressionTargetRatio: 0.5,
  contextRecentTurnCount: 6,
  contextMaxRecentTurnCount: 12,
  contextMaxUncompressedTurnCount: 32,
  contextMaxCompressionTurnsPerCompile: 128,
  contextSummaryInputKTokens: 32,
  contextSummaryMaxOutputTokens: 3000,
  contextHistoricalToolKTokens: 8,
  modelMaxOutputTokens: 8192,
  runMaxModelSteps: 64,
  runMaxInputKBytes: 64,
  runNavigateActionTtlSeconds: 120,
  toolsResultPayloadKBytes: 48,
  toolsMaxCardRepairAttempts: 5,
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

// 高级设置字段：对应 Agent 运行时中原本写死、现已由平台下发的参数。
// 每个字段提供平台默认值；普通部署保持默认即可。
type AdvancedFieldName
  = 'contextCompressionTriggerRatio'
    | 'contextCompressionTargetRatio'
    | 'contextRecentTurnCount'
    | 'contextMaxRecentTurnCount'
    | 'contextMaxUncompressedTurnCount'
    | 'contextMaxCompressionTurnsPerCompile'
    | 'contextSummaryInputKTokens'
    | 'contextSummaryMaxOutputTokens'
    | 'contextHistoricalToolKTokens'
    | 'modelMaxOutputTokens'
    | 'runMaxModelSteps'
    | 'runMaxInputKBytes'
    | 'runNavigateActionTtlSeconds'
    | 'toolsResultPayloadKBytes'
    | 'toolsMaxCardRepairAttempts'

interface AdvancedField {
  name: AdvancedFieldName
  labelKey: string
  hintKey: string
  min: number
  max: number
  step: number
}

interface AdvancedGroup {
  titleKey: string
  descriptionKey: string
  fields: AdvancedField[]
}

const advancedGroups: AdvancedGroup[] = [
  {
    titleKey: 'settings.ai.contextTitle',
    descriptionKey: 'settings.ai.contextDescription',
    fields: [
      { name: 'contextCompressionTriggerRatio', labelKey: 'settings.ai.compressionTriggerRatio', hintKey: 'settings.ai.compressionTriggerRatioHint', min: 0.5, max: 0.95, step: 0.01 },
      { name: 'contextCompressionTargetRatio', labelKey: 'settings.ai.compressionTargetRatio', hintKey: 'settings.ai.compressionTargetRatioHint', min: 0.1, max: 0.8, step: 0.01 },
      { name: 'contextRecentTurnCount', labelKey: 'settings.ai.recentTurnCount', hintKey: 'settings.ai.recentTurnCountHint', min: 1, max: 16, step: 1 },
      { name: 'contextMaxRecentTurnCount', labelKey: 'settings.ai.maxRecentTurnCount', hintKey: 'settings.ai.maxRecentTurnCountHint', min: 2, max: 32, step: 1 },
      { name: 'contextMaxUncompressedTurnCount', labelKey: 'settings.ai.maxUncompressedTurnCount', hintKey: 'settings.ai.maxUncompressedTurnCountHint', min: 4, max: 200, step: 1 },
      { name: 'contextMaxCompressionTurnsPerCompile', labelKey: 'settings.ai.maxCompressionTurnsPerCompile', hintKey: 'settings.ai.maxCompressionTurnsPerCompileHint', min: 8, max: 500, step: 1 },
      { name: 'contextSummaryInputKTokens', labelKey: 'settings.ai.summaryInputKTokens', hintKey: 'settings.ai.summaryInputKTokensHint', min: 4, max: 128, step: 1 },
      { name: 'contextSummaryMaxOutputTokens', labelKey: 'settings.ai.summaryMaxOutputTokens', hintKey: 'settings.ai.summaryMaxOutputTokensHint', min: 200, max: 8000, step: 1 },
      { name: 'contextHistoricalToolKTokens', labelKey: 'settings.ai.historicalToolKTokens', hintKey: 'settings.ai.historicalToolKTokensHint', min: 1, max: 64, step: 1 },
    ],
  },
  {
    titleKey: 'settings.ai.modelTitle',
    descriptionKey: 'settings.ai.modelDescription',
    fields: [
      { name: 'modelMaxOutputTokens', labelKey: 'settings.ai.modelMaxOutputTokens', hintKey: 'settings.ai.modelMaxOutputTokensHint', min: 256, max: 16384, step: 1 },
      { name: 'runMaxModelSteps', labelKey: 'settings.ai.runMaxModelSteps', hintKey: 'settings.ai.runMaxModelStepsHint', min: 1, max: 200, step: 1 },
      { name: 'runMaxInputKBytes', labelKey: 'settings.ai.runMaxInputKBytes', hintKey: 'settings.ai.runMaxInputKBytesHint', min: 8, max: 1024, step: 1 },
      { name: 'runNavigateActionTtlSeconds', labelKey: 'settings.ai.runNavigateActionTtlSeconds', hintKey: 'settings.ai.runNavigateActionTtlSecondsHint', min: 10, max: 600, step: 1 },
    ],
  },
  {
    titleKey: 'settings.ai.toolsTitle',
    descriptionKey: 'settings.ai.toolsDescription',
    fields: [
      { name: 'toolsResultPayloadKBytes', labelKey: 'settings.ai.toolsResultPayloadKBytes', hintKey: 'settings.ai.toolsResultPayloadKBytesHint', min: 4, max: 512, step: 1 },
      { name: 'toolsMaxCardRepairAttempts', labelKey: 'settings.ai.toolsMaxCardRepairAttempts', hintKey: 'settings.ai.toolsMaxCardRepairAttemptsHint', min: 1, max: 10, step: 1 },
    ],
  },
]

export function AIAssistantSettingsPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const configs = useQuery({ queryKey: ['configs'], queryFn: api.getConfigs })
  const form = useForm<FormValues>({ resolver: zodResolver(aiSettingsSchema), mode: 'onChange', defaultValues: defaults })

  useEffect(() => {
    if (!configs.data)
      return
    form.reset(aiSettingsFormValues(configs.data))
  }, [configs.data, form])

  const save = useMutation({
    mutationFn: (values: FormValues) => api.updateConfigs(aiSettingsPayload(values)),
    onSuccess: (values) => {
      queryClient.setQueryData(['configs'], values)
      void queryClient.invalidateQueries({ queryKey: ['ai', 'capabilities'] })
      toast.success(t('settings.ai.saved'))
      form.reset(aiSettingsFormValues(values))
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('settings.ai.saveFailed')),
  })

  const errors = form.formState.errors
  const providerTimeoutSeconds = form.watch('providerTimeoutSeconds')
  const runTimeoutSeconds = form.watch('runTimeoutSeconds')
  const agentConcurrentRuns = form.watch('agentConcurrentRuns')
  const contextInputKTokens = form.watch('contextInputKTokens')
  const webProxyEnabled = form.watch('webProxyEnabled')
  const observabilityEnabled = form.watch('observabilityEnabled')
  return (
    <form className="max-w-3xl" onSubmit={form.handleSubmit(values => save.mutate(values))}>
      <PageChromeTools>
        <SettingsTabSaveButton
          disabled={!form.formState.isDirty || !form.formState.isValid}
          label={t('settings.saveConfig')}
          pending={save.isPending}
          type="button"
          onClick={() => void form.handleSubmit(values => save.mutate(values))()}
        />
      </PageChromeTools>
      <Surface className="grid gap-5 rounded-xl p-6" variant="bordered">
        <CheckboxField description={t('settings.ai.enabledHint')} {...form.register('enabled')}>{t('settings.ai.enabled')}</CheckboxField>
        <Field hint={t('settings.ai.accessModeHint')} label={t('settings.ai.accessMode')} required>
          <Select {...form.register('accessMode')}>
            <option value="all_authenticated">{t('settings.ai.accessModeAllAuthenticated')}</option>
            <option value="admins">{t('settings.ai.accessModeAdmins')}</option>
          </Select>
        </Field>
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
            contextBudget: contextInputKTokens,
          })}
          title={t('settings.ai.runtimeTitle')}
        >
          <div className="grid gap-4 md:grid-cols-2">
            <Field error={errors.providerTimeoutSeconds?.message} hint={t('settings.ai.providerTimeoutHint')} label={t('settings.ai.providerTimeout')}>
              <Input max={120} min={1} step={1} type="number" {...form.register('providerTimeoutSeconds', { valueAsNumber: true })} />
            </Field>
            <Field error={errors.runTimeoutSeconds?.message} hint={t('settings.ai.runTimeoutHint')} label={t('settings.ai.runTimeout')}>
              <Input max={900} min={30} step={1} type="number" {...form.register('runTimeoutSeconds', { valueAsNumber: true })} />
            </Field>
            <Field error={errors.agentConcurrentRuns?.message} hint={t('settings.ai.agentConcurrentRunsHint')} label={t('settings.ai.agentConcurrentRuns')}>
              <Input max={100} min={1} step={1} type="number" {...form.register('agentConcurrentRuns', { valueAsNumber: true })} />
            </Field>
            <Field error={errors.contextInputKTokens?.message} hint={t('settings.ai.contextInputBudgetHint')} label={t('settings.ai.contextInputBudget')}>
              <Input max={1024} min={64} step={1} type="number" {...form.register('contextInputKTokens', { valueAsNumber: true })} />
            </Field>
          </div>
        </ProgressiveSection>
        <ProgressiveSection
          description={t('settings.ai.advancedDescription')}
          storageKey="luna-settings-ai-advanced-open"
          summary={t('settings.ai.advancedSummary', { count: advancedGroups.reduce((total, group) => total + group.fields.length, 0) })}
          title={t('settings.ai.advancedTitle')}
        >
          {advancedGroups.map(group => (
            <div className="grid gap-4 rounded-lg bg-surface-subtle p-4" key={group.titleKey}>
              <div className="grid gap-1">
                <p className="text-sm font-medium">{t(group.titleKey)}</p>
                <p className="text-xs text-muted-foreground">{t(group.descriptionKey)}</p>
              </div>
              <div className="grid gap-4 md:grid-cols-2">
                {group.fields.map(field => (
                  <Field key={field.name} error={errors[field.name]?.message} hint={t(field.hintKey)} label={t(field.labelKey)}>
                    <Input max={field.max} min={field.min} step={field.step} type="number" {...form.register(field.name, { valueAsNumber: true })} />
                  </Field>
                ))}
              </div>
            </div>
          ))}
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
          <p className="text-xs text-muted-foreground">{t('settings.ai.observabilitySaveWarning')}</p>
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
    </form>
  )
}

function aiSettingsFormValues(values: Record<string, string>): FormValues {
  return {
    enabled: values['ai.assistant.enabled'] === 'true',
    accessMode: values['ai.access.mode'] === 'admins' ? 'admins' : 'all_authenticated',
    baseUrl: values['ai.provider.base_url'] ?? '',
    apiKey: '',
    apiKeyConfigured: values['ai.provider.api_key'] === 'true',
    model: values['ai.provider.default_model'] ?? '',
    webProxyEnabled: values['ai.web.proxy_enabled'] === 'true',
    webProxyPool: '',
    webProxyPoolConfigured: values['ai.web.proxy_pool'] === 'true',
    providerTimeoutSeconds: Number(values['ai.runtime.provider_timeout_seconds'] ?? 30),
    runTimeoutSeconds: Number(values['ai.runtime.run_timeout_seconds'] ?? 300),
    agentConcurrentRuns: Number(values['ai.runtime.agent_concurrent_runs'] ?? 10),
    contextInputKTokens: Number(values['ai.runtime.context_input_k_tokens'] ?? 256),
    contextCompressionTriggerRatio: Number(values['ai.context.compression_trigger_ratio'] ?? 0.8),
    contextCompressionTargetRatio: Number(values['ai.context.compression_target_ratio'] ?? 0.5),
    contextRecentTurnCount: Number(values['ai.context.recent_turn_count'] ?? 4),
    contextMaxRecentTurnCount: Number(values['ai.context.max_recent_turn_count'] ?? 8),
    contextMaxUncompressedTurnCount: Number(values['ai.context.max_uncompressed_turn_count'] ?? 24),
    contextMaxCompressionTurnsPerCompile: Number(values['ai.context.max_compression_turns_per_compile'] ?? 96),
    contextSummaryInputKTokens: Number(values['ai.context.summary_input_k_tokens'] ?? 24),
    contextSummaryMaxOutputTokens: Number(values['ai.context.summary_max_output_tokens'] ?? 1500),
    contextHistoricalToolKTokens: Number(values['ai.context.historical_tool_k_tokens'] ?? 4),
    modelMaxOutputTokens: Number(values['ai.model.max_output_tokens'] ?? 4096),
    runMaxModelSteps: Number(values['ai.run.max_model_steps'] ?? 48),
    runMaxInputKBytes: Number(values['ai.run.max_input_k_bytes'] ?? 48),
    runNavigateActionTtlSeconds: Number(values['ai.run.navigate_action_ttl_seconds'] ?? 60),
    toolsResultPayloadKBytes: Number(values['ai.tools.result_payload_k_bytes'] ?? 24),
    toolsMaxCardRepairAttempts: Number(values['ai.tools.max_card_repair_attempts'] ?? 3),
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
  }
}

function ObservabilitySourceFields({ form, source, tenant = false }: { form: ReturnType<typeof useForm<FormValues>>, source: 'prometheus' | 'loki' | 'tempo', tenant?: boolean }) {
  const { t } = useTranslation()
  const urlField = `${source}Url` as const
  const tokenField = `${source}Token` as const
  const configuredField = `${source}TokenConfigured` as const
  const tenantField = `${source}TenantId` as 'lokiTenantId' | 'tempoTenantId'
  const errors = form.formState.errors
  const test = useMutation({
    mutationFn: () => api.testAgentObservabilitySource({
      source,
      url: form.getValues(urlField).trim(),
      token: form.getValues(tokenField).trim() || undefined,
      tenantId: tenant ? form.getValues(tenantField).trim() || undefined : undefined,
    }),
  })
  return (
    <div className="grid gap-4 rounded-lg bg-surface-subtle p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm font-medium">{t(`settings.ai.observabilitySources.${source}`)}</p>
        <div className="flex items-center gap-2">
          {test.data && (
            <StatusBadge tone={test.data.reachable ? (test.data.dataAvailable ? 'success' : 'warning') : 'danger'}>
              {t(`settings.ai.observabilityTestCodes.${test.data.code.split('.').at(-1)}`, { defaultValue: test.data.code })}
              {' '}
              ·
              {test.data.latencyMs}
              {' '}
              ms
            </StatusBadge>
          )}
          {test.isError && <StatusBadge tone="danger">{t('settings.ai.observabilityTestFailed')}</StatusBadge>}
          <Button disabled={!form.watch(urlField) || test.isPending} size="sm" type="button" variant="outline" onClick={() => test.mutate()}>
            {test.isPending ? <LoaderCircle className="size-4 animate-spin" /> : <FlaskConical className="size-4" />}
            {t('settings.ai.observabilityTest')}
          </Button>
        </div>
      </div>
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
