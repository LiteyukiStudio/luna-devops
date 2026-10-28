import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { HookPhase, ProjectHookConfig } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { NativeSelect } from '@/components/ui/native-select'
import { EditorSection, SwitchField } from '../deployment-target-editor-fields'
import { useFieldError } from '../deployment-target-editor-utils'
import { buildHookPhases, deploymentHookPhases } from '../deployment-target-form'

/** 钩子绑定：按阶段分组选择；已有镜像来源不会执行构建阶段，相关阶段保留但禁用。 */
export function HooksSection({ projectId }: { projectId: string }) {
  const { t } = useTranslation()
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const { append, fields, move, remove } = useFieldArray({ control, name: 'buildHookBindings' })
  const [hooksEnabled, sourceType] = useWatch({ control, name: ['buildHooksEnabled', 'sourceType'] })
  const hooks = useQuery({ queryKey: ['project-hooks', projectId], queryFn: () => api.listProjectHooks(projectId), enabled: Boolean(projectId) })
  const hookItems = hooks.data ?? []
  const canEdit = hooksEnabled && hookItems.length > 0

  return (
    <EditorSection description={t('deploymentsPage.deploymentHooksDescription')} section="hooks" title={t('deploymentTargetEditor.sections.hooks')}>
      <SwitchField description={t('deploymentsPage.deploymentHooksEnabledHint')} label={t('deploymentsPage.deploymentHooksEnabled')} name="buildHooksEnabled" />
      <p className="text-sm leading-6 text-muted-foreground">{t('deploymentsPage.deploymentHooksHint')}</p>
      {hooks.isError && (
        <Alert variant="destructive">
          <AlertDescription>{t('projectHooks.loadFailedDescription')}</AlertDescription>
        </Alert>
      )}
      {hooks.isSuccess && hookItems.length === 0 && (
        <p className="rounded-control border border-dashed border-border px-4 py-3 text-sm text-muted-foreground">{t('deploymentsPage.emptyDeploymentHooks')}</p>
      )}
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
    </EditorSection>
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
  const { register } = useFormContext<DeploymentTargetFormValues>()
  const error = useFieldError(`buildHookBindings.${index}.hookConfigId`)
  const phaseLabel = (phase: HookPhase) => t(`projectHooks.phases.${phase}`)
  const order = index + 1
  const errorId = `deployment-target-hook-${index}-error`

  return (
    <li className="grid gap-2 rounded-control bg-surface-inset/60 p-3 md:grid-cols-[2rem_minmax(0,1fr)_minmax(0,1.2fr)_auto] md:items-center">
      <span aria-hidden="true" className="text-sm font-medium text-muted-foreground tabular-nums">{order}</span>
      <NativeSelect {...register(`buildHookBindings.${index}.phase`)} aria-label={t('deploymentTargetEditor.hookPhaseFor', { index: order })} disabled={disabled}>
        <optgroup label={t('deploymentTargetEditor.buildPhases')}>
          {deploymentHookPhases.filter(phase => buildHookPhases.has(phase)).map(phase => (
            <option key={phase} disabled={imageSource} value={phase}>{phaseLabel(phase)}</option>
          ))}
        </optgroup>
        <optgroup label={t('deploymentTargetEditor.deploymentPhases')}>
          {deploymentHookPhases.filter(phase => !buildHookPhases.has(phase)).map(phase => (
            <option key={phase} value={phase}>{phaseLabel(phase)}</option>
          ))}
        </optgroup>
      </NativeSelect>
      <NativeSelect
        {...register(`buildHookBindings.${index}.hookConfigId`)}
        aria-describedby={error ? errorId : undefined}
        aria-invalid={error ? true : undefined}
        aria-label={t('deploymentTargetEditor.hookConfigFor', { index: order })}
        disabled={disabled}
      >
        {hooks.map(hook => <option key={hook.id} value={hook.id}>{hook.name}</option>)}
      </NativeSelect>
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
      {error && <p className="text-xs text-danger md:col-span-4" id={errorId} role="alert">{error}</p>}
    </li>
  )
}
