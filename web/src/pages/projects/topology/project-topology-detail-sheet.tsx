import type { ReactNode } from 'react'
import type {
  ProjectTopologyEdge,
  ProjectTopologyManualEdge,
  ProjectTopologyNode,
  ServiceBinding,
  ServiceBindingCheckResult,
} from '@/api'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Pencil, ScanSearch, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { projectTopologyKeys } from './project-topology-query'
import { serviceBindingEnvSummary } from './project-topology-utils'

interface ProjectTopologyDetailSheetProps {
  canManage: boolean
  edge?: ProjectTopologyEdge
  manualEdge?: ProjectTopologyManualEdge
  nodes: ProjectTopologyNode[]
  projectId: string
  serviceBinding?: ServiceBinding
  onEdit: () => void
  onOpenChange: (open: boolean) => void
}

export function ProjectTopologyDetailSheet({
  canManage,
  edge,
  manualEdge,
  nodes,
  projectId,
  serviceBinding,
  onEdit,
  onOpenChange,
}: ProjectTopologyDetailSheetProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [diagnostics, setDiagnostics] = useState<ServiceBindingCheckResult | null>(null)
  const sourceNode = nodes.find(node => node.id === edge?.source)
  const targetNode = nodes.find(node => node.id === edge?.target)
  const sourceTarget = sourceNode?.deploymentTargets.find(target => target.id === edge?.sourceDeploymentTargetId)
  const targetTarget = targetNode?.deploymentTargets.find(target => target.id === edge?.targetDeploymentTargetId)
  const persistedRelation = serviceBinding ?? manualEdge

  const checkBinding = useMutation({
    mutationFn: () => api.checkServiceBinding(projectId, edge?.id ?? ''),
    onSuccess: (result) => {
      setDiagnostics(result)
      toast.success(t('projectTopology.checkCompleted'))
      void queryClient.invalidateQueries({ queryKey: projectTopologyKeys.all(projectId) })
    },
    onError: error => toast.error(error.message || t('projectTopology.checkFailed')),
  })
  const deleteRelation = useMutation({
    mutationFn: async () => {
      if (!edge)
        return false
      if (edge.origin === 'service_binding')
        return (await api.deleteServiceBinding(projectId, edge.id)).requiresRedeploy
      await api.deleteProjectTopologyEdge(projectId, edge.id)
      return false
    },
    onSuccess: (requiresRedeploy) => {
      toast.success(t('projectTopology.deleted'))
      if (requiresRedeploy)
        toast.warning(t('projectTopology.deletedNeedsRelease'))
      void queryClient.invalidateQueries({ queryKey: projectTopologyKeys.all(projectId) })
      setDeleteOpen(false)
      onOpenChange(false)
    },
    onError: error => toast.error(error.message),
  })

  return (
    <>
      <Sheet open={Boolean(edge)} onOpenChange={onOpenChange}>
        <SheetContent className="w-[min(92vw,30rem)] overflow-y-auto sm:max-w-lg">
          {edge && (
            <>
              <SheetHeader>
                <SheetTitle>{t('projectTopology.details')}</SheetTitle>
                <SheetDescription>{t('projectTopology.detailDescription')}</SheetDescription>
              </SheetHeader>
              <div className="grid gap-5 px-4 pb-5 text-sm">
                <div className="grid gap-3 rounded-md border border-border bg-muted/25 p-3">
                  <ApplicationLink label={t('projectTopology.source')} name={sourceNode?.name ?? t('projectTopology.unknownApplication')} projectId={projectId} applicationId={sourceNode?.id} />
                  <div className="pl-3 text-muted-foreground">↓</div>
                  <ApplicationLink label={t('projectTopology.target')} name={targetNode?.name ?? t('projectTopology.unknownApplication')} projectId={projectId} applicationId={targetNode?.id} />
                </div>

                <div className="grid grid-cols-2 gap-4">
                  <Detail label={t('projectTopology.origin')}>
                    <StatusValueBadge labelKeyPrefix="projectTopology.origins" value={edge.origin} />
                  </Detail>
                  <Detail label={t('projectTopology.relationStatus')}>
                    <StatusValueBadge labelKeyPrefix="projectTopology.statuses" value={edge.status || 'unknown'} />
                  </Detail>
                  <Detail label={t('projectTopology.form.relationType')}>{t(`projectTopology.relationTypes.${edge.relationType}`)}</Detail>
                  <Detail label={t('projectTopology.protocolAndPort')}>{protocolSummary(edge.protocol, edge.port)}</Detail>
                </div>

                <Separator />
                <Detail label={t('projectTopology.form.sourceTarget')}>{targetSummary(sourceTarget)}</Detail>
                <Detail label={t('projectTopology.form.targetTarget')}>{targetSummary(targetTarget)}</Detail>
                {serviceBinding && (
                  <>
                    <Detail label={t('projectTopology.form.targetPort')}>{`${serviceBinding.targetPortName} · ${serviceBinding.targetPort}`}</Detail>
                    <Detail label={t('projectTopology.injection')}>{t(serviceBinding.injectionMode === 'url' ? 'projectTopology.form.urlMode' : 'projectTopology.form.hostPortMode')}</Detail>
                    <Detail label={t('projectTopology.environmentVariables')}><code className="break-all text-xs">{serviceBindingEnvSummary(serviceBinding)}</code></Detail>
                  </>
                )}
                {manualEdge?.description && <Detail label={t('projectTopology.description')}>{manualEdge.description}</Detail>}
                {persistedRelation && (
                  <div className="grid grid-cols-2 gap-4">
                    <Detail label={t('projectTopology.createdAt')}>{formatSmartDateTime(persistedRelation.createdAt, t)}</Detail>
                    <Detail label={t('projectTopology.updatedAt')}>{formatSmartDateTime(persistedRelation.updatedAt, t)}</Detail>
                  </div>
                )}

                {edge.origin === 'service_binding' && (
                  <div className="grid gap-3">
                    <div className="flex items-center justify-between gap-3">
                      <h3 className="font-semibold">{t('projectTopology.diagnostics')}</h3>
                      <Button disabled={checkBinding.isPending} size="sm" variant="outline" onClick={() => checkBinding.mutate()}>
                        <ScanSearch className={checkBinding.isPending ? 'size-4 animate-pulse' : 'size-4'} />
                        {t(checkBinding.isPending ? 'projectTopology.checking' : 'projectTopology.check')}
                      </Button>
                    </div>
                    {diagnostics
                      ? diagnostics.checks.map(item => (
                          <div key={item.code} className="flex items-start justify-between gap-3 rounded-md border border-border px-3 py-2">
                            <div className="min-w-0">
                              <p className="font-medium">{t(`projectTopology.checkCodes.${item.code}`, { defaultValue: item.code })}</p>
                              {(item.resource || item.detail) && <p className="mt-1 break-words text-xs text-muted-foreground">{item.resource || item.detail}</p>}
                            </div>
                            <StatusValueBadge labelKeyPrefix="projectTopology.checkStatuses" value={item.status} />
                          </div>
                        ))
                      : <p className="rounded-md border border-dashed border-border px-3 py-4 text-muted-foreground">{t('projectTopology.noDiagnostics')}</p>}
                  </div>
                )}

                {canManage && (
                  <div className="flex flex-wrap justify-end gap-2 border-t border-border pt-4">
                    {persistedRelation && (
                      <Button variant="outline" onClick={onEdit}>
                        <Pencil className="size-4" />
                        {t('projectTopology.edit')}
                      </Button>
                    )}
                    <Button variant="destructive" onClick={() => setDeleteOpen(true)}>
                      <Trash2 className="size-4" />
                      {t('projectTopology.delete')}
                    </Button>
                  </div>
                )}
              </div>
            </>
          )}
        </SheetContent>
      </Sheet>
      <ConfirmDialog
        confirmText={t('projectTopology.deleteConfirm')}
        description={t('projectTopology.deleteDescription', {
          source: sourceNode?.name ?? t('projectTopology.unknownApplication'),
          target: targetNode?.name ?? t('projectTopology.unknownApplication'),
        })}
        open={deleteOpen}
        pending={deleteRelation.isPending}
        title={t('projectTopology.deleteTitle')}
        onConfirm={async () => {
          await deleteRelation.mutateAsync()
        }}
        onOpenChange={setDeleteOpen}
      />
    </>
  )
}

function ApplicationLink({ applicationId, label, name, projectId }: { applicationId?: string, label: string, name: string, projectId: string }) {
  return (
    <div className="min-w-0">
      <p className="text-xs text-muted-foreground">{label}</p>
      {applicationId
        ? <Link className="mt-1 block truncate font-medium text-primary-text hover:underline" to={`/projects/${projectId}/apps/${applicationId}`}>{name}</Link>
        : <p className="mt-1 truncate font-medium">{name}</p>}
    </div>
  )
}

function Detail({ children, label }: { children: ReactNode, label: string }) {
  return (
    <div className="min-w-0">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="mt-1 min-w-0 break-words font-medium">{children}</div>
    </div>
  )
}

function targetSummary(target?: { name: string, stage: string }) {
  return target ? `${target.name} · ${target.stage}` : '—'
}

function protocolSummary(protocol?: string, port?: number) {
  if (!protocol && !port)
    return '—'
  return [protocol?.toUpperCase(), port].filter(Boolean).join(' · ')
}
