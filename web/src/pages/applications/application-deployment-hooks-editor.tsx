import type { DeploymentTargetHookBinding, HookPhase, ProjectHookConfig } from '@/api'
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { NativeSelect as Select } from '@/components/ui/native-select'

const hookPhases: HookPhase[] = [
  'prePull',
  'postPull',
  'preBuild',
  'postBuild',
  'prePush',
  'postPush',
  'preDeployment',
  'postDeployment',
]

interface DeploymentHooksEditorProps {
  bindings: DeploymentTargetHookBinding[]
  disabled?: boolean
  hooks: ProjectHookConfig[]
  onChange: (bindings: DeploymentTargetHookBinding[]) => void
}

function orderedBindings(bindings: DeploymentTargetHookBinding[]) {
  return bindings.map((binding, index) => ({
    ...binding,
    runOrder: index + 1,
  }))
}

export function ApplicationDeploymentHooksEditor({ bindings, disabled = false, hooks, onChange }: DeploymentHooksEditorProps) {
  const { t } = useTranslation()
  const rows = orderedBindings(bindings ?? [])
  const canEdit = !disabled && hooks.length > 0

  const updateRow = (index: number, patch: Partial<DeploymentTargetHookBinding>) => {
    onChange(orderedBindings(rows.map((row, rowIndex) => rowIndex === index ? { ...row, ...patch } : row)))
  }

  const removeRow = (index: number) => {
    onChange(orderedBindings(rows.filter((_, rowIndex) => rowIndex !== index)))
  }

  const moveRow = (index: number, direction: -1 | 1) => {
    const nextIndex = index + direction
    if (nextIndex < 0 || nextIndex >= rows.length)
      return
    const nextRows = [...rows]
    const [row] = nextRows.splice(index, 1)
    nextRows.splice(nextIndex, 0, row)
    onChange(orderedBindings(nextRows))
  }

  const addRow = () => {
    const firstHook = hooks[0]
    if (!firstHook)
      return
    onChange(orderedBindings([
      ...rows,
      {
        hookConfigId: firstHook.id,
        phase: 'preDeployment',
        runOrder: rows.length + 1,
      },
    ]))
  }

  return (
    <div className="grid gap-3">
      <p className="text-sm leading-6 text-muted-foreground">{t('deploymentsPage.deploymentHooksHint')}</p>
      {hooks.length === 0 && (
        <div className="rounded-md border border-dashed border-border bg-muted/30 px-4 py-3 text-sm text-muted-foreground">
          {t('deploymentsPage.emptyDeploymentHooks')}
        </div>
      )}
      {rows.length > 0 && (
        <div className="grid gap-2">
          {rows.map((binding, index) => (
            <div key={`${binding.phase}-${binding.hookConfigId}`} className="grid gap-2 rounded-md border border-border bg-card/50 p-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)_auto] md:items-end">
              <Field label={t('projectHooks.phase')}>
                <Select
                  disabled={!canEdit}
                  value={binding.phase}
                  onChange={event => updateRow(index, { phase: event.target.value as HookPhase })}
                >
                  {hookPhases.map(phase => (
                    <option key={phase} value={phase}>{t(`projectHooks.phases.${phase}`)}</option>
                  ))}
                </Select>
              </Field>
              <Field label={t('deploymentsPage.deploymentHookConfig')}>
                <Select
                  disabled={!canEdit}
                  value={binding.hookConfigId}
                  onChange={event => updateRow(index, { hookConfigId: event.target.value })}
                >
                  {hooks.map(hook => (
                    <option key={hook.id} value={hook.id}>{hook.name}</option>
                  ))}
                </Select>
              </Field>
              <div className="flex justify-end gap-1">
                <Button
                  aria-label={t('deploymentsPage.moveHookUp')}
                  disabled={!canEdit || index === 0}
                  size="icon"
                  type="button"
                  variant="ghost"
                  onClick={() => moveRow(index, -1)}
                >
                  <ArrowUp className="size-4" />
                </Button>
                <Button
                  aria-label={t('deploymentsPage.moveHookDown')}
                  disabled={!canEdit || index === rows.length - 1}
                  size="icon"
                  type="button"
                  variant="ghost"
                  onClick={() => moveRow(index, 1)}
                >
                  <ArrowDown className="size-4" />
                </Button>
                <Button
                  aria-label={t('deploymentsPage.removeHookBinding')}
                  disabled={disabled}
                  size="icon"
                  type="button"
                  variant="ghost"
                  onClick={() => removeRow(index)}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
      <div>
        <Button disabled={!canEdit} size="sm" type="button" variant="outline" onClick={addRow}>
          <Plus className="size-4" />
          {t('deploymentsPage.addHookBinding')}
        </Button>
      </div>
    </div>
  )
}
