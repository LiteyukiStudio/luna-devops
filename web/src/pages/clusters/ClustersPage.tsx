import type { ClusterResource, ClusterResourceEvent, CurrentUser, RuntimeCluster } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, FileCode2, Plus, RefreshCcw, ScrollText, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { CheckboxField } from '@/components/common/checkbox-field'
import { CodeEditor } from '@/components/common/code-editor'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { EmptyState } from '@/components/common/empty-state'
import { FormField as Field } from '@/components/common/form-field'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { ProjectSpaceMultiSelect } from '@/components/common/project-space-select'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { TabsContent } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { inspectKubeconfig, selectSingleKubeconfigContext } from '@/lib/kubeconfig'

type ClusterForm = Omit<RuntimeCluster, 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & { kubeconfig?: string }

const clusterDefaults: ClusterForm = {
  endpoint: '',
  isDefault: false,
  kubeconfig: '',
  gatewayControllerType: 'traefik',
  gatewayClassName: 'traefik',
  gatewayDefaultRequestHeaders: '',
  gatewayDefaultResponseHeaders: '',
  gatewayExternalTLSMode: 'none',
  gatewayForwardedHeadersMode: 'preserve',
  gatewayName: 'liteyuki-gateway',
  gatewayNamespace: 'kube-system',
  gatewayProvider: 'gateway-api',
  gatewayPublicPort: 80,
  gatewayPublicScheme: 'http',
  gatewayRootDomain: 'apps.local',
  gatewayHttpListenerName: 'web',
  gatewayHttpListenerPort: 8080,
  gatewayHttpsListenerName: 'websecure',
  gatewayHttpsListenerPort: 8443,
  gatewayTrustedProxyCIDRs: '',
  maxConcurrentBuilds: 4,
  name: '',
  ownerRef: '',
  projectIds: [],
  scope: 'global',
  status: 'unknown',
  type: 'kubernetes',
}

const RESOURCE_PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

interface ClusterResourcePagination {
  page: number
  pageSize: number
  total: number
  totalPages: number
  pageInfoLabel: string
  pageSizeOptions: number[]
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
}

type ClusterResourceRow = ClusterResource & {
  depth?: number
  hasChildren?: boolean
  parentId?: string
}

export function ClustersPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('clusters')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingCluster, setEditingCluster] = useState<RuntimeCluster | null>(null)
  const [clusterToDelete, setClusterToDelete] = useState<RuntimeCluster | null>(null)
  const [resourceToDelete, setResourceToDelete] = useState<ClusterResource | null>(null)
  const [resourcesToDelete, setResourcesToDelete] = useState<ClusterResource[]>([])
  const [eventResource, setEventResource] = useState<ClusterResource | null>(null)
  const [yamlResource, setYamlResource] = useState<ClusterResource | null>(null)
  const [selectedResourceClusterId, setSelectedResourceClusterId] = useState('')
  const [selectedResourceKeys, setSelectedResourceKeys] = useState<string[]>([])
  const [resourcePage, setResourcePage] = useState(1)
  const [resourcePageSize, setResourcePageSize] = useState(10)
  const [selectedKubeconfigContext, setSelectedKubeconfigContext] = useState('')
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects })
  const clusters = useQuery({ queryKey: ['runtime-clusters'], queryFn: () => api.listRuntimeClusters() })
  const projectMap = useMemo(() => Object.fromEntries((projects.data ?? []).map(project => [project.id, project])), [projects.data])
  const manageableClusters = useMemo(() => (clusters.data ?? []).filter(cluster => canManageCluster(cluster, user?.id, user?.role)), [clusters.data, user?.id, user?.role])
  const effectiveResourceClusterId = manageableClusters.some(cluster => cluster.id === selectedResourceClusterId) ? selectedResourceClusterId : manageableClusters[0]?.id ?? ''
  const selectedResourceCluster = manageableClusters.find(cluster => cluster.id === effectiveResourceClusterId)
  const resourceKind = activeTab === 'clusters' ? 'namespaces' : activeTab
  const clusterResources = useQuery({
    queryKey: ['runtime-cluster-resources', selectedResourceCluster?.id, resourceKind, resourcePage, resourcePageSize],
    queryFn: () => api.listRuntimeClusterResourcesPage(selectedResourceCluster?.id ?? '', {
      kind: resourceKind,
      page: resourcePage,
      pageSize: resourcePageSize,
      sortBy: 'updatedAt',
      sortOrder: 'desc',
    }),
    enabled: activeTab !== 'clusters' && Boolean(selectedResourceCluster?.id),
  })
  const activeResourceItems = useMemo(() => activeTab === 'clusters' ? [] : clusterResources.data?.items ?? [], [activeTab, clusterResources.data?.items])
  const activeResourceKeySet = useMemo(() => new Set(activeResourceItems.map(item => item.id)), [activeResourceItems])
  const visibleSelectedResourceKeys = useMemo(() => selectedResourceKeys.filter(key => activeResourceKeySet.has(key)), [activeResourceKeySet, selectedResourceKeys])
  const selectedDeletableResources = useMemo(() => {
    const selectedKeys = new Set(visibleSelectedResourceKeys)
    return activeResourceItems.filter(item => selectedKeys.has(item.id) && canDeleteClusterResource(user, item))
  }, [activeResourceItems, user, visibleSelectedResourceKeys])
  const resourceEvents = useQuery({
    queryKey: ['runtime-cluster-resource-events', selectedResourceCluster?.id, eventResource?.kind, eventResource?.namespace, eventResource?.name],
    queryFn: () => api.listRuntimeClusterResourceEvents(selectedResourceCluster?.id ?? '', {
      kind: eventResource?.kind ?? '',
      namespace: eventResource?.namespace,
      name: eventResource?.name ?? '',
    }),
    enabled: Boolean(selectedResourceCluster?.id && eventResource),
  })
  const resourceYAML = useQuery({
    queryKey: ['runtime-cluster-resource-yaml', selectedResourceCluster?.id, yamlResource?.kind, yamlResource?.namespace, yamlResource?.name],
    queryFn: () => api.getRuntimeClusterResourceYAML(selectedResourceCluster?.id ?? '', {
      kind: yamlResource?.kind ?? '',
      namespace: yamlResource?.namespace,
      name: yamlResource?.name ?? '',
    }),
    enabled: Boolean(selectedResourceCluster?.id && yamlResource),
  })
  const form = useForm<ClusterForm>({ defaultValues: clusterDefaults, mode: 'onChange' })
  const scope = form.watch('scope')
  const canEditKubeconfig = !editingCluster || canInspectClusterKubeconfig(editingCluster, user?.id, user?.role)
  const kubeconfigValue = form.watch('kubeconfig') ?? ''
  const kubeconfigInspection = useMemo(() => inspectKubeconfig(kubeconfigValue), [kubeconfigValue])
  const kubeconfigContextSelectionRequired = canEditKubeconfig && kubeconfigInspection.contexts.length > 1
  const effectiveKubeconfigContext = useMemo(() => {
    if (!canEditKubeconfig || kubeconfigInspection.contexts.length === 0)
      return ''
    if (kubeconfigInspection.contexts.some(context => context.name === selectedKubeconfigContext))
      return selectedKubeconfigContext
    return kubeconfigInspection.currentContext || kubeconfigInspection.contexts[0]?.name || ''
  }, [canEditKubeconfig, kubeconfigInspection, selectedKubeconfigContext])

  useEffect(() => {
    if (scope !== 'global')
      form.setValue('isDefault', false, { shouldDirty: true, shouldValidate: true })
    if (scope === 'user')
      form.setValue('ownerRef', '', { shouldDirty: true, shouldValidate: true })
    if (scope !== 'project')
      form.setValue('projectIds', [], { shouldDirty: true, shouldValidate: true })
  }, [form, scope])

  useEffect(() => {
    setResourcePage(1)
    setSelectedResourceKeys([])
  }, [activeTab, effectiveResourceClusterId])

  const saveCluster = useMutation({
    mutationFn: (values: ClusterForm) => {
      const payload = {
        ...values,
        ownerRef: '',
        projectIds: values.scope === 'project' ? values.projectIds : [],
      }
      return editingCluster ? api.updateRuntimeCluster(editingCluster.id, payload) : api.createRuntimeCluster(payload)
    },
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
  const deleteResource = useMutation({
    mutationFn: (resource: ClusterResource) => api.deleteRuntimeClusterResource(effectiveResourceClusterId, {
      kind: resource.kind,
      namespace: resource.namespace,
      name: resource.name,
    }),
    onSuccess: () => {
      toast.success(t('clustersPage.resourceDeleted'))
      setResourceToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['runtime-cluster-resources', selectedResourceCluster?.id, resourceKind] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteResources = useMutation({
    mutationFn: async (resources: ClusterResource[]) => {
      for (const resource of resources) {
        await api.deleteRuntimeClusterResource(effectiveResourceClusterId, {
          kind: resource.kind,
          namespace: resource.namespace,
          name: resource.name,
        })
      }
    },
    onSuccess: (_, resources) => {
      toast.success(t('clustersPage.resourcesDeleted', { count: resources.length }))
      setResourcesToDelete([])
      setSelectedResourceKeys([])
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['runtime-cluster-resources', selectedResourceCluster?.id, resourceKind] })
    },
    onError: error => toast.error(error.message),
  })

  function openDialog(cluster?: RuntimeCluster) {
    setEditingCluster(cluster ?? null)
    setSelectedKubeconfigContext('')
    form.reset(cluster
      ? {
          endpoint: cluster.endpoint,
          isDefault: cluster.isDefault,
          kubeconfig: '',
          gatewayControllerType: cluster.gatewayControllerType || 'traefik',
          gatewayClassName: cluster.gatewayClassName || 'traefik',
          gatewayDefaultRequestHeaders: cluster.gatewayDefaultRequestHeaders || '',
          gatewayDefaultResponseHeaders: cluster.gatewayDefaultResponseHeaders || '',
          gatewayExternalTLSMode: cluster.gatewayExternalTLSMode || 'none',
          gatewayForwardedHeadersMode: cluster.gatewayForwardedHeadersMode || 'preserve',
          gatewayName: cluster.gatewayName || 'liteyuki-gateway',
          gatewayNamespace: cluster.gatewayNamespace || 'kube-system',
          gatewayProvider: cluster.gatewayProvider || 'gateway-api',
          gatewayPublicPort: cluster.gatewayPublicPort || defaultGatewayPublicPort(cluster.gatewayPublicScheme || 'http'),
          gatewayPublicScheme: cluster.gatewayPublicScheme || 'http',
          gatewayRootDomain: cluster.gatewayRootDomain || 'apps.local',
          gatewayHttpListenerName: cluster.gatewayHttpListenerName || 'web',
          gatewayHttpListenerPort: cluster.gatewayHttpListenerPort || 8080,
          gatewayHttpsListenerName: cluster.gatewayHttpsListenerName || 'websecure',
          gatewayHttpsListenerPort: cluster.gatewayHttpsListenerPort || 8443,
          gatewayTrustedProxyCIDRs: cluster.gatewayTrustedProxyCIDRs || '',
          maxConcurrentBuilds: cluster.maxConcurrentBuilds || 4,
          name: cluster.name,
          ownerRef: cluster.ownerRef,
          projectIds: cluster.projectIds ?? [],
          scope: cluster.scope,
          status: cluster.status,
          type: cluster.type,
        }
      : clusterDefaults)
    setDialogOpen(true)
  }

  function submitCluster(values: ClusterForm) {
    let kubeconfig = values.kubeconfig ?? ''
    if (canEditKubeconfig && kubeconfig.trim() !== '') {
      if (kubeconfigInspection.error) {
        toast.error(t('clustersPage.kubeconfigParseFailed'))
        return
      }
      if (kubeconfigContextSelectionRequired && !effectiveKubeconfigContext) {
        toast.error(t('clustersPage.kubeconfigContextRequired'))
        return
      }
      try {
        kubeconfig = selectSingleKubeconfigContext(kubeconfig, effectiveKubeconfigContext)
      }
      catch {
        toast.error(t('clustersPage.kubeconfigContextInvalid'))
        return
      }
    }
    const maxConcurrentBuilds = Number.isFinite(values.maxConcurrentBuilds) && values.maxConcurrentBuilds > 0
      ? Math.floor(values.maxConcurrentBuilds)
      : 4
    saveCluster.mutate({
      ...values,
      gatewayHttpListenerPort: normalizeFormPort(values.gatewayHttpListenerPort, 8080),
      gatewayHttpsListenerPort: normalizeFormPort(values.gatewayHttpsListenerPort, 8443),
      gatewayPublicPort: normalizeFormPort(values.gatewayPublicPort, defaultGatewayPublicPort(values.gatewayPublicScheme)),
      kubeconfig,
      maxConcurrentBuilds,
    })
  }

  const resourcePagination: ClusterResourcePagination | undefined = activeTab === 'clusters'
    ? undefined
    : {
        page: clusterResources.data?.page ?? resourcePage,
        pageSize: clusterResources.data?.pageSize ?? resourcePageSize,
        pageInfoLabel: t('pagination.pageInfo', {
          page: clusterResources.data?.page ?? resourcePage,
          total: clusterResources.data?.total ?? 0,
          totalPages: clusterResources.data?.totalPages ?? 0,
        }),
        pageSizeOptions: RESOURCE_PAGE_SIZE_OPTIONS,
        total: clusterResources.data?.total ?? 0,
        totalPages: clusterResources.data?.totalPages ?? 0,
        onPageChange: (page) => {
          setResourcePage(page)
          setSelectedResourceKeys([])
        },
        onPageSizeChange: (pageSize) => {
          setResourcePageSize(pageSize)
          setResourcePage(1)
          setSelectedResourceKeys([])
        },
      }

  return (
    <div className="grid gap-4">
      <ContentTabs
        tabs={[
          { label: t('clustersPage.runtimeClustersTab'), value: 'clusters' },
          { label: t('clustersPage.namespacesTab'), value: 'namespaces' },
          { label: t('clustersPage.workloadsTab'), value: 'workloads' },
          { label: t('clustersPage.servicesTab'), value: 'services' },
          { label: t('clustersPage.configsTab'), value: 'configs' },
          { label: t('clustersPage.storageTab'), value: 'storage' },
        ]}
        tools={(
          <div className="flex flex-wrap items-center gap-2">
            {activeTab === 'clusters'
              ? (
                  <>
                    <Button onClick={() => openDialog()}>
                      <Plus className="size-4" />
                      {t('deploymentsPage.createCluster')}
                    </Button>
                  </>
                )
              : (
                  <>
                    <Select
                      aria-label={t('clustersPage.selectResourceCluster')}
                      className="h-9"
                      containerClassName="w-52 max-w-full"
                      disabled={manageableClusters.length === 0}
                      value={effectiveResourceClusterId}
                      onChange={(event) => {
                        setSelectedResourceClusterId(event.target.value)
                        setResourcePage(1)
                        setSelectedResourceKeys([])
                      }}
                    >
                      {manageableClusters.length > 0
                        ? manageableClusters.map(cluster => <option key={cluster.id} value={cluster.id}>{cluster.name}</option>)
                        : <option value="">{t('clustersPage.noManageableClusterTitle')}</option>}
                    </Select>
                    <Button disabled={!selectedResourceCluster || clusterResources.isFetching} variant="secondary" onClick={() => clusterResources.refetch()}>
                      <RefreshCcw className="size-4" />
                      {t('common.refresh')}
                    </Button>
                    {visibleSelectedResourceKeys.length > 0 && (
                      <span className="text-xs text-muted-foreground">
                        {t('clustersPage.selectedResources', { count: selectedDeletableResources.length })}
                      </span>
                    )}
                    <Button
                      disabled={selectedDeletableResources.length === 0 || deleteResources.isPending}
                      variant="destructive"
                      onClick={() => setResourcesToDelete(selectedDeletableResources)}
                    >
                      <Trash2 className="size-4" />
                      {t('clustersPage.deleteSelectedResources')}
                    </Button>
                  </>
                )}
          </div>
        )}
        value={activeTab}
        onValueChange={(value) => {
          setActiveTab(value)
          setResourcePage(1)
          setSelectedResourceKeys([])
        }}
      >
        <TabsContent value="clusters">
          <DataList
            columns={[
              { key: 'name', header: t('common.name'), width: 'primary', render: item => item.name },
              { key: 'type', header: t('common.type'), width: 'secondary', render: item => clusterTypeLabel(item.type, t) },
              { key: 'scope', header: t('common.scope'), width: 'status', render: item => scopeLabel(item, projectMap, t) },
              { key: 'default', header: t('clustersPage.defaultCluster'), width: 'status', render: item => item.isDefault ? t('common.yes') : t('common.no') },
              { key: 'buildConcurrency', header: t('clustersPage.maxConcurrentBuilds'), width: 'number', render: item => item.maxConcurrentBuilds || 4 },
              { key: 'gatewayRootDomain', header: t('clustersPage.gatewayRootDomain'), width: 'secondary', render: item => item.gatewayRootDomain || 'apps.local' },
              { key: 'gatewayPublicScheme', header: t('clustersPage.gatewayPublicScheme'), width: 'compact', render: item => item.gatewayPublicScheme || 'http' },
              { key: 'gatewayPublicPort', header: t('clustersPage.gatewayPublicPort'), width: 'compact', render: item => gatewayPublicPortSummary(item) },
              { key: 'status', header: t('common.status'), width: 'status', render: item => <StatusValueBadge value={item.status} /> },
              { key: 'actions', header: t('common.actions'), className: 'text-right whitespace-nowrap', sticky: 'right', width: 'actions', render: item => (
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
        </TabsContent>
        {['namespaces', 'workloads', 'services', 'configs', 'storage'].map(tab => (
          <TabsContent key={tab} value={tab}>
            <ClusterResourcesPanel
              items={activeTab === tab ? activeResourceItems : []}
              loading={activeTab === tab && clusterResources.isFetching}
              pagination={activeTab === tab ? resourcePagination : undefined}
              selectedCluster={selectedResourceCluster}
              selectedResourceKeys={activeTab === tab ? selectedResourceKeys : []}
              tab={tab}
              user={user}
              onDeleteResource={setResourceToDelete}
              onOpenEvents={setEventResource}
              onOpenYAML={setYamlResource}
              onSelectionChange={setSelectedResourceKeys}
            />
          </TabsContent>
        ))}
      </ContentTabs>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="flex max-h-[min(88vh,52rem)] w-[min(92vw,48rem)] max-w-[92vw] min-w-0 flex-col gap-0 overflow-hidden p-0">
          <DialogHeader className="shrink-0 border-b border-border p-5 pb-4">
            <DialogTitle>{editingCluster ? t('deploymentsPage.editCluster') : t('deploymentsPage.createCluster')}</DialogTitle>
            <DialogDescription>{t('clustersPage.dialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="flex min-h-0 min-w-0 flex-1 flex-col" onSubmit={form.handleSubmit(submitCluster)}>
            <div className="min-h-0 min-w-0 max-w-full flex-1 overflow-y-auto overflow-x-hidden px-5 py-4">
              <div className="grid min-w-0 max-w-full gap-3 overflow-x-hidden">
                <ProgressiveSection
                  defaultOpen
                  description={t('clustersPage.basicClusterConfigDescription')}
                  title={t('clustersPage.basicClusterConfig')}
                >
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
                      <ProjectSpaceMultiSelect
                        projects={projects.data ?? []}
                        value={form.watch('projectIds')}
                        onChange={value => form.setValue('projectIds', value, { shouldDirty: true, shouldValidate: true })}
                      />
                    </Field>
                  )}
                  <Field label={t('common.type')}>
                    <Select {...form.register('type')}>
                      <option value="kubernetes">{t('deploymentsPage.typeKubernetes')}</option>
                    </Select>
                  </Field>
                  <Field hint={t('clustersPage.maxConcurrentBuildsHint')} label={t('clustersPage.maxConcurrentBuilds')} required>
                    <Input
                      {...form.register('maxConcurrentBuilds', { min: 1, required: true, valueAsNumber: true })}
                      inputMode="numeric"
                      min={1}
                      placeholder={t('clustersPage.maxConcurrentBuildsPlaceholder')}
                      type="number"
                    />
                  </Field>
                  {scope === 'global' && (
                    <CheckboxField {...form.register('isDefault')}>
                      {t('clustersPage.defaultCluster')}
                    </CheckboxField>
                  )}
                </ProgressiveSection>
                <ProgressiveSection
                  defaultOpen
                  description={t('clustersPage.gatewayClusterConfigDescription')}
                  title={t('clustersPage.gatewayClusterConfig')}
                >
                  <div className="grid gap-3 md:grid-cols-2">
                    <Field hint={t('clustersPage.gatewayRootDomainHint')} label={t('clustersPage.gatewayRootDomain')} required>
                      <Input {...form.register('gatewayRootDomain', { required: true })} placeholder={t('clustersPage.gatewayRootDomainPlaceholder')} />
                    </Field>
                    <Field hint={t('clustersPage.gatewayPublicSchemeHint')} label={t('clustersPage.gatewayPublicScheme')} required>
                      <Select {...form.register('gatewayPublicScheme', {
                        onChange: (event) => {
                          const nextScheme = event.target.value as RuntimeCluster['gatewayPublicScheme']
                          const currentPort = form.getValues('gatewayPublicPort')
                          if (currentPort === 80 || currentPort === 443 || !currentPort) {
                            form.setValue('gatewayPublicPort', defaultGatewayPublicPort(nextScheme), { shouldDirty: true, shouldValidate: true })
                          }
                        },
                      })}
                      >
                        <option value="http">http</option>
                        <option value="https">https</option>
                      </Select>
                    </Field>
                    <Field hint={t('clustersPage.gatewayPublicPortHint')} label={t('clustersPage.gatewayPublicPort')} required>
                      <Input {...form.register('gatewayPublicPort', { min: 1, required: true, valueAsNumber: true })} inputMode="numeric" max={65535} min={1} placeholder={String(defaultGatewayPublicPort(form.watch('gatewayPublicScheme')))} type="number" />
                    </Field>
                    <Field hint={t('clustersPage.gatewayControllerTypeHint')} label={t('clustersPage.gatewayControllerType')}>
                      <Select {...form.register('gatewayControllerType')}>
                        <option value="traefik">Traefik</option>
                        <option value="generic">{t('clustersPage.gatewayControllerGeneric')}</option>
                      </Select>
                    </Field>
                    <Field hint={t('clustersPage.gatewayProviderHint')} label={t('clustersPage.gatewayProvider')}>
                      <Select {...form.register('gatewayProvider')}>
                        <option value="gateway-api">{t('clustersPage.gatewayProviderGatewayAPI')}</option>
                      </Select>
                    </Field>
                    <Field hint={t('clustersPage.gatewayClassNameHint')} label={t('clustersPage.gatewayClassName')}>
                      <Input {...form.register('gatewayClassName')} placeholder={t('clustersPage.gatewayClassNamePlaceholder')} />
                    </Field>
                    <Field hint={t('clustersPage.gatewayNameHint')} label={t('clustersPage.gatewayName')}>
                      <Input {...form.register('gatewayName')} placeholder={t('clustersPage.gatewayNamePlaceholder')} />
                    </Field>
                    <Field hint={t('clustersPage.gatewayNamespaceHint')} label={t('clustersPage.gatewayNamespace')}>
                      <Input {...form.register('gatewayNamespace')} placeholder={t('clustersPage.gatewayNamespacePlaceholder')} />
                    </Field>
                    <Field hint={t('clustersPage.gatewayHttpListenerNameHint')} label={t('clustersPage.gatewayHttpListenerName')}>
                      <Input {...form.register('gatewayHttpListenerName')} placeholder={t('clustersPage.gatewayHttpListenerNamePlaceholder')} />
                    </Field>
                    <Field hint={t('clustersPage.gatewayHttpListenerPortHint')} label={t('clustersPage.gatewayHttpListenerPort')} required>
                      <Input {...form.register('gatewayHttpListenerPort', { min: 1, required: true, valueAsNumber: true })} inputMode="numeric" max={65535} min={1} placeholder="8080" type="number" />
                    </Field>
                    <Field hint={t('clustersPage.gatewayHttpsListenerNameHint')} label={t('clustersPage.gatewayHttpsListenerName')}>
                      <Input {...form.register('gatewayHttpsListenerName')} placeholder={t('clustersPage.gatewayHttpsListenerNamePlaceholder')} />
                    </Field>
                    <Field hint={t('clustersPage.gatewayHttpsListenerPortHint')} label={t('clustersPage.gatewayHttpsListenerPort')} required>
                      <Input {...form.register('gatewayHttpsListenerPort', { min: 1, required: true, valueAsNumber: true })} inputMode="numeric" max={65535} min={1} placeholder="8443" type="number" />
                    </Field>
                    <Field hint={t('clustersPage.gatewayExternalTLSModeHint')} label={t('clustersPage.gatewayExternalTLSMode')}>
                      <Select {...form.register('gatewayExternalTLSMode')}>
                        <option value="none">{t('clustersPage.gatewayTLSNone')}</option>
                        <option value="gateway">{t('clustersPage.gatewayTLSGateway')}</option>
                        <option value="upstream">{t('clustersPage.gatewayTLSUpstream')}</option>
                      </Select>
                    </Field>
                    <Field hint={t('clustersPage.gatewayForwardedHeadersModeHint')} label={t('clustersPage.gatewayForwardedHeadersMode')}>
                      <Select {...form.register('gatewayForwardedHeadersMode')}>
                        <option value="preserve">{t('clustersPage.gatewayForwardedPreserve')}</option>
                        <option value="overwrite">{t('clustersPage.gatewayForwardedOverwrite')}</option>
                        <option value="none">{t('clustersPage.gatewayForwardedNone')}</option>
                      </Select>
                    </Field>
                  </div>
                </ProgressiveSection>
                <ProgressiveSection
                  description={t('clustersPage.gatewayAdvancedDefaultsDescription')}
                  title={t('clustersPage.gatewayAdvancedDefaults')}
                >
                  <Field hint={t('clustersPage.gatewayTrustedProxyCIDRsHint')} label={t('clustersPage.gatewayTrustedProxyCIDRs')}>
                    <Textarea {...form.register('gatewayTrustedProxyCIDRs')} placeholder={t('clustersPage.gatewayTrustedProxyCIDRsPlaceholder')} rows={3} />
                  </Field>
                  <Field hint={t('clustersPage.gatewayDefaultRequestHeadersHint')} label={t('clustersPage.gatewayDefaultRequestHeaders')}>
                    <Textarea {...form.register('gatewayDefaultRequestHeaders')} placeholder={t('gatewayRoutesPage.headersPlaceholder')} rows={4} />
                  </Field>
                  <Field hint={t('clustersPage.gatewayDefaultResponseHeadersHint')} label={t('clustersPage.gatewayDefaultResponseHeaders')}>
                    <Textarea {...form.register('gatewayDefaultResponseHeaders')} placeholder={t('gatewayRoutesPage.headersPlaceholder')} rows={4} />
                  </Field>
                </ProgressiveSection>
                <ProgressiveSection
                  defaultOpen
                  description={canEditKubeconfig ? t('clustersPage.kubeconfigHint') : t('clustersPage.kubeconfigRestrictedHint')}
                  title={t('deploymentsPage.kubeconfig')}
                >
                  <Field hint={canEditKubeconfig ? t('clustersPage.kubeconfigHint') : t('clustersPage.kubeconfigRestrictedHint')} label={t('deploymentsPage.kubeconfig')} required={!editingCluster}>
                    <Controller
                      control={form.control}
                      name="kubeconfig"
                      rules={{ required: !editingCluster }}
                      render={({ field }) => (
                        <div className="min-w-0 max-w-full overflow-x-hidden">
                          <CodeEditor
                            ariaInvalid={Boolean(form.formState.errors.kubeconfig) || kubeconfigInspection.error}
                            className="w-full"
                            height="22rem"
                            language="yaml"
                            placeholder={t('clustersPage.kubeconfigPlaceholder')}
                            readOnly={!canEditKubeconfig}
                            value={field.value ?? ''}
                            onChange={field.onChange}
                          />
                          {canEditKubeconfig && kubeconfigInspection.error && (
                            <p className="mt-2 text-sm text-danger">{t('clustersPage.kubeconfigParseFailed')}</p>
                          )}
                          {canEditKubeconfig && kubeconfigInspection.contexts.length === 1 && (
                            <p className="mt-2 text-sm text-muted-foreground">
                              {t('clustersPage.kubeconfigSingleContext', { context: kubeconfigInspection.contexts[0].name })}
                            </p>
                          )}
                          {kubeconfigContextSelectionRequired && (
                            <div className="mt-3 grid gap-2">
                              <label className="text-sm font-medium text-foreground" htmlFor="cluster-kubeconfig-context">
                                {t('clustersPage.kubeconfigContextLabel')}
                              </label>
                              <Select
                                id="cluster-kubeconfig-context"
                                value={effectiveKubeconfigContext}
                                onChange={event => setSelectedKubeconfigContext(event.target.value)}
                              >
                                {kubeconfigInspection.contexts.map(context => (
                                  <option key={context.name} value={context.name}>
                                    {kubeconfigContextOptionLabel(context)}
                                  </option>
                                ))}
                              </Select>
                              <p className="text-xs text-muted-foreground">{t('clustersPage.kubeconfigContextHint')}</p>
                            </div>
                          )}
                        </div>
                      )}
                    />
                  </Field>
                </ProgressiveSection>
              </div>
            </div>
            <DialogFooter className="shrink-0 border-t border-border p-5 pt-4">
              <Button disabled={!form.formState.isValid || saveCluster.isPending || kubeconfigInspection.error || (kubeconfigContextSelectionRequired && !effectiveKubeconfigContext)} type="submit">{t('common.save')}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog cancelText={t('common.cancel')} confirmText={t('common.delete')} description={t('deploymentsPage.deleteClusterDescription')} open={Boolean(clusterToDelete)} title={t('deploymentsPage.deleteClusterTitle')} onConfirm={() => clusterToDelete && deleteCluster.mutate(clusterToDelete.id)} onOpenChange={open => !open && setClusterToDelete(null)} />
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('common.delete')}
        description={t('clustersPage.deleteResourceDescription', { kind: resourceToDelete?.kind ?? '', namespace: resourceToDelete?.namespace || '-', name: resourceToDelete?.name ?? '' })}
        open={Boolean(resourceToDelete)}
        pending={deleteResource.isPending}
        title={t('clustersPage.deleteResourceTitle')}
        onConfirm={() => resourceToDelete && deleteResource.mutate(resourceToDelete)}
        onOpenChange={open => !open && setResourceToDelete(null)}
      />
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('common.delete')}
        description={t('clustersPage.deleteResourcesDescription', { count: resourcesToDelete.length })}
        open={resourcesToDelete.length > 0}
        pending={deleteResources.isPending}
        title={t('clustersPage.deleteResourcesTitle')}
        onConfirm={() => {
          if (resourcesToDelete.length > 0) {
            deleteResources.mutate(resourcesToDelete)
          }
        }}
        onOpenChange={open => !open && setResourcesToDelete([])}
      />
      <Dialog open={Boolean(eventResource)} onOpenChange={open => !open && setEventResource(null)}>
        <DialogContent className="flex max-h-[min(88vh,42rem)] w-[min(92vw,56rem)] max-w-[92vw] min-w-0 flex-col overflow-hidden p-0">
          <DialogHeader className="shrink-0 border-b border-border p-5 pb-4">
            <DialogTitle>{t('clustersPage.resourceEventsTitle')}</DialogTitle>
            <DialogDescription>
              {eventResource ? t('clustersPage.resourceEventsDescription', { kind: eventResource.kind, namespace: eventResource.namespace || '-', name: eventResource.name }) : ''}
            </DialogDescription>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto p-5">
            <ClusterResourceEventsList events={resourceEvents.data ?? []} loading={resourceEvents.isFetching} />
          </div>
        </DialogContent>
      </Dialog>
      <Dialog open={Boolean(yamlResource)} onOpenChange={open => !open && setYamlResource(null)}>
        <DialogContent className="flex max-h-[min(88vh,46rem)] w-[min(92vw,64rem)] max-w-[92vw] min-w-0 flex-col overflow-hidden p-0">
          <DialogHeader className="shrink-0 border-b border-border p-5 pb-4">
            <DialogTitle>{t('clustersPage.resourceYamlTitle')}</DialogTitle>
            <DialogDescription>
              {yamlResource ? t('clustersPage.resourceYamlDescription', { kind: yamlResource.kind, namespace: yamlResource.namespace || '-', name: yamlResource.name }) : ''}
            </DialogDescription>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto p-5">
            {resourceYAML.isFetching
              ? (
                  <EmptyState
                    title={t('common.loading')}
                    description={t('clustersPage.resourceYamlLoading')}
                  />
                )
              : (
                  <CodeEditor
                    height="32rem"
                    language="yaml"
                    readOnly
                    value={resourceYAML.data?.yaml ?? ''}
                    onChange={() => {}}
                  />
                )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function ClusterResourcesPanel({ items, loading, pagination, selectedCluster, selectedResourceKeys, tab, user, onDeleteResource, onOpenEvents, onOpenYAML, onSelectionChange }: {
  items: ClusterResource[]
  loading: boolean
  pagination?: ClusterResourcePagination
  selectedCluster?: RuntimeCluster
  selectedResourceKeys: string[]
  tab: string
  user?: CurrentUser
  onDeleteResource: (resource: ClusterResource) => void
  onOpenEvents: (resource: ClusterResource) => void
  onOpenYAML: (resource: ClusterResource) => void
  onSelectionChange: (keys: string[]) => void
}) {
  const { t } = useTranslation()
  const [expandedResourceKeys, setExpandedResourceKeys] = useState<string[]>([])
  const expandedResourceKeySet = useMemo(() => new Set(expandedResourceKeys), [expandedResourceKeys])
  const rowItems = useMemo<ClusterResourceRow[]>(() => {
    if (tab !== 'workloads')
      return items
    return items.flatMap((item) => {
      const children = item.children ?? []
      const parent: ClusterResourceRow = { ...item, hasChildren: children.length > 0 }
      if (!expandedResourceKeySet.has(item.id))
        return [parent]
      return [
        parent,
        ...children.map(child => ({ ...child, depth: 1, parentId: item.id })),
      ]
    })
  }, [expandedResourceKeySet, items, tab])
  useEffect(() => {
    if (tab !== 'workloads')
      return
    const validKeys = new Set(items.map(item => item.id))
    setExpandedResourceKeys(keys => keys.filter(key => validKeys.has(key)))
  }, [items, tab])
  const itemKeys = new Set(rowItems.map(item => item.id))
  const visibleSelectedResourceKeys = selectedResourceKeys.filter(key => itemKeys.has(key))
  const selectedResources = rowItems.filter(item => visibleSelectedResourceKeys.includes(item.id) && canDeleteClusterResource(user, item) && !item.parentId)
  const toggleResourceExpansion = (resource: ClusterResourceRow) => {
    setExpandedResourceKeys((keys) => {
      if (keys.includes(resource.id))
        return keys.filter(key => key !== resource.id)
      return [...keys, resource.id]
    })
  }
  if (!selectedCluster) {
    return (
      <EmptyState
        title={t('clustersPage.noManageableClusterTitle')}
        description={t('clustersPage.noManageableClusterDescription')}
      />
    )
  }
  return (
    <DataList
      columns={[
        { key: 'kind', header: t('clustersPage.resourceKind'), className: 'w-32 whitespace-nowrap', render: item => item.kind },
        {
          key: 'name',
          header: t('common.name'),
          className: 'min-w-56 whitespace-nowrap',
          render: item => (
            <div className="flex min-w-0 items-center gap-2">
              {tab === 'workloads' && !item.parentId && (
                item.hasChildren
                  ? (
                      <button
                        aria-label={expandedResourceKeySet.has(item.id) ? t('clustersPage.collapseWorkloadPods') : t('clustersPage.expandWorkloadPods')}
                        className="inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground transition hover:bg-muted hover:text-foreground"
                        type="button"
                        onClick={() => toggleResourceExpansion(item)}
                      >
                        {expandedResourceKeySet.has(item.id) ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
                      </button>
                    )
                  : <span className="size-6 shrink-0" />
              )}
              {tab === 'workloads' && item.parentId && (
                <span className="ml-8 flex h-6 w-9 shrink-0 items-center border-l border-border">
                  <span className="h-px w-full border-t border-border" />
                </span>
              )}
              <TruncatedResourceText className={`${item.parentId ? 'max-w-64 text-muted-foreground' : 'max-w-72'} font-mono text-sm`} value={clusterResourceName(item, tab === 'namespaces')} />
            </div>
          ),
        },
        ...(tab === 'namespaces'
          ? []
          : [{ key: 'namespace', header: t('deploymentsPage.namespace'), className: 'w-44 whitespace-nowrap', render: (item: ClusterResource) => <TruncatedResourceText className="max-w-44 font-mono text-sm" value={item.namespace || '-'} /> }]),
        { key: 'status', header: t('common.status'), className: 'w-28 whitespace-nowrap', render: item => <StatusValueBadge value={normalizeClusterResourceStatus(item.status)} /> },
        { key: 'updatedAt', header: t('clustersPage.resourceUpdatedAt'), className: 'w-32 whitespace-nowrap', render: item => formatSmartDateTime(item.updatedAt || item.createdAt, t) },
        { key: 'owner', header: t('clustersPage.resourceOwner'), className: 'min-w-56', render: item => <TruncatedResourceText className="max-w-72 text-sm" value={clusterResourceOwner(item)} /> },
        { key: 'summary', header: t('clustersPage.resourceSummary'), className: 'min-w-64', render: item => <TruncatedResourceText className="max-w-80 text-sm text-muted-foreground" value={item.summary || '-'} /> },
        {
          key: 'actions',
          header: t('common.actions'),
          className: 'w-52 min-w-52 whitespace-nowrap text-right',
          sticky: 'right',
          render: item => (
            <div className="flex justify-end gap-2">
              <Button size="sm" variant="ghost" onClick={() => onOpenEvents(item)}>
                <ScrollText className="size-4" />
                {t('clustersPage.viewEvents')}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => onOpenYAML(item)}>
                <FileCode2 className="size-4" />
                {t('clustersPage.viewYaml')}
              </Button>
              {canDeleteClusterResource(user, item) && (
                <Button size="sm" variant="ghost" onClick={() => onDeleteResource(item)}>
                  <Trash2 className="size-4" />
                  {t('common.delete')}
                </Button>
              )}
            </div>
          ),
        },
      ]}
      emptyDescription={loading ? t('common.loading') : t('clustersPage.resourceEmptyDescription')}
      emptyTitle={loading ? t('common.loading') : t(`clustersPage.${tab}EmptyTitle`)}
      items={rowItems}
      pagination={pagination}
      rowKey={item => item.id}
      selection={{
        isRowSelectable: item => canDeleteClusterResource(user, item) && !item.parentId,
        selectAllLabel: t('clustersPage.selectAllResources'),
        selectedKeys: visibleSelectedResourceKeys,
        selectedLabel: t('clustersPage.selectedResources', { count: selectedResources.length }),
        selectRowLabel: item => t('clustersPage.selectResource', { name: clusterResourceDisplayName(item) }),
        onSelectionChange,
      }}
    />
  )
}

function TruncatedResourceText({ className = 'max-w-56', value }: { className?: string, value: string }) {
  const content = value || '-'
  return (
    <CopyableHoverText
      className={className}
      display={content}
      value={content === '-' ? undefined : content}
    />
  )
}

function clusterResourceDisplayName(item: ClusterResource) {
  if (item.namespace?.trim())
    return `${item.namespace}/${item.name}`
  return item.name
}

function kubeconfigContextOptionLabel(context: { cluster: string, name: string, namespace: string, server: string }) {
  const details = [context.cluster, context.server, context.namespace].filter(Boolean).join(' · ')
  return details ? `${context.name} (${details})` : context.name
}

function clusterResourceName(item: ClusterResource, includeNamespace: boolean) {
  if (includeNamespace)
    return clusterResourceDisplayName(item)
  return item.name
}

function ClusterResourceEventsList({ events, loading }: { events: ClusterResourceEvent[], loading: boolean }) {
  const { t } = useTranslation()
  if (loading) {
    return (
      <EmptyState
        title={t('common.loading')}
        description={t('clustersPage.resourceEventsLoading')}
      />
    )
  }
  if (events.length === 0) {
    return (
      <EmptyState
        title={t('clustersPage.resourceEventsEmptyTitle')}
        description={t('clustersPage.resourceEventsEmptyDescription')}
      />
    )
  }
  return (
    <div className="grid gap-3">
      {events.map(event => (
        <div key={event.id} className="rounded-md border border-border bg-surface-subtle p-3">
          <div className="flex flex-wrap items-center gap-2">
            <StatusValueBadge value={event.type || 'normal'} />
            <span className="font-medium text-foreground">{event.reason || t('common.none')}</span>
            <span className="text-xs text-muted-foreground">{formatSmartDateTime(event.lastSeen, t)}</span>
            {event.count > 1 && <span className="text-xs text-muted-foreground">{t('clustersPage.resourceEventCount', { count: event.count })}</span>}
          </div>
          <p className="mt-2 break-words text-sm text-foreground">{event.message || '-'}</p>
          <div className="mt-2 text-xs text-muted-foreground">
            {event.source || t('common.none')}
          </div>
        </div>
      ))}
    </div>
  )
}

function clusterResourceOwner(item: ClusterResource) {
  const project = item.projectName?.trim() || item.projectId?.trim()
  const application = item.applicationName?.trim() || item.applicationId?.trim()
  const deploymentTarget = item.deploymentTargetName?.trim() || item.deploymentTargetId?.trim()
  return [project, application, deploymentTarget].filter(Boolean).join(' / ') || '-'
}

function normalizeClusterResourceStatus(status: string) {
  const value = status.toLowerCase()
  if (value === 'running' || value === 'ready' || value === 'active' || value === 'bound')
    return 'ready'
  if (value === 'failed' || value === 'pending')
    return value
  return status || 'unknown'
}

function canDeleteClusterResource(user: CurrentUser | undefined, item: ClusterResource) {
  return user?.role === 'platform_admin' || Boolean(item.projectId?.trim())
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

function normalizeFormPort(value: number, fallback: number) {
  return Number.isFinite(value) && value >= 1 && value <= 65535 ? Math.floor(value) : fallback
}

function gatewayPublicPortSummary(cluster: RuntimeCluster) {
  const scheme = cluster.gatewayPublicScheme || 'http'
  const port = cluster.gatewayPublicPort || defaultGatewayPublicPort(scheme)
  return `${scheme}:${port}`
}

function defaultGatewayPublicPort(scheme: RuntimeCluster['gatewayPublicScheme']) {
  return scheme === 'https' ? 443 : 80
}

function clusterTypeLabel(type: RuntimeCluster['type'], t: (key: string, options?: Record<string, unknown>) => string) {
  if (type === 'k3s')
    return t('deploymentsPage.typeKubernetes')
  return t(`deploymentsPage.typeLabels.${type}`, { defaultValue: type })
}

function scopeLabel(cluster: RuntimeCluster, projectMap: Record<string, { name: string }>, t: (key: string, options?: Record<string, unknown>) => string) {
  if (cluster.scope === 'project') {
    return (
      <div className="flex flex-wrap gap-2">
        {(cluster.projectIds ?? []).map(projectId => (
          <StatusBadge key={projectId}>{projectMap[projectId]?.name ?? projectId}</StatusBadge>
        ))}
      </div>
    )
  }
  if (cluster.scope === 'user')
    return t('codeRepositoriesView.scopeUser')
  return t('codeRepositoriesView.scopeGlobal')
}
