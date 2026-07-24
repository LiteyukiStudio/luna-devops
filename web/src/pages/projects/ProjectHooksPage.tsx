import type { Ref } from 'react'
import type { ProjectHookConfig } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ScrollText, Trash2 } from 'lucide-react'
import { useImperativeHandle, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'

type HookForm = Omit<ProjectHookConfig, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'updatedAt'>

export interface ProjectHooksPageHandle {
  openCreateDialog: () => void
}

const hookDefaults: HookForm = {
  failurePolicy: 'fail',
  name: '',
  script: '',
  shell: 'sh',
  timeoutSeconds: 300,
}

export function ProjectHooksPage({ projectId, ref }: { projectId: string, ref?: Ref<ProjectHooksPageHandle> }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingHook, setEditingHook] = useState<ProjectHookConfig | null>(null)
  const [hookToDelete, setHookToDelete] = useState<ProjectHookConfig | null>(null)
  const form = useForm<HookForm>({ defaultValues: hookDefaults, mode: 'onChange' })
  const hooks = useQuery({ queryKey: ['project-hooks', projectId], queryFn: () => api.listProjectHooks(projectId), enabled: Boolean(projectId) })

  const saveHook = useMutation({
    mutationFn: (values: HookForm) => editingHook ? api.updateProjectHook(projectId, editingHook.id, values) : api.createProjectHook(projectId, values),
    onSuccess: () => {
      toast.success(t(editingHook ? 'projectHooks.updated' : 'projectHooks.created'))
      setDialogOpen(false)
      setEditingHook(null)
      form.reset(hookDefaults)
      queryClient.invalidateQueries({ queryKey: ['project-hooks', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteHook = useMutation({
    mutationFn: (hookId: string) => api.deleteProjectHook(projectId, hookId),
    onSuccess: () => {
      toast.success(t('projectHooks.deleted'))
      setHookToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['project-hooks', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  function openDialog(hook?: ProjectHookConfig) {
    setEditingHook(hook ?? null)
    if (hook) {
      form.reset({
        failurePolicy: hook.failurePolicy,
        name: hook.name,
        script: hook.script,
        shell: hook.shell,
        timeoutSeconds: hook.timeoutSeconds,
      })
    }
    else {
      form.reset(hookDefaults)
    }
    setDialogOpen(true)
  }

  useImperativeHandle(ref, () => ({
    openCreateDialog: () => {
      setEditingHook(null)
      form.reset(hookDefaults)
      setDialogOpen(true)
    },
  }), [form])

  if (hooks.isError) {
    return (
      <ErrorState
        description={t('projectHooks.loadFailedDescription')}
        title={t('projectHooks.loadFailedTitle')}
      />
    )
  }

  const hookItems = [...(hooks.data ?? [])].sort((left, right) => {
    return left.createdAt.localeCompare(right.createdAt)
  })

  return (
    <Card className="min-w-0 overflow-hidden p-0">
      <div className="border-b border-border px-4 py-4">
        <div className="min-w-0">
          <h2 className="text-base font-semibold">{t('projectHooks.title')}</h2>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">{t('projectHooks.description')}</p>
        </div>
      </div>
      {hookItems.length > 0
        ? (
            <DataList
              columns={[
                { key: 'name', header: t('common.name'), className: 'w-52 px-4 py-3 align-middle', render: item => <span className="block max-w-44 truncate" title={item.name}>{item.name}</span> },
                { key: 'shell', header: t('projectHooks.shell'), className: 'w-24 px-4 py-3 align-middle', render: item => t(`projectHooks.shells.${item.shell}`) },
                { key: 'failurePolicy', header: t('projectHooks.failurePolicy'), className: 'w-32 px-4 py-3 align-middle', render: item => t(`projectHooks.failurePolicies.${item.failurePolicy}`) },
                { key: 'timeout', header: t('projectHooks.timeoutSeconds'), className: 'w-24 px-4 py-3 align-middle', render: item => `${item.timeoutSeconds}s` },
                { key: 'actions', header: t('common.actions'), className: 'whitespace-nowrap text-right', render: item => (
                  <div className="flex justify-end gap-2">
                    <EditActionButton label={t('common.edit')} onClick={() => openDialog(item)} />
                    <Button size="sm" variant="ghost" onClick={() => setHookToDelete(item)}>
                      <Trash2 className="size-4" />
                      {t('common.delete')}
                    </Button>
                  </div>
                ) },
              ]}
              emptyTitle={t('projectHooks.emptyTitle')}
              items={hookItems}
              rowKey={item => item.id}
            />
          )
        : <EmptyState description={t('projectHooks.emptyDescription')} title={t('projectHooks.emptyTitle')} variant="plain" />}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{editingHook ? t('projectHooks.editTitle') : t('projectHooks.createTitle')}</DialogTitle>
            <DialogDescription>{t('projectHooks.dialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-4" onSubmit={form.handleSubmit(values => saveHook.mutate(values))}>
            <div className="grid gap-3 md:grid-cols-2">
              <Field label={t('common.name')} required><Input {...form.register('name', { required: true })} /></Field>
              <Field label={t('projectHooks.shell')}>
                <Select {...form.register('shell')}>
                  <option value="sh">{t('projectHooks.shells.sh')}</option>
                  <option value="bash">{t('projectHooks.shells.bash')}</option>
                </Select>
              </Field>
            </div>
            <div className="grid gap-3 md:grid-cols-2">
              <Field label={t('projectHooks.failurePolicy')}>
                <Select {...form.register('failurePolicy')}>
                  <option value="fail">{t('projectHooks.failurePolicies.fail')}</option>
                  <option value="ignore">{t('projectHooks.failurePolicies.ignore')}</option>
                </Select>
              </Field>
              <Field label={t('projectHooks.timeoutSeconds')}>
                <Input min={1} max={3600} type="number" {...form.register('timeoutSeconds', { valueAsNumber: true })} />
              </Field>
            </div>
            <Field hint={t('projectHooks.scriptHint')} label={t('projectHooks.script')} required>
              <textarea
                className="min-h-56 w-full resize-y rounded-md border border-input bg-background px-3 py-2 font-mono text-sm outline-none ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                {...form.register('script', { required: true })}
              />
            </Field>
            <DialogFooter>
              <Button disabled={!form.formState.isValid || saveHook.isPending} type="submit">
                <ScrollText className="size-4" />
                {t('common.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('common.delete')}
        description={t('projectHooks.deleteDescription')}
        open={Boolean(hookToDelete)}
        title={t('projectHooks.deleteTitle')}
        onConfirm={() => hookToDelete && deleteHook.mutate(hookToDelete.id)}
        onOpenChange={open => !open && setHookToDelete(null)}
      />
    </Card>
  )
}
