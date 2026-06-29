import type { ProjectRuntimeConfigSet } from '@/api'
import { FileCode2, Pencil, Rocket } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'

interface ApplicationRuntimeConfigSelectorProps {
  redeployableCount: number
  redeployPending: boolean
  restartAffectedCount: number
  selectedIds: string[]
  sets: ProjectRuntimeConfigSet[]
  onCreate: () => void
  onDismissRestart: () => void
  onEdit: (set: ProjectRuntimeConfigSet) => void
  onRedeployAffected: () => void
  onToggle: (id: string, checked: boolean) => void
}

export function ApplicationRuntimeConfigSelector({
  onCreate,
  onDismissRestart,
  onEdit,
  onRedeployAffected,
  onToggle,
  redeployableCount,
  redeployPending,
  restartAffectedCount,
  selectedIds,
  sets,
}: ApplicationRuntimeConfigSelectorProps) {
  const { t } = useTranslation()

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
            ? sets.map(set => (
                <div key={set.id} className="flex items-center justify-between gap-3 rounded-md px-2 py-1.5 text-sm hover:bg-muted/60">
                  <label className="flex min-w-0 flex-1 items-center gap-3">
                    <input
                      checked={selectedIds.includes(set.id)}
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
                  <Button aria-label={t('runtimeConfigSets.editTitle')} size="sm" type="button" variant="ghost" onClick={() => onEdit(set)}>
                    <Pencil className="size-4" />
                  </Button>
                </div>
              ))
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
