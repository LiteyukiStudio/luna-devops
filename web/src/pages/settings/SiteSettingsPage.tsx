import type { TFunction } from 'i18next'
import type { BillingRateRule, BillingRateRulePayload, ConfigDefinition, DataRetentionPayload, DataRetentionResult } from '@/api/types'
import type { DataListColumn } from '@/components/common/data-list'
import type { KeyValueRow } from '@/components/common/key-value-rows-editor'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Eye, Save, Settings2, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { applySiteBrandColorPreset } from '@/app/brand-theme'
import { BuildEnvironmentEditorDialog } from '@/components/common/build-environment-editor-dialog'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { FormActions } from '@/components/common/form-actions'
import { FormField as Field } from '@/components/common/form-field'
import { SettingsSkeleton } from '@/components/common/loading-states'
import { PageShell } from '@/components/common/page-shell'
import { SearchMultiSelect } from '@/components/common/search-select'
import { Section } from '@/components/common/section'
import { Surface } from '@/components/common/surface'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { TabsContent } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { buildVariableRecordToRows, buildVariableRowsToRecord, secretStateToRows } from '@/lib/build-variables'
import { AuthRegistrationSettingsPanel } from './auth-registration-settings-panel'
import { BrandColorPresetField } from './brand-color-preset-field'
import { configDefinitionText } from './config-definition-text'
import { changedConfigValues } from './site-settings-values'

const siteBrandColorPresetKey = 'site.brandColorPreset'

export function SiteSettingsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('brand')
  const [environmentDialogOpen, setEnvironmentDialogOpen] = useState(false)
  const [environmentVariableRows, setEnvironmentVariableRows] = useState<KeyValueRow[]>([])
  const [environmentSecretRows, setEnvironmentSecretRows] = useState<KeyValueRow[]>([])
  const form = useForm<Record<string, unknown>>({ mode: 'onChange', defaultValues: {} })
  const definitions = useQuery({ queryKey: ['config-definitions'], queryFn: api.listConfigDefinitions })
  const keys = useMemo(() => (definitions.data ?? []).map(definition => definition.key), [definitions.data])
  const values = useQuery({
    queryKey: ['configs'],
    queryFn: api.getConfigs,
    enabled: keys.length > 0,
  })
  const siteDefinitions = useMemo(() => (definitions.data ?? [])
    .filter(definition => definition.key.startsWith('site.'))
    .sort((left, right) => Number(right.key === siteBrandColorPresetKey) - Number(left.key === siteBrandColorPresetKey)), [definitions.data])
  const securityDefinitions = useMemo(() => (definitions.data ?? []).filter(definition => definition.key.startsWith('security.')), [definitions.data])
  const billingDefinitions = useMemo(() => (definitions.data ?? []).filter(definition => definition.key.startsWith('billing.')), [definitions.data])
  const retentionDefinitions = useMemo(() => (definitions.data ?? []).filter(definition => definition.key.startsWith('retention.')), [definitions.data])
  const resolvedValues = useMemo(() => {
    const nextValues: Record<string, string> = {}
    for (const definition of definitions.data ?? [])
      nextValues[definition.key] = values.data?.[definition.key] ?? definition.default
    return nextValues
  }, [definitions.data, values.data])

  useEffect(() => {
    if (definitions.data && values.data)
      form.reset(unflattenConfigValues(resolvedValues))
  }, [definitions.data, form, resolvedValues, values.data])

  const save = useMutation({
    mutationFn: api.updateConfigs,
    onSuccess: (result) => {
      toast.success(t('settings.siteSaved'))
      applySiteBrandColorPreset(result['site.brandColorPreset'])
      queryClient.setQueryData(['configs'], result)
      queryClient.invalidateQueries({ queryKey: ['configs'] })
      queryClient.invalidateQueries({ queryKey: ['public-configs'] })
    },
    onError: error => toast.error(error.message),
  })

  const submitChangedValues = (formValues: Record<string, unknown>) => {
    const changedValues = changedConfigValues(flattenConfigValues(formValues), resolvedValues)
    if (Object.keys(changedValues).length > 0)
      save.mutate(changedValues)
  }
  const saveGlobalEnvironment = useMutation({
    mutationFn: () => api.updateBuildEnvironmentConfig(
      { scope: 'global' },
      {
        variables: buildVariableRowsToRecord(environmentVariableRows),
        secrets: buildVariableRowsToRecord(environmentSecretRows),
      },
    ),
    onSuccess: (config) => {
      queryClient.setQueryData(['build-environment-config', 'global'], config)
      setEnvironmentDialogOpen(false)
      toast.success(t('buildsPage.buildEnvironmentSaved'))
    },
    onError: error => toast.error(error.message),
  })
  const openGlobalEnvironment = async () => {
    try {
      const config = await queryClient.fetchQuery({
        queryKey: ['build-environment-config', 'global'],
        queryFn: () => api.getBuildEnvironmentConfig({ scope: 'global' }),
      })
      setEnvironmentVariableRows(buildVariableRecordToRows(config.variables))
      setEnvironmentSecretRows(secretStateToRows(config.secrets))
      setEnvironmentDialogOpen(true)
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('buildsPage.buildEnvironmentLoadFailed'))
    }
  }

  if (definitions.isLoading || (keys.length > 0 && values.isLoading)) {
    return (
      <PageShell spacing="compact" width="settings">
        <SettingsSkeleton />
      </PageShell>
    )
  }

  return (
    <PageShell spacing="compact" width="settings">
      {definitions.isError && <ErrorState title={t('settings.configDefinitionsFailedTitle')} description={t('settings.configDefinitionsFailedDescription')} />}

      <form
        id="site-settings-form"
        onSubmit={form.handleSubmit(submitChangedValues)}
      >
        <ContentTabs
          headerClassName={['billing', 'retention'].includes(activeTab) ? undefined : 'max-w-3xl'}
          tabs={[
            { value: 'brand', label: t('settings.siteConfigTitle') },
            { value: 'registration', label: t('settings.registration.tab') },
            { value: 'security', label: t('settings.securityEgressTitle') },
            { value: 'build', label: t('settings.buildConfigTitle') },
            { value: 'billing', label: t('settings.billingConfigTitle') },
            { value: 'retention', label: t('settings.retentionConfigTitle') },
          ]}
          value={activeTab}
          onValueChange={setActiveTab}
        >
          <TabsContent value="brand">
            <Surface className="max-w-3xl rounded-xl p-6" variant="bordered">
              <ConfigSection definitions={siteDefinitions} form={form} />
            </Surface>
          </TabsContent>
          <TabsContent value="registration">
            <AuthRegistrationSettingsPanel />
          </TabsContent>
          <TabsContent value="security">
            <Surface className="max-w-3xl rounded-xl p-6" variant="bordered">
              <ConfigSection definitions={securityDefinitions} form={form} />
            </Surface>
          </TabsContent>
          <TabsContent value="build">
            <Section
              className="max-w-3xl"
              description={t('buildsPage.globalBuildEnvironmentDescription')}
              title={t('buildsPage.globalBuildEnvironment')}
              tools={(
                <Button type="button" variant="outline" onClick={() => void openGlobalEnvironment()}>
                  <Settings2 className="size-4" />
                  {t('common.edit')}
                </Button>
              )}
              variant="bordered"
            />
          </TabsContent>
          <TabsContent value="billing">
            <div className="grid gap-4">
              <Surface className="p-6" variant="bordered">
                <ConfigSection definitions={billingDefinitions} form={form} />
              </Surface>
              <BillingRateRulesSection />
            </div>
          </TabsContent>
          <TabsContent value="retention">
            <div className="grid gap-4">
              <Section description={t('settings.retentionAutomaticDescription')} title={t('settings.retentionAutomaticTitle')} variant="bordered">
                <ConfigSection definitions={retentionDefinitions} form={form} />
              </Section>
              <DataRetentionSection />
            </div>
          </TabsContent>
        </ContentTabs>
        {!['registration', 'build'].includes(activeTab) && (
          <FormActions className={['brand', 'security'].includes(activeTab) ? 'mt-4 max-w-3xl' : 'mt-4'} separated={false}>
            <Button disabled={save.isPending || !form.formState.isValid || !form.formState.isDirty} type="submit">
              <Save size={16} />
              {t('settings.saveConfig')}
            </Button>
          </FormActions>
        )}
      </form>
      <BuildEnvironmentEditorDialog
        description={t('buildsPage.globalBuildEnvironmentDescription')}
        open={environmentDialogOpen}
        pending={saveGlobalEnvironment.isPending}
        secretRows={environmentSecretRows}
        title={t('buildsPage.globalBuildEnvironment')}
        variableRows={environmentVariableRows}
        onOpenChange={setEnvironmentDialogOpen}
        onSave={() => saveGlobalEnvironment.mutate()}
        onSecretRowsChange={setEnvironmentSecretRows}
        onVariableRowsChange={setEnvironmentVariableRows}
      />
    </PageShell>
  )
}

interface ConfigSectionProps {
  definitions: ConfigDefinition[]
  form: ReturnType<typeof useForm<Record<string, unknown>>>
}

function ConfigSection({ definitions, form }: ConfigSectionProps) {
  const { t } = useTranslation()

  if (definitions.length === 0)
    return null

  return (
    <div className="grid gap-4">
      {definitions.map((definition) => {
        const label = configDefinitionText(definition, 'label', t)
        const description = configDefinitionText(definition, 'description', t)
        const retentionDays = definition.type === 'number' && definition.key.startsWith('retention.')
        const error = form.getFieldState(definition.key, form.formState).error?.message
        return (
          <Field key={definition.key} error={error} hint={description} label={label}>
            {definition.key === siteBrandColorPresetKey
              ? (
                  <BrandColorPresetField
                    ariaLabel={label}
                    options={definition.options}
                    value={String(form.watch(definition.key) || definition.default)}
                    onValueChange={nextValue => form.setValue(definition.key, nextValue, { shouldDirty: true, shouldValidate: true })}
                  />
                )
              : definition.type === 'textarea'
                ? <Textarea className="min-h-28 resize-y font-mono text-sm" {...form.register(definition.key)} />
                : definition.type === 'select' || definition.type === 'boolean'
                  ? <ConfigSelect definition={definition} form={form} options={definition.type === 'boolean' ? ['true', 'false'] : definition.options} />
                  : (
                      <Input
                        aria-invalid={Boolean(error)}
                        inputMode={definition.type === 'number' ? 'numeric' : undefined}
                        max={retentionDays ? 3650 : undefined}
                        min={retentionDays ? 0 : undefined}
                        step={retentionDays ? 1 : undefined}
                        type={definition.type === 'number' ? 'number' : 'text'}
                        {...form.register(definition.key, retentionDays
                          ? { validate: value => validRetentionDays(value) || t('settings.retentionDaysInvalid') }
                          : undefined)}
                      />
                    )}
          </Field>
        )
      })}
    </div>
  )
}

interface DataRetentionInputs {
  datasets: string[]
  startAt: string
  endAt: string
}

interface DataRetentionResultView {
  kind: 'preview' | 'cleanup'
  payload: DataRetentionPayload
  items: DataRetentionResult[]
}

function DataRetentionSection() {
  const { i18n, t } = useTranslation()
  const catalog = useQuery({ queryKey: ['data-retention-catalog'], queryFn: api.getDataRetentionCatalog })
  const [inputs, setInputs] = useState<DataRetentionInputs>({ datasets: [], startAt: '', endAt: '' })
  const [result, setResult] = useState<DataRetentionResultView | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const previewRequestIdRef = useRef(0)
  const payload = useMemo(() => createDataRetentionPayload(inputs), [inputs])
  const payloadKey = payload ? dataRetentionPayloadKey(payload) : ''
  const previewResult = result?.kind === 'preview' && dataRetentionPayloadKey(result.payload) === payloadKey ? result : null
  const previewTotal = previewResult ? sumDataRetentionResults(previewResult.items, 'matched') : 0
  const resultTotal = result ? sumDataRetentionResults(result.items, result.kind === 'preview' ? 'matched' : 'deleted') : 0
  const invalidRange = Boolean(inputs.startAt && inputs.endAt && !validDataRetentionRange(inputs.startAt, inputs.endAt))

  const catalogOptions = useMemo(() => (catalog.data?.items ?? []).map(dataset => ({
    label: t(`settings.retentionDatasetLabels.${dataset.key}`, { defaultValue: dataset.key }),
    value: dataset.key,
  })), [catalog.data, t])
  const resultColumns = useMemo<DataListColumn<DataRetentionResult>[]>(() => [
    {
      key: 'dataset',
      header: t('settings.retentionDatasetColumn'),
      width: 'primary',
      render: item => <span className="font-medium text-foreground">{t(`settings.retentionDatasetLabels.${item.dataset}`, { defaultValue: item.dataset })}</span>,
    },
    {
      key: 'matchedCount',
      header: t('settings.retentionMatchedColumn'),
      width: 'number',
      render: item => item.matched.toLocaleString(i18n.resolvedLanguage),
    },
    {
      key: 'deletedCount',
      header: t('settings.retentionDeletedColumn'),
      width: 'number',
      render: item => item.deleted.toLocaleString(i18n.resolvedLanguage),
    },
  ], [i18n.resolvedLanguage, t])

  const preview = useMutation({
    mutationFn: ({ requestPayload }: { requestId: number, requestPayload: DataRetentionPayload }) => api.previewDataRetention(requestPayload),
    onSuccess: (response, variables) => {
      if (variables.requestId === previewRequestIdRef.current)
        setResult({ kind: 'preview', payload: variables.requestPayload, items: response.items })
    },
    onError: (error, variables) => {
      if (variables.requestId === previewRequestIdRef.current)
        toast.error(dataRetentionErrorMessage(error, t, 'preview'))
    },
  })
  const cleanup = useMutation({
    mutationFn: api.cleanupDataRetention,
    onSuccess: (response, requestPayload) => {
      const deleted = sumDataRetentionResults(response.items, 'deleted')
      toast.success(t('settings.retentionDeletedTotal', { count: deleted }))
      setResult({ kind: 'cleanup', payload: requestPayload, items: response.items })
      setConfirmOpen(false)
    },
    onError: error => toast.error(dataRetentionErrorMessage(error, t, 'cleanup')),
  })

  const updateInputs = (nextInputs: DataRetentionInputs) => {
    previewRequestIdRef.current += 1
    setInputs(nextInputs)
    setResult(null)
    setConfirmOpen(false)
  }
  const runPreview = () => {
    if (!payload)
      return
    const requestId = previewRequestIdRef.current + 1
    previewRequestIdRef.current = requestId
    setResult(null)
    preview.mutate({ requestId, requestPayload: payload })
  }

  if (catalog.isError)
    return <ErrorState title={t('settings.retentionCatalogFailedTitle')} description={t('settings.retentionCatalogFailedDescription')} />

  return (
    <div className="grid gap-4">
      <Section description={t('settings.retentionManualDescription')} title={t('settings.retentionManualTitle')} variant="bordered">
        <p className="rounded-md bg-surface-inset px-3 py-2 text-sm leading-6 text-muted-foreground">
          {t('settings.retentionProtectedNote')}
        </p>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="md:col-span-2">
            <Field hint={t('settings.retentionDatasetsHint')} label={t('settings.retentionDatasets')} required>
              <SearchMultiSelect
                ariaLabel={t('settings.retentionDatasets')}
                disabled={catalog.isLoading || cleanup.isPending}
                emptyLabel={t('settings.retentionDatasetsEmpty')}
                loading={catalog.isLoading}
                options={catalogOptions}
                placeholder={t('settings.retentionDatasetsPlaceholder')}
                searchPlaceholder={t('settings.retentionDatasetsSearchPlaceholder')}
                selectedLabel={options => t('settings.retentionDatasetsSelected', { count: options.length })}
                value={inputs.datasets}
                onValueChange={datasets => updateInputs({ ...inputs, datasets })}
              />
            </Field>
          </div>
          <Field label={t('settings.retentionStartAt')} required>
            <Input
              aria-label={t('settings.retentionStartAt')}
              disabled={cleanup.isPending}
              step="60"
              type="datetime-local"
              value={inputs.startAt}
              onChange={event => updateInputs({ ...inputs, startAt: event.target.value })}
            />
          </Field>
          <Field error={invalidRange ? t('settings.retentionRangeInvalid') : undefined} label={t('settings.retentionEndAt')} required>
            <Input
              aria-label={t('settings.retentionEndAt')}
              disabled={cleanup.isPending}
              step="60"
              type="datetime-local"
              value={inputs.endAt}
              onChange={event => updateInputs({ ...inputs, endAt: event.target.value })}
            />
          </Field>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Button disabled={!payload || preview.isPending || cleanup.isPending} type="button" variant="secondary" onClick={runPreview}>
            <Eye size={16} />
            {t('settings.retentionPreview')}
          </Button>
          <Button disabled={!previewResult || preview.isPending || cleanup.isPending} type="button" variant="destructive" onClick={() => setConfirmOpen(true)}>
            <Trash2 size={16} />
            {t('settings.retentionCleanup')}
          </Button>
        </div>
      </Section>

      {result && (
        <div className="grid gap-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h3 className="text-base font-semibold text-foreground">
              {t(result.kind === 'preview' ? 'settings.retentionPreviewResults' : 'settings.retentionCleanupResults')}
            </h3>
            <span className="text-sm text-muted-foreground">
              {result.kind === 'preview'
                ? t('settings.retentionPreviewTotal', { count: resultTotal })
                : t('settings.retentionDeletedTotal', { count: resultTotal })}
            </span>
          </div>
          <DataList
            columns={resultColumns}
            emptyTitle={t(result.kind === 'preview' ? 'settings.retentionPreviewResults' : 'settings.retentionCleanupResults')}
            items={result.items}
            rowKey={item => item.dataset}
          />
        </div>
      )}

      <ConfirmDialog
        cancelText={t('common.cancel')}
        closeOnConfirm={false}
        confirmDisabled={!previewResult}
        confirmText={t('settings.retentionCleanup')}
        content={previewResult && (
          <dl className="grid gap-3 rounded-md border border-border p-3 text-sm">
            <div className="grid gap-1">
              <dt className="text-muted-foreground">{t('settings.retentionConfirmRange')}</dt>
              <dd className="font-medium text-foreground">
                {t('settings.retentionRangeValue', {
                  start: formatDataRetentionDateTime(previewResult.payload.startAt, i18n.resolvedLanguage),
                  end: formatDataRetentionDateTime(previewResult.payload.endAt, i18n.resolvedLanguage),
                })}
              </dd>
            </div>
            <div className="grid gap-1">
              <dt className="text-muted-foreground">{t('settings.retentionConfirmTotal')}</dt>
              <dd className="font-medium text-foreground">{previewTotal.toLocaleString(i18n.resolvedLanguage)}</dd>
            </div>
          </dl>
        )}
        description={t('settings.retentionConfirmDescription')}
        open={confirmOpen}
        pending={cleanup.isPending}
        title={t('settings.retentionConfirmTitle')}
        onConfirm={() => {
          if (previewResult)
            cleanup.mutate(previewResult.payload)
        }}
        onOpenChange={setConfirmOpen}
      />
    </div>
  )
}

function BillingRateRulesSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const rateRules = useQuery({ queryKey: ['billing-rate-rules'], queryFn: api.listBillingRateRules })
  const [drafts, setDrafts] = useState<Record<string, BillingRateRulePayload>>({})

  const save = useMutation({
    mutationFn: (rules: BillingRateRulePayload[]) => api.updateBillingRateRules(rules),
    onSuccess: (rules) => {
      toast.success(t('settings.billingRateRulesSaved'))
      setDrafts({})
      queryClient.setQueryData(['billing-rate-rules'], rules)
      queryClient.invalidateQueries({ queryKey: ['billing-rate-rules'] })
    },
    onError: error => toast.error(error.message),
  })

  const columns = useMemo<DataListColumn<BillingRateRule>[]>(() => [
    {
      key: 'meter',
      header: t('settings.billingRateMeter'),
      className: 'min-w-44',
      render: rule => <span className="font-mono text-xs text-foreground">{rule.meter}</span>,
    },
    {
      key: 'unit',
      header: t('settings.billingRateUnit'),
      className: 'min-w-32',
      render: rule => (
        <span className="text-sm text-muted-foreground" title={rule.unit}>
          {t(`settings.billingRateUnits.${rule.unit}`, { defaultValue: rule.unit })}
        </span>
      ),
    },
    {
      key: 'price',
      header: t('settings.billingRatePrice'),
      className: 'w-44',
      render: (rule) => {
        const draft = drafts[rule.meter] ?? billingRateRulePayloadFromRule(rule)
        return (
          <Input
            className="w-36"
            inputMode="decimal"
            min="0"
            step="0.0001"
            type="number"
            value={draft.creditsPerUnit}
            onChange={event => setDrafts(current => ({ ...current, [rule.meter]: { ...draft, creditsPerUnit: event.target.value } }))}
          />
        )
      },
    },
    {
      key: 'enabled',
      header: t('settings.billingRateEnabled'),
      className: 'w-36',
      render: (rule) => {
        const draft = drafts[rule.meter] ?? billingRateRulePayloadFromRule(rule)
        return (
          <Select value={String(draft.enabled)} onValueChange={nextValue => setDrafts(current => ({ ...current, [rule.meter]: { ...draft, enabled: nextValue === 'true' } }))}>
            <SelectTrigger className="w-28">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="true">{t('common.enabled')}</SelectItem>
              <SelectItem value="false">{t('common.disabled')}</SelectItem>
            </SelectContent>
          </Select>
        )
      },
    },
    {
      key: 'description',
      header: t('settings.billingRateDescription'),
      className: 'min-w-80',
      render: rule => (
        <span className="text-muted-foreground">
          {t(`settings.billingRateRuleDescriptions.${rule.meter}`, { defaultValue: rule.description })}
        </span>
      ),
    },
  ], [drafts, t])
  if (rateRules.isError)
    return <ErrorState title={t('settings.billingRateRulesFailedTitle')} description={t('settings.billingRateRulesFailedDescription')} />

  const rules = rateRules.data ?? []
  const payload = rules
    .map(rule => drafts[rule.meter] ?? billingRateRulePayloadFromRule(rule))
    .filter((rule): rule is BillingRateRulePayload => Boolean(rule))

  return (
    <Section
      title={t('settings.billingRateRulesTitle')}
      tools={(
        <Button disabled={save.isPending || rules.length === 0} type="button" onClick={() => save.mutate(payload)}>
          <Save size={16} />
          {t('settings.saveBillingRateRules')}
        </Button>
      )}
    >
      <DataList
        columns={columns}
        emptyTitle={t('settings.billingRateRulesTitle')}
        items={rules}
        rowKey={rule => rule.meter}
      />
    </Section>
  )
}

function billingRateRulePayloadFromRule(rule: BillingRateRule): BillingRateRulePayload {
  return {
    meter: rule.meter,
    creditsPerUnit: rule.creditsPerUnit,
    enabled: rule.enabled,
  }
}

function ConfigSelect({ definition, form, options }: { definition: ConfigSectionProps['definitions'][number], form: ConfigSectionProps['form'], options?: string[] }) {
  const { t } = useTranslation()
  const value = form.watch(definition.key) as string | undefined

  return (
    <Select value={value || definition.default} onValueChange={nextValue => form.setValue(definition.key, nextValue, { shouldDirty: true, shouldValidate: true })}>
      <SelectTrigger className="w-full">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {(options ?? []).map(option => (
          <SelectItem key={option} value={option}>
            {option === 'true' ? t('common.enabled') : option === 'false' ? t('common.disabled') : option}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function unflattenConfigValues(values: Record<string, string>) {
  const result: Record<string, unknown> = {}

  for (const [key, value] of Object.entries(values)) {
    const parts = key.split('.')
    let cursor = result

    for (const part of parts.slice(0, -1)) {
      if (!cursor[part] || typeof cursor[part] !== 'object')
        cursor[part] = {}
      cursor = cursor[part] as Record<string, unknown>
    }

    cursor[parts[parts.length - 1]] = value
  }

  return result
}

function flattenConfigValues(values: Record<string, unknown>, prefix = '') {
  const result: Record<string, unknown> = {}

  for (const [key, value] of Object.entries(values)) {
    const nextKey = prefix ? `${prefix}.${key}` : key
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      Object.assign(result, flattenConfigValues(value as Record<string, unknown>, nextKey))
      continue
    }

    result[nextKey] = value ?? ''
  }

  return result
}

function validRetentionDays(value: unknown) {
  const normalized = String(value ?? '').trim()
  if (!/^\d+$/.test(normalized))
    return false
  const days = Number.parseInt(normalized, 10)
  return days >= 0 && days <= 3650
}

function validDataRetentionRange(startAt: string, endAt: string) {
  const start = new Date(startAt)
  const end = new Date(endAt)
  return Number.isFinite(start.getTime()) && Number.isFinite(end.getTime()) && start < end
}

function createDataRetentionPayload(inputs: DataRetentionInputs): DataRetentionPayload | null {
  if (inputs.datasets.length === 0 || !validDataRetentionRange(inputs.startAt, inputs.endAt))
    return null

  return {
    datasets: [...new Set(inputs.datasets)],
    startAt: new Date(inputs.startAt).toISOString(),
    endAt: new Date(inputs.endAt).toISOString(),
  }
}

function dataRetentionPayloadKey(payload: DataRetentionPayload) {
  return JSON.stringify(payload)
}

function sumDataRetentionResults(items: DataRetentionResult[], field: 'matched' | 'deleted') {
  return items.reduce((total, item) => total + item[field], 0)
}

function formatDataRetentionDateTime(value: string, locale?: string) {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

function dataRetentionErrorMessage(error: unknown, t: TFunction, operation: 'preview' | 'cleanup') {
  const code = error && typeof error === 'object' && 'code' in error ? String(error.code) : ''
  if (code === 'retention.invalid_range')
    return t('settings.retentionRangeInvalid')
  if (code === 'retention.invalid_dataset')
    return t('settings.retentionInvalidDataset')
  return t(operation === 'preview' ? 'settings.retentionPreviewFailed' : 'settings.retentionCleanupFailed')
}
