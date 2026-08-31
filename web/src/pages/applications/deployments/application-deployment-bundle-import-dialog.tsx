import type { DeploymentBundleReference, DeploymentBundleReferenceCandidatePage, DeploymentBundleReferenceResolution, DeploymentTargetBundle, DeploymentTargetBundlePreview } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { AlertTriangle, ChevronLeft, ChevronRight, FileJson2, RefreshCw, Search, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { api, ApiError } from '@/api'
import { StatusBadge } from '@/components/common/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import i18next from '@/i18n'

import '@/i18n/lazy/deployment-bundle'

const DEPLOYMENT_BUNDLE_MAX_BYTES = 1024 * 1024

const importFormSchema = z.object({
  name: z.string().trim().min(1),
  stage: z.enum(['dev', 'test', 'staging', 'prod']),
  mappings: z.record(z.string(), z.string()),
  secretValues: z.record(z.string(), z.string()),
})

type ImportForm = z.infer<typeof importFormSchema>

const emptyForm: ImportForm = { mappings: {}, name: '', secretValues: {}, stage: 'dev' }

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
  const [stageOverrideSet, setStageOverrideSet] = useState(false)
  const form = useForm<ImportForm>({ defaultValues: emptyForm, resolver: zodResolver(importFormSchema) })
  const values = form.watch()

  const reset = () => {
    setBundle(null)
    setFileName('')
    setPreview(null)
    setPreviewStale(false)
    setError('')
    setStageOverrideSet(false)
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

  const previewBundle = async (nextBundle: DeploymentTargetBundle, nextValues: ImportForm, includeStageOverride: boolean) => {
    setPreviewing(true)
    setError('')
    try {
      const overrides = {
        name: nextValues.name,
        ...(includeStageOverride ? { stage: nextValues.stage } : {}),
      }
      const result = await api.previewDeploymentTargetBundleImport(projectId, applicationId, {
        bundle: nextBundle,
        mappings: nextValues.mappings,
        overrides,
      })
      applyPreview(result, nextValues.mappings)
    }
    catch (caught) {
      setPreview(null)
      setPreviewStale(false)
      setError(deploymentBundleRequestError(caught, 'deploymentsPage.bundleImport.previewFailed'))
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
      if (parsed.kind !== 'luna-devops.deployment-target' || parsed.schemaVersion !== 1 || !parsed.configuration) {
        setBundle(null)
        setFileName('')
        setError(t('deploymentsPage.bundleImport.unsupportedFile'))
        return
      }
      const nextBundle = parsed as DeploymentTargetBundle
      const sourceStage = nextBundle.configuration.stage === 'production' ? 'prod' : nextBundle.configuration.stage
      const stage = ['dev', 'test', 'staging', 'prod'].includes(sourceStage) ? sourceStage as ImportForm['stage'] : 'dev'
      const nextValues: ImportForm = {
        mappings: {},
        name: nextBundle.configuration.name || nextBundle.configuration.stage || '',
        secretValues: {},
        stage,
      }
      setBundle(nextBundle)
      setFileName(file.name)
      setStageOverrideSet(false)
      form.reset(nextValues)
      await previewBundle(nextBundle, nextValues, false)
    }
    catch {
      setBundle(null)
      setFileName('')
      setError(t('deploymentsPage.bundleImport.invalidFile'))
    }
  }

  const runPreview = form.handleSubmit(async (nextValues) => {
    if (!bundle)
      return
    setStageOverrideSet(true)
    await previewBundle(bundle, nextValues, true)
  })
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
        overrides: {
          name: nextValues.name,
          ...(stageOverrideSet ? { stage: nextValues.stage } : {}),
        },
        secretValues: nextValues.secretValues,
      })
      toast.success(t('deploymentsPage.bundleImport.imported'))
      onImported()
      handleOpenChange(false)
    }
    catch (caught) {
      setError(deploymentBundleRequestError(caught, 'deploymentsPage.bundleImport.importFailed'))
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
                        setStageOverrideSet(true)
                        setPreviewStale(true)
                      }}
                    >
                      <SelectTrigger><SelectValue /></SelectTrigger>
                      <SelectContent>
                        {(['dev', 'test', 'staging', 'prod'] as const).map(stage => <SelectItem key={stage} value={stage}>{t(`deploymentsPage.stageLabels.${stage}`)}</SelectItem>)}
                      </SelectContent>
                    </Select>
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
                        key={`${preview.digest}:${reference.key}:${reference.kind}:${reference.candidateCount}:${reference.resolvedId ?? ''}`}
                        applicationId={applicationId}
                        projectId={projectId}
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
                  {preview.warnings.map((warning) => {
                    const warningKey = `deploymentsPage.bundleImport.warnings.${warning}`
                    return (
                      <Alert key={warning}>
                        <AlertTriangle className="size-4" />
                        <AlertTitle>{t('deploymentsPage.bundleImport.reviewRequired')}</AlertTitle>
                        <AlertDescription>{t(i18next.exists(warningKey) ? warningKey : 'deploymentsPage.bundleImport.warnings.unknown')}</AlertDescription>
                      </Alert>
                    )
                  })}
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

function deploymentBundleRequestError(caught: unknown, fallbackKey: string) {
  if (caught instanceof ApiError) {
    const errorKey = `errors.${caught.code}`
    if (i18next.exists(errorKey))
      return i18next.t(errorKey)
  }
  return i18next.t(fallbackKey)
}

function ReferenceMappingField({ applicationId, onChange, projectId, reference, value }: {
  applicationId: string
  onChange: (value: string) => void
  projectId: string
  reference: DeploymentBundleReferenceResolution
  value: string
}) {
  const { t } = useTranslation()
  const source = reference.source.name || [reference.source.owner, reference.source.repository].filter(Boolean).join('/') || reference.source.logicalName || reference.key
  const requestNumberRef = useRef(0)
  const [search, setSearch] = useState('')
  const [sortBy, setSortBy] = useState<'name' | 'createdAt'>('name')
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('asc')
  const [loading, setLoading] = useState(false)
  const [loadFailed, setLoadFailed] = useState(false)
  const [retainedCandidate, setRetainedCandidate] = useState(() => reference.candidates.find(candidate => candidate.id === value))
  const [candidatePage, setCandidatePage] = useState<DeploymentBundleReferenceCandidatePage>({
    items: reference.candidates,
    page: 1,
    pageSize: 20,
    sortBy: 'name',
    sortOrder: 'asc',
    total: reference.candidateCount,
    totalPages: Math.ceil(reference.candidateCount / 20),
  })
  const selectedCandidate = retainedCandidate ?? reference.candidates.find(candidate => candidate.id === value)
  const candidates = selectedCandidate && !candidatePage.items.some(candidate => candidate.id === selectedCandidate.id)
    ? [selectedCandidate, ...candidatePage.items]
    : candidatePage.items

  const loadCandidates = async (page: number, nextSearch = search, nextSortBy = sortBy, nextSortOrder = sortOrder) => {
    const currentRequest = ++requestNumberRef.current
    setLoading(true)
    setLoadFailed(false)
    try {
      const result = await api.listDeploymentTargetBundleReferenceCandidates(projectId, applicationId, {
        reference: portableDeploymentBundleReference(reference),
      }, { page, pageSize: 20, search: nextSearch, sortBy: nextSortBy, sortOrder: nextSortOrder })
      if (requestNumberRef.current === currentRequest)
        setCandidatePage(result)
    }
    catch {
      if (requestNumberRef.current === currentRequest)
        setLoadFailed(true)
    }
    finally {
      if (requestNumberRef.current === currentRequest)
        setLoading(false)
    }
  }
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
      <div className="grid gap-2">
        <div className="flex gap-2">
          <Input
            aria-label={t('deploymentsPage.bundleImport.searchCandidates', { reference: source })}
            placeholder={t('deploymentsPage.bundleImport.searchPlaceholder')}
            value={search}
            onChange={event => setSearch(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault()
                void loadCandidates(1)
              }
            }}
          />
          <Button aria-label={t('deploymentsPage.bundleImport.searchAction')} disabled={loading} size="icon" type="button" variant="outline" onClick={() => void loadCandidates(1)}>
            <Search className="size-4" />
          </Button>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <Select
            value={sortBy}
            onValueChange={(next: 'name' | 'createdAt') => {
              setSortBy(next)
              void loadCandidates(1, search, next, sortOrder)
            }}
          >
            <SelectTrigger aria-label={t('deploymentsPage.bundleImport.sortCandidates')}><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="name">{t('deploymentsPage.bundleImport.sortByName')}</SelectItem>
              <SelectItem value="createdAt">{t('deploymentsPage.bundleImport.sortByCreatedAt')}</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={sortOrder}
            onValueChange={(next: 'asc' | 'desc') => {
              setSortOrder(next)
              void loadCandidates(1, search, sortBy, next)
            }}
          >
            <SelectTrigger aria-label={t('deploymentsPage.bundleImport.sortOrder')}><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="asc">{t('deploymentsPage.bundleImport.sortAscending')}</SelectItem>
              <SelectItem value="desc">{t('deploymentsPage.bundleImport.sortDescending')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <Select
          value={value}
          onValueChange={(nextValue) => {
            setRetainedCandidate(candidates.find(candidate => candidate.id === nextValue))
            onChange(nextValue)
          }}
        >
          <SelectTrigger aria-label={t('deploymentsPage.bundleImport.selectReference', { reference: source })}>
            <SelectValue placeholder={t('deploymentsPage.bundleImport.selectDestination')} />
          </SelectTrigger>
          <SelectContent>
            {candidates.map(candidate => (
              <SelectItem key={candidate.id} disabled={!candidate.compatible} value={candidate.id}>
                {candidate.name}
                {candidate.description ? ` · ${candidate.description}` : ''}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
          <span>{loadFailed ? t('deploymentsPage.bundleImport.candidatesLoadFailed') : t('deploymentsPage.bundleImport.candidateCount', { count: candidatePage.total })}</span>
          <div className="flex items-center gap-1">
            <Button aria-label={t('deploymentsPage.bundleImport.previousCandidates')} disabled={loading || candidatePage.page <= 1} size="sm" type="button" variant="ghost" onClick={() => void loadCandidates(candidatePage.page - 1)}><ChevronLeft className="size-4" /></Button>
            <span>{t('deploymentsPage.bundleImport.candidatePage', { page: candidatePage.page, totalPages: Math.max(candidatePage.totalPages, 1) })}</span>
            <Button aria-label={t('deploymentsPage.bundleImport.nextCandidates')} disabled={loading || candidatePage.page >= candidatePage.totalPages} size="sm" type="button" variant="ghost" onClick={() => void loadCandidates(candidatePage.page + 1)}><ChevronRight className="size-4" /></Button>
          </div>
        </div>
      </div>
    </div>
  )
}

function portableDeploymentBundleReference(reference: DeploymentBundleReferenceResolution): DeploymentBundleReference {
  return {
    key: reference.key,
    kind: reference.kind,
    required: reference.required,
    source: reference.source,
    usage: reference.usage,
  }
}
