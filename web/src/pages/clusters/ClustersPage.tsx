import type { RuntimeCluster } from '@/api/client'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api/client'
import { useSession } from '@/app/session-context'
import { CodeEditor } from '@/components/common/code-editor'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { FormField as Field } from '@/components/common/form-field'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'

type ClusterForm = Omit<RuntimeCluster, 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & { kubeconfig?: string }

const clusterDefaults: ClusterForm = {
  endpoint: '',
  isDefault: false,
  kubeconfig: '',
  name: '',
  ownerRef: '',
  scope: 'global',
  status: 'unknown',
  type: 'kubernetes',
}

export function ClustersPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingCluster, setEditingCluster] = useState<RuntimeCluster | null>(null)
  const [clusterToDelete, setClusterToDelete] = useState<RuntimeCluster | null>(null)
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects })
  const clusters = useQuery({ queryKey: ['runtime-clusters'], queryFn: () => api.listRuntimeClusters() })
  const projectMap = useMemo(() => Object.fromEntries((projects.data ?? []).map(project => [project.id, project])), [projects.data])
  const form = useForm<ClusterForm>({ defaultValues: clusterDefaults, mode: 'onChange' })
  const scope = form.watch('scope')
  const canEditKubeconfig = !editingCluster || canInspectClusterKubeconfig(editingCluster, user?.id, user?.role)

  useEffect(() => {
    if (scope !== 'global')
      form.setValue('isDefault', false, { shouldDirty: true, shouldValidate: true })
    if (scope === 'user')
      form.setValue('ownerRef', '', { shouldDirty: true, shouldValidate: true })
  }, [form, scope])

  const saveCluster = useMutation({
    mutationFn: (values: ClusterForm) => editingCluster ? api.updateRuntimeCluster(editingCluster.id, values) : api.createRuntimeCluster(values),
    onSuccess: () => {
      toast.success(t(editingCluster ? 'deploymentsPage.clusterUpdated' : 'deploymentsPage.clusterCreated'))
      setDialogOpen(false)
      setEditingCluster(null)
      form.reset(clusterDefaults)
      queryClient.invalidateQueries({ queryKey: ['runtime-clusters'] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteCluster = useMutation({
    mutationFn: api.deleteRuntimeCluster,
    onSuccess: () => {
      toast.success(t('deploymentsPage.clusterDeleted'))
      setClusterToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['runtime-clusters'] })
    },
    onError: error => toast.error(error.message),
  })
  const testCluster = useMutation({
    mutationFn: api.testRuntimeCluster,
    onSuccess: () => {
      toast.success(t('deploymentsPage.clusterTested'))
    },
    onError: error => toast.error(error.message),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['runtime-clusters'] }),
  })

  function openDialog(cluster?: RuntimeCluster) {
    setEditingCluster(cluster ?? null)
    form.reset(cluster
      ? {
          endpoint: cluster.endpoint,
          isDefault: cluster.isDefault,
          kubeconfig: cluster.kubeconfig ?? '',
          name: cluster.name,
          ownerRef: cluster.ownerRef,
          scope: cluster.scope,
          status: cluster.status,
          type: cluster.type,
        }
      : clusterDefaults)
    setDialogOpen(true)
  }

  return (
    <div className="grid gap-4">
      <div className="flex justify-end">
        <Button onClick={() => openDialog()}>
          <Plus className="size-4" />
          {t('deploymentsPage.createCluster')}
        </Button>
      </div>

      <DataList
        columns={[
          { key: 'name', header: t('common.name'), render: item => item.name },
          { key: 'type', header: t('common.type'), render: item => clusterTypeLabel(item.type, t) },
          { key: 'scope', header: t('common.scope'), render: item => scopeLabel(item, projectMap, t) },
          { key: 'default', header: t('clustersPage.defaultCluster'), render: item => item.isDefault ? t('common.yes') : t('common.no') },
          { key: 'status', header: t('common.status'), render: item => <StatusValueBadge value={item.status} /> },
          { key: 'actions', header: t('common.actions'), className: 'text-right whitespace-nowrap', render: item => (
            canManageCluster(item, user?.id, user?.role)
              ? (
                  <div className="flex justify-end gap-2">
                    <Button size="sm" variant="ghost" onClick={() => testCluster.mutate(item.id)}>{t('common.test')}</Button>
                    <EditActionButton label={t('common.edit')} onClick={() => openDialog(item)} />
                    <Button size="sm" variant="ghost" onClick={() => setClusterToDelete(item)}>
                      <Trash2 className="size-4" />
                      {t('common.delete')}
                    </Button>
                  </div>
                )
              : <span className="text-xs text-muted-foreground">{t('common.viewOnly')}</span>
          ) },
        ]}
        emptyTitle={t('deploymentsPage.emptyClusters')}
        items={clusters.data ?? []}
        rowKey={item => item.id}
      />

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="w-[min(92vw,48rem)] max-w-[92vw]">
          <DialogHeader>
            <DialogTitle>{editingCluster ? t('deploymentsPage.editCluster') : t('deploymentsPage.createCluster')}</DialogTitle>
            <DialogDescription>{t('clustersPage.dialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => saveCluster.mutate(values))}>
            <Field label={t('common.name')} required><Input {...form.register('name', { required: true })} /></Field>
            <Field label={t('common.scope')}>
              <Select {...form.register('scope')}>
                <option value="global">{t('codeRepositoriesView.scopeGlobal')}</option>
                <option value="project">{t('codeRepositoriesView.scopeProject')}</option>
                <option value="user">{t('codeRepositoriesView.scopeUser')}</option>
              </Select>
            </Field>
            {scope === 'project' && (
              <Field label={t('projectSpaces.title')} required>
                <Select {...form.register('ownerRef', { required: true })}>
                  <option value="">{t('common.select')}</option>
                  {(projects.data ?? []).map(project => <option key={project.id} value={project.id}>{project.name}</option>)}
                </Select>
              </Field>
            )}
            <Field label={t('common.type')}>
              <Select {...form.register('type')}>
                <option value="kubernetes">{t('deploymentsPage.typeKubernetes')}</option>
              </Select>
            </Field>
            <Field hint={canEditKubeconfig ? t('clustersPage.kubeconfigHint') : t('clustersPage.kubeconfigRestrictedHint')} label={t('deploymentsPage.kubeconfig')} required={!editingCluster}>
              <Controller
                control={form.control}
                name="kubeconfig"
                rules={{ required: !editingCluster }}
                render={({ field }) => (
                  <CodeEditor
                    ariaInvalid={Boolean(form.formState.errors.kubeconfig)}
                    className="w-full"
                    language="yaml"
                    minHeight="16rem"
                    placeholder={t('clustersPage.kubeconfigPlaceholder')}
                    readOnly={!canEditKubeconfig}
                    value={field.value ?? ''}
                    onChange={field.onChange}
                  />
                )}
              />
            </Field>
            {scope === 'global' && (
              <label className="flex items-center gap-2 text-sm text-foreground">
                <input className="size-4 accent-primary" type="checkbox" {...form.register('isDefault')} />
                <span>{t('clustersPage.defaultCluster')}</span>
              </label>
            )}
            <DialogFooter><Button disabled={!form.formState.isValid || saveCluster.isPending} type="submit">{t('common.save')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog cancelText={t('common.cancel')} confirmText={t('common.delete')} description={t('deploymentsPage.deleteClusterDescription')} open={Boolean(clusterToDelete)} title={t('deploymentsPage.deleteClusterTitle')} onConfirm={() => clusterToDelete && deleteCluster.mutate(clusterToDelete.id)} onOpenChange={open => !open && setClusterToDelete(null)} />
    </div>
  )
}

function canManageCluster(cluster: RuntimeCluster, userID?: string, role?: string) {
  if (role === 'platform_admin')
    return true
  if (cluster.scope === 'user')
    return cluster.ownerRef === userID
  if (cluster.scope === 'project')
    return true
  return false
}

function canInspectClusterKubeconfig(cluster: RuntimeCluster, userID?: string, role?: string) {
  return role === 'platform_admin' || cluster.createdBy === userID
}

function clusterTypeLabel(type: RuntimeCluster['type'], t: (key: string, options?: Record<string, unknown>) => string) {
  if (type === 'k3s')
    return t('deploymentsPage.typeKubernetes')
  return t(`deploymentsPage.typeLabels.${type}`, { defaultValue: type })
}

function scopeLabel(cluster: RuntimeCluster, projectMap: Record<string, { name: string }>, t: (key: string, options?: Record<string, unknown>) => string) {
  if (cluster.scope === 'project')
    return projectMap[cluster.ownerRef]?.name ?? cluster.ownerRef
  if (cluster.scope === 'user')
    return t('codeRepositoriesView.scopeUser')
  return t('codeRepositoriesView.scopeGlobal')
}
