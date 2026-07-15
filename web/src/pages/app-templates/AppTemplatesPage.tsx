import type { ReactNode } from 'react'
import type { AppTemplate, AppTemplateInstallPayload, Project, RuntimeCluster } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Box, CircleHelp, Database, Link2, PackageOpen, Rocket, Search, ShieldCheck } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { CheckboxField } from '@/components/common/checkbox-field'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { ProjectSpaceSelect } from '@/components/common/project-space-select'
import { StatusBadge } from '@/components/common/status-badge'
import { UnitInput } from '@/components/common/unit-input'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
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
  const [selectedTemplateOverride, setSelectedTemplateOverride] = useState<AppTemplate | null>(null)
  const [selectedProjectId, setSelectedProjectId] = useState('')
  const [formState, setFormState] = useState<{ templateId: string, value: AppTemplateInstallPayload } | null>(null)
  const requestedTemplateId = searchParams.get('template')
  const templates = useQuery({ queryKey: ['app-templates'], queryFn: api.listAppTemplates })
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects })
  const projectItems = useMemo(() => projects.data ?? [], [projects.data])
  const projectId = projectItems.some(project => project.id === selectedProjectId)
    ? selectedProjectId
    : projectItems[0]?.id ?? ''
  const requestedTemplate = useMemo(
    () => templates.data?.find(template => template.id === requestedTemplateId) ?? null,
    [requestedTemplateId, templates.data],
  )
  const selectedTemplate = selectedTemplateOverride ?? requestedTemplate
  const selectedTemplateIsSystem = isSystemComponentTemplate(selectedTemplate)
  const canInstallSystemComponent = user?.role === 'platform_admin'
  const defaultForm = useMemo(
    () => selectedTemplate ? payloadFromTemplate(selectedTemplate) : emptyInstallPayload(),
    [selectedTemplate],
  )
  const form = formState && formState.templateId === selectedTemplate?.id ? formState.value : defaultForm
  const clusters = useQuery({
    queryKey: ['runtime-clusters', selectedTemplateIsSystem ? 'system' : projectId],
    queryFn: () => api.listRuntimeClusters(selectedTemplateIsSystem ? undefined : projectId),
    enabled: selectedTemplateIsSystem || Boolean(projectId),
  })
  const clusterItems = clusters.data ?? []

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
    mutationFn: (payload: { templateId: string, clusterId: string, apiBaseUrl: string, traefikMetricsUrl?: string }) =>
      api.installSystemAppTemplate(payload.templateId, {
        apiBaseUrl: payload.apiBaseUrl,
        clusterId: payload.clusterId,
        mode: 'traefik-metrics',
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

  function openInstallDialog(template: AppTemplate) {
    setSelectedTemplateOverride(template)
    setFormState({ templateId: template.id, value: payloadFromTemplate(template) })
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
        ...(current?.templateId === selectedTemplate.id ? current.value : payloadFromTemplate(selectedTemplate)),
        [key]: value,
      },
    }))
  }

  function updateTemplateValue(key: string, value: string) {
    if (!selectedTemplate)
      return
    setFormState((current) => {
      const currentForm = current?.templateId === selectedTemplate.id ? current.value : payloadFromTemplate(selectedTemplate)
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
    <div className="grid gap-4 sm:gap-5">
      <Card className="grid gap-3 p-3 sm:gap-4 sm:p-4 xl:grid-cols-[minmax(0,1fr)_18rem_12rem_10rem_10rem] xl:items-center">
        <div className="min-w-0">
          <h2 className="text-base font-semibold">{t('appTemplatesPage.heroTitle')}</h2>
        </div>
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="h-11 rounded-full pl-10"
            placeholder={t('appTemplatesPage.searchPlaceholder')}
            value={search}
            onChange={event => setSearch(event.target.value)}
          />
        </div>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:contents">
          <Select
            aria-label={t('appTemplatesPage.categoryFilter')}
            className="h-11 min-w-0 rounded-full"
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
            className="h-11 min-w-0 rounded-full"
            value={sortBy}
            onChange={event => setSortBy(event.target.value as typeof sortBy)}
          >
            <option value="popularity">{t('appTemplatesPage.sortByPopularity')}</option>
            <option value="name">{t('appTemplatesPage.sortByName')}</option>
          </Select>
          <Select
            aria-label={t('appTemplatesPage.sortOrder')}
            className="col-span-2 h-11 min-w-0 rounded-full sm:col-span-1"
            value={sortOrder}
            onChange={event => setSortOrder(event.target.value as typeof sortOrder)}
          >
            <option value="desc">{t('appTemplatesPage.sortDesc')}</option>
            <option value="asc">{t('appTemplatesPage.sortAsc')}</option>
          </Select>
        </div>
      </Card>

      {templates.isError && <ErrorState title={templates.error.message} />}
      {templates.isLoading && <EmptyState title={t('appTemplatesPage.loading')} variant="plain" />}
      {templates.isSuccess && sortedTemplates.length === 0 && (
        <EmptyState description={t('appTemplatesPage.emptyDescription')} title={t('appTemplatesPage.emptyTitle')} />
      )}
      {sortedTemplates.length > 0 && (
        <div className="grid gap-3 sm:grid-cols-2 sm:gap-4 xl:grid-cols-3">
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

      <InstallTemplateDialog
        clusterItems={clusterItems}
        clustersLoading={clusters.isLoading}
        canInstallSystemComponent={canInstallSystemComponent}
        form={form}
        installing={installTemplate.isPending || installSystemTemplate.isPending}
        projectId={projectId}
        projects={projectItems}
        template={selectedTemplate}
        onClose={closeInstallDialog}
        onProjectChange={setSelectedProjectId}
        onSubmit={submitInstall}
        onTemplateValueChange={updateTemplateValue}
        onUpdate={updateForm}
      />
    </div>
  )
}

function TemplateCard({ canInstallSystemComponent, template, onInstall }: { canInstallSystemComponent: boolean, template: AppTemplate, onInstall: () => void }) {
  const { t } = useTranslation()
  const CategoryIcon = template.category === 'database' ? Database : Box
  const systemComponent = isSystemComponentTemplate(template)
  const installDisabled = systemComponent && !canInstallSystemComponent
  return (
    <Card className="flex min-h-0 flex-col gap-3 p-4 sm:min-h-56 sm:gap-4 sm:p-5">
      <div className="flex items-start gap-3 sm:gap-4">
        <div className="flex size-12 shrink-0 items-center justify-center rounded-lg border border-border bg-surface sm:size-14 sm:rounded-xl">
          <img
            alt=""
            className="size-8 object-contain sm:size-9"
            src={template.icon || FALLBACK_ICON}
            onError={(event) => {
              event.currentTarget.src = FALLBACK_ICON
            }}
          />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-1.5 sm:gap-2">
            <h2 className="min-w-0 max-w-full truncate text-base font-semibold sm:text-lg">{template.name}</h2>
            <StatusBadge tone="neutral">{t(`appTemplatesPage.categories.${template.category}`, { defaultValue: template.category })}</StatusBadge>
            {systemComponent && <StatusBadge tone="info">{t('appTemplatesPage.platformComponent')}</StatusBadge>}
          </div>
          <p className="mt-1 line-clamp-3 text-sm text-muted-foreground sm:line-clamp-2">
            {t(`appTemplatesPage.templates.${template.id}.description`, { defaultValue: template.description || t('common.noDescription') })}
          </p>
        </div>
      </div>
      <div className="grid gap-2 text-sm text-muted-foreground">
        <TemplateFact label={t('appTemplatesPage.port')} value={String(template.servicePort)} />
        <TemplateFact label={t('appTemplatesPage.resources')} value={`${template.defaultCPU} / ${template.defaultMemory}`} />
      </div>
      <div className="mt-auto flex min-w-0 flex-col gap-3 sm:flex-row sm:items-end sm:justify-between sm:gap-4">
        <div className="grid min-w-0 flex-1 gap-1.5">
          <div className="flex min-w-0 items-center gap-2">
            <TemplateSourceLink
              href={template.officialWebsite}
              icon={<Link2 className="size-4" />}
              label={t('appTemplatesPage.officialWebsite')}
            />
            <TemplateSourceLink
              href={template.officialRepository}
              icon={<GithubMark className="size-4" />}
              label={t('appTemplatesPage.officialRepository')}
            />
          </div>
          <span className="inline-flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground" title={template.image}>
            <CategoryIcon className="size-4 shrink-0" />
            <span className="min-w-0 truncate font-mono">{template.image}</span>
          </span>
        </div>
        <div className="shrink-0 sm:self-end">
          <Button className="w-full rounded-full sm:w-auto" disabled={installDisabled} type="button" onClick={onInstall}>
            {systemComponent ? <ShieldCheck className="size-4" /> : <Rocket className="size-4" />}
            {t('appTemplatesPage.install')}
          </Button>
        </div>
      </div>
    </Card>
  )
}

function TemplateSourceLink({ href, icon, label }: { href: string, icon: ReactNode, label: string }) {
  if (!href)
    return null
  return (
    <a
      aria-label={label}
      className="inline-flex size-4 shrink-0 items-center justify-center text-muted-foreground transition-colors hover:text-primary focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
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

function TemplateFact({ label, value }: { label: string, value: string }) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-3">
      <span className="shrink-0">{label}</span>
      <span className="min-w-0 truncate font-mono text-foreground">{value}</span>
    </div>
  )
}

function InstallTemplateDialog({
  clusterItems,
  clustersLoading,
  canInstallSystemComponent,
  form,
  installing,
  projectId,
  projects,
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
  form: AppTemplateInstallPayload
  installing: boolean
  projectId: string
  projects: Project[]
  template: AppTemplate | null
  onClose: () => void
  onProjectChange: (value: string) => void
  onSubmit: () => void
  onTemplateValueChange: (key: string, value: string) => void
  onUpdate: <K extends keyof AppTemplateInstallPayload>(key: K, value: AppTemplateInstallPayload[K]) => void
}) {
  const { t } = useTranslation()
  const systemComponent = isSystemComponentTemplate(template)
  const canSubmit = systemComponent
    ? Boolean(template && canInstallSystemComponent && form.clusterId.trim() && (form.values.apiBaseUrl ?? '').trim() && !installing)
    : Boolean(template && projectId && form.applicationName.trim() && form.applicationSlug.trim() && form.imageRef.trim() && !installing)
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
                  onChange={event => onUpdate('clusterId', event.target.value)}
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
              <Field label={t('appTemplatesPage.applicationSlug')} required>
                <Input value={form.applicationSlug} onChange={event => onUpdate('applicationSlug', normalizeSlugInput(event.target.value))} />
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
              <div className="grid gap-4 md:col-span-2 md:grid-cols-4 md:gap-5">
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
                <Field label={t('appTemplatesPage.dataCapacity')}>
                  <UnitInput
                    disabled={!template?.dataRetentionEnabled}
                    inputProps={{ placeholder: t('deploymentsPage.dataCapacityPlaceholder') }}
                    unitSelectLabel={t('appTemplatesPage.dataCapacity')}
                    units={[
                      { label: 'Mi', value: 'Mi' },
                      { label: 'Gi', value: 'Gi' },
                    ]}
                    value={form.dataCapacity}
                    onChange={value => onUpdate('dataCapacity', value)}
                  />
                </Field>
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
            </div>
          )}

          {template && template.values.length > 0 && (
            <div className="mt-5 grid gap-4 border-t border-border pt-5 sm:mt-6">
              <div>
                <h3 className="font-semibold">{t('appTemplatesPage.templateParameters')}</h3>
                <p className="mt-1 text-sm text-muted-foreground">{t('appTemplatesPage.templateParametersDescription')}</p>
              </div>
              <div className="grid gap-4 md:grid-cols-2 md:gap-5">
                {template.values.map(value => (
                  <Field
                    key={value.key}
                    hint={templateValueHint(value.key, t)}
                    label={t(`appTemplatesPage.valueLabels.${value.key}`, { defaultValue: value.label || value.key })}
                    required={value.required && !value.autoGenerate}
                  >
                    <Input
                      placeholder={templateValuePlaceholder(value.key, value.autoGenerate, value.default, t)}
                      type={value.secret ? 'password' : 'text'}
                      value={form.values[value.key] ?? ''}
                      onChange={event => onTemplateValueChange(value.key, event.target.value)}
                    />
                  </Field>
                ))}
              </div>
            </div>
          )}

          {!systemComponent && (
            <CheckboxField
              checked={form.installNow}
              className="mt-5 rounded-lg border border-border p-3 sm:mt-6 sm:p-4"
              description={t('appTemplatesPage.installNowDescription')}
              disabled={installing}
              onChange={event => onUpdate('installNow', event.target.checked)}
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
            <PackageOpen className={cn('size-4', installing && 'animate-pulse')} />
            {installing ? t('appTemplatesPage.installing') : t('appTemplatesPage.install')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Field({ children, hint, label, required }: { children: React.ReactNode, hint?: string, label: string, required?: boolean }) {
  const { t } = useTranslation()

  return (
    <div className="grid gap-2">
      <Label className="flex w-fit items-center gap-1.5">
        <span>
          {label}
          {required && <span className="ml-1 text-primary">*</span>}
        </span>
        {hint && (
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                aria-label={`${label}${t('common.helpSuffix')}`}
                className="inline-flex shrink-0 text-muted-foreground outline-none hover:text-primary focus:text-primary"
                tabIndex={-1}
                type="button"
              >
                <CircleHelp className="size-3.5 transition" />
              </button>
            </TooltipTrigger>
            <TooltipContent className="max-w-80 leading-5" side="top">
              {hint}
            </TooltipContent>
          </Tooltip>
        )}
      </Label>
      {children}
    </div>
  )
}

function templateValueHint(key: string, t: ReturnType<typeof useTranslation>['t']) {
  if (key === 'apiBaseUrl')
    return t('appTemplatesPage.valueHints.apiBaseUrl')
  if (key === 'traefikMetricsUrl')
    return t('appTemplatesPage.valueHints.traefikMetricsUrl')
}

function templateValuePlaceholder(key: string, autoGenerate: boolean, defaultValue: string, t: ReturnType<typeof useTranslation>['t']) {
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
    applicationSlug: '',
    deploymentName: 'default',
    stage: 'prod',
    clusterId: '',
    namespace: '',
    imageRef: '',
    replicas: 1,
    cpuRequest: '1',
    memoryRequest: '1Gi',
    dataCapacity: '',
    installNow: true,
    values: {},
  }
}

function payloadFromTemplate(template: AppTemplate): AppTemplateInstallPayload {
  const suffix = Math.random().toString(36).slice(2, 8)
  return {
    ...emptyInstallPayload(),
    applicationName: template.name,
    applicationSlug: normalizeSlugInput(`${template.slug}-${suffix}`).slice(0, 20),
    imageRef: template.image,
    replicas: template.defaultReplicas || 1,
    cpuRequest: template.defaultCPU || '1',
    memoryRequest: template.defaultMemory || '1Gi',
    dataCapacity: template.dataCapacity,
    values: Object.fromEntries(template.values.filter(value => !value.autoGenerate).map(value => [value.key, value.default])),
  }
}

function isSystemComponentTemplate(template: AppTemplate | null | undefined) {
  return template?.kind === 'system_component' || Boolean(template?.systemComponent)
}

function normalizeSlugInput(value: string) {
  return value.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/-+/g, '-').replace(/^-+/, '')
}
