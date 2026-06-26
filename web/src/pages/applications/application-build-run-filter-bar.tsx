import type { ReactNode } from 'react'
import type { BuildRun } from '@/api'
import { useTranslation } from 'react-i18next'
import { NativeSelect as Select } from '@/components/ui/native-select'

const buildRunStatusFilters: Array<BuildRun['status']> = ['queued', 'running', 'succeeded', 'failed', 'canceled', 'lost', 'timeout']
const buildRunEventFilters: Array<BuildRun['triggerType']> = ['manual', 'push', 'tag', 'webhook', 'api', 'retry']

export function ApplicationBuildRunFilterBar({ actor, actorOptions, branch, branchOptions, event, onActorChange, onBranchChange, onEventChange, onStatusChange, status }: {
  actor: string
  actorOptions: string[]
  branch: string
  branchOptions: string[]
  event: string
  status: string
  onActorChange: (value: string) => void
  onBranchChange: (value: string) => void
  onEventChange: (value: string) => void
  onStatusChange: (value: string) => void
}) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-wrap items-center gap-1 border-b border-border bg-muted/25 px-4 py-2">
      <BuildRunFilterSelect
        label={t('buildsPage.eventFilter')}
        value={event}
        onChange={onEventChange}
      >
        <option value="">{t('buildsPage.allEvents')}</option>
        {buildRunEventFilters.map(value => (
          <option key={value} value={value}>{t(`buildsPage.events.${value}`)}</option>
        ))}
      </BuildRunFilterSelect>
      <BuildRunFilterSelect
        label={t('buildsPage.statusFilter')}
        value={status}
        onChange={onStatusChange}
      >
        <option value="">{t('buildsPage.allStatuses')}</option>
        {buildRunStatusFilters.map(value => (
          <option key={value} value={value}>{t(`buildsPage.statuses.${value}`)}</option>
        ))}
      </BuildRunFilterSelect>
      <BuildRunFilterSelect
        label={t('buildsPage.branchFilter')}
        value={branch}
        onChange={onBranchChange}
      >
        <option value="">{t('buildsPage.allBranches')}</option>
        {branchOptions.map(value => (
          <option key={value} value={value}>{value}</option>
        ))}
      </BuildRunFilterSelect>
      <BuildRunFilterSelect
        label={t('buildsPage.actorFilter')}
        value={actor}
        onChange={onActorChange}
      >
        <option value="">{t('buildsPage.allActors')}</option>
        {actorOptions.map(value => (
          <option key={value} value={value}>{shortActorLabel(value)}</option>
        ))}
      </BuildRunFilterSelect>
    </div>
  )
}

function BuildRunFilterSelect({ children, label, onChange, value }: {
  children: ReactNode
  label: string
  value: string
  onChange: (value: string) => void
}) {
  return (
    <label className="min-w-32">
      <span className="sr-only">{label}</span>
      <Select
        aria-label={label}
        className="h-8 rounded-md border-transparent bg-transparent px-2.5 pr-8 text-sm text-muted-foreground shadow-none hover:bg-background/70 focus-visible:border-primary/40 focus-visible:ring-primary/20"
        value={value}
        onChange={event => onChange(event.target.value)}
      >
        {children}
      </Select>
    </label>
  )
}

function shortActorLabel(value: string) {
  if (!value)
    return '-'
  const index = value.indexOf('_')
  if (index >= 0)
    return value.slice(index + 1, index + 9)
  return value.length > 12 ? value.slice(0, 12) : value
}
