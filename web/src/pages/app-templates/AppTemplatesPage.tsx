import type { ReactNode } from 'react'
import type { AppTemplate, AppTemplateInstallPayload, AppTemplateSummary, Project, ProjectVolume, RuntimeCluster } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, ExternalLink, PackagePlus, Plus, Search, Sparkles } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { CheckboxField } from '@/components/common/checkbox-field'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { TemplateGridSkeleton } from '@/components/common/loading-states'
import { PageShell } from '@/components/common/page-shell'
import { ProjectSpaceSelect } from '@/components/common/project-space-select'
import { ProjectVolumeCreateDialog } from '@/components/common/project-volumes/project-volume-create-dialog'
import { StatusBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
import { UnitInput } from '@/components/common/unit-input'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { APPLICATION_IDENTIFIER_MAX_LENGTH, APPLICATION_IDENTIFIER_MIN_LENGTH } from '@/lib/identifier-limits'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { projectVolumeCapabilities } from '@/lib/project-volume-capabilities'
import { isPlatformAdmin } from '@/lib/roles'
import { cn } from '@/lib/utils'

const FALLBACK_ICON = '/app-templates/icons/fallback.svg'

export function AppTemplatesPage() {
  const { i18n, t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState('all')
  const [sortBy, setSortBy] = useState<'popularity' | 'name'>('popularity')
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc')
  const [selectedTemplateOverride, setSelectedTemplateOverride] = useState<AppTemplateSummary | null>(null)
  const [selectedProjectId, setSelectedProjectId] = useState('')
  const [formState, setFormState] = useState<{ templateId: string, value: AppTemplateInstallPayload } | null>(null)
  const requestedTemplateId = searchParams.get('template')
  const templates = useQuery({ queryKey: ['app-templates'], queryFn: () => api.listAppTemplates() })
  const projects = useQuery({ queryKey: ['projects'], queryFn: () => api.listProjects() })
  const projectItems = useMemo(() => projects.data ?? [], [projects.data])
  const projectId = projectItems.some(project => project.id === selectedProjectId)
    ? selectedProjectId
    : projectItems[0]?.id ?? ''
  const projectDetail = useQuery({
    queryKey: ['project', projectId],
    queryFn: () => api.getProject(projectId),
    enabled: Boolean(projectId),
  })
  const requestedTemplate = useMemo(
    () => templates.data?.find(template => template.id === requestedTemplateId) ?? null,
    [requestedTemplateId, templates.data],
  )
  const selectedTemplateSummary = selectedTemplateOverride ?? requestedTemplate
  const templateDetail = useQuery({
    queryKey: ['app-template', selectedTemplateSummary?.id],
    queryFn: () => api.getAppTemplate(selectedTemplateSummary!.id),
    enabled: Boolean(selectedTemplateSummary?.id),
  })
  const selectedTemplate = templateDetail.data ?? null
  const selectedTemplateIsSystem = isSystemComponentTemplate(selectedTemplate)
  const canInstallSystemComponent = isPlatformAdmin(user?.role)
  const defaultForm = useMemo(
    () => selectedTemplate ? payloadFromTemplate(selectedTemplate) : emptyInstallPayload(),
    [selectedTemplate],
  )
  const form = formState && formState.templateId === selectedTemplate?.id ? formState.value : defaultForm
  const clusters = useQuery({
    queryKey: ['runtime-clusters', selectedTemplateIsSystem ? 'system' : projectId],
    queryFn: () => api.listRuntimeClusters(selectedTemplateIsSystem ? undefined : projectId),
    enabled: selectedTemplateIsSystem || Boolean(projectId),
    ...liveObservationQueryPolicy,
  })
  const clusterItems = clusters.data ?? []
  const effectiveCluster = clusterItems.find(cluster => cluster.id === form.clusterId)
    ?? clusterItems.find(cluster => cluster.isDefault)
    ?? clusterItems[0]
  const effectiveClusterId = effectiveCluster?.id ?? ''
  const canWriteProject = projectVolumeCapabilities(user?.role, projectDetail.data?.currentUserRole, user?.id).canWrite
  const selectedProjectVolumeDeclaration = selectedTemplate?.dataVolumes.find(volume => volume.sourceType === 'projectVolume')
  const requiredProjectVolumeMode = selectedProjectVolumeDeclaration?.devicePath ? 'Block' : 'Filesystem'

  const categoryOptions = useMemo(() => {
    const categories = new Set((templates.data ?? []).map(template => template.category).filter(Boolean))
    return Array.from(categories).sort((a, b) =>
      t(`appTemplatesPage.categories.${a}`, { defaultValue: a }).localeCompare(
        t(`appTemplatesPage.categories.${b}`, { defaultValue: b }),
        i18n.language,
      ),
    )
  }, [i18n.language, t, templates.data])

  const sortedTemplates = useMemo(() => {
    const keyword = search.trim().toLowerCase()
    const items = templates.data ?? []
    const categoryFiltered = category === 'all'
      ? items
      : items.filter(template => template.category === category)
    const filtered = keyword
      ? categoryFiltered.filter(template => [template.name, template.slug, template.image, template.officialWebsite, template.officialRepository]
          .some(value => value.toLowerCase().includes(keyword)))
      : categoryFiltered
    const direction = sortOrder === 'asc' ? 1 : -1
    return [...filtered].sort((a, b) => {
      if (sortBy === 'name') {
        const nameResult = a.name.localeCompare(b.name, i18n.language)
        return nameResult === 0 ? a.slug.localeCompare(b.slug) : direction * nameResult
      }
      const popularityResult = (a.popularityWeight ?? 0) - (b.popularityWeight ?? 0)
      return popularityResult === 0 ? a.name.localeCompare(b.name, i18n.language) : direction * popularityResult
    })
  }, [category, i18n.language, search, sortBy, sortOrder, templates.data])
  const installTemplate = useMutation({
    mutationFn: (payload: AppTemplateInstallPayload & { templateId: string, projectId: string }) =>
      api.installAppTemplate(payload.projectId, payload.templateId, payload),
    onSuccess: async (result) => {
      toast.success(t('appTemplatesPage.installStarted'))
      setSelectedTemplateOverride(null)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['projects'] }),
        queryClient.invalidateQueries({ queryKey: ['applications', result.application.projectId] }),
      ])
      navigate(`/projects/${result.application.projectId}/apps/${result.application.id}#tab=deployments`)
    },
    onError: error => toast.error(error.message),
  })

  const installSystemTemplate = useMutation({
    mutationFn: (payload: { templateId: string, clusterId: string, apiBaseUrl: string, traefikMetricsUrl?: string, image?: string, provisionAccess?: boolean }) =>
      api.installSystemAppTemplate(payload.templateId, {
        apiBaseUrl: payload.apiBaseUrl,
        clusterId: payload.clusterId,
        image: payload.image,
        mode: 'traefik-metrics',
        provisionAccess: payload.provisionAccess,
        traefikMetricsUrl: payload.traefikMetricsUrl,
      }),
    onSuccess: async (result) => {
      toast.success(t('appTemplatesPage.systemInstallStarted'))
      setSelectedTemplateOverride(null)
      setSearchParams((current) => {
        const next = new URLSearchParams(current)
        next.delete('template')
        return next
      }, { replace: true })
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['projects'] }),
        queryClient.invalidateQueries({ queryKey: ['system-components'] }),
        queryClient.invalidateQueries({ queryKey: ['billing', 'gateway-traffic-status'] }),
      ])
      if (result.application)
        navigate(`/projects/${result.application.projectId}/apps/${result.application.id}#tab=deployments`)
    },
    onError: error => toast.error(error.message),
  })

  function openInstallDialog(template: AppTemplateSummary) {
    setSelectedTemplateOverride(template)
    setFormState(null)
  }

  function closeInstallDialog() {
    setSelectedTemplateOverride(null)
    setFormState(null)
    setSearchParams((current) => {
      const next = new URLSearchParams(current)
      next.delete('template')
      return next
    }, { replace: true })
  }

  function updateForm<K extends keyof AppTemplateInstallPayload>(key: K, value: AppTemplateInstallPayload[K]) {
    if (!selectedTemplate)
      return
    setFormState(current => ({
      templateId: selectedTemplate.id,
      value: {
        ...(current?.templateId === selectedTemplate.id ? current.value : defaultForm),
        [key]: value,
      },
    }))
  }

  function updateTemplateValue(key: string, value: string) {
    if (!selectedTemplate)
      return
    setFormState((current) => {
      const currentForm = current?.templateId === selectedTemplate.id ? current.value : defaultForm
      return {
        templateId: selectedTemplate.id,
        value: { ...currentForm, values: { ...currentForm.values, [key]: value } },
      }
    })
  }

  function submitInstall() {
    if (!selectedTemplate)
      return
    if (isSystemComponentTemplate(selectedTemplate)) {
      if (!canInstallSystemComponent)
        return
      installSystemTemplate.mutate({
        apiBaseUrl: form.values.apiBaseUrl ?? '',
        clusterId: form.clusterId,
        image: form.imageRef.trim(),
        provisionAccess: form.provisionAccess,
        templateId: selectedTemplate.id,
        traefikMetricsUrl: form.values.traefikMetricsUrl ?? '',
      })
      return
    }
    if (!projectId)
      return
    installTemplate.mutate({ ...form, projectId, templateId: selectedTemplate.id })
  }

  return (
    <PageShell width="full">
      <section className="relative overflow-hidden rounded-feature border border-transparent bg-surface-raised px-5 py-8 sm:px-8 sm:py-10">
        <div className="relative grid gap-7 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
          <div className="max-w-3xl">
            <div className="mb-4 inline-flex items-center gap-2 text-sm font-medium text-primary-text">
              <Sparkles className="size-4" />
              {t('appTemplatesPage.heroEyebrow')}
            </div>
            <h1 className="max-w-2xl text-2xl font-semibold tracking-tight sm:text-3xl">{t('appTemplatesPage.heroTitle')}</h1>
            <div className="relative mt-5 max-w-2xl">
              <Search className="pointer-events-none absolute left-4 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                className="h-12 rounded-xl border-transparent bg-surface-subtle pl-11 pr-4 shadow-none focus-visible:border-primary-border focus-visible:ring-2"
                placeholder={t('appTemplatesPage.searchPlaceholder')}
                value={search}
                onChange={event => setSearch(event.target.value)}
              />
            </div>
          </div>
          <div className="flex gap-8 border-t border-border/70 pt-5 lg:border-l lg:border-t-0 lg:pl-8 lg:pt-0">
            <MarketplaceMetric label={t('appTemplatesPage.templateCount')} value={templates.data?.length ?? 0} />
            <MarketplaceMetric label={t('appTemplatesPage.categoryCount')} value={categoryOptions.length} />
          </div>
        </div>
      </section>

      {templates.isError && <ErrorState title={templates.error.message} />}
      {templates.isLoading && <TemplateGridSkeleton />}
      {templates.isSuccess && (
        <section className="grid gap-5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <h2 className="text-lg font-semibold">{t('appTemplatesPage.browseTitle')}</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                {t('appTemplatesPage.resultCount', { count: sortedTemplates.length })}
              </p>
            </div>
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
              <Select
                aria-label={t('appTemplatesPage.categoryFilter')}
                className="h-9 min-w-36 border-0 bg-surface-inset shadow-none"
                value={category}
                onChange={event => setCategory(event.target.value)}
              >
                <option value="all">{t('appTemplatesPage.allCategories')}</option>
                {categoryOptions.map(item => (
                  <option key={item} value={item}>
                    {t(`appTemplatesPage.categories.${item}`, { defaultValue: item })}
                  </option>
                ))}
              </Select>
              <Select
                aria-label={t('appTemplatesPage.sortBy')}
                className="h-9 min-w-32 border-0 bg-surface-inset shadow-none"
                value={sortBy}
                onChange={event => setSortBy(event.target.value as typeof sortBy)}
              >
                <option value="popularity">{t('appTemplatesPage.sortByPopularity')}</option>
                <option value="name">{t('appTemplatesPage.sortByName')}</option>
              </Select>
              <Select
                aria-label={t('appTemplatesPage.sortOrder')}
                className="h-9 min-w-24 border-0 bg-surface-inset shadow-none"
                value={sortOrder}
                onChange={event => setSortOrder(event.target.value as typeof sortOrder)}
              >
                <option value="desc">{t('appTemplatesPage.sortDesc')}</option>
                <option value="asc">{t('appTemplatesPage.sortAsc')}</option>
              </Select>
            </div>
          </div>
          {sortedTemplates.length === 0
            ? <EmptyState description={t('appTemplatesPage.emptyDescription')} title={t('appTemplatesPage.emptyTitle')} />
            : (
                <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
                  {sortedTemplates.map(template => (
                    <TemplateCard
                      key={template.id}
                      canInstallSystemComponent={canInstallSystemComponent}
                      template={template}
                      onInstall={() => openInstallDialog(template)}
                    />
                  ))}
                </div>
              )}
        </section>
      )}

      <InstallTemplateDialog
        clusterItems={clusterItems}
        clustersLoading={clusters.isLoading}
        canInstallSystemComponent={canInstallSystemComponent}
        canInstallProjectTemplate={canWriteProject}
        form={form}
        installing={installTemplate.isPending || installSystemTemplate.isPending}
        projectId={projectId}
        projects={projectItems}
        canCreateProjectVolume={canWriteProject}
        projectVolumeClusterId={effectiveClusterId}
        projectVolumeClusterName={effectiveCluster?.name ?? effectiveClusterId}
        projectVolumeMode={requiredProjectVolumeMode}
        template={selectedTemplate}
        onClose={closeInstallDialog}
        onProjectChange={(value) => {
          setSelectedProjectId(value)
          updateForm('clusterId', '')
          updateForm('projectVolumeId', '')
        }}
        onSubmit={submitInstall}
        onTemplateValueChange={updateTemplateValue}
        onUpdate={updateForm}
      />
    </PageShell>
  )
}

function TemplateCard({ canInstallSystemComponent, template, onInstall }: { canInstallSystemComponent: boolean, template: AppTemplateSummary, onInstall: () => void }) {
  const { t } = useTranslation()
  const systemComponent = isSystemComponentTemplate(template)
  const installDisabled = systemComponent && !canInstallSystemComponent
  return (
    <Surface className="group flex min-h-48 flex-col rounded-xl p-5 transition-[background-color,box-shadow,transform] duration-standard ease-emphasized hover:bg-surface-subtle hover:shadow-raised motion-safe:hover:-translate-y-0.5">
      <div className="flex items-start gap-4">
        <div className="flex size-12 shrink-0 items-center justify-center rounded-xl bg-surface-inset">
          <img
            alt=""
            className="size-8 object-contain transition-transform duration-standard ease-emphasized motion-safe:group-hover:scale-105"
            src={template.icon || FALLBACK_ICON}
            onError={(event) => {
              event.currentTarget.src = FALLBACK_ICON
            }}
          />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-start justify-between gap-2">
            <h3 className="min-w-0 truncate text-base font-semibold tracking-tight">{template.name}</h3>
            <TemplateSourceLinks template={template} />
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
            <span>{t(`appTemplatesPage.categories.${template.category}`, { defaultValue: template.category })}</span>
            {systemComponent && <StatusBadge tone="info">{t('appTemplatesPage.platformComponent')}</StatusBadge>}
          </div>
        </div>
      </div>
      <p className="mt-4 line-clamp-2 min-h-10 text-sm leading-5 text-muted-foreground">
        {t(`appTemplatesPage.templates.${template.id}.description`, { defaultValue: template.description || t('common.noDescription') })}
      </p>
      <div className="mt-auto flex justify-end pt-4">
        <Button disabled={installDisabled} size="sm" type="button" variant="ghost" onClick={onInstall}>
          {t('appTemplatesPage.install')}
          <ArrowRight className="size-4" />
        </Button>
      </div>
    </Surface>
  )
}

function MarketplaceMetric({ label, value }: { label: string, value: number }) {
  return (
    <div>
      <div className="text-2xl font-semibold tabular-nums">{value}</div>
      <div className="mt-0.5 text-xs text-muted-foreground">{label}</div>
    </div>
  )
}

function TemplateSourceLinks({ template }: { template: Pick<AppTemplate, 'officialRepository' | 'officialWebsite'> }) {
  const { t } = useTranslation()
  return (
    <div className="flex shrink-0 items-center gap-2">
      <TemplateSourceLink
        href={template.officialWebsite}
        icon={<ExternalLink className="size-4" />}
        label={t('appTemplatesPage.officialWebsite')}
      />
      <TemplateSourceLink
        href={template.officialRepository}
        icon={<GithubMark className="size-4" />}
        label={t('appTemplatesPage.officialRepository')}
      />
    </div>
  )
}

function TemplateSourceLink({ href, icon, label }: { href: string, icon: ReactNode, label: string }) {
  if (!href)
    return null
  return (
    <a
      aria-label={label}
      className="inline-flex size-4 shrink-0 items-center justify-center text-muted-foreground transition-colors hover:text-primary-text focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      href={href}
      rel="noreferrer"
      target="_blank"
      title={label}
    >
      {icon}
    </a>
  )
}

function GithubMark({ className }: { className?: string }) {
  return (
    <svg
      aria-hidden="true"
      className={className}
      fill="currentColor"
      viewBox="0 0 24 24"
    >
      <path d="M12 2C6.48 2 2 6.58 2 12.25c0 4.53 2.87 8.37 6.84 9.72.5.09.68-.22.68-.49v-1.9c-2.78.62-3.37-1.22-3.37-1.22-.45-1.19-1.11-1.5-1.11-1.5-.91-.64.07-.63.07-.63 1 .07 1.53 1.06 1.53 1.06.89 1.56 2.34 1.11 2.91.85.09-.66.35-1.11.63-1.37-2.22-.26-4.56-1.14-4.56-5.06 0-1.12.39-2.03 1.03-2.75-.1-.26-.45-1.3.1-2.71 0 0 .84-.28 2.75 1.05A9.3 9.3 0 0 1 12 6.98c.85 0 1.7.12 2.5.34 1.91-1.33 2.75-1.05 2.75-1.05.55 1.41.2 2.45.1 2.71.64.72 1.03 1.63 1.03 2.75 0 3.93-2.34 4.79-4.57 5.05.36.32.68.95.68 1.91v2.79c0 .27.18.59.69.49A10.12 10.12 0 0 0 22 12.25C22 6.58 17.52 2 12 2Z" />
    </svg>
  )
}

function InstallTemplateDialog({
  clusterItems,
  clustersLoading,
  canInstallSystemComponent,
  canInstallProjectTemplate,
  canCreateProjectVolume,
  form,
  installing,
  projectId,
  projects,
  projectVolumeClusterId,
  projectVolumeClusterName,
  projectVolumeMode,
  template,
  onClose,
  onProjectChange,
  onSubmit,
  onTemplateValueChange,
  onUpdate,
}: {
  clusterItems: RuntimeCluster[]
  clustersLoading: boolean
  canInstallSystemComponent: boolean
  canInstallProjectTemplate: boolean
  canCreateProjectVolume: boolean
  form: AppTemplateInstallPayload
  installing: boolean
  projectId: string
  projects: Project[]
  projectVolumeClusterId: string
  projectVolumeClusterName: string
  projectVolumeMode: 'Block' | 'Filesystem'
  template: AppTemplate | null
  onClose: () => void
  onProjectChange: (value: string) => void
  onSubmit: () => void
  onTemplateValueChange: (key: string, value: string) => void
  onUpdate: <K extends keyof AppTemplateInstallPayload>(key: K, value: AppTemplateInstallPayload[K]) => void
}) {
  const { t } = useTranslation()
  const systemComponent = isSystemComponentTemplate(template)
  const requiresProjectVolume = Boolean(template?.dataVolumes.some(volume => volume.sourceType === 'projectVolume'))
  const canSubmit = systemComponent
    ? Boolean(template && canInstallSystemComponent && form.clusterId.trim() && (form.values.apiBaseUrl ?? '').trim() && !installing)
    : Boolean(template && canInstallProjectTemplate && projectId && form.applicationName.trim() && form.applicationIdentifier.trim().length >= APPLICATION_IDENTIFIER_MIN_LENGTH && form.imageRef.trim()
      && (!requiresProjectVolume || form.projectVolumeId?.trim()) && !installing)
  return (
    <Dialog open={Boolean(template)} onOpenChange={open => !open && onClose()}>
      <DialogContent className="flex max-h-[min(94dvh,54rem)] w-[calc(100vw-1rem)] max-w-4xl flex-col gap-0 overflow-hidden rounded-lg p-0 sm:w-[calc(100%-2rem)]">
        <DialogHeader className="shrink-0 border-b border-border px-4 py-4 sm:px-6 sm:py-5">
          <DialogTitle className="truncate pr-2">{t('appTemplatesPage.installDialogTitle', { name: template?.name ?? '' })}</DialogTitle>
          <DialogDescription>{systemComponent ? t('appTemplatesPage.systemInstallDialogDescription') : t('appTemplatesPage.installDialogDescription')}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4 sm:px-6 sm:py-5">
          {!systemComponent && (
            <div className="grid gap-4 md:grid-cols-2 md:gap-5">
              <Field label={t('projectSpaces.title')}>
                <ProjectSpaceSelect
                  disabled={projects.length === 0 || installing}
                  projects={projects}
                  value={projectId}
                  onChange={onProjectChange}
                />
              </Field>
              <Field label={t('appTemplatesPage.runtimeCluster')}>
                <Select
                  disabled={clustersLoading || installing}
                  value={form.clusterId}
                  onChange={(event) => {
                    onUpdate('clusterId', event.target.value)
                    onUpdate('projectVolumeId', '')
                  }}
                >
                  <option value="">{t('appTemplatesPage.defaultCluster')}</option>
                  {clusterItems.map(cluster => (
                    <option key={cluster.id} value={cluster.id}>
                      {cluster.name}
                      {cluster.isDefault ? ` (${t('common.default')})` : ''}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field label={t('appTemplatesPage.applicationName')} required>
                <Input value={form.applicationName} onChange={event => onUpdate('applicationName', event.target.value)} />
              </Field>
              <Field label={t('appTemplatesPage.applicationIdentifier')} required>
                <Input
                  maxLength={APPLICATION_IDENTIFIER_MAX_LENGTH}
                  minLength={APPLICATION_IDENTIFIER_MIN_LENGTH}
                  value={form.applicationIdentifier}
                  onChange={event => onUpdate('applicationIdentifier', normalizeIdentifierInput(event.target.value))}
                />
              </Field>
              <Field label={t('appTemplatesPage.deploymentName')}>
                <Input value={form.deploymentName} onChange={event => onUpdate('deploymentName', event.target.value)} />
              </Field>
              <Field label={t('appTemplatesPage.stage')}>
                <Select value={form.stage} onChange={event => onUpdate('stage', event.target.value)}>
                  {['prod', 'staging', 'test', 'dev'].map(stage => (
                    <option key={stage} value={stage}>{t(`appTemplatesPage.stageOptions.${stage}`)}</option>
                  ))}
                </Select>
              </Field>
              <div className="md:col-span-2">
                <Field label={t('appTemplatesPage.imageRef')} required>
                  <Input
                    value={form.imageRef}
                    onChange={event => onUpdate('imageRef', event.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">{t('appTemplatesPage.imageRefHint')}</p>
                </Field>
              </div>
              <div className="grid gap-4 md:col-span-2 md:grid-cols-3 md:gap-5">
                <Field label={t('appTemplatesPage.replicas')}>
                  <Input min={1} type="number" value={form.replicas} onChange={event => onUpdate('replicas', Number(event.target.value || 1))} />
                </Field>
                <Field label={t('appTemplatesPage.cpu')}>
                  <UnitInput
                    unitSelectLabel={t('appTemplatesPage.cpu')}
                    units={[
                      { label: 'm', value: 'm' },
                      { label: t('deploymentsPage.cpuUnits.core'), value: '' },
                    ]}
                    value={form.cpuRequest}
                    onChange={value => onUpdate('cpuRequest', value)}
                  />
                </Field>
                <Field label={t('appTemplatesPage.memory')}>
                  <UnitInput
                    unitSelectLabel={t('appTemplatesPage.memory')}
                    units={[
                      { label: 'Mi', value: 'Mi' },
                      { label: 'Gi', value: 'Gi' },
                    ]}
                    value={form.memoryRequest}
                    onChange={value => onUpdate('memoryRequest', value)}
                  />
                </Field>
                <div className="md:col-span-3">
                  <Field label={t('appTemplatesPage.projectVolume')} required={requiresProjectVolume}>
                    <TemplateProjectVolumePicker
                      key={`${projectId}:${projectVolumeClusterId}:${projectVolumeMode}:${template?.id ?? ''}`}
                      clusterId={projectVolumeClusterId}
                      clusterName={projectVolumeClusterName}
                      canCreate={canCreateProjectVolume}
                      disabled={installing}
                      mode={projectVolumeMode}
                      projectId={projectId}
                      required={requiresProjectVolume}
                      value={form.projectVolumeId ?? ''}
                      onChange={(value) => {
                        if (value)
                          onUpdate('clusterId', projectVolumeClusterId)
                        onUpdate('projectVolumeId', value)
                      }}
                    />
                  </Field>
                </div>
              </div>
            </div>
          )}

          {systemComponent && (
            <div className="grid gap-4 md:grid-cols-2 md:gap-5">
              <Field label={t('appTemplatesPage.runtimeCluster')} required>
                <Select
                  disabled={clustersLoading || installing}
                  value={form.clusterId}
                  onChange={event => onUpdate('clusterId', event.target.value)}
                >
                  <option value="">{t('appTemplatesPage.selectRuntimeCluster')}</option>
                  {clusterItems.map(cluster => (
                    <option key={cluster.id} value={cluster.id}>
                      {cluster.name}
                      {cluster.isDefault ? ` (${t('common.default')})` : ''}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field label={t('appTemplatesPage.componentNamespace')}>
                <Input disabled value="luna-system" />
              </Field>
              <div className="md:col-span-2">
                <Field hint={t('appTemplatesPage.imageRefHint')} label={t('appTemplatesPage.imageRef')} required>
                  <Input
                    value={form.imageRef}
                    onChange={event => onUpdate('imageRef', event.target.value)}
                  />
                </Field>
              </div>
              <CheckboxField
                checked={form.provisionAccess}
                className="rounded-lg border border-border p-3 md:col-span-2"
                description={t('appTemplatesPage.provisionAccessDescription')}
                disabled={installing}
                onCheckedChange={checked => onUpdate('provisionAccess', checked === true)}
              >
                {t('appTemplatesPage.provisionAccess')}
              </CheckboxField>
            </div>
          )}

          {template && template.values.length > 0 && (
            <div className="mt-5 grid gap-4 border-t border-border pt-5 sm:mt-6">
              <div>
                <h3 className="font-semibold">{t('appTemplatesPage.templateParameters')}</h3>
                <p className="mt-1 text-sm text-muted-foreground">{t('appTemplatesPage.templateParametersDescription')}</p>
              </div>
              <div className="grid gap-4 md:grid-cols-2 md:gap-5">
                {template.values.map((value) => {
                  const label = t(`appTemplatesPage.valueLabels.${value.key}`, { defaultValue: value.label || value.key })
                  return (
                    <Field
                      key={value.key}
                      hint={templateValueHint(template.id, value.key, t)}
                      label={label}
                      required={value.required && !value.autoGenerate}
                    >
                      <Input
                        aria-label={label}
                        autoComplete={value.secret ? 'new-password' : undefined}
                        placeholder={templateValuePlaceholder(template.id, value.key, value.autoGenerate, value.default, t)}
                        required={value.required && !value.autoGenerate}
                        type={value.secret ? 'password' : 'text'}
                        value={form.values[value.key] ?? ''}
                        onChange={event => onTemplateValueChange(value.key, event.target.value)}
                      />
                    </Field>
                  )
                })}
              </div>
            </div>
          )}

          {!systemComponent && (
            <CheckboxField
              checked={form.installNow}
              className="mt-5 rounded-lg border border-border p-3 sm:mt-6 sm:p-4"
              description={t('appTemplatesPage.installNowDescription')}
              disabled={installing}
              onCheckedChange={checked => onUpdate('installNow', checked === true)}
            >
              {t('appTemplatesPage.installNow')}
            </CheckboxField>
          )}

          {systemComponent && !canInstallSystemComponent && (
            <div className="mt-5 rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive sm:mt-6 sm:p-4">
              {t('appTemplatesPage.systemInstallAdminOnly')}
            </div>
          )}
        </div>
        <DialogFooter className="shrink-0 border-t border-border bg-surface px-4 py-3 sm:px-6 sm:py-4 [&>button]:w-full sm:[&>button]:w-auto">
          <Button disabled={installing} type="button" variant="outline" onClick={onClose}>{t('common.cancel')}</Button>
          <Button disabled={!canSubmit} type="button" onClick={onSubmit}>
            <PackagePlus className={cn('size-4', installing && 'animate-pulse')} />
            {installing ? t('appTemplatesPage.installing') : t('appTemplatesPage.install')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function TemplateProjectVolumePicker({ canCreate, clusterId, clusterName, disabled, mode, onChange, projectId, required, value }: {
  canCreate: boolean
  clusterId: string
  clusterName: string
  disabled: boolean
  mode: 'Block' | 'Filesystem'
  onChange: (value: string) => void
  projectId: string
  required: boolean
  value: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [createdVolume, setCreatedVolume] = useState<ProjectVolume | null>(null)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const volumes = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['app-template-project-volumes', projectId, clusterId, mode, page, search],
    queryFn: () => api.listProjectVolumes(projectId, {
      page,
      pageSize: 20,
      search: search.trim() || undefined,
      availability: 'available',
      clusterId,
      volumeMode: mode,
      sortBy: 'displayName',
      sortOrder: 'asc',
    }),
    enabled: required && Boolean(projectId && clusterId),
  })
  const current = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['app-template-project-volume-current', projectId, clusterId, value],
    queryFn: () => api.getProjectVolume(projectId, value),
    enabled: required && Boolean(projectId && clusterId && value),
  })
  if (!required) {
    return (
      <Select disabled value="">
        <option value="">{t('appTemplatesPage.projectVolumeNotRequired')}</option>
      </Select>
    )
  }

  const options = [...(volumes.data?.items ?? [])]
  if (createdVolume?.clusterId === clusterId && createdVolume.volumeMode === mode && !options.some(item => item.id === createdVolume.id))
    options.unshift(createdVolume)
  if (current.data?.clusterId === clusterId && current.data.volumeMode === mode && !options.some(item => item.id === current.data?.id))
    options.unshift(current.data)
  const totalPages = volumes.data?.totalPages ?? 0
  return (
    <div className="grid min-w-0 gap-2">
      <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
        <Input
          disabled={disabled || !clusterId}
          placeholder={t('projectVolumes.deploymentSelectorPlaceholder')}
          value={search}
          onChange={(event) => {
            setSearch(event.target.value)
            setPage(1)
          }}
        />
        {canCreate && (
          <Button disabled={disabled || !clusterId} type="button" variant="outline" onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            {t('projectVolumes.create')}
          </Button>
        )}
      </div>
      <Select
        disabled={disabled || !clusterId || volumes.isLoading || volumes.isError}
        value={value}
        onChange={event => onChange(event.target.value)}
      >
        <option value="">{t('appTemplatesPage.selectProjectVolume')}</option>
        {options.map(volume => (
          <option key={volume.id} value={volume.id}>
            {volume.displayName}
            {' '}
            ·
            {' '}
            {volume.capacity}
          </option>
        ))}
      </Select>
      {!volumes.isLoading && !volumes.isError && options.length === 0 && <p className="text-xs text-muted-foreground">{t('projectVolumes.deploymentSelectorEmpty')}</p>}
      {volumes.isError && <p className="text-xs text-danger">{t('projectVolumes.loadFailedDescription')}</p>}
      {totalPages > 1 && (
        <div className="flex items-center justify-between gap-2">
          <Button disabled={disabled || page <= 1} size="sm" type="button" variant="ghost" onClick={() => setPage(currentPage => Math.max(1, currentPage - 1))}>
            {t('pagination.previous')}
          </Button>
          <Button disabled={disabled || page >= totalPages} size="sm" type="button" variant="ghost" onClick={() => setPage(currentPage => currentPage + 1)}>
            {t('pagination.next')}
          </Button>
        </div>
      )}
      {canCreate && (
        <ProjectVolumeCreateDialog
          key={`${projectId}:${clusterId}:${mode}`}
          deploymentContext={{ clusterId, clusterName, volumeMode: mode }}
          open={createOpen}
          projectId={projectId}
          onCreated={(volume) => {
            setCreatedVolume(volume)
            onChange(volume.id)
            void queryClient.invalidateQueries({ queryKey: ['app-template-project-volumes', projectId, clusterId, mode] })
          }}
          onOpenChange={setCreateOpen}
        />
      )}
    </div>
  )
}

function templateValueHint(templateId: string, key: string, t: ReturnType<typeof useTranslation>['t']) {
  if (templateId === 'redis' && key === 'password')
    return t('appTemplatesPage.valueHints.redisPassword')
  if (key === 'apiBaseUrl')
    return t('appTemplatesPage.valueHints.apiBaseUrl')
  if (key === 'traefikMetricsUrl')
    return t('appTemplatesPage.valueHints.traefikMetricsUrl')
}

function templateValuePlaceholder(templateId: string, key: string, autoGenerate: boolean, defaultValue: string, t: ReturnType<typeof useTranslation>['t']) {
  if (templateId === 'redis' && key === 'password')
    return t('appTemplatesPage.valuePlaceholders.redisPassword')
  if (key === 'apiBaseUrl')
    return t('appTemplatesPage.valuePlaceholders.apiBaseUrl')
  if (key === 'traefikMetricsUrl')
    return t('appTemplatesPage.valuePlaceholders.traefikMetricsUrl')
  if (autoGenerate)
    return t('appTemplatesPage.autoGeneratePlaceholder')
  return defaultValue
}

function emptyInstallPayload(): AppTemplateInstallPayload {
  return {
    applicationName: '',
    applicationIdentifier: '',
    deploymentName: 'default',
    stage: 'dev',
    clusterId: '',
    namespace: '',
    imageRef: '',
    replicas: 1,
    cpuRequest: '1',
    memoryRequest: '1Gi',
    projectVolumeId: '',
    installNow: true,
    provisionAccess: false,
    values: {},
  }
}

function payloadFromTemplate(template: AppTemplate): AppTemplateInstallPayload {
  const suffix = Math.random().toString(36).slice(2, 8)
  return {
    ...emptyInstallPayload(),
    applicationName: template.name,
    applicationIdentifier: normalizeIdentifierInput(`${template.slug}-${suffix}`).slice(0, APPLICATION_IDENTIFIER_MAX_LENGTH),
    imageRef: template.image,
    replicas: template.defaultReplicas || 1,
    cpuRequest: template.defaultCPU || '1',
    memoryRequest: template.defaultMemory || '1Gi',
    values: Object.fromEntries(template.values.filter(value => !value.autoGenerate).map(value => [value.key, value.default])),
  }
}

function isSystemComponentTemplate(template: Pick<AppTemplate, 'kind' | 'systemComponent'> | null | undefined) {
  return template?.kind === 'system_component' || Boolean(template?.systemComponent)
}

function normalizeIdentifierInput(value: string) {
  return value.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/-+/g, '-').replace(/^-+/, '')
}
