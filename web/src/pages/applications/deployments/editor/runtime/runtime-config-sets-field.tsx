import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { DeploymentTarget, ProjectRuntimeConfigSet, ProjectRuntimeConfigSetPayload, Release, RuntimeConfigRefMode } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { FileCode2, Pencil, Rocket, RotateCw } from 'lucide-react'
import { useState } from 'react'
import { useForm, useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { HelpTip } from '@/components/common/help-tip'
import { StatusBadge } from '@/components/common/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { publicRuntimeEnvironmentInputs, publicRuntimeEnvironmentRecord } from '@/lib/runtime-environment'
import { deploymentReleaseKey } from '@/pages/applications/application-config-utils'
import { DeferredRuntimeConfigSetDialog } from '@/pages/applications/deployments/application-deployment-dialogs'
import { normalizeRuntimeConfigPayload, redeployReleasePayload, runtimeConfigDefaults, runtimeConfigLiveSetIds } from '@/pages/applications/deployments/application-deployments-panel-utils'

/** 项目空间公共配置引用：勾选、引用策略、就地新建/编辑，以及配置变更后的批量重新部署提示。 */
export function RuntimeConfigSetsField({ canManageSecrets, deploymentTargets, projectId, releases }: {
  canManageSecrets: boolean
  deploymentTargets: DeploymentTarget[]
  projectId: string
  releases: Release[]
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { control, setValue } = useFormContext<DeploymentTargetFormValues>()
  const selectedRefs = useWatch({ control, name: 'runtimeConfigRefs' })
  const sets = useQuery({ queryKey: ['runtime-config-sets', projectId], queryFn: () => api.listProjectRuntimeConfigSets(projectId), enabled: Boolean(projectId) })
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingSet, setEditingSet] = useState<ProjectRuntimeConfigSet | null>(null)
  const [configFilesValid, setConfigFilesValid] = useState(true)
  const [secretFilesValid, setSecretFilesValid] = useState(true)
  const [restartSet, setRestartSet] = useState<{ affectedCount: number, setId: string } | null>(null)
  const setForm = useForm<ProjectRuntimeConfigSetPayload>({ defaultValues: runtimeConfigDefaults, mode: 'onChange' })
  const selectedById = new Map(selectedRefs.map(ref => [ref.setId, ref]))
  const latestReleaseByTarget = latestReleases(releases)
  const restartTargets = restartSet
    ? deploymentTargets.filter(target => runtimeConfigLiveSetIds(target.runtimeConfigRefs ?? []).includes(restartSet.setId))
    : []
  const redeployableTargets = restartTargets.filter(target => Boolean(redeployReleasePayload(target, latestReleaseByTarget[deploymentReleaseKey(target.id)])))

  const updateRefs = (next: DeploymentTargetFormValues['runtimeConfigRefs']) => setValue('runtimeConfigRefs', next, { shouldDirty: true, shouldValidate: true })
  const toggleSet = (setId: string, checked: boolean) => updateRefs(checked
    ? [...selectedRefs.filter(ref => ref.setId !== setId), { mode: 'live', setId }]
    : selectedRefs.filter(ref => ref.setId !== setId))
  const changeMode = (setId: string, mode: RuntimeConfigRefMode) => updateRefs(selectedRefs.map(ref => ref.setId === setId ? { ...ref, mode } : ref))

  const openDialog = (set?: ProjectRuntimeConfigSet) => {
    setEditingSet(set ?? null)
    setConfigFilesValid(true)
    setSecretFilesValid(true)
    setForm.reset(set
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
  const closeDialog = () => {
    setDialogOpen(false)
    setEditingSet(null)
    setForm.reset(runtimeConfigDefaults)
  }

  const saveSet = useMutation({
    mutationFn: (values: ProjectRuntimeConfigSetPayload) => editingSet
      ? api.updateProjectRuntimeConfigSet(projectId, editingSet.id, normalizeRuntimeConfigPayload(values))
      : api.createProjectRuntimeConfigSet(projectId, normalizeRuntimeConfigPayload(values)),
    onSuccess: (set) => {
      toast.success(t(editingSet ? 'runtimeConfigSets.updated' : 'runtimeConfigSets.created'))
      if (!editingSet)
        toggleSet(set.id, true)
      else if ((set.affectedDeploymentTargetCount ?? 0) > 0)
        setRestartSet({ affectedCount: set.affectedDeploymentTargetCount ?? 0, setId: set.id })
      closeDialog()
      void queryClient.invalidateQueries({ queryKey: ['runtime-config-sets', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const redeployAffected = useMutation({
    mutationFn: async () => {
      let queued = 0
      let skipped = 0
      for (const target of restartTargets) {
        const releasePayload = redeployReleasePayload(target, latestReleaseByTarget[deploymentReleaseKey(target.id)])
        if (!releasePayload) {
          skipped++
          continue
        }
        await api.createRelease(projectId, releasePayload)
        queued++
      }
      return { queued, skipped }
    },
    onSuccess: ({ queued, skipped }) => {
      toast.success(t('deploymentsPage.runtimeConfigRedeployQueued', { queued, skipped }))
      setRestartSet(null)
      void queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  return (
    <div className="grid min-w-0 gap-3 border-t border-border/70 pt-3">
      <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
        <h4 className="inline-flex min-w-0 items-center gap-1.5 text-sm font-medium text-foreground">
          {t('deploymentsPage.runtimeConfigSets')}
          <HelpTip label={t('deploymentsPage.runtimeConfigSets')}>{t('deploymentsPage.runtimeConfigSetsHint')}</HelpTip>
        </h4>
        <Button size="sm" type="button" variant="outline" onClick={() => openDialog()}>
          <FileCode2 className="size-4" />
          {t('runtimeConfigSets.createTitle')}
        </Button>
      </div>
      {sets.isLoading && <Skeleton className="h-12 w-full rounded-control" />}
      {sets.isError && (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-control border border-danger-border bg-danger-subtle px-3 py-2 text-xs text-danger" role="alert">
          <span>{t('deploymentTargetEditor.runtimeConfigSetsLoadFailed')}</span>
          <Button disabled={sets.isFetching} size="sm" type="button" variant="outline" onClick={() => void sets.refetch()}>
            <RotateCw className="size-3.5" />
            {t('common.retry')}
          </Button>
        </div>
      )}
      {sets.data && sets.data.length === 0 && <p className="text-sm text-muted-foreground">{t('deploymentsPage.emptyRuntimeConfigSets')}</p>}
      {sets.data && sets.data.length > 0 && (
        <ul className="grid gap-1 rounded-control border border-border p-1">
          {sets.data.map((set) => {
            const selectedRef = selectedById.get(set.id)
            const checkboxId = `runtime-config-set-${set.id}`
            return (
              <li key={set.id} className="flex flex-wrap items-center justify-between gap-2 rounded-control px-3 py-2 hover:bg-surface-subtle">
                <label className="flex min-w-0 flex-1 items-center gap-3" htmlFor={checkboxId}>
                  <Checkbox checked={Boolean(selectedRef)} disabled={!set.enabled && !selectedRef} id={checkboxId} onCheckedChange={checked => toggleSet(set.id, checked === true)} />
                  <span className="min-w-0 truncate font-mono text-sm font-medium" title={set.name}>{set.name}</span>
                  {!set.enabled && <StatusBadge tone="neutral">{t('common.disabled')}</StatusBadge>}
                </label>
                <div className="flex shrink-0 items-center gap-1.5">
                  {selectedRef && (
                    <Select
                      value={selectedRef.mode}
                      onValueChange={value => changeMode(set.id, value === 'snapshot' ? 'snapshot' : 'live')}
                    >
                      <SelectTrigger
                        aria-label={t('deploymentTargetEditor.runtimeConfigRefModeFor', { name: set.name })}
                        className="w-auto shrink-0 text-xs"
                        size="sm"
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="live">{t('deploymentsPage.runtimeConfigRefModes.live')}</SelectItem>
                        <SelectItem value="snapshot">{t('deploymentsPage.runtimeConfigRefModes.snapshot')}</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                  <Button aria-label={t('deploymentTargetEditor.editRuntimeConfigSet', { name: set.name })} size="icon" type="button" variant="ghost" onClick={() => openDialog(set)}>
                    <Pencil className="size-4" />
                  </Button>
                </div>
              </li>
            )
          })}
        </ul>
      )}
      {restartSet && (
        <Alert variant="warning">
          <Rocket />
          <AlertTitle>{t('deploymentsPage.runtimeConfigSetChangedTitle')}</AlertTitle>
          <AlertDescription>
            <span>{t('deploymentsPage.runtimeConfigSetChangedDescription', { count: restartSet.affectedCount, redeployable: redeployableTargets.length })}</span>
            <span className="flex flex-wrap gap-2 pt-1">
              <Button disabled={redeployableTargets.length === 0 || redeployAffected.isPending} size="sm" type="button" variant="outline" onClick={() => redeployAffected.mutate()}>
                <Rocket className="size-4" />
                {t('deploymentsPage.redeployAffectedRuntimeConfig')}
              </Button>
              <Button size="sm" type="button" variant="ghost" onClick={() => setRestartSet(null)}>
                {t('common.close')}
              </Button>
            </span>
          </AlertDescription>
        </Alert>
      )}
      <DeferredRuntimeConfigSetDialog
        canManageSecrets={canManageSecrets}
        configFilesValid={configFilesValid}
        editingSet={editingSet}
        form={setForm}
        open={dialogOpen}
        pending={saveSet.isPending}
        projectId={projectId}
        secretFilesValid={secretFilesValid}
        onConfigFilesValidityChange={setConfigFilesValid}
        onOpenChange={open => open ? setDialogOpen(true) : closeDialog()}
        onSecretFilesValidityChange={setSecretFilesValid}
        onSubmit={values => saveSet.mutate(values)}
      />
    </div>
  )
}

function latestReleases(releases: Release[]) {
  const output: Record<string, Release> = {}
  for (const release of releases) {
    const key = deploymentReleaseKey(release.deploymentTargetId)
    const existing = output[key]
    if (!existing || new Date(release.createdAt).getTime() > new Date(existing.createdAt).getTime())
      output[key] = release
  }
  return output
}
