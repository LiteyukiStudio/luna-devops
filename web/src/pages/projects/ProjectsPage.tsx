import type { Project, ProjectListScope } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { ArrowDownWideNarrow, ArrowUpNarrowWide, FolderKanban, MoreHorizontal, Pencil, Plus, Search, Trash2 } from 'lucide-react'
import { useDeferredValue, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { HoverText } from '@/components/common/hover-text'
import { PageShell } from '@/components/common/page-shell'
import { PageToolbar } from '@/components/common/page-toolbar'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { PROJECT_SLUG_MAX_LENGTH } from '@/lib/slug-limits'

const schema = z.object({
  name: z.string().min(1, i18next.t('projectSpaces.nameRequired')),
  slug: z.string().min(1, i18next.t('projectSpaces.slugRequired')).max(PROJECT_SLUG_MAX_LENGTH, i18next.t('projectSpaces.slugMaxLength', { count: PROJECT_SLUG_MAX_LENGTH })).regex(/^[a-z0-9-]+$/, i18next.t('common.lowercaseSlugOnly')),
  description: z.string().optional(),
  maxConcurrentBuilds: z.number().int().min(1, i18next.t('projectSpaces.maxConcurrentBuildsMin')),
  webConsoleEnabled: z.boolean(),
})

type ProjectForm = z.infer<typeof schema>

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]
const PROJECT_SCOPE_OPTIONS = ['related', 'all'] as const
const PROJECT_SORT_OPTIONS = ['lastUsed', 'useCount', 'createdAt', 'updatedAt', 'name'] as const

type ProjectSortBy = typeof PROJECT_SORT_OPTIONS[number]
type ProjectSortOrder = 'asc' | 'desc'

export function ProjectsPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const [editingProject, setEditingProject] = useState<Project | null>(null)
  const [projectToDelete, setProjectToDelete] = useState<Project | null>(null)
  const [deleteConfirmation, setDeleteConfirmation] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [search, setSearch] = useState('')
  const [scope, setScope] = useState<ProjectListScope>('related')
  const [sortBy, setSortBy] = useState<ProjectSortBy>('lastUsed')
  const [sortOrder, setSortOrder] = useState<ProjectSortOrder>('desc')
  const deferredSearch = useDeferredValue(search.trim())
  const canViewAllProjects = user?.role === 'platform_admin'
  const effectiveScope: ProjectListScope = canViewAllProjects ? scope : 'related'
  const projects = useQuery({
    queryKey: ['projects', 'page', page, pageSize, effectiveScope, sortBy, sortOrder, deferredSearch],
    queryFn: () => api.listProjectsPage({ page, pageSize, scope: effectiveScope, search: deferredSearch || undefined, sortBy, sortOrder }),
  })
  const projectItems = Array.isArray(projects.data) ? projects.data : projects.data?.items ?? []
  const projectTotal = Array.isArray(projects.data) ? projects.data.length : projects.data?.total ?? 0
  const projectTotalPages = Math.max(1, Array.isArray(projects.data) ? 1 : projects.data?.totalPages ?? 1)
  const projectPage = Array.isArray(projects.data) ? 1 : projects.data?.page ?? page
  const projectPageSize = Array.isArray(projects.data) ? pageSize : projects.data?.pageSize ?? pageSize
  const deleteConfirmationTarget = projectToDelete?.name ?? ''
  const deleteConfirmationMatches = Boolean(deleteConfirmationTarget) && deleteConfirmation === deleteConfirmationTarget
  const form = useForm<ProjectForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: { name: '', slug: '', description: '', maxConcurrentBuilds: 2, webConsoleEnabled: true },
  })

  const createProject = useMutation({
    mutationFn: api.createProject,
    onSuccess: () => {
      toast.success(t('projectSpaces.created'))
      form.reset()
      setDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['projects'] })
    },
    onError: error => toast.error(error.message),
  })

  const deleteProject = useMutation({
    mutationFn: api.deleteProject,
    onSuccess: async () => {
      toast.success(t('projectSpaces.deleted'))
      setProjectToDelete(null)
      setDeleteConfirmation('')
      if (projectItems.length <= 1 && page > 1)
        setPage(page - 1)
      await queryClient.invalidateQueries({ queryKey: ['projects'] })
      await queryClient.refetchQueries({ queryKey: ['projects', 'page'], type: 'active' })
    },
    onError: error => toast.error(error.message),
  })

  const updateProject = useMutation({
    mutationFn: ({ projectId, payload }: { projectId: string, payload: Pick<Project, 'slug' | 'name' | 'description' | 'maxConcurrentBuilds' | 'webConsoleEnabled'> }) =>
      api.updateProject(projectId, payload),
    onSuccess: () => {
      toast.success(t('projectSpaces.updated'))
      form.reset()
      setEditingProject(null)
      setDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['projects'] })
    },
    onError: error => toast.error(error.message),
  })

  return (
    <PageShell spacing="compact" width="full">
      <PageToolbar
        actions={(
          <Button
            className="shrink-0"
            onClick={() => {
              setEditingProject(null)
              form.reset({ name: '', slug: '', description: '', maxConcurrentBuilds: 2, webConsoleEnabled: true })
              setDialogOpen(true)
            }}
          >
            <Plus size={16} />
            <span className="hidden sm:inline">{t('projectSpaces.createTitle')}</span>
            <span className="sm:hidden">{t('create')}</span>
          </Button>
        )}
      >
        <div className="relative min-w-0 flex-1 sm:max-w-sm sm:basis-72">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            aria-label={t('projectSpaces.searchProjects')}
            className="h-9 pl-9"
            placeholder={t('projectSpaces.searchProjects')}
            type="search"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value)
              setPage(1)
            }}
          />
        </div>
        {canViewAllProjects && (
          <Select
            aria-label={t('projectSpaces.scope')}
            containerClassName="min-w-32 flex-1 sm:w-40 sm:flex-none"
            value={scope}
            onChange={(event) => {
              setScope(event.target.value as ProjectListScope)
              setPage(1)
            }}
          >
            {PROJECT_SCOPE_OPTIONS.map(option => (
              <option key={option} value={option}>{t(`projectSpaces.scopeOptions.${option}`)}</option>
            ))}
          </Select>
        )}
        <Select
          aria-label={t('projectSpaces.sortBy')}
          containerClassName="min-w-32 flex-1 sm:w-40 sm:flex-none"
          value={sortBy}
          onChange={(event) => {
            setSortBy(event.target.value as ProjectSortBy)
            setPage(1)
          }}
        >
          {PROJECT_SORT_OPTIONS.map(option => (
            <option key={option} value={option}>{t(`projectSpaces.sort.${option}`)}</option>
          ))}
        </Select>
        <Button
          aria-label={t('projectSpaces.sortOrder')}
          size="icon"
          title={t(`projectSpaces.sortOrderOptions.${sortOrder}`)}
          variant="outline"
          onClick={() => {
            setSortOrder(current => current === 'desc' ? 'asc' : 'desc')
            setPage(1)
          }}
        >
          {sortOrder === 'desc' ? <ArrowDownWideNarrow size={16} /> : <ArrowUpNarrowWide size={16} />}
          <span className="sr-only">{t(`projectSpaces.sortOrderOptions.${sortOrder}`)}</span>
        </Button>
      </PageToolbar>
      {projects.isError && <ErrorState title={t('projectSpaces.loadFailedTitle')} description={t('projectSpaces.loadFailedDescription')} />}
      <DataList
        columns={[
          {
            key: 'name',
            header: t('projectSpaces.title'),
            className: 'px-4 py-3 align-middle',
            width: 'primary',
            render: project => <ProjectSummary project={project} />,
          },
          {
            key: 'slug',
            header: t('common.slug'),
            className: 'px-4 py-3 align-middle text-muted-foreground',
            width: 'secondary',
            render: project => <code className="rounded bg-background px-2 py-1 text-xs">{project.slug}</code>,
          },
          {
            key: 'namespaceStrategy',
            header: t('projectSpaces.namespaceStrategy'),
            className: 'px-4 py-3 align-middle',
            width: 'status',
            render: project => (
              project.namespaceStrategy === 'project'
                ? <StatusBadge>{t('projectSpaces.namespaceProject')}</StatusBadge>
                : <span className="text-muted-foreground">—</span>
            ),
          },
          {
            key: 'usage',
            header: t('projectSpaces.usage'),
            className: 'px-4 py-3 align-middle',
            width: 'secondary',
            render: project => <ProjectUsage project={project} />,
          },
          {
            key: 'actions',
            header: t('common.actions'),
            className: 'whitespace-nowrap text-right',
            sticky: 'right',
            render: (project) => {
              const deleting = project.deleteStatus === 'deleting'
              const systemProject = Boolean(project.systemKey)
              const openEditDialog = () => {
                if (systemProject)
                  return
                setEditingProject(project)
                form.reset({
                  name: project.name,
                  slug: project.slug,
                  description: project.description,
                  maxConcurrentBuilds: project.maxConcurrentBuilds || 2,
                  webConsoleEnabled: project.webConsoleEnabled ?? true,
                })
                setDialogOpen(true)
              }
              const openDeleteDialog = () => {
                if (systemProject)
                  return
                setProjectToDelete(project)
                setDeleteConfirmation('')
              }
              return (
                <div className="flex w-max items-center justify-end gap-1">
                  <Link
                    aria-disabled={deleting}
                    aria-label={t('projectSpaces.openWorkspace')}
                    className={buttonVariants({ className: deleting ? 'pointer-events-none opacity-50' : undefined, size: 'sm', variant: 'ghost' })}
                    title={t('projectSpaces.openWorkspace')}
                    to={`/projects/${project.id}`}
                  >
                    <FolderKanban size={16} />
                    <span className="hidden lg:inline">{t('projectSpaces.openWorkspace')}</span>
                  </Link>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button aria-label={t('common.actions')} size="icon" variant="ghost">
                        <MoreHorizontal size={16} />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem disabled={deleting || systemProject} onSelect={openEditDialog}>
                        <Pencil size={16} />
                        {t('edit')}
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem disabled={deleting || systemProject} variant="destructive" onSelect={openDeleteDialog}>
                        <Trash2 size={16} />
                        {t('common.delete')}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              )
            },
          },
        ]}
        emptyDescription={t('projectSpaces.emptyDescription')}
        emptyTitle={t('projectSpaces.emptyTitle')}
        items={projectItems}
        pagination={{
          page: projectPage,
          pageSize: projectPageSize,
          pageSizeOptions: PAGE_SIZE_OPTIONS,
          total: projectTotal,
          totalPages: projectTotalPages,
          pageInfoLabel: t('pagination.pageInfo', {
            page: projectPage,
            totalPages: projectTotalPages,
            total: projectTotal,
          }),
          onPageChange: setPage,
          onPageSizeChange: (nextPageSize) => {
            setPageSize(nextPageSize)
            setPage(1)
          },
        }}
        rowKey={project => project.id}
      />
      <ConfirmDialog
        confirmDisabled={!deleteConfirmationMatches}
        confirmText={t('projectSpaces.deleteConfirm')}
        content={(
          <div className="grid gap-2">
            <Label htmlFor="project-delete-confirmation">{t('projectSpaces.deleteConfirmationLabel', { name: deleteConfirmationTarget })}</Label>
            <Input
              id="project-delete-confirmation"
              aria-invalid={Boolean(deleteConfirmation) && !deleteConfirmationMatches}
              autoComplete="off"
              placeholder={deleteConfirmationTarget}
              value={deleteConfirmation}
              onChange={event => setDeleteConfirmation(event.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t('projectSpaces.deleteConfirmationHint')}</p>
          </div>
        )}
        description={projectToDelete ? t('projectSpaces.deleteDescription', { name: projectToDelete.name }) : ''}
        open={Boolean(projectToDelete)}
        pending={deleteProject.isPending || projectToDelete?.deleteStatus === 'deleting'}
        title={t('projectSpaces.deleteTitle')}
        onConfirm={() => {
          if (projectToDelete && deleteConfirmationMatches)
            deleteProject.mutate(projectToDelete.id)
        }}
        onOpenChange={(open) => {
          if (!open) {
            setProjectToDelete(null)
            setDeleteConfirmation('')
          }
        }}
      />
      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open) {
            setEditingProject(null)
            form.reset({ name: '', slug: '', description: '', maxConcurrentBuilds: 2, webConsoleEnabled: true })
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingProject ? t('projectSpaces.editTitle') : t('projectSpaces.createTitle')}</DialogTitle>
            <DialogDescription>{t('projectSpaces.description')}</DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-3"
            onSubmit={form.handleSubmit((values) => {
              const payload = { ...values, description: values.description ?? '' }
              if (editingProject) {
                updateProject.mutate({ projectId: editingProject.id, payload })
                return
              }
              createProject.mutate(payload)
            })}
          >
            <Field error={form.formState.errors.name?.message} hint={t('projectSpaces.nameHint')} label={t('projectSpaces.name')} required>
              <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder={t('projectSpaces.namePlaceholder')} />
            </Field>
            <Field error={form.formState.errors.slug?.message} hint={t('projectSpaces.slugHint', { count: PROJECT_SLUG_MAX_LENGTH })} label={t('projectSpaces.slug')} required>
              <Input {...form.register('slug')} aria-invalid={Boolean(form.formState.errors.slug)} maxLength={PROJECT_SLUG_MAX_LENGTH} placeholder={t('projectSpaces.slugPlaceholder')} />
            </Field>
            <Field error={form.formState.errors.description?.message} hint={t('projectSpaces.descriptionHint')} label={t('projectSpaces.descriptionLabel')}>
              <Textarea {...form.register('description')} placeholder={t('projectSpaces.descriptionPlaceholder')} />
            </Field>
            <Field error={form.formState.errors.maxConcurrentBuilds?.message} hint={t('projectSpaces.maxConcurrentBuildsHint')} label={t('projectSpaces.maxConcurrentBuilds')} required>
              <Input
                {...form.register('maxConcurrentBuilds', { valueAsNumber: true })}
                aria-invalid={Boolean(form.formState.errors.maxConcurrentBuilds)}
                inputMode="numeric"
                min={1}
                placeholder={t('projectSpaces.maxConcurrentBuildsPlaceholder')}
                type="number"
              />
            </Field>
            <ProgressiveSection
              description={t('projectSpaces.webConsoleSettingsDescription')}
              storageKey="luna.projects.form.webConsole"
              summary={t(form.watch('webConsoleEnabled') ? 'projectSpaces.webConsoleEnabledSummary' : 'projectSpaces.webConsoleDisabledSummary')}
              title={t('projectSpaces.webConsoleSettingsTitle')}
            >
              <Field hint={t('projectSpaces.webConsoleEnabledHint')} label={t('projectSpaces.webConsoleEnabled')}>
                <Select {...form.register('webConsoleEnabled', { setValueAs: value => String(value) !== 'false' })}>
                  <option value="true">{t('common.enabled')}</option>
                  <option value="false">{t('common.disabled')}</option>
                </Select>
              </Field>
            </ProgressiveSection>
            <DialogFooter>
              <Button disabled={createProject.isPending || updateProject.isPending || !form.formState.isValid} type="submit">
                <Plus size={16} />
                {editingProject ? t('save') : t('create')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </PageShell>
  )
}

function ProjectUsage({ project }: { project: Project }) {
  const { t } = useTranslation()
  const useCount = project.useCount ?? 0

  if (!project.lastUsedAt && useCount === 0)
    return <span className="text-muted-foreground">—</span>

  return (
    <div className="grid gap-1">
      {project.lastUsedAt && <span className="text-sm text-foreground">{formatSmartDateTime(project.lastUsedAt, t)}</span>}
      {useCount > 0 && <span className="text-xs text-muted-foreground">{t('projectSpaces.useCount', { count: useCount })}</span>}
    </div>
  )
}

function ProjectSummary({ project }: { project: Project }) {
  const { t } = useTranslation()
  const deleting = project.deleteStatus === 'deleting'
  const deleteFailedMessage = project.deleteStatus === 'delete_failed' ? project.deleteMessage?.trim() : ''
  return (
    <div className="flex min-w-0 items-center gap-3">
      <span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <FolderKanban size={18} />
      </span>
      <div className="min-w-0 w-full">
        <Link aria-disabled={deleting} className={`truncate font-medium transition hover:text-primary-text ${deleting ? 'pointer-events-none opacity-60' : ''}`} to={`/projects/${project.id}`}>
          {project.name}
        </Link>
        {project.deleteStatus && project.deleteStatus !== 'active' && (
          <div className="mt-1 flex min-w-0 items-center gap-2">
            <StatusValueBadge labelKeyPrefix="apps.deleteStatuses" value={project.deleteStatus} />
            {deleteFailedMessage && (
              <HoverText className="max-w-60 flex-1 text-xs text-muted-foreground" value={deleteFailedMessage}>
                {compactProjectDeleteMessage(deleteFailedMessage, t)}
              </HoverText>
            )}
          </div>
        )}
        <p className="truncate text-sm text-muted-foreground">
          {project.description || t('common.noDescription')}
        </p>
      </div>
    </div>
  )
}

function compactProjectDeleteMessage(message: string, t: (key: string, options?: Record<string, unknown>) => string) {
  const normalized = message.trim()
  if (!normalized)
    return ''
  if (normalized.includes('kubeconfig') || normalized.includes('KUBERNETES_MASTER'))
    return t('projectSpaces.deleteFailedReasons.kubeconfigInvalid')
  if (normalized.includes('connection refused') || normalized.includes('connect:'))
    return t('projectSpaces.deleteFailedReasons.clusterUnreachable')
  return t('projectSpaces.deleteFailedReasons.generic')
}
