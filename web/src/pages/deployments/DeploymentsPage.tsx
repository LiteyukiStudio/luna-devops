import type { BuildRun, Environment, Release, RuntimeCluster } from '@/api/client'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, RotateCcw, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api/client'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { FormField as Field } from '@/components/common/form-field'
import { ProjectSpaceSelect } from '@/components/common/project-space-select'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { TabsContent } from '@/components/ui/tabs'

type ClusterForm = Omit<RuntimeCluster, 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & { kubeconfig?: string }
type EnvironmentForm = Omit<Environment, 'id' | 'projectId' | 'createdBy' | 'createdAt'>
type ReleaseForm = Omit<Release, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'rollbackFromId'>

const clusterDefaults: ClusterForm = { endpoint: '', isDefault: false, kubeconfig: '', name: '', ownerRef: '', scope: 'global', status: 'unknown', type: 'kubernetes' }
const environmentDefaults: EnvironmentForm = { clusterId: '', configRefs: '', cpuRequest: '100m', envVars: '{}', memoryRequest: '128Mi', name: '', namespace: '', replicas: 1, secretRefs: '', slug: '', stage: 'dev' }
const releaseDefaults: ReleaseForm = { applicationId: '', buildRunId: '', environmentId: '', imageRef: '', message: '', revision: 1, status: 'pending', type: 'deploy' }

export function DeploymentsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('clusters')
  const [selectedProjectId, setSelectedProjectId] = useState('')
  const [clusterDialogOpen, setClusterDialogOpen] = useState(false)
  const [environmentDialogOpen, setEnvironmentDialogOpen] = useState(false)
  const [releaseDialogOpen, setReleaseDialogOpen] = useState(false)
  const [editingCluster, setEditingCluster] = useState<RuntimeCluster | null>(null)
  const [editingEnvironment, setEditingEnvironment] = useState<Environment | null>(null)
  const [clusterToDelete, setClusterToDelete] = useState<RuntimeCluster | null>(null)
  const [environmentToDelete, setEnvironmentToDelete] = useState<Environment | null>(null)
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects })
  const clusters = useQuery({ queryKey: ['runtime-clusters'], queryFn: () => api.listRuntimeClusters() })
  const environments = useQuery({ queryKey: ['environments', selectedProjectId], queryFn: () => api.listEnvironments(selectedProjectId), enabled: Boolean(selectedProjectId) })
  const releases = useQuery({ queryKey: ['releases', selectedProjectId], queryFn: () => api.listReleases(selectedProjectId), enabled: Boolean(selectedProjectId) })
  const applications = useQuery({ queryKey: ['applications', selectedProjectId], queryFn: () => api.listApplications(selectedProjectId), enabled: Boolean(selectedProjectId) })
  const buildRuns = useQuery({ queryKey: ['build-runs', selectedProjectId], queryFn: () => api.listBuildRuns(selectedProjectId), enabled: Boolean(selectedProjectId) })

  const clusterForm = useForm<ClusterForm>({ defaultValues: clusterDefaults, mode: 'onChange' })
  const environmentForm = useForm<EnvironmentForm>({ defaultValues: environmentDefaults, mode: 'onChange' })
  const releaseForm = useForm<ReleaseForm>({ defaultValues: releaseDefaults, mode: 'onChange' })
  const buildRunMap = useMemo(() => Object.fromEntries((buildRuns.data ?? []).map(run => [run.id, run])), [buildRuns.data])
  const selectedBuildRun = buildRunMap[releaseForm.watch('buildRunId')]

  useEffect(() => {
    if (!selectedBuildRun)
      return
    releaseForm.setValue('applicationId', selectedBuildRun.applicationId, { shouldDirty: true, shouldValidate: true })
    releaseForm.setValue('imageRef', selectedBuildRun.imageRef || buildRunImageRef(selectedBuildRun), { shouldDirty: true, shouldValidate: true })
  }, [releaseForm, selectedBuildRun])

  const saveCluster = useMutation({
    mutationFn: (values: ClusterForm) => editingCluster ? api.updateRuntimeCluster(editingCluster.id, values) : api.createRuntimeCluster(values),
    onSuccess: () => {
      toast.success(t(editingCluster ? 'deploymentsPage.clusterUpdated' : 'deploymentsPage.clusterCreated'))
      setClusterDialogOpen(false)
      setEditingCluster(null)
      clusterForm.reset(clusterDefaults)
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
  const saveEnvironment = useMutation({
    mutationFn: (values: EnvironmentForm) => editingEnvironment ? api.updateEnvironment(selectedProjectId, editingEnvironment.id, values) : api.createEnvironment(selectedProjectId, values),
    onSuccess: () => {
      toast.success(t(editingEnvironment ? 'deploymentsPage.environmentUpdated' : 'deploymentsPage.environmentCreated'))
      setEnvironmentDialogOpen(false)
      setEditingEnvironment(null)
      environmentForm.reset(environmentDefaults)
      queryClient.invalidateQueries({ queryKey: ['environments', selectedProjectId] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteEnvironment = useMutation({
    mutationFn: (environmentId: string) => api.deleteEnvironment(selectedProjectId, environmentId),
    onSuccess: () => {
      toast.success(t('deploymentsPage.environmentDeleted'))
      setEnvironmentToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['environments', selectedProjectId] })
    },
    onError: error => toast.error(error.message),
  })
  const createRelease = useMutation({
    mutationFn: (values: ReleaseForm) => api.createRelease(selectedProjectId, values),
    onSuccess: () => {
      toast.success(t('deploymentsPage.releaseCreated'))
      setReleaseDialogOpen(false)
      releaseForm.reset(releaseDefaults)
      queryClient.invalidateQueries({ queryKey: ['releases', selectedProjectId] })
    },
    onError: error => toast.error(error.message),
  })
  const rollbackRelease = useMutation({
    mutationFn: (releaseId: string) => api.rollbackRelease(selectedProjectId, releaseId),
    onSuccess: () => {
      toast.success(t('deploymentsPage.rollbackQueued'))
      queryClient.invalidateQueries({ queryKey: ['releases', selectedProjectId] })
    },
    onError: error => toast.error(error.message),
  })

  function openClusterDialog(cluster?: RuntimeCluster) {
    setEditingCluster(cluster ?? null)
    clusterForm.reset(cluster ? { endpoint: cluster.endpoint, isDefault: cluster.isDefault, kubeconfig: '', name: cluster.name, ownerRef: cluster.ownerRef, scope: cluster.scope, status: cluster.status, type: cluster.type } : clusterDefaults)
    setClusterDialogOpen(true)
  }

  function openEnvironmentDialog(environment?: Environment) {
    setEditingEnvironment(environment ?? null)
    environmentForm.reset(environment ?? environmentDefaults)
    setEnvironmentDialogOpen(true)
  }

  return (
    <div className="grid gap-4">
      <ContentTabs
        tabs={[
          { label: t('deploymentsPage.clusters'), value: 'clusters' },
          { label: t('deploymentsPage.environments'), value: 'environments' },
          { label: t('deploymentsPage.releases'), value: 'releases' },
        ]}
        tools={(
          <>
            {activeTab !== 'clusters' && <ProjectSpaceSelect projects={projects.data ?? []} value={selectedProjectId} onChange={setSelectedProjectId} />}
            {activeTab === 'clusters' && (
              <Button onClick={() => openClusterDialog()}>
                <Plus className="size-4" />
                {t('deploymentsPage.createCluster')}
              </Button>
            )}
            {activeTab === 'environments' && (
              <Button disabled={!selectedProjectId} onClick={() => openEnvironmentDialog()}>
                <Plus className="size-4" />
                {t('deploymentsPage.createEnvironment')}
              </Button>
            )}
            {activeTab === 'releases' && (
              <Button disabled={!selectedProjectId} onClick={() => setReleaseDialogOpen(true)}>
                <Plus className="size-4" />
                {t('deploymentsPage.createRelease')}
              </Button>
            )}
          </>
        )}
        value={activeTab}
        onValueChange={setActiveTab}
      >
        <TabsContent value="clusters">
          <DataList
            columns={[
              { key: 'name', header: t('common.name'), render: item => item.name },
              { key: 'type', header: t('common.type'), render: item => item.type },
              { key: 'status', header: t('common.status'), render: item => <StatusValueBadge value={item.status} /> },
              { key: 'actions', header: t('common.actions'), className: 'text-right whitespace-nowrap', render: item => (
                <div className="flex justify-end gap-2">
                  <Button size="sm" variant="ghost" onClick={() => testCluster.mutate(item.id)}>{t('common.test')}</Button>
                  <EditActionButton label={t('common.edit')} onClick={() => openClusterDialog(item)} />
                  <Button size="sm" variant="ghost" onClick={() => setClusterToDelete(item)}>
                    <Trash2 className="size-4" />
                    {t('common.delete')}
                  </Button>
                </div>
              ) },
            ]}
            emptyTitle={t('deploymentsPage.emptyClusters')}
            items={clusters.data ?? []}
            rowKey={item => item.id}
            variant="plain"
          />
        </TabsContent>
        <TabsContent value="environments">
          <DataList
            columns={[
              { key: 'name', header: t('common.name'), render: item => item.name },
              { key: 'stage', header: t('deploymentsPage.stage'), render: item => item.stage },
              { key: 'namespace', header: t('deploymentsPage.namespace'), render: item => item.namespace || '-' },
              { key: 'replicas', header: t('deploymentsPage.replicas'), render: item => item.replicas },
              { key: 'actions', header: t('common.actions'), className: 'text-right whitespace-nowrap', render: item => (
                <div className="flex justify-end gap-2">
                  <EditActionButton label={t('common.edit')} onClick={() => openEnvironmentDialog(item)} />
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
            variant="plain"
          />
        </TabsContent>
        <TabsContent value="releases">
          <DataList
            columns={[
              { key: 'id', header: t('common.id'), render: item => item.id },
              { key: 'image', header: t('deploymentsPage.image'), render: item => item.imageRef },
              { key: 'branch', header: t('deploymentsPage.sourceBranch'), render: item => buildRunMap[item.buildRunId]?.sourceBranch || '-' },
              { key: 'status', header: t('common.status'), render: item => <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={item.status} /> },
              { key: 'actions', header: t('common.actions'), className: 'text-right whitespace-nowrap', render: item => (
                <Button size="sm" variant="ghost" onClick={() => rollbackRelease.mutate(item.id)}>
                  <RotateCcw className="size-4" />
                  {t('deploymentsPage.rollback')}
                </Button>
              ) },
            ]}
            emptyTitle={t('deploymentsPage.emptyReleases')}
            items={releases.data ?? []}
            rowKey={item => item.id}
            variant="plain"
          />
        </TabsContent>
      </ContentTabs>

      <Dialog open={clusterDialogOpen} onOpenChange={setClusterDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingCluster ? t('deploymentsPage.editCluster') : t('deploymentsPage.createCluster')}</DialogTitle>
            <DialogDescription>{t('deploymentsPage.clusterDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={clusterForm.handleSubmit(values => saveCluster.mutate(values))}>
            <Field label={t('common.name')} required><Input {...clusterForm.register('name', { required: true })} /></Field>
            <Field label={t('common.type')}>
              <Select {...clusterForm.register('type')}>
                <option value="kubernetes">{t('deploymentsPage.typeKubernetes')}</option>
              </Select>
            </Field>
            <Field label={t('deploymentsPage.kubeconfig')}><Input {...clusterForm.register('kubeconfig')} type="password" /></Field>
            <DialogFooter><Button disabled={!clusterForm.formState.isValid || saveCluster.isPending} type="submit">{t('common.save')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={environmentDialogOpen} onOpenChange={setEnvironmentDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingEnvironment ? t('deploymentsPage.editEnvironment') : t('deploymentsPage.createEnvironment')}</DialogTitle>
            <DialogDescription>{t('deploymentsPage.environmentDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={environmentForm.handleSubmit(values => saveEnvironment.mutate(values))}>
            <Field label={t('common.name')} required><Input {...environmentForm.register('name', { required: true })} /></Field>
            <Field label={t('common.slug')} required><Input {...environmentForm.register('slug', { required: true })} /></Field>
            <Field label={t('deploymentsPage.stage')}>
              <Select {...environmentForm.register('stage')}>
                <option value="dev">{t('deploymentsPage.stageDev')}</option>
                <option value="test">{t('deploymentsPage.stageTest')}</option>
                <option value="staging">{t('deploymentsPage.stageStaging')}</option>
                <option value="prod">{t('deploymentsPage.stageProd')}</option>
              </Select>
            </Field>
            <Field label={t('deploymentsPage.cluster')}>
              <Select {...environmentForm.register('clusterId')}>
                <option value="">{t('common.none')}</option>
                {(clusters.data ?? []).map(cluster => <option key={cluster.id} value={cluster.id}>{cluster.name}</option>)}
              </Select>
            </Field>
            <Field label={t('deploymentsPage.namespace')}><Input {...environmentForm.register('namespace')} /></Field>
            <Field label={t('deploymentsPage.replicas')}><Input {...environmentForm.register('replicas', { valueAsNumber: true })} type="number" /></Field>
            <DialogFooter><Button disabled={!selectedProjectId || !environmentForm.formState.isValid || saveEnvironment.isPending} type="submit">{t('common.save')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={releaseDialogOpen} onOpenChange={setReleaseDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('deploymentsPage.createRelease')}</DialogTitle>
            <DialogDescription>{t('deploymentsPage.releaseDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={releaseForm.handleSubmit(values => createRelease.mutate(values))}>
            <Field hint={t('deploymentsPage.buildRunHint')} label={t('deploymentsPage.buildRun')} required>
              <Select {...releaseForm.register('buildRunId', { required: true })}>
                <option value="">{t('common.select')}</option>
                {(buildRuns.data ?? []).map(run => <option key={run.id} value={run.id}>{buildRunOptionLabel(run)}</option>)}
              </Select>
            </Field>
            <Field label={t('apps.title')} required>
              <Select {...releaseForm.register('applicationId', { required: true })}>
                <option value="">{t('common.select')}</option>
                {(applications.data ?? []).map(app => <option key={app.id} value={app.id}>{app.name}</option>)}
              </Select>
            </Field>
            <Field label={t('deploymentsPage.environment')} required>
              <Select {...releaseForm.register('environmentId', { required: true })}>
                <option value="">{t('common.select')}</option>
                {(environments.data ?? []).map(environment => <option key={environment.id} value={environment.id}>{environment.name}</option>)}
              </Select>
            </Field>
            <Field label={t('deploymentsPage.image')} required><Input {...releaseForm.register('imageRef', { required: true })} /></Field>
            <DialogFooter><Button disabled={!selectedProjectId || !releaseForm.formState.isValid || createRelease.isPending} type="submit">{t('common.save')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <ConfirmDialog cancelText={t('common.cancel')} confirmText={t('common.delete')} description={t('deploymentsPage.deleteClusterDescription')} open={Boolean(clusterToDelete)} title={t('deploymentsPage.deleteClusterTitle')} onConfirm={() => clusterToDelete && deleteCluster.mutate(clusterToDelete.id)} onOpenChange={open => !open && setClusterToDelete(null)} />
      <ConfirmDialog cancelText={t('common.cancel')} confirmText={t('common.delete')} description={t('deploymentsPage.deleteEnvironmentDescription')} open={Boolean(environmentToDelete)} title={t('deploymentsPage.deleteEnvironmentTitle')} onConfirm={() => environmentToDelete && deleteEnvironment.mutate(environmentToDelete.id)} onOpenChange={open => !open && setEnvironmentToDelete(null)} />
    </div>
  )
}

function buildRunImageRef(run: BuildRun) {
  if (run.imageRef)
    return run.imageRef
  if (run.targetRepository)
    return `${run.targetRepository}:${run.targetTag || 'latest'}`
  return ''
}

function buildRunOptionLabel(run: BuildRun) {
  const branch = run.sourceBranch || run.sourceTag || '-'
  const image = buildRunImageRef(run) || run.targetRepository || run.id
  return `${branch} · ${run.status} · ${image}`
}
