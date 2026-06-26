import type { Ref } from 'react'
import type { Application } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { LayoutDashboard, MoreHorizontal, Pencil, Plus, Save, Trash2 } from 'lucide-react'
import { useImperativeHandle, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { ApplicationBasicFields } from '@/components/common/application-basic-fields'
import { ApplicationIcon } from '@/components/common/application-icon-picker'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { ErrorState } from '@/components/common/error-state'
import { HoverText } from '@/components/common/hover-text'
import { PageHeader } from '@/components/common/page-header'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { APPLICATION_SLUG_MAX_LENGTH } from '@/lib/slug-limits'

const schema = z.object({
  name: z.string().min(1, i18next.t('apps.nameRequired')),
  slug: z.string().min(1, i18next.t('apps.slugRequired')).max(APPLICATION_SLUG_MAX_LENGTH, i18next.t('apps.slugMaxLength', { count: APPLICATION_SLUG_MAX_LENGTH })).regex(/^[a-z0-9-]+$/, i18next.t('common.lowercaseSlugOnly')),
  icon: z.string().default('box'),
})

type ApplicationFormInput = z.input<typeof schema>
type ApplicationForm = z.output<typeof schema>
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

export interface ApplicationsPageHandle {
  openCreateDialog: () => void
}

interface ApplicationsPageProps {
  embedded?: boolean
  projectId?: string
  projectName?: string
  ref?: Ref<ApplicationsPageHandle>
}

export function ApplicationsPage({ embedded = false, projectId: projectIdProp, ref }: ApplicationsPageProps = {}) {
  const { t } = useTranslation()
  const { projectId: routeProjectId = '' } = useParams()
  const projectId = projectIdProp ?? routeProjectId
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingApplication, setEditingApplication] = useState<Application | null>(null)
  const [applicationToDelete, setApplicationToDelete] = useState<Application | null>(null)
  const [deleteConfirmation, setDeleteConfirmation] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)

  const applications = useQuery({
    queryKey: ['applications', projectId, page, pageSize],
    queryFn: () => api.listApplicationsPage(projectId, { page, pageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
    enabled: Boolean(projectId),
  })
  const deleteConfirmationTarget = applicationToDelete?.name ?? ''
  const deleteConfirmationMatches = deleteConfirmation === deleteConfirmationTarget
  const form = useForm<ApplicationFormInput, undefined, ApplicationForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: {
      name: '',
      slug: '',
      icon: 'box',
    },
  })

  const resetApplicationForm = (application?: Application) => {
    form.reset({
      name: application?.name ?? '',
      slug: application?.slug ?? '',
      icon: application?.icon ?? 'box',
    })
  }

  const openCreateDialog = () => {
    setEditingApplication(null)
    resetApplicationForm()
    setDialogOpen(true)
  }

  useImperativeHandle(ref, () => ({ openCreateDialog }))

  const createApplication = useMutation({
    mutationFn: (payload: ApplicationForm) =>
      (async () => {
        const appPayload = {
          name: payload.name,
          slug: payload.slug,
          icon: payload.icon,
        }
        return api.createApplication(projectId, appPayload)
      })(),
    onSuccess: () => {
      toast.success(t('apps.created'))
      form.reset()
      setDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const updateApplication = useMutation({
    mutationFn: (payload: ApplicationForm) =>
      (async () => {
        if (!editingApplication)
          throw new Error(t('apps.editMissing'))

        const appPayload = {
          name: payload.name,
          slug: payload.slug,
          icon: payload.icon,
        }
        return api.updateApplication(projectId, editingApplication.id, appPayload)
      })(),
    onSuccess: () => {
      toast.success(t('apps.updated'))
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
      setEditingApplication(null)
      setDialogOpen(false)
      resetApplicationForm()
    },
    onError: error => toast.error(error.message),
  })

  const deleteApplication = useMutation({
    mutationFn: (applicationId: string) => api.deleteApplication(projectId, applicationId),
    onSuccess: () => {
      toast.success(t('apps.deleteQueued'))
      setApplicationToDelete(null)
      setDeleteConfirmation('')
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
      queryClient.invalidateQueries({ queryKey: ['repository-bindings', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  return (
    <div className="grid gap-6">
      {!embedded && (
        <PageHeader
          actions={(
            <div className="flex items-center gap-3">
              <Button onClick={openCreateDialog}>
                <Plus size={16} />
                {t('apps.createTitle')}
              </Button>
              <Link className="text-sm text-primary hover:underline" to="/projects">{t('backToProjectSpaces')}</Link>
            </div>
          )}
          description={t('apps.description')}
          title={t('apps.title')}
        />
      )}

      {applications.isError && <ErrorState title={t('apps.loadFailedTitle')} description={t('apps.loadFailedDescription')} />}
      <DataList
        columns={[
          {
            key: 'name',
            header: t('apps.title'),
            className: 'min-w-64 px-4 py-3 align-middle',
            render: application => <ApplicationSummary application={application} projectId={projectId} />,
          },
          {
            key: 'actions',
            header: t('common.actions'),
            className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle text-right',
            render: (application) => {
              const deleting = application.deleteStatus === 'deleting'
              const openEditDialog = () => {
                setEditingApplication(application)
                resetApplicationForm(application)
                setDialogOpen(true)
              }
              const openDeleteDialog = () => {
                setApplicationToDelete(application)
                setDeleteConfirmation('')
              }
              return (
                <div className="flex justify-end">
                  <div className="hidden justify-end gap-2 sm:flex">
                    <Link
                      aria-label={t('apps.openDetailAria')}
                      aria-disabled={deleting}
                      className={buttonVariants({ className: deleting ? 'pointer-events-none opacity-50' : undefined, size: 'sm', variant: 'ghost' })}
                      to={`/projects/${projectId}/apps/${application.id}`}
                    >
                      <LayoutDashboard size={16} />
                      {t('apps.openDetail')}
                    </Link>
                    <EditActionButton
                      aria-label={t('apps.editAria')}
                      label={t('edit')}
                      disabled={deleting}
                      onClick={openEditDialog}
                    />
                    <Button
                      aria-label={t('apps.deleteAria')}
                      disabled={deleting}
                      variant="ghost"
                      onClick={openDeleteDialog}
                    >
                      <Trash2 size={16} />
                    </Button>
                  </div>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button aria-label={t('common.actions')} className="sm:hidden" size="icon" variant="ghost">
                        <MoreHorizontal size={16} />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem asChild disabled={deleting}>
                        <Link className={deleting ? 'pointer-events-none opacity-50' : undefined} to={`/projects/${projectId}/apps/${application.id}`}>
                          <LayoutDashboard size={16} />
                          {t('apps.openDetail')}
                        </Link>
                      </DropdownMenuItem>
                      <DropdownMenuItem disabled={deleting} onSelect={openEditDialog}>
                        <Pencil size={16} />
                        {t('edit')}
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem disabled={deleting} variant="destructive" onSelect={openDeleteDialog}>
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
        emptyDescription={t('apps.emptyDescription')}
        emptyTitle={t('apps.emptyTitle')}
        items={applications.data?.items ?? []}
        pagination={{
          page: applications.data?.page ?? page,
          pageSize: applications.data?.pageSize ?? pageSize,
          pageSizeOptions: PAGE_SIZE_OPTIONS,
          total: applications.data?.total ?? 0,
          totalPages: applications.data?.totalPages ?? 0,
          pageInfoLabel: t('pagination.pageInfo', {
            page: applications.data?.page ?? page,
            totalPages: applications.data?.totalPages ?? 0,
            total: applications.data?.total ?? 0,
          }),
          onPageChange: setPage,
          onPageSizeChange: (nextPageSize) => {
            setPageSize(nextPageSize)
            setPage(1)
          },
        }}
        rowKey={application => application.id}
      />

      <ConfirmDialog
        confirmDisabled={!deleteConfirmationMatches}
        confirmText={t('apps.deleteConfirm')}
        content={(
          <div className="grid gap-2">
            <Label htmlFor="application-delete-confirmation">{t('apps.deleteConfirmationLabel', { name: deleteConfirmationTarget })}</Label>
            <Input
              id="application-delete-confirmation"
              aria-invalid={Boolean(deleteConfirmation) && !deleteConfirmationMatches}
              autoComplete="off"
              placeholder={deleteConfirmationTarget}
              value={deleteConfirmation}
              onChange={event => setDeleteConfirmation(event.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t('apps.deleteConfirmationHint')}</p>
          </div>
        )}
        description={applicationToDelete ? t('apps.deleteDescription', { name: applicationToDelete.name }) : ''}
        open={Boolean(applicationToDelete)}
        pending={deleteApplication.isPending || applicationToDelete?.deleteStatus === 'deleting'}
        title={t('apps.deleteTitle')}
        onConfirm={() => {
          if (applicationToDelete && deleteConfirmationMatches)
            deleteApplication.mutate(applicationToDelete.id)
        }}
        onOpenChange={(open) => {
          if (!open) {
            setApplicationToDelete(null)
            setDeleteConfirmation('')
          }
        }}
      />

      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open) {
            setEditingApplication(null)
            resetApplicationForm()
          }
        }}
      >
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{editingApplication ? t('apps.editTitle') : t('apps.createTitle')}</DialogTitle>
            <DialogDescription>{t('apps.description')}</DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-3"
            onSubmit={form.handleSubmit((values) => {
              if (editingApplication)
                updateApplication.mutate(values)
              else
                createApplication.mutate(values)
            })}
          >
            <ApplicationBasicFields
              compact
              icon={form.watch('icon')}
              nameError={form.formState.errors.name?.message}
              nameField={form.register('name')}
              slugError={form.formState.errors.slug?.message}
              slugField={form.register('slug')}
              slugMaxLength={APPLICATION_SLUG_MAX_LENGTH}
              onIconChange={icon => form.setValue('icon', icon, { shouldDirty: true, shouldValidate: true })}
            />
            <DialogFooter>
              <Button
                disabled={createApplication.isPending || updateApplication.isPending || !form.formState.isValid}
                type="submit"
              >
                {editingApplication ? <Save size={16} /> : <Plus size={16} />}
                {editingApplication ? t('save') : t('apps.createTitle')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function ApplicationSummary({ application, projectId }: { application: Application, projectId: string }) {
  const deleting = application.deleteStatus === 'deleting'
  const deleteFailedMessage = application.deleteStatus === 'delete_failed' ? application.deleteMessage?.trim() : ''
  return (
    <div className="flex min-w-0 items-center gap-3">
      <span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <ApplicationIcon name={application.icon} />
      </span>
      <div className="min-w-0 w-full">
        <div className="flex items-center gap-2">
          <Link className={`min-w-0 truncate font-medium transition hover:text-primary ${deleting ? 'pointer-events-none opacity-60' : ''}`} to={`/projects/${projectId}/apps/${application.id}`}>
            {application.name}
          </Link>
          {application.deleteStatus && application.deleteStatus !== 'active' && (
            <StatusValueBadge labelKeyPrefix="apps.deleteStatuses" value={application.deleteStatus} />
          )}
          {deleteFailedMessage && (
            <HoverText className="flex-1 text-xs text-muted-foreground" value={deleteFailedMessage} />
          )}
        </div>
        <p className="truncate text-sm text-muted-foreground">
          {application.slug}
        </p>
      </div>
    </div>
  )
}
