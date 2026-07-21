import type { UseFormReturn } from 'react-hook-form'
import type { BuildTemplate, BuildTemplatePreview, DeploymentTargetPayload } from '@/api'
import { useMutation } from '@tanstack/react-query'
import { FileCode2, Sparkles } from 'lucide-react'
import { useMemo, useState } from 'react'
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
import { NativeSelect } from '@/components/ui/native-select'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

export function ApplicationBuildTemplateFields({
  dockerfileSuggestions,
  form,
  recommendedTemplateIds,
  templates,
}: {
  dockerfileSuggestions: string[]
  form: UseFormReturn<DeploymentTargetPayload>
  recommendedTemplateIds: string[]
  templates: BuildTemplate[]
}) {
  const { t } = useTranslation()
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
    form.setValue('buildTemplateValues', JSON.stringify(defaultTemplateValues(next)), { shouldDirty: true })
  }

  const switchMode = (nextMode: DeploymentTargetPayload['buildDefinitionMode']) => {
    form.setValue('buildDefinitionMode', nextMode, { shouldDirty: true, shouldValidate: true })
    if (nextMode !== 'template' || selectedTemplate)
      return
    selectTemplate(recommendedTemplateIds.find(id => templates.some(item => item.id === id)) ?? templates[0]?.id ?? '')
  }

  const updateValue = (key: string, value: string) => {
    form.setValue('buildTemplateValues', JSON.stringify({ ...templateValues, [key]: value }), { shouldDirty: true, shouldValidate: true })
  }

  return (
    <div className="grid gap-3 md:col-span-2">
      <Field hint={t('buildTemplates.modeHint')} label={t('buildTemplates.mode')} required>
        <NativeSelect value={mode} onChange={event => switchMode(event.target.value as DeploymentTargetPayload['buildDefinitionMode'])}>
          <option value="repository_dockerfile">{t('buildTemplates.repositoryDockerfile')}</option>
          <option value="template">{t('buildTemplates.platformTemplate')}</option>
        </NativeSelect>
      </Field>

      {mode === 'repository_dockerfile' && dockerfileSuggestions.length === 0 && (
        <Alert className="md:col-span-2">
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
          <Field hint={t('buildTemplates.templateHint')} label={t('buildTemplates.template')} required>
            <Select value={templateID || undefined} onValueChange={selectTemplate}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder={t('common.select')} />
              </SelectTrigger>
              <SelectContent position="popper">
                {orderedTemplates.map((item) => {
                  return (
                    <SelectItem key={`${item.id}:${item.version}`} value={item.id}>
                      {buildTemplateIcon(item)}
                      <span>{t(`buildTemplates.names.${item.id}`)}</span>
                      {recommendedTemplateIds.includes(item.id) && <Badge className="ml-auto" variant="secondary">{t('buildTemplates.recommended')}</Badge>}
                    </SelectItem>
                  )
                })}
              </SelectContent>
            </Select>
          </Field>
          {selectedTemplate?.id === 'nextjs-service' && (
            <Alert className="md:col-span-2">
              <FileCode2 className="size-4" />
              <AlertTitle>{t('buildTemplates.nextjsStandaloneTitle')}</AlertTitle>
              <AlertDescription>{t('buildTemplates.nextjsStandaloneDescription')}</AlertDescription>
            </Alert>
          )}
          {selectedTemplate && (
            <div className="flex min-w-0 items-end gap-2">
              <div className="grid min-w-0 flex-1 gap-1">
                <span className="text-sm font-medium">{t('buildTemplates.selectedVersion')}</span>
                <div className="flex h-9 items-center gap-2 rounded-md border border-input bg-muted/40 px-3">
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
          {selectedTemplate?.parameters.map(parameter => (
            <Field key={parameter.key} label={t(`buildTemplates.parameters.${parameter.key}`)} required={parameter.required}>
              {parameter.type === 'select'
                ? (
                    <NativeSelect value={templateValues[parameter.key] ?? parameter.defaultValue} onChange={event => updateValue(parameter.key, event.target.value)}>
                      {(parameter.options ?? []).map(option => <option key={option} value={option}>{option}</option>)}
                    </NativeSelect>
                  )
                : (
                    <Input
                      inputMode={parameter.type === 'port' ? 'numeric' : undefined}
                      value={templateValues[parameter.key] ?? parameter.defaultValue}
                      onChange={event => updateValue(parameter.key, event.target.value)}
                    />
                  )}
            </Field>
          ))}
          <p className="text-xs text-muted-foreground md:col-span-2">{t('buildTemplates.overrideNotice')}</p>
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
