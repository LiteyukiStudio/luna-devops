import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { HookPhase, ProjectHookConfig } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, Plus, RotateCw, Trash2 } from 'lucide-react'
import { Controller, useFieldArray, useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { SwitchField } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'
import { buildHookPhases, deploymentHookPhases } from '../deployment-target-form'

/** 钩子绑定：按阶段分组选择；已有镜像来源不会执行构建阶段，相关阶段保留但禁用。 */
export function HookBindingsFields({ projectId }: { projectId: string }) {
  const { t } = useTranslation()
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const { append, fields, move, remove } = useFieldArray({ control, name: 'buildHookBindings' })
  const [hooksEnabled, sourceType] = useWatch({ control, name: ['buildHooksEnabled', 'sourceType'] })
  const hooks = useQuery({ queryKey: ['project-hooks', projectId], queryFn: () => api.listProjectHooks(projectId), enabled: Boolean(projectId) })
  const hookItems = hooks.data ?? []
  const canEdit = hooksEnabled && hookItems.length > 0

  return (
    <div className="grid gap-3">
      <SwitchField description={t('deploymentsPage.deploymentHooksEnabledHint')} label={t('deploymentsPage.deploymentHooksEnabled')} name="buildHooksEnabled" />
      {hooks.isPending
        ? <Skeleton className="h-10 w-full rounded-control" />
        : hooks.isError
          ? (
              <Alert variant="destructive">
                <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
                  <span>{t('projectHooks.loadFailedDescription')}</span>
                  <Button disabled={hooks.isFetching} size="sm" type="button" variant="outline" onClick={() => void hooks.refetch()}>
                    <RotateCw className="size-3.5" />
                    {t('common.retry')}
                  </Button>
                </AlertDescription>
              </Alert>
            )
          : hookItems.length === 0 && fields.length === 0
            ? <p className="text-sm text-muted-foreground">{t('deploymentsPage.emptyDeploymentHooks')}</p>
            : (
                <>
                  {fields.length > 0 && (
                    <ol className="grid gap-2">
                      {fields.map((field, index) => (
                        <HookBindingRow
                          key={field.id}
                          disabled={!canEdit}
                          hooks={hookItems}
                          imageSource={sourceType === 'image'}
                          index={index}
                          isFirst={index === 0}
                          isLast={index === fields.length - 1}
                          onMove={direction => move(index, index + direction)}
                          onRemove={() => remove(index)}
                        />
                      ))}
                    </ol>
                  )}
                  <div>
                    <Button
                      disabled={!canEdit}
                      size="sm"
                      type="button"
                      variant="outline"
                      onClick={() => append({ hookConfigId: hookItems[0]?.id ?? '', phase: 'preDeployment' })}
                    >
                      <Plus className="size-4" />
                      {t('deploymentsPage.addHookBinding')}
                    </Button>
                  </div>
                </>
              )}
    </div>
  )
}

function HookBindingRow({ disabled, hooks, imageSource, index, isFirst, isLast, onMove, onRemove }: {
  disabled: boolean
  hooks: ProjectHookConfig[]
  imageSource: boolean
  index: number
  isFirst: boolean
  isLast: boolean
  onMove: (direction: -1 | 1) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const error = useFieldError(`buildHookBindings.${index}.hookConfigId`)
  const phaseLabel = (phase: HookPhase) => t(`projectHooks.phases.${phase}`)
  const order = index + 1
  const errorId = `deployment-target-hook-${index}-error`

  return (
    <li className="grid gap-2 @xl/workspace:grid-cols-[2rem_minmax(0,1fr)_minmax(0,1.2fr)_auto] @xl/workspace:items-center">
      <span aria-hidden="true" className="text-sm font-medium text-muted-foreground tabular-nums">{order}</span>
      <Controller
        control={control}
        name={`buildHookBindings.${index}.phase`}
        render={({ field }) => (
          <Select disabled={disabled} value={field.value} onValueChange={field.onChange}>
            <SelectTrigger
              ref={field.ref}
              aria-label={t('deploymentTargetEditor.hookPhaseFor', { index: order })}
              className="w-full"
              onBlur={field.onBlur}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectLabel>{t('deploymentTargetEditor.buildPhases')}</SelectLabel>
                {deploymentHookPhases.filter(phase => buildHookPhases.has(phase)).map(phase => (
                  <SelectItem key={phase} disabled={imageSource} value={phase}>{phaseLabel(phase)}</SelectItem>
                ))}
              </SelectGroup>
              <SelectGroup>
                <SelectLabel>{t('deploymentTargetEditor.deploymentPhases')}</SelectLabel>
                {deploymentHookPhases.filter(phase => !buildHookPhases.has(phase)).map(phase => (
                  <SelectItem key={phase} value={phase}>{phaseLabel(phase)}</SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        )}
      />
      <Controller
        control={control}
        name={`buildHookBindings.${index}.hookConfigId`}
        render={({ field }) => (
          <Select disabled={disabled} value={field.value || undefined} onValueChange={field.onChange}>
            <SelectTrigger
              ref={field.ref}
              aria-describedby={error ? errorId : undefined}
              aria-invalid={error ? true : undefined}
              aria-label={t('deploymentTargetEditor.hookConfigFor', { index: order })}
              className="w-full"
              onBlur={field.onBlur}
            >
              <SelectValue placeholder={t('common.select')} />
            </SelectTrigger>
            <SelectContent>
              {hooks.map(hook => <SelectItem key={hook.id} value={hook.id}>{hook.name}</SelectItem>)}
            </SelectContent>
          </Select>
        )}
      />
      <div className="flex justify-end gap-1">
        <Button aria-label={t('deploymentTargetEditor.moveHookUpAt', { index: order })} disabled={disabled || isFirst} size="icon" type="button" variant="ghost" onClick={() => onMove(-1)}>
          <ArrowUp className="size-4" />
        </Button>
        <Button aria-label={t('deploymentTargetEditor.moveHookDownAt', { index: order })} disabled={disabled || isLast} size="icon" type="button" variant="ghost" onClick={() => onMove(1)}>
          <ArrowDown className="size-4" />
        </Button>
        <Button aria-label={t('deploymentTargetEditor.removeHookAt', { index: order })} size="icon" type="button" variant="ghost" onClick={onRemove}>
          <Trash2 className="size-4" />
        </Button>
      </div>
      {error && <p className="text-xs text-danger @xl/workspace:col-span-4" id={errorId} role="alert">{error}</p>}
    </li>
  )
}
