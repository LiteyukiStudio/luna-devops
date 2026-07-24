import type { Ref } from 'react'
import type { BuildVariableSet } from '@/api'
import type { KeyValueRow } from '@/components/common/key-value-rows-editor'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Trash2 } from 'lucide-react'
import { useImperativeHandle, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { CheckboxField } from '@/components/common/checkbox-field'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { KeyValueRowsEditor } from '@/components/common/key-value-rows-editor'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { buildVariableCount, buildVariableRecordToRows, buildVariableRowsToRecord, emptyKeyValueRow, secretStateToRows } from '@/lib/build-variables'

interface VariableSetForm {
  name: string
  variables: KeyValueRow[]
  secrets: KeyValueRow[]
  enabled: boolean
}

export interface ProjectBuildVariableSetsPageHandle {
  openCreateDialog: () => void
}

const variableSetDefaults: VariableSetForm = { enabled: true, name: '', secrets: [emptyKeyValueRow()], variables: [emptyKeyValueRow()] }
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

export function ProjectBuildVariableSetsPage({ projectId, ref }: { projectId: string, ref?: Ref<ProjectBuildVariableSetsPageHandle> }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingSet, setEditingSet] = useState<BuildVariableSet | null>(null)
  const [setToDelete, setSetToDelete] = useState<BuildVariableSet | null>(null)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const form = useForm<VariableSetForm>({ defaultValues: variableSetDefaults, mode: 'onChange' })
  const variableSets = useQuery({
    queryKey: ['build-variable-sets', projectId, page, pageSize],
    queryFn: () => api.listBuildVariableSetsPage({ projectId, page, pageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
    enabled: Boolean(projectId),
  })

  const saveVariableSet = useMutation({
    mutationFn: (values: VariableSetForm) => {
      const payload = {
        enabled: values.enabled,
        name: values.name,
        ownerRef: '',
        projectIds: [projectId],
        scope: 'project' as const,
        secrets: buildVariableRowsToRecord(values.secrets),
        variables: buildVariableRowsToRecord(values.variables),
      }
      return editingSet ? api.updateBuildVariableSet(editingSet.id, payload) : api.createBuildVariableSet(payload)
    },
    onSuccess: () => {
      toast.success(t(editingSet ? 'buildsPage.variableSetUpdated' : 'buildsPage.variableSetCreated'))
      setDialogOpen(false)
      setEditingSet(null)
      form.reset(variableSetDefaults)
      queryClient.invalidateQueries({ queryKey: ['build-variable-sets', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteVariableSet = useMutation({
    mutationFn: api.deleteBuildVariableSet,
    onSuccess: () => {
      toast.success(t('buildsPage.variableSetDeleted'))
      setSetToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['build-variable-sets', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  function openDialog(set?: BuildVariableSet) {
    setEditingSet(set ?? null)
    form.reset(set
      ? { enabled: set.enabled, name: set.name, secrets: secretStateToRows(set.secrets), variables: buildVariableRecordToRows(set.variables) }
      : variableSetDefaults)
    setDialogOpen(true)
  }

  useImperativeHandle(ref, () => ({
    openCreateDialog: () => {
      setEditingSet(null)
      form.reset(variableSetDefaults)
      setDialogOpen(true)
    },
  }), [form])

  if (variableSets.isError) {
    return (
      <ErrorState
        description={t('buildsPage.variableSetLoadFailedDescription')}
        title={t('buildsPage.variableSetLoadFailedTitle')}
      />
    )
  }

  return (
    <Card className="min-w-0 overflow-hidden p-0">
      <div className="border-b border-border px-4 py-4">
        <div className="min-w-0">
          <h2 className="text-base font-semibold">{t('buildsPage.variablesAndSecrets')}</h2>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">{t('buildsPage.projectVariableSetDescription')}</p>
        </div>
      </div>
      <DataList
        columns={[
          { key: 'name', header: t('common.name'), className: 'min-w-40 px-4 py-3 align-middle', render: item => <span className="block truncate whitespace-nowrap" title={item.name}>{item.name}</span> },
          { key: 'variables', header: t('buildsPage.variables'), className: 'w-32 whitespace-nowrap px-4 py-3 align-middle', render: item => t('buildsPage.variableCount', { count: item.variableCount ?? buildVariableCount(item.variables) }) },
          { key: 'secrets', header: t('buildsPage.secrets'), className: 'w-32 whitespace-nowrap px-4 py-3 align-middle', render: item => t('buildsPage.secretCount', { count: Object.keys(item.secrets ?? {}).length }) },
          { key: 'enabled', header: t('common.status'), className: 'w-28 whitespace-nowrap px-4 py-3 align-middle', render: item => <StatusValueBadge value={item.enabled ? 'enabled' : 'disabled'} /> },
          { key: 'actions', header: t('common.actions'), className: 'w-[1%] whitespace-nowrap px-4 py-3 text-right align-middle', render: item => (
            <div className="flex justify-end gap-2">
              {item.canInspectVariables
                ? (
                    <>
                      <EditActionButton label={t('common.edit')} onClick={() => openDialog(item)} />
                      <Button size="sm" variant="ghost" onClick={() => setSetToDelete(item)}>
                        <Trash2 className="size-4" />
                        {t('common.delete')}
                      </Button>
                    </>
                  )
                : <span className="text-sm text-muted-foreground">{t('buildsPage.variableSetReadOnly')}</span>}
            </div>
          ) },
        ]}
        emptyTitle={t('buildsPage.emptyVariableSets')}
        items={variableSets.data?.items ?? []}
        pagination={{
          page: variableSets.data?.page ?? page,
          pageSize: variableSets.data?.pageSize ?? pageSize,
          pageSizeOptions: PAGE_SIZE_OPTIONS,
          total: variableSets.data?.total ?? 0,
          totalPages: variableSets.data?.totalPages ?? 0,
          pageInfoLabel: t('pagination.pageInfo', {
            page: variableSets.data?.page ?? page,
            totalPages: variableSets.data?.totalPages ?? 0,
            total: variableSets.data?.total ?? 0,
          }),
          onPageChange: setPage,
          onPageSizeChange: (nextPageSize) => {
            setPageSize(nextPageSize)
            setPage(1)
          },
        }}
        rowKey={item => item.id}
      />
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{editingSet ? t('buildsPage.editVariableSet') : t('buildsPage.createVariableSet')}</DialogTitle>
            <DialogDescription>{t('buildsPage.variableSetDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-4" onSubmit={form.handleSubmit(values => saveVariableSet.mutate(values))}>
            <Field label={t('common.name')} required><Input {...form.register('name', { required: true })} /></Field>
            <KeyValueRowsEditor
              rows={form.watch('variables')}
              title={t('buildsPage.variables')}
              valuePlaceholder={t('buildsPage.variableValuePlaceholder')}
              onChange={rows => form.setValue('variables', rows, { shouldDirty: true, shouldValidate: true })}
            />
            <KeyValueRowsEditor
              secret
              rows={form.watch('secrets')}
              title={t('buildsPage.secrets')}
              valuePlaceholder={editingSet ? t('buildsPage.secretKeepPlaceholder') : t('buildsPage.secretValuePlaceholder')}
              onChange={rows => form.setValue('secrets', rows, { shouldDirty: true, shouldValidate: true })}
            />
            <CheckboxField {...form.register('enabled')}>
              {t('common.enabled')}
            </CheckboxField>
            <DialogFooter><Button disabled={!form.formState.isValid || saveVariableSet.isPending} type="submit">{t('common.save')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('common.delete')}
        description={t('buildsPage.deleteVariableSetDescription')}
        open={Boolean(setToDelete)}
        title={t('buildsPage.deleteVariableSetTitle')}
        onConfirm={() => setToDelete && deleteVariableSet.mutate(setToDelete.id)}
        onOpenChange={open => !open && setSetToDelete(null)}
      />
    </Card>
  )
}
