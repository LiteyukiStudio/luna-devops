import type { DeploymentTarget, GatewayRoute } from '@/api'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { SearchCheck, Trash2 } from 'lucide-react'
import { useImperativeHandle, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { GatewayRouteFormFields } from '@/components/common/gateway-route-form-fields'
import { HoverText } from '@/components/common/hover-text'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { gatewayDeploymentTargetLabel } from './application-config-utils'

type RouteForm = Omit<GatewayRoute, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'cnameName' | 'cnameTarget' | 'accessUrl' | 'deleteStatus' | 'deleteMessage' | 'deleteStartedAt' | 'deleteFinishedAt'>

const routeDefaults: RouteForm = { applicationId: '', certificateStatus: 'disabled', deploymentTargetId: '', dnsStatus: 'pending', enabled: true, environmentId: '', host: '', isDefault: false, path: '/', servicePort: 8080, status: 'pending', tlsMode: 'http-only' }

export interface ApplicationGatewayPanelHandle {
  openCreateDialog: (environmentId?: string, deploymentTargetId?: string) => void
}
export function ApplicationGatewayPanel({ applicationId, deploymentTargets, projectId, ref, routes }: {
  applicationId: string
  deploymentTargets: DeploymentTarget[]
  projectId: string
  ref?: React.Ref<ApplicationGatewayPanelHandle>
  routes: GatewayRoute[]
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingRoute, setEditingRoute] = useState<GatewayRoute | null>(null)
  const [routeToDelete, setRouteToDelete] = useState<GatewayRoute | null>(null)
  const form = useForm<RouteForm>({ defaultValues: routeDefaults, mode: 'onChange' })
  const deploymentTargetOptions = useMemo(() => deploymentTargets.map(target => ({
    id: target.id,
    label: gatewayDeploymentTargetLabel(target, t),
  })), [deploymentTargets, t])
  const saveRoute = useMutation({
    mutationFn: (values: RouteForm) => {
      const target = deploymentTargets.find(item => item.id === values.deploymentTargetId)
      const payload = {
        ...values,
        applicationId,
        environmentId: target?.environmentId ?? values.environmentId,
        servicePort: values.servicePort || target?.servicePort || 8080,
      }
      return editingRoute ? api.updateGatewayRoute(projectId, editingRoute.id, payload) : api.createGatewayRoute(projectId, payload)
    },
    onSuccess: () => {
      toast.success(t(editingRoute ? 'gatewayRoutesPage.routeUpdated' : 'gatewayRoutesPage.routeCreated'))
      setDialogOpen(false)
      setEditingRoute(null)
      form.reset(routeDefaults)
      queryClient.invalidateQueries({ queryKey: ['gateway-routes', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteRoute = useMutation({
    mutationFn: (routeId: string) => api.deleteGatewayRoute(projectId, routeId),
    onSuccess: () => {
      toast.success(t('gatewayRoutesPage.routeDeleted'))
      setRouteToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['gateway-routes', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const checkDomain = useMutation({
    mutationFn: ({ host, routeId }: { host: string, routeId?: string }) => api.checkGatewayDomain(projectId, host, routeId),
    onSuccess: (result) => {
      const messageKey = result.status === 'current'
        ? 'gatewayRoutesPage.domainCurrent'
        : result.available ? 'gatewayRoutesPage.domainAvailable' : 'gatewayRoutesPage.domainUnavailable'
      toast.success(t(messageKey, { host: result.host }))
    },
    onError: error => toast.error(error.message),
  })
  function openRouteDialog(route?: GatewayRoute) {
    setEditingRoute(route ?? null)
    const defaultTarget = deploymentTargets[0]
    const matchedTarget = route?.deploymentTargetId
      ? deploymentTargets.find(target => target.id === route.deploymentTargetId)
      : deploymentTargets.find(target => target.environmentId === route?.environmentId)
    form.reset(route
      ? { ...route, deploymentTargetId: route.deploymentTargetId || matchedTarget?.id || '', environmentId: matchedTarget?.environmentId ?? route.environmentId }
      : { ...routeDefaults, applicationId, deploymentTargetId: defaultTarget?.id ?? '', environmentId: defaultTarget?.environmentId ?? '', servicePort: defaultTarget?.servicePort ?? 8080 })
    setDialogOpen(true)
  }
  useImperativeHandle(ref, () => ({ openCreateDialog: () => openRouteDialog() }))
  return (
    <div className="grid gap-4">
      <DataList
        columns={[
          { key: 'host', header: t('gatewayRoutesPage.host'), render: item => <GatewayRouteSummary item={item} /> },
          { key: 'path', header: t('gatewayRoutesPage.path'), render: item => item.path },
          { key: 'tls', header: t('gatewayRoutesPage.tlsMode'), render: item => item.tlsMode },
          { key: 'status', header: t('common.status'), render: item => (
            <div className="flex flex-wrap items-center gap-2">
              <StatusValueBadge value={item.enabled ? 'enabled' : 'disabled'} />
              <StatusValueBadge value={item.status} />
            </div>
          ) },
          { key: 'actions', header: t('common.actions'), className: 'text-right whitespace-nowrap', render: (item) => {
            const deleting = item.deleteStatus === 'deleting'
            return (
              <div className="flex justify-end gap-2">
                <Button disabled={deleting} size="sm" variant="ghost" onClick={() => checkDomain.mutate({ host: item.host, routeId: item.id })}>
                  <SearchCheck className="size-4" />
                  {t('gatewayRoutesPage.checkDomain')}
                </Button>
                <EditActionButton disabled={deleting} label={t('common.edit')} onClick={() => openRouteDialog(item)} />
                <Button disabled={deleting} size="sm" variant="ghost" onClick={() => setRouteToDelete(item)}>
                  <Trash2 className="size-4" />
                  {t('common.delete')}
                </Button>
              </div>
            )
          } },
        ]}
        emptyTitle={t('gatewayRoutesPage.emptyRoutes')}
        items={routes}
        rowKey={item => item.id}
      />
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingRoute ? t('gatewayRoutesPage.editRoute') : t('gatewayRoutesPage.createRoute')}</DialogTitle>
            <DialogDescription>{t('gatewayRoutesPage.routeDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => saveRoute.mutate(values))}>
            <GatewayRouteFormFields
              deploymentTargetIdField={form.register('deploymentTargetId', {
                required: true,
                onChange: (event) => {
                  const target = deploymentTargets.find(item => item.id === event.target.value)
                  form.setValue('environmentId', target?.environmentId ?? '', { shouldDirty: true, shouldValidate: true })
                  form.setValue('servicePort', target?.servicePort ?? 8080, { shouldDirty: true, shouldValidate: true })
                },
              })}
              deploymentTargets={deploymentTargetOptions}
              enabledField={form.register('enabled')}
              hostField={form.register('host')}
              pathField={form.register('path')}
              servicePortField={form.register('servicePort', { valueAsNumber: true })}
              tlsModeField={form.register('tlsMode')}
            />
            <DialogFooter><Button disabled={!form.formState.isValid || saveRoute.isPending} type="submit">{t('common.save')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <ConfirmDialog cancelText={t('common.cancel')} confirmText={t('common.delete')} description={t('gatewayRoutesPage.deleteRouteDescription')} open={Boolean(routeToDelete)} title={t('gatewayRoutesPage.deleteRouteTitle')} onConfirm={() => routeToDelete && deleteRoute.mutate(routeToDelete.id)} onOpenChange={open => !open && setRouteToDelete(null)} />
    </div>
  )
}

function GatewayRouteSummary({ item }: { item: GatewayRoute }) {
  const deleteFailedMessage = item.deleteStatus === 'delete_failed' ? item.deleteMessage?.trim() : ''
  const displayUrl = item.accessUrl?.trim() || item.host
  return (
    <div className="min-w-0">
      <span className="block truncate" title={displayUrl}>{displayUrl}</span>
      {item.deleteStatus && item.deleteStatus !== 'active' && (
        <div className="mt-1 flex min-w-0 items-center gap-2">
          <StatusValueBadge labelKeyPrefix="apps.deleteStatuses" value={item.deleteStatus} />
          {deleteFailedMessage && (
            <HoverText className="flex-1 text-xs text-muted-foreground" value={deleteFailedMessage} />
          )}
        </div>
      )}
    </div>
  )
}
