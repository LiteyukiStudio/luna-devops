import type { Project } from '@/api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { FolderKanban, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api/client'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { EditActionButton } from '@/components/common/edit-action-button'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { MotionItem, MotionList } from '@/components/common/motion'
import { PageHeader } from '@/components/common/page-header'
import { PaginationController } from '@/components/common/pagination'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

const schema = z.object({
  name: z.string().min(1, i18next.t('projectSpaces.nameRequired')),
  slug: z.string().min(1, i18next.t('projectSpaces.slugRequired')).regex(/^[a-z0-9-]+$/, i18next.t('common.lowercaseSlugOnly')),
  description: z.string().optional(),
})

type ProjectForm = z.infer<typeof schema>

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

export function ProjectsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editingProject, setEditingProject] = useState<Project | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const projects = useQuery({
    queryKey: ['projects', 'page', page, pageSize],
    queryFn: () => api.listProjectsPage({ page, pageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const projectItems = Array.isArray(projects.data) ? projects.data : projects.data?.items ?? []
  const projectTotal = Array.isArray(projects.data) ? projects.data.length : projects.data?.total ?? 0
  const projectTotalPages = Array.isArray(projects.data) ? 1 : projects.data?.totalPages ?? 0
  const projectPage = Array.isArray(projects.data) ? 1 : projects.data?.page ?? page
  const projectPageSize = Array.isArray(projects.data) ? pageSize : projects.data?.pageSize ?? pageSize
  const form = useForm<ProjectForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: { name: '', slug: '', description: '' },
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
    onSuccess: () => {
      toast.success(t('projectSpaces.deleted'))
      queryClient.invalidateQueries({ queryKey: ['projects'] })
    },
    onError: error => toast.error(error.message),
  })

  const updateProject = useMutation({
    mutationFn: ({ projectId, payload }: { projectId: string, payload: Pick<Project, 'slug' | 'name' | 'description'> }) =>
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
    <div className="grid gap-6">
      <PageHeader
        actions={(
          <Button
            onClick={() => {
              setEditingProject(null)
              form.reset({ name: '', slug: '', description: '' })
              setDialogOpen(true)
            }}
          >
            <Plus size={16} />
            {t('projectSpaces.createTitle')}
          </Button>
        )}
        description={t('projectSpaces.description')}
        title={t('projectSpaces.title')}
      />
      <div className="grid min-h-0 gap-4">
        <div className="max-h-[calc(100vh-18rem)] min-h-0 overflow-y-auto pr-1">
          <MotionList className="grid gap-3">
            {projects.isError && <ErrorState title={t('projectSpaces.loadFailedTitle')} description={t('projectSpaces.loadFailedDescription')} />}
            {projectItems.map(project => (
              <MotionItem key={project.id}>
                <ProjectRow
                  deletePending={deleteProject.isPending}
                  onDelete={() => deleteProject.mutate(project.id)}
                  onEdit={() => {
                    setEditingProject(project)
                    form.reset({
                      name: project.name,
                      slug: project.slug,
                      description: project.description,
                    })
                    setDialogOpen(true)
                  }}
                  project={project}
                />
              </MotionItem>
            ))}
            {projectItems.length === 0 && <EmptyState title={t('projectSpaces.emptyTitle')} description={t('projectSpaces.emptyDescription')} />}
          </MotionList>
        </div>
        {projectTotal > 0 && (
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-3 text-sm text-muted-foreground">
            <span>
              {t('pagination.pageInfo', {
                page: projectPage,
                totalPages: projectTotalPages,
                total: projectTotal,
              })}
            </span>
            <PaginationController
              initialPage={projectPage}
              pageSize={projectPageSize}
              pageSizeOptions={PAGE_SIZE_OPTIONS}
              total={projectTotal}
              onPageChange={setPage}
              onPageSizeChange={(nextPageSize) => {
                setPageSize(nextPageSize)
                setPage(1)
              }}
            />
          </div>
        )}
      </div>
      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open) {
            setEditingProject(null)
            form.reset({ name: '', slug: '', description: '' })
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
            <Field error={form.formState.errors.slug?.message} hint={t('projectSpaces.slugHint')} label={t('projectSpaces.slug')} required>
              <Input {...form.register('slug')} aria-invalid={Boolean(form.formState.errors.slug)} placeholder={t('projectSpaces.slugPlaceholder')} />
            </Field>
            <Field error={form.formState.errors.description?.message} hint={t('projectSpaces.descriptionHint')} label={t('projectSpaces.descriptionLabel')}>
              <Textarea {...form.register('description')} placeholder={t('projectSpaces.descriptionPlaceholder')} />
            </Field>
            <DialogFooter>
              <Button disabled={createProject.isPending || updateProject.isPending || !form.formState.isValid} type="submit">
                <Plus size={16} />
                {editingProject ? t('save') : t('create')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function ProjectRow({ project, deletePending, onDelete, onEdit }: { project: Project, deletePending?: boolean, onDelete: () => void, onEdit: () => void }) {
  const { t } = useTranslation()
  return (
    <Card className="flex items-center justify-between gap-4">
      <div className="flex min-w-0 items-center gap-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <FolderKanban size={18} />
        </span>
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <Link className="truncate font-medium transition hover:text-primary" to={`/projects/${project.id}`}>
              {project.name}
            </Link>
            <StatusBadge>{project.namespaceStrategy}</StatusBadge>
          </div>
          <p className="truncate text-sm text-muted-foreground">
            {project.slug}
            {' '}
            ·
            {' '}
            {project.description || t('common.noDescription')}
          </p>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Link className="inline-flex h-9 items-center justify-center rounded-full border border-border bg-surface px-4 text-sm font-medium text-foreground transition hover:bg-muted" to={`/projects/${project.id}`}>
          {t('projectSpaces.openWorkspace')}
        </Link>
        <EditActionButton aria-label={t('projectSpaces.editAria')} label={t('edit')} onClick={onEdit} />
        <ConfirmDialog
          confirmText={t('projectSpaces.deleteConfirm')}
          description={t('projectSpaces.deleteDescription', { name: project.name })}
          pending={deletePending}
          title={t('projectSpaces.deleteTitle')}
          onConfirm={onDelete}
        >
          <Button aria-label={t('projectSpaces.deleteAria')} variant="ghost">
            <Trash2 size={16} />
          </Button>
        </ConfirmDialog>
      </div>
    </Card>
  )
}
