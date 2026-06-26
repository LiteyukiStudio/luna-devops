import type { BillingRateRule, BillingRateRulePayload, ConfigDefinition } from '@/api/types'
import type { DataListColumn } from '@/components/common/data-list'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ContentTabs } from '@/components/common/content-tabs'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { TabsContent } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'

export function SiteSettingsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('brand')
  const form = useForm<Record<string, unknown>>({ mode: 'onChange', defaultValues: {} })
  const definitions = useQuery({ queryKey: ['config-definitions'], queryFn: api.listConfigDefinitions })
  const keys = useMemo(() => (definitions.data ?? []).map(definition => definition.key), [definitions.data])
  const values = useQuery({
    queryKey: ['configs'],
    queryFn: api.getConfigs,
    enabled: keys.length > 0,
  })
  const siteDefinitions = useMemo(() => (definitions.data ?? []).filter(definition => definition.key.startsWith('site.')), [definitions.data])
  const securityDefinitions = useMemo(() => (definitions.data ?? []).filter(definition => definition.key.startsWith('security.')), [definitions.data])
  const billingDefinitions = useMemo(() => (definitions.data ?? []).filter(definition => definition.key.startsWith('billing.')), [definitions.data])
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
      queryClient.setQueryData(['configs'], result)
      queryClient.invalidateQueries({ queryKey: ['configs'] })
      queryClient.invalidateQueries({ queryKey: ['public-configs'] })
    },
    onError: error => toast.error(error.message),
  })

  return (
    <div className="grid gap-4">
      {definitions.isError && <ErrorState title={t('settings.configDefinitionsFailedTitle')} description={t('settings.configDefinitionsFailedDescription')} />}

      <form
        id="site-settings-form"
        onSubmit={form.handleSubmit(formValues => save.mutate(flattenConfigValues(formValues)))}
      >
        <ContentTabs
          tabs={[
            { value: 'brand', label: t('settings.siteConfigTitle') },
            { value: 'security', label: t('settings.securityEgressTitle') },
            { value: 'billing', label: t('settings.billingConfigTitle') },
          ]}
          tools={(
            <Button disabled={save.isPending || !form.formState.isValid} form="site-settings-form" type="submit">
              <Save size={16} />
              {t('settings.saveConfig')}
            </Button>
          )}
          value={activeTab}
          onValueChange={setActiveTab}
        >
          <TabsContent value="brand">
            <Card className="max-w-3xl p-4">
              <ConfigSection definitions={siteDefinitions} form={form} />
            </Card>
          </TabsContent>
          <TabsContent value="security">
            <Card className="max-w-3xl p-4">
              <ConfigSection definitions={securityDefinitions} form={form} />
            </Card>
          </TabsContent>
          <TabsContent value="billing">
            <div className="grid max-w-5xl gap-4">
              <Card className="p-4">
                <ConfigSection definitions={billingDefinitions} form={form} />
              </Card>
              <BillingRateRulesSection />
            </div>
          </TabsContent>
        </ContentTabs>
      </form>
    </div>
  )
}

interface ConfigSectionProps {
  definitions: ConfigDefinition[]
  form: ReturnType<typeof useForm<Record<string, unknown>>>
}

function ConfigSection({ definitions, form }: ConfigSectionProps) {
  if (definitions.length === 0)
    return null

  return (
    <div className="grid gap-4">
      {definitions.map(definition => (
        <Field key={definition.key} hint={definition.description} label={definition.label}>
          {definition.type === 'textarea'
            ? <Textarea className="min-h-28 resize-y font-mono text-sm" {...form.register(definition.key)} />
            : definition.type === 'select'
              ? <ConfigSelect definition={definition} form={form} />
              : <Input {...form.register(definition.key)} />}
          <p className="text-xs font-normal text-muted-foreground">
            {definition.key}
            {' · '}
            {definition.description}
          </p>
        </Field>
      ))}
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
      render: rule => <span className="font-mono text-xs text-muted-foreground">{rule.unit}</span>,
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
    <div className="grid gap-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="text-base font-semibold text-foreground">{t('settings.billingRateRulesTitle')}</h3>
          <p className="mt-1 text-sm text-muted-foreground">{t('settings.billingRateRulesDescription')}</p>
        </div>
        <Button disabled={save.isPending || rules.length === 0} type="button" onClick={() => save.mutate(payload)}>
          <Save size={16} />
          {t('settings.saveBillingRateRules')}
        </Button>
      </div>
      <DataList
        columns={columns}
        emptyTitle={t('settings.billingRateRulesTitle')}
        emptyDescription={t('settings.billingRateRulesDescription')}
        items={rules}
        rowKey={rule => rule.meter}
      />
    </div>
  )
}

function billingRateRulePayloadFromRule(rule: BillingRateRule): BillingRateRulePayload {
  return {
    meter: rule.meter,
    creditsPerUnit: rule.creditsPerUnit,
    enabled: rule.enabled,
  }
}

function ConfigSelect({ definition, form }: { definition: ConfigSectionProps['definitions'][number], form: ConfigSectionProps['form'] }) {
  const value = form.watch(definition.key) as string | undefined

  return (
    <Select value={value || definition.default} onValueChange={nextValue => form.setValue(definition.key, nextValue, { shouldDirty: true, shouldValidate: true })}>
      <SelectTrigger className="w-full">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {(definition.options ?? []).map(option => (
          <SelectItem key={option} value={option}>{option}</SelectItem>
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
