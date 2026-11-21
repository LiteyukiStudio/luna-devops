import type { ProjectVolumeStorageClass } from '@/api'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { EMPTY_SELECT_VALUE, fromSelectValue, toSelectValue } from '@/components/ui/select-value'
import { useProjectRuntimeClusters, useProjectVolumeStorageClasses } from './volume-resource-queries'

interface ControlAccessibilityProps {
  'id'?: string
  'aria-describedby'?: string
  'aria-invalid'?: boolean | 'true' | 'false'
  'aria-label'?: string
}

export function ProjectVolumeClusterSelect({
  allowAll = false,
  disabled,
  onChange,
  projectId,
  value,
  ...a11yProps
}: ControlAccessibilityProps & {
  allowAll?: boolean
  disabled?: boolean
  onChange: (value: string) => void
  projectId: string
  value: string
}) {
  const { t } = useTranslation()
  const query = useProjectRuntimeClusters(projectId, !disabled)
  const clusters = useMemo(() => query.data?.pages.flatMap(page => page.items) ?? [], [query.data?.pages])

  return (
    <div className="flex min-w-0 gap-2">
      <Select
        disabled={disabled || query.isLoading || query.isError}
        value={allowAll ? toSelectValue(value) : (value || undefined)}
        onValueChange={next => onChange(allowAll ? fromSelectValue(next) : next)}
      >
        <SelectTrigger {...a11yProps} className="min-w-0 flex-1">
          <SelectValue placeholder={allowAll ? t('projectVolumes.allClusters') : t('projectVolumes.selectCluster')} />
        </SelectTrigger>
        <SelectContent>
          {allowAll && <SelectItem value={EMPTY_SELECT_VALUE}>{t('projectVolumes.allClusters')}</SelectItem>}
          {clusters.map(cluster => (
            <SelectItem key={cluster.id} value={cluster.id}>
              {cluster.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {query.hasNextPage && (
        <Button
          disabled={query.isFetchingNextPage}
          size="sm"
          type="button"
          variant="outline"
          onClick={() => query.fetchNextPage()}
        >
          {t('projectVolumes.loadMoreClusters')}
        </Button>
      )}
    </div>
  )
}

export function ProjectVolumeStorageClassSelect({
  clusterId,
  disabled,
  onChange,
  projectId,
  value,
  ...a11yProps
}: ControlAccessibilityProps & {
  clusterId: string
  disabled?: boolean
  onChange: (value: string, item?: ProjectVolumeStorageClass) => void
  projectId: string
  value: string
}) {
  const { t } = useTranslation()
  const query = useProjectVolumeStorageClasses(projectId, clusterId, !disabled)
  const items = useMemo(() => query.data?.pages.flatMap(page => page.items) ?? [], [query.data?.pages])

  return (
    <div className="flex min-w-0 gap-2">
      <Select
        disabled={disabled || !clusterId || query.isLoading || query.isError}
        value={value || undefined}
        onValueChange={(next) => {
          onChange(next, items.find(item => item.name === next))
        }}
      >
        <SelectTrigger {...a11yProps} className="min-w-0 flex-1">
          <SelectValue placeholder={t('common.select')} />
        </SelectTrigger>
        <SelectContent>
          {items.map(item => (
            <SelectItem key={item.name} value={item.name}>
              {item.isDefault ? t('projectVolumes.storageClassDefault', { name: item.name }) : item.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {query.hasNextPage && (
        <Button disabled={query.isFetchingNextPage} size="sm" type="button" variant="outline" onClick={() => query.fetchNextPage()}>
          {t('projectVolumes.loadMoreStorageClasses')}
        </Button>
      )}
    </div>
  )
}
