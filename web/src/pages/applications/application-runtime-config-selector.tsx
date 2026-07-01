import type { DeploymentRuntimeConfigRef, ProjectRuntimeConfigSet, RuntimeConfigRefMode } from '@/api'
import { FileCode2, Pencil, Rocket } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { NativeSelect as Select } from '@/components/ui/native-select'

interface ApplicationRuntimeConfigSelectorProps {
  redeployableCount: number
  redeployPending: boolean
  restartAffectedCount: number
  selectedRefs: DeploymentRuntimeConfigRef[]
  sets: ProjectRuntimeConfigSet[]
  onCreate: () => void
  onDismissRestart: () => void
  onEdit: (set: ProjectRuntimeConfigSet) => void
  onModeChange: (id: string, mode: RuntimeConfigRefMode) => void
  onRedeployAffected: () => void
  onToggle: (id: string, checked: boolean) => void
}

export function ApplicationRuntimeConfigSelector({
  onCreate,
  onDismissRestart,
  onEdit,
  onModeChange,
  onRedeployAffected,
  onToggle,
  redeployableCount,
  redeployPending,
  restartAffectedCount,
  selectedRefs,
  sets,
}: ApplicationRuntimeConfigSelectorProps) {
  const { t } = useTranslation()
  const selectedById = new Map(selectedRefs.map(ref => [ref.setId, ref]))

  return (
    <>
      <Field hint={t('deploymentsPage.runtimeConfigSetsHint')} label={t('deploymentsPage.runtimeConfigSets')}>
        <div className="grid gap-3 rounded-md border border-input bg-background p-3">
          <div className="flex items-center justify-between gap-3">
            <span className="text-sm font-medium text-foreground">{t('deploymentsPage.runtimeConfigSets')}</span>
            <Button size="sm" type="button" variant="secondary" onClick={onCreate}>
              <FileCode2 className="size-4" />
              {t('runtimeConfigSets.createTitle')}
            </Button>
          </div>
          {sets.length > 0
            ? sets.map((set) => {
                const selectedRef = selectedById.get(set.id)
                const selected = Boolean(selectedRef)
                return (
                  <div key={set.id} className="flex items-center justify-between gap-3 rounded-md px-2 py-1.5 text-sm hover:bg-muted/60">
                    <label className="flex min-w-0 flex-1 items-center gap-3">
                      <input
                        checked={selected}
                        className="size-4 shrink-0 accent-primary"
                        disabled={!set.enabled}
                        type="checkbox"
                        onChange={event => onToggle(set.id, event.target.checked)}
                      />
                      <span className="min-w-0">
                        <span className="block truncate font-medium" title={set.name}>{set.name}</span>
                        <span className="block truncate text-xs text-muted-foreground">{set.enabled ? t('common.enabled') : t('common.disabled')}</span>
                      </span>
                    </label>
                    {selected && (
                      <Select
                        aria-label={t('deploymentsPage.runtimeConfigRefMode')}
                        className="h-8 w-32 text-xs"
                        value={selectedRef?.mode ?? 'live'}
                        onChange={event => onModeChange(set.id, event.target.value as RuntimeConfigRefMode)}
                      >
                        <option value="live">{t('deploymentsPage.runtimeConfigRefModes.live')}</option>
                        <option value="snapshot">{t('deploymentsPage.runtimeConfigRefModes.snapshot')}</option>
                      </Select>
                    )}
                    <Button aria-label={t('runtimeConfigSets.editTitle')} size="sm" type="button" variant="ghost" onClick={() => onEdit(set)}>
                      <Pencil className="size-4" />
                    </Button>
                  </div>
                )
              })
            : <p className="text-sm text-muted-foreground">{t('deploymentsPage.emptyRuntimeConfigSets')}</p>}
        </div>
      </Field>
      {restartAffectedCount > 0 && (
        <div className="flex gap-3 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-amber-950 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-100">
          <Rocket className="mt-0.5 size-4 shrink-0" />
          <div className="grid flex-1 gap-2 text-sm">
            <div className="grid gap-1">
              <p className="font-medium">{t('deploymentsPage.runtimeConfigSetChangedTitle')}</p>
              <p className="text-amber-900/80 dark:text-amber-100/80">
                {t('deploymentsPage.runtimeConfigSetChangedDescription', {
                  count: restartAffectedCount,
                  redeployable: redeployableCount,
                })}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                disabled={redeployableCount === 0 || redeployPending}
                size="sm"
                type="button"
                variant="secondary"
                onClick={onRedeployAffected}
              >
                <Rocket className="size-4" />
                {t('deploymentsPage.redeployAffectedRuntimeConfig')}
              </Button>
              <Button size="sm" type="button" variant="ghost" onClick={onDismissRestart}>
                {t('common.close')}
              </Button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
