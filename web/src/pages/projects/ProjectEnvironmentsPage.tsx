import type { Environment, RuntimeCluster } from '@/api/client'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Trash2 } from 'lucide-react'
import { useImperativeHandle, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api/client'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { FormField as Field } from '@/components/common/form-field'
import { UnitInput } from '@/components/common/unit-input'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { useBillingDisplay } from '@/lib/billing-display'
import { ENVIRONMENT_SLUG_MAX_LENGTH } from '@/lib/slug-limits'

export interface ProjectEnvironmentsPageHandle {
  openCreateDialog: () => void
}

type EnvironmentForm = Omit<Environment, 'id' | 'projectId' | 'createdBy' | 'createdAt'>

const environmentDefaults: EnvironmentForm = {
  clusterId: '',
  configRefs: '',
  cpuRequest: '100m',
  envVars: '{}',
  memoryRequest: '128Mi',
  name: '',
  namespace: '',
  replicas: 1,
  secretRefs: '',
  slug: '',
  stage: 'dev',
}

export function ProjectEnvironmentsPage({ projectId, ref }: { projectId: string, ref?: React.Ref<ProjectEnvironmentsPageHandle> }) {
  const { i18n, t } = useTranslation()
  const queryClient = useQueryClient()
  const billingDisplay = useBillingDisplay(i18n.language)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingEnvironment, setEditingEnvironment] = useState<Environment | null>(null)
  const [environmentToDelete, setEnvironmentToDelete] = useState<Environment | null>(null)
  const environments = useQuery({ queryKey: ['environments', projectId], queryFn: () => api.listEnvironments(projectId), enabled: Boolean(projectId) })
  const clusters = useQuery({ queryKey: ['runtime-clusters', projectId], queryFn: () => api.listRuntimeClusters(projectId), enabled: Boolean(projectId) })
  const clusterMap = useMemo(() => Object.fromEntries((clusters.data ?? []).map(cluster => [cluster.id, cluster])), [clusters.data])
  const form = useForm<EnvironmentForm>({ defaultValues: environmentDefaults, mode: 'onChange' })
  const runtimeHourCost = billingDisplay.runtimeHourCost(form.watch('replicas'), form.watch('cpuRequest'), form.watch('memoryRequest'))

  useImperativeHandle(ref, () => ({
    openCreateDialog: () => openDialog(),
  }))

  const saveEnvironment = useMutation({
    mutationFn: (values: EnvironmentForm) => {
      return editingEnvironment ? api.updateEnvironment(projectId, editingEnvironment.id, values) : api.createEnvironment(projectId, values)
    },
    onSuccess: () => {
      toast.success(t(editingEnvironment ? 'deploymentsPage.environmentUpdated' : 'deploymentsPage.environmentCreated'))
      setDialogOpen(false)
      setEditingEnvironment(null)
      form.reset(environmentDefaults)
      queryClient.invalidateQueries({ queryKey: ['environments', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const deleteEnvironment = useMutation({
    mutationFn: (environmentId: string) => api.deleteEnvironment(projectId, environmentId),
    onSuccess: () => {
      toast.success(t('deploymentsPage.environmentDeleted'))
      setEnvironmentToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['environments', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  function openDialog(environment?: Environment) {
    setEditingEnvironment(environment ?? null)
    form.reset(environment ? environmentFormFromEnvironment(environment) : environmentDefaults)
    setDialogOpen(true)
  }

  return (
    <div className="grid gap-4">
      <DataList
        columns={[
          { key: 'name', header: t('common.name'), render: item => item.name },
          { key: 'stage', header: t('deploymentsPage.stage'), render: item => t(`deploymentsPage.stageLabels.${item.stage}`, { defaultValue: item.stage }) },
          { key: 'cluster', header: t('deploymentsPage.cluster'), render: item => clusterLabel(clusterMap[item.clusterId], item.clusterId, t) },
          { key: 'runtime', header: t('deploymentsPage.runtimeProfile'), render: item => `${item.replicas || 1} / ${item.cpuRequest || '-'} / ${item.memoryRequest || '-'}` },
          { key: 'actions', header: t('common.actions'), className: 'text-right whitespace-nowrap', render: item => (
            <div className="flex justify-end gap-2">
              <EditActionButton label={t('common.edit')} onClick={() => openDialog(item)} />
              <Button size="sm" variant="ghost" onClick={() => setEnvironmentToDelete(item)}>
                <Trash2 className="size-4" />
                {t('common.delete')}
              </Button>
            </div>
          ) },
        ]}
        emptyTitle={t('deploymentsPage.emptyEnvironments')}
        items={environments.data ?? []}
        rowKey={item => item.id}
      />

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingEnvironment ? t('deploymentsPage.editEnvironment') : t('deploymentsPage.createEnvironment')}</DialogTitle>
            <DialogDescription>{t('deploymentsPage.environmentDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => saveEnvironment.mutate(values))}>
            <Field label={t('common.name')} required><Input {...form.register('name', { required: true })} /></Field>
            <Field hint={t('deploymentsPage.environmentSlugHint', { count: ENVIRONMENT_SLUG_MAX_LENGTH })} label={t('common.slug')} required>
              <Input {...form.register('slug', { maxLength: ENVIRONMENT_SLUG_MAX_LENGTH, required: true })} maxLength={ENVIRONMENT_SLUG_MAX_LENGTH} />
            </Field>
            <Field label={t('deploymentsPage.stage')}>
              <Select {...form.register('stage')}>
                <option value="dev">{t('deploymentsPage.stageDev')}</option>
                <option value="test">{t('deploymentsPage.stageTest')}</option>
                <option value="staging">{t('deploymentsPage.stageStaging')}</option>
                <option value="prod">{t('deploymentsPage.stageProd')}</option>
              </Select>
            </Field>
            <Field hint={t('deploymentsPage.clusterHint')} label={t('deploymentsPage.cluster')}>
              <Select {...form.register('clusterId')}>
                <option value="">{t('deploymentsPage.defaultCluster')}</option>
                {(clusters.data ?? []).map(cluster => <option key={cluster.id} value={cluster.id}>{clusterOptionLabel(cluster, t)}</option>)}
              </Select>
            </Field>
            <div className="grid gap-3 sm:grid-cols-3">
              <Field label={t('deploymentsPage.replicas')}><Input {...form.register('replicas', { valueAsNumber: true })} min={1} type="number" /></Field>
              <Field label={t('deploymentsPage.cpuRequest')}>
                <UnitInput
                  unitSelectLabel={t('deploymentsPage.cpuRequest')}
                  units={[
                    { label: t('deploymentsPage.cpuUnits.m'), value: 'm' },
                    { label: t('deploymentsPage.cpuUnits.core'), suffix: '', value: 'core' },
                  ]}
                  value={form.watch('cpuRequest')}
                  onChange={value => form.setValue('cpuRequest', value, { shouldDirty: true, shouldValidate: true })}
                />
              </Field>
              <Field label={t('deploymentsPage.memoryRequest')}>
                <UnitInput
                  unitSelectLabel={t('deploymentsPage.memoryRequest')}
                  units={[
                    { label: 'Mi', value: 'Mi' },
                    { label: 'Gi', value: 'Gi' },
                  ]}
                  value={form.watch('memoryRequest')}
                  onChange={value => form.setValue('memoryRequest', value, { shouldDirty: true, shouldValidate: true })}
                />
              </Field>
            </div>
            <p className="text-xs text-muted-foreground">
              {t('deploymentsPage.runtimeEstimatedPrice', { price: billingDisplay.formatAmountWithUnit(runtimeHourCost) })}
            </p>
            <DialogFooter><Button disabled={!projectId || !form.formState.isValid || saveEnvironment.isPending} type="submit">{t('common.save')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('common.delete')}
        description={t('deploymentsPage.deleteEnvironmentDescription')}
        open={Boolean(environmentToDelete)}
        title={t('deploymentsPage.deleteEnvironmentTitle')}
        onConfirm={() => environmentToDelete && deleteEnvironment.mutate(environmentToDelete.id)}
        onOpenChange={open => !open && setEnvironmentToDelete(null)}
      />
    </div>
  )
}

function clusterLabel(cluster: RuntimeCluster | undefined, clusterID: string, t: (key: string) => string) {
  if (cluster)
    return cluster.name
  if (clusterID)
    return clusterID
  return t('deploymentsPage.defaultCluster')
}

function clusterOptionLabel(cluster: RuntimeCluster, t: (key: string, options?: Record<string, unknown>) => string) {
  if (cluster.isDefault)
    return t('deploymentsPage.clusterDefaultOption', { name: cluster.name })
  return cluster.name
}

function environmentFormFromEnvironment(environment: Environment): EnvironmentForm {
  return {
    clusterId: environment.clusterId,
    configRefs: environment.configRefs,
    cpuRequest: environment.cpuRequest,
    envVars: environment.envVars,
    memoryRequest: environment.memoryRequest,
    name: environment.name,
    namespace: environment.namespace,
    replicas: environment.replicas,
    secretRefs: environment.secretRefs,
    slug: environment.slug,
    stage: environment.stage,
  }
}
