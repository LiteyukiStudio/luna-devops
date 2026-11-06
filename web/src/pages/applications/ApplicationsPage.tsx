import type { Ref } from 'react'
import type { Application } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { MoreHorizontal, Pencil, Plus, Rocket, Save, Trash2 } from 'lucide-react'
import { useImperativeHandle, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { ApplicationBasicFields } from '@/components/common/application-basic-fields'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'

import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { APPLICATION_IDENTIFIER_MAX_LENGTH, APPLICATION_IDENTIFIER_MIN_LENGTH } from '@/lib/identifier-limits'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { ApplicationRuntimeStatus, ApplicationSummary } from './overview/application-summary'

const schema = z.object({
  // 校验文案在校验时按当前语言求值，切换语言后新的错误提示随之生效。
  name: z.string().min(1, { error: () => i18next.t('apps.nameRequired') }),
  identifier: z.string()
    .min(APPLICATION_IDENTIFIER_MIN_LENGTH, { error: () => i18next.t('apps.identifierLength', { min: APPLICATION_IDENTIFIER_MIN_LENGTH, max: APPLICATION_IDENTIFIER_MAX_LENGTH }) })
    .max(APPLICATION_IDENTIFIER_MAX_LENGTH, { error: () => i18next.t('apps.identifierLength', { min: APPLICATION_IDENTIFIER_MIN_LENGTH, max: APPLICATION_IDENTIFIER_MAX_LENGTH }) })
    .regex(/^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/, { error: () => i18next.t('common.identifierFormat') }),
  icon: z.string().default('box'),
})

type ApplicationFormInput = z.input<typeof schema>
type ApplicationForm = z.output<typeof schema>
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

export interface ApplicationsPageHandle {
  openCreateDialog: () => void
}

interface ApplicationsPageProps {
  projectId?: string
  ref?: Ref<ApplicationsPageHandle>
}

/** 项目空间内的应用列表；页面级“新建应用”由项目工作区放入页头。 */
export function ApplicationsPage({ projectId: projectIdProp, ref }: ApplicationsPageProps = {}) {
  const { t } = useTranslation()
  const { projectId: routeProjectId = '' } = useParams()
  const projectId = projectIdProp ?? routeProjectId
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingApplication, setEditingApplication] = useState<Application | null>(null)
  const [applicationToDelete, setApplicationToDelete] = useState<Application | null>(null)
  const [deleteConfirmation, setDeleteConfirmation] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)

  const applications = useQuery({
    queryKey: ['applications', projectId, page, pageSize],
    queryFn: () => api.listApplicationsPage(projectId, { page, pageSize, sortBy: 'createdAt', sortOrder: 'desc', includeRuntime: true }),
    enabled: Boolean(projectId),
    ...liveObservationQueryPolicy,
  })
  const deleteConfirmationTarget = applicationToDelete?.name ?? ''
  const deleteConfirmationMatches = deleteConfirmation === deleteConfirmationTarget
  const deletionPreview = useQuery({
    queryKey: ['application-deletion-preview', projectId, applicationToDelete?.id],
    queryFn: () => api.previewApplicationDeletion(projectId, applicationToDelete!.id),
    enabled: Boolean(applicationToDelete),
  })
  const persistentVolumeCount = deletionPreview.data?.targets.reduce((total, target) => total + target.volumes.length, 0) ?? 0
  const form = useForm<ApplicationFormInput, undefined, ApplicationForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: {
      name: '',
      identifier: '',
      icon: 'box',
    },
  })

  const resetApplicationForm = (application?: Application) => {
    form.reset({
      name: application?.name ?? '',
      identifier: application?.identifier ?? '',
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
          identifier: payload.identifier,
          icon: payload.icon,
        }
        return api.createApplication(projectId, appPayload)
      })(),
    onSuccess: (application) => {
      toast.success(t('apps.created'))
      form.reset()
      setDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
      // 新应用直接进入概览，由上线引导带用户完成部署配置、构建和发布。
      navigate(`/projects/${projectId}/apps/${application.id}`)
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
          identifier: payload.identifier,
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
    <div className="grid gap-section">
      <DataList
        columns={[
          {
            key: 'name',
            header: t('apps.title'),
            width: 'primary',
            render: application => <ApplicationSummary application={application} projectId={projectId} />,
          },
          {
            key: 'runtimeStatus',
            header: t('apps.runtimeStatus'),
            width: 'normal',
            render: application => <ApplicationRuntimeStatus application={application} />,
          },
          {
            key: 'actions',
            header: t('common.actions'),
            mobileActions: 'inline',
            sticky: 'right',
            width: 'actions',
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
                <div className="flex items-center justify-end gap-1">
                  <Link
                    aria-disabled={deleting}
                    className={buttonVariants({ className: deleting ? 'pointer-events-none opacity-50' : undefined, size: 'sm', variant: 'outline' })}
                    tabIndex={deleting ? -1 : undefined}
                    to={`/projects/${projectId}/apps/${application.id}?tab=deployments`}
                  >
                    <Rocket size={16} />
                    {t('apps.deployAction')}
                  </Link>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button aria-label={t('apps.moreActionsAria', { name: application.name })} size="icon" variant="ghost">
                        <MoreHorizontal size={16} />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem disabled={deleting} onSelect={openEditDialog}>
                        <Pencil size={16} />
                        {t('common.edit')}
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
        emptyActions={(
          <Button type="button" variant="outline" onClick={openCreateDialog}>
            <Plus size={16} />
            {t('apps.createTitle')}
          </Button>
        )}
        emptyDescription={t('apps.emptyDescription')}
        emptyTitle={t('apps.emptyTitle')}
        error={applications.isError
          ? { description: t('apps.loadFailedDescription'), onRetry: () => void applications.refetch(), retrying: applications.isFetching, title: t('apps.loadFailedTitle') }
          : undefined}
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
        loading={applications.isLoading}
        rowActionLabel={application => t('apps.openDetailAria', { name: application.name })}
        rowKey={application => application.id}
        onRowClick={(application) => {
          if (application.deleteStatus !== 'deleting')
            navigate(`/projects/${projectId}/apps/${application.id}`)
        }}
      />

      <ConfirmDialog
        confirmDisabled={!deleteConfirmationMatches}
        confirmText={t('apps.deleteConfirm')}
        content={(
          <div className="grid gap-3">
            {persistentVolumeCount > 0 && (
              <div className="grid gap-2">
                <p className="text-sm text-warning-foreground">{t('apps.deleteDataDetected', { count: persistentVolumeCount })}</p>
                <p className="text-xs text-muted-foreground">{t('apps.deleteDataRetainHint')}</p>
              </div>
            )}
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
        <DialogContent className="sm:max-w-md">
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
              identifierError={form.formState.errors.identifier?.message}
              identifierField={form.register('identifier')}
              identifierMaxLength={APPLICATION_IDENTIFIER_MAX_LENGTH}
              identifierReadOnly={Boolean(editingApplication)}
              onIconChange={icon => form.setValue('icon', icon, { shouldDirty: true, shouldValidate: true })}
            />
            <DialogFooter>
              <DialogClose asChild>
                <Button disabled={createApplication.isPending || updateApplication.isPending} type="button" variant="outline">{t('common.cancel')}</Button>
              </DialogClose>
              <Button
                disabled={createApplication.isPending || updateApplication.isPending || !form.formState.isValid}
                type="submit"
              >
                {editingApplication ? <Save size={16} /> : <Plus size={16} />}
                {editingApplication ? t('common.save') : t('apps.createTitle')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
