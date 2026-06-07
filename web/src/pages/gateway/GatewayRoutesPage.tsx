import type { GatewayRoute } from '@/api/client'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, SearchCheck, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api/client'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { FormField as Field } from '@/components/common/form-field'
import { ProjectSpaceSelect } from '@/components/common/project-space-select'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'

type RouteForm = Omit<GatewayRoute, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'cnameName' | 'cnameTarget'> & { applicationSlug?: string, stage?: string }

const routeDefaults: RouteForm = {
  applicationId: '',
  applicationSlug: '',
  certificateStatus: 'disabled',
  dnsStatus: 'pending',
  environmentId: '',
  host: '',
  isDefault: false,
  path: '/',
  servicePort: 8080,
  stage: 'dev',
  status: 'pending',
  tlsMode: 'http-only',
}

export function GatewayRoutesPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selectedProjectId, setSelectedProjectId] = useState('')
  const [routeDialogOpen, setRouteDialogOpen] = useState(false)
  const [editingRoute, setEditingRoute] = useState<GatewayRoute | null>(null)
  const [routeToDelete, setRouteToDelete] = useState<GatewayRoute | null>(null)
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects })
  const applications = useQuery({ queryKey: ['applications', selectedProjectId], queryFn: () => api.listApplications(selectedProjectId), enabled: Boolean(selectedProjectId) })
  const environments = useQuery({ queryKey: ['environments', selectedProjectId], queryFn: () => api.listEnvironments(selectedProjectId), enabled: Boolean(selectedProjectId) })
  const routes = useQuery({ queryKey: ['gateway-routes', selectedProjectId], queryFn: () => api.listGatewayRoutes(selectedProjectId), enabled: Boolean(selectedProjectId) })
  const form = useForm<RouteForm>({ defaultValues: routeDefaults, mode: 'onChange' })
  const selectedApplicationId = form.watch('applicationId')

  const saveRoute = useMutation({
    mutationFn: (values: RouteForm) => {
      const app = (applications.data ?? []).find(item => item.id === values.applicationId)
      const payload = { ...values, applicationSlug: app?.slug ?? values.applicationSlug ?? '' }
      return editingRoute ? api.updateGatewayRoute(selectedProjectId, editingRoute.id, payload) : api.createGatewayRoute(selectedProjectId, payload)
    },
    onSuccess: () => {
      toast.success(t(editingRoute ? 'gatewayRoutesPage.routeUpdated' : 'gatewayRoutesPage.routeCreated'))
      setRouteDialogOpen(false)
      setEditingRoute(null)
      form.reset(routeDefaults)
      queryClient.invalidateQueries({ queryKey: ['gateway-routes', selectedProjectId] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteRoute = useMutation({
    mutationFn: (routeId: string) => api.deleteGatewayRoute(selectedProjectId, routeId),
    onSuccess: () => {
      toast.success(t('gatewayRoutesPage.routeDeleted'))
      setRouteToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['gateway-routes', selectedProjectId] })
    },
    onError: error => toast.error(error.message),
  })
  const checkDomain = useMutation({
    mutationFn: (host: string) => api.checkGatewayDomain(selectedProjectId, host),
    onSuccess: result => toast.success(result.available ? t('gatewayRoutesPage.domainAvailable') : t('gatewayRoutesPage.domainUnavailable')),
    onError: error => toast.error(error.message),
  })

  function openRouteDialog(route?: GatewayRoute) {
    setEditingRoute(route ?? null)
    form.reset(route ? { ...route, applicationSlug: '', stage: 'dev' } : routeDefaults)
    setRouteDialogOpen(true)
  }

  return (
    <div className="grid gap-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <ProjectSpaceSelect projects={projects.data ?? []} value={selectedProjectId} onChange={setSelectedProjectId} />
        <Button disabled={!selectedProjectId} onClick={() => openRouteDialog()}>
          <Plus className="size-4" />
          {t('gatewayRoutesPage.createRoute')}
        </Button>
      </div>
      <DataList
        columns={[
          { key: 'host', header: t('gatewayRoutesPage.host'), render: item => item.host },
          { key: 'path', header: t('gatewayRoutesPage.path'), render: item => item.path },
          { key: 'tls', header: t('gatewayRoutesPage.tlsMode'), render: item => item.tlsMode },
          { key: 'cname', header: t('gatewayRoutesPage.cnameTarget'), render: item => item.cnameTarget || '-' },
          { key: 'status', header: t('common.status'), render: item => <StatusValueBadge value={item.status} /> },
          { key: 'actions', header: t('common.actions'), className: 'text-right whitespace-nowrap', render: item => (
            <div className="flex justify-end gap-2">
              <Button size="sm" variant="ghost" onClick={() => checkDomain.mutate(item.host)}>
                <SearchCheck className="size-4" />
                {t('gatewayRoutesPage.checkDomain')}
              </Button>
              <EditActionButton label={t('common.edit')} onClick={() => openRouteDialog(item)} />
              <Button size="sm" variant="ghost" onClick={() => setRouteToDelete(item)}>
                <Trash2 className="size-4" />
                {t('common.delete')}
              </Button>
            </div>
          ) },
        ]}
        emptyTitle={selectedProjectId ? t('gatewayRoutesPage.emptyRoutes') : t('buildsPage.selectProject')}
        items={routes.data ?? []}
        rowKey={item => item.id}
      />

      <Dialog open={routeDialogOpen} onOpenChange={setRouteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingRoute ? t('gatewayRoutesPage.editRoute') : t('gatewayRoutesPage.createRoute')}</DialogTitle>
            <DialogDescription>{t('gatewayRoutesPage.routeDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => saveRoute.mutate(values))}>
            <Field label={t('apps.title')} required>
              <Select {...form.register('applicationId', { required: true })}>
                <option value="">{t('common.select')}</option>
                {(applications.data ?? []).map(app => <option key={app.id} value={app.id}>{app.name}</option>)}
              </Select>
            </Field>
            <Field label={t('deploymentsPage.environment')}>
              <Select {...form.register('environmentId')}>
                <option value="">{t('common.none')}</option>
                {(environments.data ?? []).map(environment => <option key={environment.id} value={environment.id}>{environment.name}</option>)}
              </Select>
            </Field>
            <Field hint={t('gatewayRoutesPage.hostHint')} label={t('gatewayRoutesPage.host')}><Input {...form.register('host')} /></Field>
            <Field label={t('deploymentsPage.stage')}>
              <Select {...form.register('stage')}>
                <option value="dev">{t('deploymentsPage.stageDev')}</option>
                <option value="test">{t('deploymentsPage.stageTest')}</option>
                <option value="staging">{t('deploymentsPage.stageStaging')}</option>
                <option value="prod">{t('deploymentsPage.stageProd')}</option>
              </Select>
            </Field>
            <Field label={t('gatewayRoutesPage.path')}><Input {...form.register('path')} /></Field>
            <Field label={t('gatewayRoutesPage.servicePort')}><Input {...form.register('servicePort', { valueAsNumber: true })} type="number" /></Field>
            <Field label={t('gatewayRoutesPage.tlsMode')}>
              <Select {...form.register('tlsMode')}>
                <option value="http-only">{t('gatewayRoutesPage.tlsHttpOnly')}</option>
                <option value="http-challenge">{t('gatewayRoutesPage.tlsHttpChallenge')}</option>
                <option value="manual-cert">{t('gatewayRoutesPage.tlsManualCert')}</option>
              </Select>
            </Field>
            <DialogFooter><Button disabled={!selectedProjectId || !selectedApplicationId || saveRoute.isPending} type="submit">{t('common.save')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('common.delete')}
        description={t('gatewayRoutesPage.deleteRouteDescription')}
        open={Boolean(routeToDelete)}
        title={t('gatewayRoutesPage.deleteRouteTitle')}

        onConfirm={() => routeToDelete && deleteRoute.mutate(routeToDelete.id)}
        onOpenChange={open => !open && setRouteToDelete(null)}
      />
    </div>
  )
}
