import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api/client'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { PageHeader } from '@/components/common/page-header'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

export function SiteSettingsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
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
    <div className="grid gap-6">
      <PageHeader
        description={t('settings.siteDescription')}
        title={t('siteSettings')}
      />

      {definitions.isError && <ErrorState title={t('settings.configDefinitionsFailedTitle')} description={t('settings.configDefinitionsFailedDescription')} />}

      <Card className="max-w-3xl">
        <form
          className="grid gap-4"
          onSubmit={form.handleSubmit(formValues => save.mutate(flattenConfigValues(formValues)))}
        >
          <ConfigSection definitions={siteDefinitions} form={form} title={t('settings.siteConfigTitle')} />
          <ConfigSection definitions={securityDefinitions} form={form} title={t('settings.securityEgressTitle')} />

          <Button className="w-fit" disabled={save.isPending || !form.formState.isValid} type="submit">
            <Save size={16} />
            {t('settings.saveConfig')}
          </Button>
        </form>
      </Card>
    </div>
  )
}

interface ConfigSectionProps {
  definitions: Array<{ key: string, label: string, description: string, type: 'string' | 'textarea' }>
  form: ReturnType<typeof useForm<Record<string, unknown>>>
  title: string
}

function ConfigSection({ definitions, form, title }: ConfigSectionProps) {
  if (definitions.length === 0)
    return null

  return (
    <section className="grid gap-4">
      <h2 className="text-sm font-semibold text-foreground">{title}</h2>
      {definitions.map(definition => (
        <Field key={definition.key} hint={definition.description} label={definition.label}>
          {definition.type === 'textarea'
            ? <Textarea className="min-h-28 resize-y font-mono text-sm" {...form.register(definition.key)} />
            : <Input {...form.register(definition.key)} />}
          <p className="text-xs font-normal text-muted-foreground">
            {definition.key}
            {' · '}
            {definition.description}
          </p>
        </Field>
      ))}
    </section>
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
