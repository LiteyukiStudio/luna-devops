import type { ReactNode } from 'react'
import type { AppTemplate, AppTemplateInstallPayload, Project, RuntimeCluster } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Box, Database, Link2, PackageOpen, Rocket, Search } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
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
import { cn } from '@/lib/utils'

const FALLBACK_ICON = '/app-templates/icons/fallback.svg'

export function AppTemplatesPage() {
  const { i18n, t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState('all')
  const [sortBy, setSortBy] = useState<'popularity' | 'name'>('popularity')
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc')
  const [selectedTemplate, setSelectedTemplate] = useState<AppTemplate | null>(null)
  const [projectId, setProjectId] = useState('')
  const [form, setForm] = useState<AppTemplateInstallPayload>(emptyInstallPayload())

  const templates = useQuery({ queryKey: ['app-templates'], queryFn: api.listAppTemplates })
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects })
  const clusters = useQuery({
    queryKey: ['runtime-clusters', projectId],
    queryFn: () => api.listRuntimeClusters(projectId),
    enabled: Boolean(projectId),
  })
  const projectItems = projects.data ?? []
  const clusterItems = clusters.data ?? []

  useEffect(() => {
    if (!projectId && projectItems.length > 0)
      setProjectId(projectItems[0].id)
  }, [projectId, projectItems])

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
      setSelectedTemplate(null)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['projects'] }),
        queryClient.invalidateQueries({ queryKey: ['applications', result.application.projectId] }),
      ])
      navigate(`/projects/${result.application.projectId}/apps/${result.application.id}#tab=deployments`)
    },
    onError: error => toast.error(error.message),
  })

  function openInstallDialog(template: AppTemplate) {
    setSelectedTemplate(template)
    setForm(payloadFromTemplate(template))
  }

  function updateForm<K extends keyof AppTemplateInstallPayload>(key: K, value: AppTemplateInstallPayload[K]) {
    setForm(current => ({ ...current, [key]: value }))
  }

  function updateTemplateValue(key: string, value: string) {
    setForm(current => ({ ...current, values: { ...current.values, [key]: value } }))
  }

  function submitInstall() {
    if (!selectedTemplate || !projectId)
      return
    installTemplate.mutate({ ...form, projectId, templateId: selectedTemplate.id })
  }

  return (
    <div className="grid gap-5">
      <Card className="grid gap-4 p-4 xl:grid-cols-[minmax(0,1fr)_12rem_18rem_10rem_10rem] xl:items-center">
        <div className="min-w-0">
          <h2 className="text-base font-semibold">{t('appTemplatesPage.heroTitle')}</h2>
          <p className="mt-1 text-sm text-muted-foreground">{t('appTemplatesPage.heroDescription')}</p>
        </div>
        <Select
          aria-label={t('appTemplatesPage.categoryFilter')}
          className="h-11 rounded-full"
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
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="h-11 rounded-full pl-10"
            placeholder={t('appTemplatesPage.searchPlaceholder')}
            value={search}
            onChange={event => setSearch(event.target.value)}
          />
        </div>
        <Select
          aria-label={t('appTemplatesPage.sortBy')}
          className="h-11 rounded-full"
          value={sortBy}
          onChange={event => setSortBy(event.target.value as typeof sortBy)}
        >
          <option value="popularity">{t('appTemplatesPage.sortByPopularity')}</option>
          <option value="name">{t('appTemplatesPage.sortByName')}</option>
        </Select>
        <Select
          aria-label={t('appTemplatesPage.sortOrder')}
          className="h-11 rounded-full"
          value={sortOrder}
          onChange={event => setSortOrder(event.target.value as typeof sortOrder)}
        >
          <option value="desc">{t('appTemplatesPage.sortDesc')}</option>
          <option value="asc">{t('appTemplatesPage.sortAsc')}</option>
        </Select>
      </Card>

      {templates.isError && <ErrorState title={templates.error.message} />}
      {templates.isLoading && <EmptyState title={t('appTemplatesPage.loading')} variant="plain" />}
      {templates.isSuccess && sortedTemplates.length === 0 && (
        <EmptyState description={t('appTemplatesPage.emptyDescription')} title={t('appTemplatesPage.emptyTitle')} />
      )}
      {sortedTemplates.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {sortedTemplates.map(template => (
            <TemplateCard key={template.id} template={template} onInstall={() => openInstallDialog(template)} />
          ))}
        </div>
      )}

      <InstallTemplateDialog
        clusterItems={clusterItems}
        clustersLoading={clusters.isLoading}
        form={form}
        installing={installTemplate.isPending}
        projectId={projectId}
        projects={projectItems}
        template={selectedTemplate}
        onClose={() => setSelectedTemplate(null)}
        onProjectChange={setProjectId}
        onSubmit={submitInstall}
        onTemplateValueChange={updateTemplateValue}
        onUpdate={updateForm}
      />
    </div>
  )
}

function TemplateCard({ template, onInstall }: { template: AppTemplate, onInstall: () => void }) {
  const { t } = useTranslation()
  const CategoryIcon = template.category === 'database' ? Database : Box
  return (
    <Card className="flex min-h-56 flex-col gap-4 p-5">
      <div className="flex items-start gap-4">
        <div className="flex size-14 shrink-0 items-center justify-center rounded-xl border border-border bg-surface">
          <img
            alt=""
            className="size-9 object-contain"
            src={template.icon || FALLBACK_ICON}
            onError={(event) => {
              event.currentTarget.src = FALLBACK_ICON
            }}
          />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h2 className="truncate text-lg font-semibold">{template.name}</h2>
            <StatusBadge tone="neutral">{t(`appTemplatesPage.categories.${template.category}`, { defaultValue: template.category })}</StatusBadge>
          </div>
          <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">
            {t(`appTemplatesPage.templates.${template.id}.description`, { defaultValue: template.description || t('common.noDescription') })}
          </p>
        </div>
      </div>
      <div className="grid gap-2 text-sm text-muted-foreground">
        <TemplateFact label={t('appTemplatesPage.port')} value={String(template.servicePort)} />
        <TemplateFact label={t('appTemplatesPage.resources')} value={`${template.defaultCPU} / ${template.defaultMemory}`} />
      </div>
      <div className="mt-auto flex items-end justify-between gap-4">
        <div className="grid min-w-0 flex-1 gap-1.5">
          <div className="flex items-center gap-2">
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
        <div className="shrink-0">
          <Button className="rounded-full" type="button" onClick={onInstall}>
            <Rocket className="size-4" />
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
  const canSubmit = Boolean(template && projectId && form.applicationName.trim() && form.applicationSlug.trim() && form.imageRef.trim() && !installing)
  return (
    <Dialog open={Boolean(template)} onOpenChange={open => !open && onClose()}>
      <DialogContent className="flex max-h-[min(92vh,54rem)] max-w-4xl flex-col gap-0 overflow-hidden p-0">
        <DialogHeader className="shrink-0 border-b border-border px-6 py-5">
          <DialogTitle>{t('appTemplatesPage.installDialogTitle', { name: template?.name ?? '' })}</DialogTitle>
          <DialogDescription>{t('appTemplatesPage.installDialogDescription')}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
          <div className="grid gap-5 md:grid-cols-2">
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
            <div className="grid gap-5 md:col-span-2 md:grid-cols-4">
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

          {template && template.values.length > 0 && (
            <div className="mt-6 grid gap-4 border-t border-border pt-5">
              <div>
                <h3 className="font-semibold">{t('appTemplatesPage.templateParameters')}</h3>
                <p className="mt-1 text-sm text-muted-foreground">{t('appTemplatesPage.templateParametersDescription')}</p>
              </div>
              <div className="grid gap-5 md:grid-cols-2">
                {template.values.map(value => (
                  <Field
                    key={value.key}
                    label={t(`appTemplatesPage.valueLabels.${value.key}`, { defaultValue: value.label || value.key })}
                    required={value.required && !value.autoGenerate}
                  >
                    <Input
                      placeholder={value.autoGenerate ? t('appTemplatesPage.autoGeneratePlaceholder') : value.default}
                      type={value.secret ? 'password' : 'text'}
                      value={form.values[value.key] ?? ''}
                      onChange={event => onTemplateValueChange(value.key, event.target.value)}
                    />
                  </Field>
                ))}
              </div>
            </div>
          )}

          <CheckboxField
            checked={form.installNow}
            className="mt-6 rounded-lg border border-border p-4"
            description={t('appTemplatesPage.installNowDescription')}
            disabled={installing}
            onChange={event => onUpdate('installNow', event.target.checked)}
          >
            {t('appTemplatesPage.installNow')}
          </CheckboxField>
        </div>
        <DialogFooter className="shrink-0 border-t border-border bg-surface px-6 py-4">
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

function Field({ children, label, required }: { children: React.ReactNode, label: string, required?: boolean }) {
  return (
    <div className="grid gap-2">
      <Label>
        {label}
        {required && <span className="ml-1 text-primary">*</span>}
      </Label>
      {children}
    </div>
  )
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

function normalizeSlugInput(value: string) {
  return value.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/-+/g, '-').replace(/^-+/, '')
}
