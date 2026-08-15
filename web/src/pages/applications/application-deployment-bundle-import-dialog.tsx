import type { DeploymentBundleReferenceResolution, DeploymentTargetBundle, DeploymentTargetBundlePreview } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { AlertTriangle, FileJson2, RefreshCw, Upload } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { api } from '@/api'
import { StatusBadge } from '@/components/common/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

import '@/i18n/lazy/deployment-bundle'

const DEPLOYMENT_BUNDLE_MAX_BYTES = 1024 * 1024

const importFormSchema = z.object({
  name: z.string().trim().min(1),
  stage: z.enum(['dev', 'test', 'staging', 'prod']),
  namespace: z.string(),
  mappings: z.record(z.string(), z.string()),
  secretValues: z.record(z.string(), z.string()),
})

type ImportForm = z.infer<typeof importFormSchema>

const emptyForm: ImportForm = { mappings: {}, name: '', namespace: '', secretValues: {}, stage: 'dev' }

export function ApplicationDeploymentBundleImportDialog({ applicationId, onImported, onOpenChange, open, projectId }: {
  applicationId: string
  onImported: () => void
  onOpenChange: (open: boolean) => void
  open: boolean
  projectId: string
}) {
  const { t } = useTranslation()
  const [bundle, setBundle] = useState<DeploymentTargetBundle | null>(null)
  const [fileName, setFileName] = useState('')
  const [preview, setPreview] = useState<DeploymentTargetBundlePreview | null>(null)
  const [previewStale, setPreviewStale] = useState(false)
  const [error, setError] = useState('')
  const [previewing, setPreviewing] = useState(false)
  const [importing, setImporting] = useState(false)
  const form = useForm<ImportForm>({ defaultValues: emptyForm, resolver: zodResolver(importFormSchema) })
  const values = form.watch()

  const reset = () => {
    setBundle(null)
    setFileName('')
    setPreview(null)
    setPreviewStale(false)
    setError('')
    form.reset(emptyForm)
  }

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen)
      reset()
    onOpenChange(nextOpen)
  }

  const applyPreview = (result: DeploymentTargetBundlePreview, currentMappings: Record<string, string>) => {
    const mappings = { ...currentMappings }
    for (const reference of result.references) {
      if (reference.status === 'resolved' && reference.resolvedId)
        mappings[reference.key] = reference.resolvedId
    }
    form.setValue('mappings', mappings)
    setPreview(result)
    setPreviewStale(false)
    setError('')
  }

  const previewBundle = async (nextBundle: DeploymentTargetBundle, nextValues: ImportForm) => {
    setPreviewing(true)
    setError('')
    try {
      const result = await api.previewDeploymentTargetBundleImport(projectId, applicationId, {
        bundle: nextBundle,
        mappings: nextValues.mappings,
        overrides: { name: nextValues.name, namespace: nextValues.namespace, stage: nextValues.stage },
      })
      applyPreview(result, nextValues.mappings)
    }
    catch (caught) {
      setPreview(null)
      setPreviewStale(false)
      setError(caught instanceof Error ? caught.message : t('deploymentsPage.bundleImport.previewFailed'))
    }
    finally {
      setPreviewing(false)
    }
  }

  const selectFile = async (file?: File) => {
    if (!file)
      return
    setError('')
    setPreview(null)
    setPreviewStale(false)
    if (file.size > DEPLOYMENT_BUNDLE_MAX_BYTES) {
      setError(t('deploymentsPage.bundleImport.fileTooLarge'))
      return
    }
    try {
      const parsed = JSON.parse(await file.text()) as Partial<DeploymentTargetBundle>
      if (parsed.kind !== 'luna-devops.deployment-target' || parsed.schemaVersion !== 1 || !parsed.configuration)
        throw new Error(t('deploymentsPage.bundleImport.unsupportedFile'))
      const nextBundle = parsed as DeploymentTargetBundle
      const stage = ['dev', 'test', 'staging', 'prod'].includes(nextBundle.configuration.stage)
        ? nextBundle.configuration.stage as ImportForm['stage']
        : 'dev'
      const nextValues: ImportForm = {
        mappings: {},
        name: nextBundle.configuration.name || nextBundle.configuration.stage || '',
        namespace: nextBundle.configuration.namespace || '',
        secretValues: {},
        stage,
      }
      setBundle(nextBundle)
      setFileName(file.name)
      form.reset(nextValues)
      await previewBundle(nextBundle, nextValues)
    }
    catch (caught) {
      setBundle(null)
      setFileName('')
      setError(caught instanceof Error ? caught.message : t('deploymentsPage.bundleImport.invalidFile'))
    }
  }

  const runPreview = form.handleSubmit(async nextValues => bundle && previewBundle(bundle, nextValues))
  const allSecretsPresent = preview?.secretRequirements.every(requirement => Boolean(values.secretValues[requirement.key]?.trim())) ?? false
  const canImport = Boolean(bundle && preview?.status === 'ready' && !previewStale && allSecretsPresent && !previewing && !importing)

  const importBundle = form.handleSubmit(async (nextValues) => {
    if (!bundle || !preview)
      return
    setImporting(true)
    setError('')
    try {
      await api.importDeploymentTargetBundle(projectId, applicationId, {
        bundle,
        digest: preview.digest,
        mappings: nextValues.mappings,
        overrides: { name: nextValues.name, namespace: nextValues.namespace, stage: nextValues.stage },
        secretValues: nextValues.secretValues,
      })
      toast.success(t('deploymentsPage.bundleImport.imported'))
      onImported()
      handleOpenChange(false)
    }
    catch (caught) {
      setError(caught instanceof Error ? caught.message : t('deploymentsPage.bundleImport.importFailed'))
    }
    finally {
      setImporting(false)
    }
  })

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-3xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t('deploymentsPage.bundleImport.title')}</DialogTitle>
          <DialogDescription>{t('deploymentsPage.bundleImport.description')}</DialogDescription>
        </DialogHeader>
        <form className="grid min-w-0 gap-6" onSubmit={importBundle}>
          <section className="grid gap-3">
            <div className="flex items-center gap-2 font-medium">
              <FileJson2 className="size-4" />
              {t('deploymentsPage.bundleImport.selectFile')}
            </div>
            <Input accept="application/json,.json" aria-label={t('deploymentsPage.bundleImport.selectFile')} type="file" onChange={event => void selectFile(event.target.files?.[0])} />
            {fileName && <p className="text-sm text-muted-foreground">{t('deploymentsPage.bundleImport.selectedFile', { file: fileName })}</p>}
          </section>

          {bundle && (
            <>
              <section className="grid gap-4">
                <div>
                  <h3 className="font-medium">{t('deploymentsPage.bundleImport.destination')}</h3>
                  <p className="text-sm text-muted-foreground">{t('deploymentsPage.bundleImport.destinationHint')}</p>
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="grid gap-2">
                    <Label htmlFor="deployment-bundle-name">{t('common.name')}</Label>
                    <Input id="deployment-bundle-name" {...form.register('name', { onChange: () => setPreviewStale(true) })} />
                  </div>
                  <div className="grid gap-2">
                    <Label>{t('deploymentsPage.stage')}</Label>
                    <Select
                      value={values.stage}
                      onValueChange={(value: ImportForm['stage']) => {
                        form.setValue('stage', value)
                        setPreviewStale(true)
                      }}
                    >
                      <SelectTrigger><SelectValue /></SelectTrigger>
                      <SelectContent>
                        {(['dev', 'test', 'staging', 'prod'] as const).map(stage => <SelectItem key={stage} value={stage}>{t(`deploymentsPage.stageLabels.${stage}`)}</SelectItem>)}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="grid gap-2 sm:col-span-2">
                    <Label htmlFor="deployment-bundle-namespace">{t('deploymentsPage.bundleImport.namespace')}</Label>
                    <Input id="deployment-bundle-namespace" {...form.register('namespace', { onChange: () => setPreviewStale(true) })} />
                    <p className="text-xs text-muted-foreground">{t('deploymentsPage.bundleImport.namespaceHint')}</p>
                  </div>
                </div>
              </section>

              {preview && (
                <section className="grid gap-4">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div>
                      <h3 className="font-medium">{t('deploymentsPage.bundleImport.references')}</h3>
                      <p className="text-sm text-muted-foreground">{t('deploymentsPage.bundleImport.referencesHint')}</p>
                    </div>
                    <StatusBadge tone={preview.status === 'ready' ? 'success' : preview.status === 'invalid' ? 'danger' : 'warning'}>
                      {t(`deploymentsPage.bundleImport.statuses.${preview.status}`)}
                    </StatusBadge>
                  </div>
                  <div className="grid gap-3">
                    {preview.references.map(reference => (
                      <ReferenceMappingField
                        key={reference.key}
                        reference={reference}
                        value={values.mappings[reference.key] ?? reference.resolvedId ?? ''}
                        onChange={(value) => {
                          form.setValue(`mappings.${reference.key}`, value)
                          setPreviewStale(true)
                        }}
                      />
                    ))}
                    {preview.references.length === 0 && <p className="text-sm text-muted-foreground">{t('deploymentsPage.bundleImport.noReferences')}</p>}
                  </div>
                  {preview.warnings.map(warning => (
                    <Alert key={warning}>
                      <AlertTriangle className="size-4" />
                      <AlertTitle>{t('deploymentsPage.bundleImport.reviewRequired')}</AlertTitle>
                      <AlertDescription>{t(`deploymentsPage.bundleImport.warnings.${warning}`, { defaultValue: warning })}</AlertDescription>
                    </Alert>
                  ))}
                  {previewStale && (
                    <Alert>
                      <AlertTriangle className="size-4" />
                      <AlertTitle>{t('deploymentsPage.bundleImport.previewStaleTitle')}</AlertTitle>
                      <AlertDescription>{t('deploymentsPage.bundleImport.previewStaleDescription')}</AlertDescription>
                    </Alert>
                  )}
                </section>
              )}

              {preview && preview.secretRequirements.length > 0 && (
                <section className="grid gap-4">
                  <div>
                    <h3 className="font-medium">{t('deploymentsPage.bundleImport.secrets')}</h3>
                    <p className="text-sm text-muted-foreground">{t('deploymentsPage.bundleImport.secretsHint')}</p>
                  </div>
                  <div className="grid gap-3 sm:grid-cols-2">
                    {preview.secretRequirements.map(requirement => (
                      <div key={requirement.key} className="grid gap-2">
                        <Label htmlFor={`deployment-bundle-secret-${requirement.key}`}>{requirement.name || requirement.path}</Label>
                        <Input
                          autoComplete="new-password"
                          id={`deployment-bundle-secret-${requirement.key}`}
                          type="password"
                          {...form.register(`secretValues.${requirement.key}`)}
                        />
                        <p className="text-xs text-muted-foreground">{t(`deploymentsPage.bundleImport.secretTargets.${requirement.target}`)}</p>
                      </div>
                    ))}
                  </div>
                </section>
              )}
            </>
          )}

          {error && (
            <Alert variant="destructive">
              <AlertTriangle className="size-4" />
              <AlertTitle>{t('deploymentsPage.bundleImport.failed')}</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>{t('common.cancel')}</Button>
            {bundle && (
              <Button disabled={previewing} type="button" variant="outline" onClick={() => void runPreview()}>
                <RefreshCw className={previewing ? 'size-4 animate-spin' : 'size-4'} />
                {t('deploymentsPage.bundleImport.recheck')}
              </Button>
            )}
            <Button disabled={!canImport} type="submit">
              <Upload className="size-4" />
              {importing ? t('deploymentsPage.bundleImport.importing') : t('deploymentsPage.bundleImport.import')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function ReferenceMappingField({ onChange, reference, value }: { onChange: (value: string) => void, reference: DeploymentBundleReferenceResolution, value: string }) {
  const { t } = useTranslation()
  const source = reference.source.name || [reference.source.owner, reference.source.repository].filter(Boolean).join('/') || reference.source.logicalName || reference.key
  return (
    <div className="grid gap-2 rounded-control bg-muted/50 p-4 sm:grid-cols-[minmax(0,1fr)_minmax(14rem,1fr)] sm:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate font-medium">{source}</span>
          <StatusBadge tone={reference.status === 'resolved' ? 'success' : reference.status === 'incompatible' ? 'danger' : 'warning'}>
            {t(`deploymentsPage.bundleImport.referenceStatuses.${reference.status}`)}
          </StatusBadge>
        </div>
        <p className="mt-1 text-xs text-muted-foreground">{t(`deploymentsPage.bundleImport.referenceKinds.${reference.kind}`)}</p>
      </div>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger aria-label={t('deploymentsPage.bundleImport.selectReference', { reference: source })}>
          <SelectValue placeholder={t('deploymentsPage.bundleImport.selectDestination')} />
        </SelectTrigger>
        <SelectContent>
          {reference.candidates.map(candidate => (
            <SelectItem key={candidate.id} disabled={!candidate.compatible} value={candidate.id}>
              {candidate.name}
              {candidate.description ? ` · ${candidate.description}` : ''}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {reference.truncated && <p className="text-xs text-muted-foreground sm:col-span-2">{t('deploymentsPage.bundleImport.candidatesTruncated')}</p>}
    </div>
  )
}
