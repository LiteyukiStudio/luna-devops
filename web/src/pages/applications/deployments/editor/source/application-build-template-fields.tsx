import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { BuildTemplate, BuildTemplateParameter, BuildTemplatePreview } from '@/api'
import { useMutation } from '@tanstack/react-query'
import { FileCode2, RotateCw, Sparkles } from 'lucide-react'
import { useMemo, useState } from 'react'
import { get, useFormContext, useFormState } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { CodeEditor } from '@/components/common/code-editor'
import { FormField as Field } from '@/components/common/form-field'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { EMPTY_SELECT_VALUE, fromSelectValue, toSelectValue } from '@/components/ui/select-value'
import { Skeleton } from '@/components/ui/skeleton'
import { useFieldError } from '../deployment-target-editor-utils'

export function ApplicationBuildTemplateFields({
  onRetryTemplates,
  recommendedTemplateIds,
  showNoDockerfileNotice = false,
  templates,
  templatesError = false,
  templatesLoading = false,
  templatesRetrying = false,
}: {
  dockerfileSuggestions?: string[]
  onRetryTemplates?: () => void
  recommendedTemplateIds: string[]
  showNoDockerfileNotice?: boolean
  templates: BuildTemplate[]
  templatesError?: boolean
  templatesLoading?: boolean
  templatesRetrying?: boolean
}) {
  const { t } = useTranslation()
  const form = useFormContext<DeploymentTargetFormValues>()
  const templateError = useFieldError('buildTemplateId')
  const templateValuesRootError = useFieldError('buildTemplateValues')
  const [preview, setPreview] = useState<BuildTemplatePreview | null>(null)
  const mode = form.watch('buildDefinitionMode') || 'repository_dockerfile'
  const templateID = form.watch('buildTemplateId')
  const templateValuesRaw = form.watch('buildTemplateValues')
  const selectedTemplate = templates.find(item => item.id === templateID)
  const templateValues = useMemo(() => parseTemplateValues(templateValuesRaw), [templateValuesRaw])
  const orderedTemplates = useMemo(() => [...templates].sort((left, right) => {
    const leftIndex = recommendedTemplateIds.indexOf(left.id)
    const rightIndex = recommendedTemplateIds.indexOf(right.id)
    if (leftIndex === -1 && rightIndex === -1)
      return left.id.localeCompare(right.id)
    if (leftIndex === -1)
      return 1
    if (rightIndex === -1)
      return -1
    return leftIndex - rightIndex
  }), [recommendedTemplateIds, templates])
  const previewTemplate = useMutation({
    mutationFn: () => api.previewBuildTemplate(selectedTemplate?.id ?? '', selectedTemplate?.version ?? '', templateValues),
    onSuccess: setPreview,
    onError: () => toast.error(t('buildTemplates.previewFailed')),
  })

  const selectTemplate = (nextID: string) => {
    const next = templates.find(item => item.id === nextID)
    form.setValue('buildTemplateId', next?.id ?? '', { shouldDirty: true, shouldValidate: true })
    form.setValue('buildTemplateVersion', next?.version ?? '', { shouldDirty: true })
    form.setValue('buildTemplateValues', JSON.stringify(defaultTemplateValues(next)), { shouldDirty: true, shouldValidate: form.formState.isSubmitted })
  }

  const switchMode = (nextMode: DeploymentTargetFormValues['buildDefinitionMode']) => {
    form.setValue('buildDefinitionMode', nextMode, { shouldDirty: true, shouldValidate: true })
    if (nextMode === 'template' && !selectedTemplate && templates.length > 0)
      selectTemplate(recommendedTemplateIds.find(id => templates.some(item => item.id === id)) ?? templates[0]?.id ?? '')
    if (form.formState.isSubmitted)
      void form.trigger(['dockerfilePath', 'buildTemplateId', 'buildTemplateValues'])
  }

  const updateValue = (key: string, value: string) => {
    form.setValue('buildTemplateValues', JSON.stringify({ ...defaultTemplateValues(selectedTemplate), ...templateValues, [key]: value }), {
      shouldDirty: true,
      shouldValidate: true,
    })
  }

  return (
    <div className="grid gap-4 @xl/workspace:col-span-2 @xl/workspace:grid-cols-2">
      <Field className="@xl/workspace:col-span-2" hint={t('buildTemplates.modeHint')} label={t('buildTemplates.mode')} required>
        {control => (
          <Select value={mode} onValueChange={value => switchMode(value === 'template' ? 'template' : 'repository_dockerfile')}>
            <SelectTrigger {...control} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="repository_dockerfile">{t('buildTemplates.repositoryDockerfile')}</SelectItem>
              <SelectItem value="template">{t('buildTemplates.platformTemplate')}</SelectItem>
            </SelectContent>
          </Select>
        )}
      </Field>

      {mode === 'repository_dockerfile' && showNoDockerfileNotice && (
        <Alert className="@xl/workspace:col-span-2">
          <Sparkles className="size-4" />
          <AlertTitle>{t('buildTemplates.noDockerfileTitle')}</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
            <span>{t('buildTemplates.noDockerfileDescription')}</span>
            <Button size="sm" type="button" variant="secondary" onClick={() => switchMode('template')}>
              {t('buildTemplates.useTemplate')}
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {mode === 'template' && (
        <>
          {templatesLoading
            ? <Skeleton className="h-9 w-full rounded-control @xl/workspace:col-span-2" />
            : templatesError
              ? (
                  <div className="flex flex-wrap items-center justify-between gap-2 rounded-control border border-danger-border bg-danger-subtle px-3 py-2 text-xs text-danger @xl/workspace:col-span-2" role="alert">
                    <span>{t('deploymentTargetEditor.buildTemplatesLoadFailed')}</span>
                    {onRetryTemplates && (
                      <Button disabled={templatesRetrying} size="sm" type="button" variant="outline" onClick={onRetryTemplates}>
                        <RotateCw className="size-3.5" />
                        {t('common.retry')}
                      </Button>
                    )}
                  </div>
                )
              : (
                  <Field error={templateError} hint={t('buildTemplates.templateHint')} label={t('buildTemplates.template')} required>
                    {control => (
                      <Select value={templateID || undefined} onValueChange={selectTemplate}>
                        <SelectTrigger {...control} className="w-full">
                          <SelectValue placeholder={orderedTemplates.length === 0 ? t('common.noOptions') : t('common.select')} />
                        </SelectTrigger>
                        <SelectContent position="popper">
                          {orderedTemplates.length === 0
                            ? <SelectItem disabled value="__empty__">{t('common.noOptions')}</SelectItem>
                            : orderedTemplates.map(item => (
                                <SelectItem key={`${item.id}:${item.version}`} value={item.id}>
                                  {buildTemplateIcon(item)}
                                  <span>{t(`buildTemplates.names.${item.id}`)}</span>
                                  {recommendedTemplateIds.includes(item.id) && <Badge className="ml-auto" variant="secondary">{t('buildTemplates.recommended')}</Badge>}
                                </SelectItem>
                              ))}
                        </SelectContent>
                      </Select>
                    )}
                  </Field>
                )}
          {selectedTemplate?.id === 'nextjs-service' && (
            <Alert className="@xl/workspace:col-span-2">
              <FileCode2 className="size-4" />
              <AlertTitle>{t('buildTemplates.nextjsStandaloneTitle')}</AlertTitle>
              <AlertDescription>{t('buildTemplates.nextjsStandaloneDescription')}</AlertDescription>
            </Alert>
          )}
          {selectedTemplate && (
            <div className="flex min-w-0 items-end gap-2">
              <div className="grid min-w-0 flex-1 gap-1.5">
                <span className="text-sm font-medium">{t('buildTemplates.selectedVersion')}</span>
                <div className="flex h-9 items-center gap-2 rounded-control border border-border bg-surface-inset px-3">
                  <Badge variant="secondary">
                    v
                    {selectedTemplate.version}
                  </Badge>
                  <span className="truncate text-sm text-muted-foreground">{t(`buildTemplates.descriptions.${selectedTemplate.id}`)}</span>
                </div>
              </div>
              <Button disabled={previewTemplate.isPending} type="button" variant="outline" onClick={() => previewTemplate.mutate()}>
                <FileCode2 className="size-4" />
                {t('buildTemplates.preview')}
              </Button>
            </div>
          )}
          {templateValuesRootError && (
            <p className="text-xs text-danger @xl/workspace:col-span-2" role="alert">{templateValuesRootError}</p>
          )}
          {selectedTemplate?.parameters.map(parameter => (
            <TemplateParameterField
              key={parameter.key}
              parameter={parameter}
              value={Object.hasOwn(templateValues, parameter.key) ? templateValues[parameter.key] : parameter.defaultValue}
              onChange={nextValue => updateValue(parameter.key, nextValue)}
            />
          ))}
          <p className="text-xs text-muted-foreground @xl/workspace:col-span-2">{t('buildTemplates.overrideNotice')}</p>
        </>
      )}

      <Dialog open={Boolean(preview)} onOpenChange={open => !open && setPreview(null)}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{t('buildTemplates.previewTitle')}</DialogTitle>
            <DialogDescription>{t('buildTemplates.previewDescription', { checksum: preview?.checksum.slice(0, 12) })}</DialogDescription>
          </DialogHeader>
          <CodeEditor readOnly height="28rem" language="text" value={preview?.dockerfile ?? ''} onChange={() => {}} />
        </DialogContent>
      </Dialog>
    </div>
  )
}

function TemplateParameterField({
  onChange,
  parameter,
  value,
}: {
  onChange: (value: string) => void
  parameter: BuildTemplateParameter
  value: string
}) {
  const { t } = useTranslation()
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const { errors } = useFormState({ control, name: 'buildTemplateValues' })
  const rawError: unknown = get(errors, `buildTemplateValues.${parameter.key}`)
  const error = rawError && typeof rawError === 'object' && 'message' in rawError && typeof rawError.message === 'string'
    ? rawError.message
    : undefined

  const options = parameter.options ?? []
  const hasEmptyOption = options.includes('')

  return (
    <Field error={error} label={t(`buildTemplates.parameters.${parameter.key}`)} required={parameter.required}>
      {fieldControl => parameter.type === 'select'
        ? (
            <Select
              value={hasEmptyOption ? toSelectValue(value) : (value || undefined)}
              onValueChange={next => onChange(hasEmptyOption ? fromSelectValue(next) : next)}
            >
              <SelectTrigger {...fieldControl} className="w-full">
                <SelectValue placeholder={hasEmptyOption ? undefined : t('common.select')} />
              </SelectTrigger>
              <SelectContent>
                {options.map(option => (
                  <SelectItem key={option || EMPTY_SELECT_VALUE} value={hasEmptyOption ? toSelectValue(option) : option}>
                    {option}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )
        : (
            <Input
              {...fieldControl}
              className="font-mono"
              inputMode={parameter.type === 'port' ? 'numeric' : undefined}
              value={value}
              onChange={event => onChange(event.target.value)}
            />
          )}
    </Field>
  )
}

function defaultTemplateValues(template?: BuildTemplate) {
  return Object.fromEntries((template?.parameters ?? []).map(parameter => [parameter.key, parameter.defaultValue]))
}

function parseTemplateValues(raw?: string) {
  try {
    const value = JSON.parse(raw || '{}')
    return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, string> : {}
  }
  catch {
    return {}
  }
}

const buildTemplateLogoByRuntime: Partial<Record<BuildTemplate['runtime'], string>> = {
  bun: '/build-templates/icons/bun.svg',
  dotnet: '/build-templates/icons/dotnet.svg',
  go: '/build-templates/icons/go.svg',
  java: '/build-templates/icons/java.png',
  nextjs: '/build-templates/icons/nextjs.svg',
  node: '/build-templates/icons/nodejs.svg',
  python: '/build-templates/icons/python.png',
  ruby: '/build-templates/icons/ruby.svg',
  rust: '/build-templates/icons/rust.svg',
}

function buildTemplateIcon(template: BuildTemplate) {
  const source = template.id === 'static-site'
    ? '/build-templates/icons/html5.svg'
    : buildTemplateLogoByRuntime[template.runtime]

  if (!source)
    return null

  const containerClassName = template.id === 'nextjs-service'
    ? 'flex h-5 w-12 shrink-0 items-center justify-center rounded-sm bg-white p-0.5 ring-1 ring-border/60'
    : 'flex size-5 shrink-0 items-center justify-center rounded-sm bg-white p-0.5 ring-1 ring-border/60'

  return (
    <span aria-hidden="true" className={containerClassName}>
      <img alt="" className="max-h-full max-w-full object-contain" src={source} />
    </span>
  )
}
