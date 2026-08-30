import type { Ref } from 'react'
import type { ProjectRuntimeConfigSet, ProjectRuntimeConfigSetPayload } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Trash2 } from 'lucide-react'
import { useImperativeHandle, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { ErrorState } from '@/components/common/error-state'
import { ResourceDeletionStatus } from '@/components/common/resource-deletion-status'
import { RuntimeConfigSetDialog } from '@/components/common/runtime-config-set-dialog'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { runtimeConfigFileCount } from '@/lib/runtime-config-files'
import { publicRuntimeEnvironmentInputs, publicRuntimeEnvironmentRecord } from '@/lib/runtime-environment'

export interface ProjectRuntimeConfigSetsPageHandle {
  openCreateDialog: () => void
}

const runtimeConfigDefaults: ProjectRuntimeConfigSetPayload = {
  configFiles: '',
  enabled: true,
  environmentVariables: [],
  name: '',
  secretFiles: '',
}
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

function RuntimeConfigSetSummary({ item }: { item: ProjectRuntimeConfigSet }) {
  return (
    <div className="min-w-0">
      <span className="block truncate whitespace-nowrap" title={item.name}>{item.name}</span>
      <ResourceDeletionStatus className="mt-1" message={item.deleteMessage} status={item.deleteStatus} />
    </div>
  )
}

export function ProjectRuntimeConfigSetsPage({ projectId, ref }: { projectId: string, ref?: Ref<ProjectRuntimeConfigSetsPageHandle> }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingSet, setEditingSet] = useState<ProjectRuntimeConfigSet | null>(null)
  const [configFilesValid, setConfigFilesValid] = useState(true)
  const [secretFilesValid, setSecretFilesValid] = useState(true)
  const [setToDelete, setSetToDelete] = useState<ProjectRuntimeConfigSet | null>(null)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const form = useForm<ProjectRuntimeConfigSetPayload>({ defaultValues: runtimeConfigDefaults, mode: 'onChange' })
  const configSets = useQuery({
    queryKey: ['runtime-config-sets', projectId, page, pageSize],
    queryFn: () => api.listProjectRuntimeConfigSetsPage(projectId, { page, pageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
    enabled: Boolean(projectId),
  })

  const saveConfigSet = useMutation({
    mutationFn: (values: ProjectRuntimeConfigSetPayload) => editingSet
      ? api.updateProjectRuntimeConfigSet(projectId, editingSet.id, normalizeRuntimeConfigPayload(values))
      : api.createProjectRuntimeConfigSet(projectId, normalizeRuntimeConfigPayload(values)),
    onSuccess: (set) => {
      toast.success(t(editingSet ? 'runtimeConfigSets.updated' : 'runtimeConfigSets.created'))
      if (editingSet && (set.affectedDeploymentTargetCount ?? 0) > 0)
        toast.warning(t('runtimeConfigSets.updatedNeedsRestart', { count: set.affectedDeploymentTargetCount }))
      setDialogOpen(false)
      setEditingSet(null)
      form.reset(runtimeConfigDefaults)
      queryClient.invalidateQueries({ queryKey: ['runtime-config-sets', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteConfigSet = useMutation({
    mutationFn: (set: ProjectRuntimeConfigSet) => api.deleteProjectRuntimeConfigSet(projectId, set.id),
    onSuccess: () => {
      toast.success(t('runtimeConfigSets.deleted'))
      setSetToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['runtime-config-sets', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  function openDialog(set?: ProjectRuntimeConfigSet) {
    setEditingSet(set ?? null)
    setConfigFilesValid(true)
    setSecretFilesValid(true)
    form.reset(set
      ? {
          configFiles: set.configFiles,
          enabled: set.enabled,
          environmentVariables: publicRuntimeEnvironmentInputs(publicRuntimeEnvironmentRecord(set.environmentVariables)),
          name: set.name,
          secretFiles: '',
        }
      : runtimeConfigDefaults)
    setDialogOpen(true)
  }

  useImperativeHandle(ref, () => ({
    openCreateDialog: () => {
      setEditingSet(null)
      setConfigFilesValid(true)
      setSecretFilesValid(true)
      form.reset(runtimeConfigDefaults)
      setDialogOpen(true)
    },
  }), [form])

  if (configSets.isError) {
    return (
      <ErrorState
        description={t('runtimeConfigSets.loadFailedDescription')}
        title={t('runtimeConfigSets.loadFailedTitle')}
      />
    )
  }

  return (
    <Card className="min-w-0 overflow-hidden p-0">
      <div className="border-b border-border px-4 py-4">
        <div className="min-w-0">
          <h2 className="text-base font-semibold">{t('runtimeConfigSets.title')}</h2>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">{t('runtimeConfigSets.description')}</p>
        </div>
      </div>
      <DataList
        columns={[
          { key: 'name', header: t('common.name'), className: 'min-w-48 px-4 py-3 align-middle', render: item => <RuntimeConfigSetSummary item={item} /> },
          { key: 'configFiles', header: t('runtimeConfigSets.configFiles'), className: 'w-32 whitespace-nowrap px-4 py-3 align-middle', render: item => t('runtimeConfigSets.configFileState', { count: runtimeConfigFileCount(item.configFiles) }) },
          { key: 'secretFiles', header: t('runtimeConfigSets.secretFiles'), className: 'w-32 whitespace-nowrap px-4 py-3 align-middle', render: item => item.secretFilesSet ? t('runtimeConfigSets.configured') : t('runtimeConfigSets.notConfigured') },
          { key: 'enabled', header: t('common.status'), className: 'w-28 whitespace-nowrap px-4 py-3 align-middle', render: item => <StatusValueBadge value={item.enabled ? 'enabled' : 'disabled'} /> },
          { key: 'actions', header: t('common.actions'), className: 'w-[1%] whitespace-nowrap px-4 py-3 text-right align-middle', render: (item) => {
            const deleting = item.deleteStatus === 'deleting'
            return (
              <div className="flex justify-end gap-2">
                <EditActionButton disabled={deleting} label={t('common.edit')} onClick={() => openDialog(item)} />
                <Button disabled={deleting} size="sm" variant="ghost" onClick={() => setSetToDelete(item)}>
                  <Trash2 className="size-4" />
                  {t('common.delete')}
                </Button>
              </div>
            )
          } },
        ]}
        emptyTitle={t('runtimeConfigSets.emptyTitle')}
        items={configSets.data?.items ?? []}
        pagination={{
          page: configSets.data?.page ?? page,
          pageSize: configSets.data?.pageSize ?? pageSize,
          pageSizeOptions: PAGE_SIZE_OPTIONS,
          total: configSets.data?.total ?? 0,
          totalPages: configSets.data?.totalPages ?? 0,
          pageInfoLabel: t('pagination.pageInfo', {
            page: configSets.data?.page ?? page,
            totalPages: configSets.data?.totalPages ?? 0,
            total: configSets.data?.total ?? 0,
          }),
          onPageChange: setPage,
          onPageSizeChange: (nextPageSize) => {
            setPageSize(nextPageSize)
            setPage(1)
          },
        }}
        rowKey={item => item.id}
      />
      <RuntimeConfigSetDialog
        canManageSecrets
        configFilesValid={configFilesValid}
        editingSet={editingSet}
        form={form}
        open={dialogOpen}
        pending={saveConfigSet.isPending}
        projectId={projectId}
        secretFilesValid={secretFilesValid}
        onConfigFilesValidityChange={setConfigFilesValid}
        onOpenChange={setDialogOpen}
        onSecretFilesValidityChange={setSecretFilesValid}
        onSubmit={values => saveConfigSet.mutate(values)}
      />
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('common.delete')}
        description={t('runtimeConfigSets.deleteDescription')}
        open={Boolean(setToDelete)}
        title={t('runtimeConfigSets.deleteTitle')}
        onConfirm={() => setToDelete && deleteConfigSet.mutate(setToDelete)}
        onOpenChange={open => !open && setSetToDelete(null)}
      />
    </Card>
  )
}

function normalizeRuntimeConfigPayload(values: ProjectRuntimeConfigSetPayload): ProjectRuntimeConfigSetPayload {
  return {
    configFiles: values.configFiles?.trim() ?? '',
    enabled: Boolean(values.enabled),
    environmentVariables: publicRuntimeEnvironmentInputs(publicRuntimeEnvironmentRecord(values.environmentVariables)),
    name: values.name.trim(),
    secretFiles: values.secretFiles?.trim() ?? '',
  }
}
