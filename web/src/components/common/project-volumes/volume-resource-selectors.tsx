import type { ProjectVolumeStorageClass } from '@/api'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { NativeSelect } from '@/components/ui/native-select'
import { useProjectRuntimeClusters, useProjectVolumeStorageClasses } from './volume-resource-queries'

export function ProjectVolumeClusterSelect({ allowAll = false, disabled, onChange, projectId, value }: {
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
      <NativeSelect
        className="min-w-0 flex-1"
        disabled={disabled || query.isLoading || query.isError}
        value={value}
        onChange={event => onChange(event.target.value)}
      >
        <option value="">{allowAll ? t('projectVolumes.allClusters') : t('projectVolumes.selectCluster')}</option>
        {clusters.map(cluster => <option key={cluster.id} value={cluster.id}>{cluster.name}</option>)}
      </NativeSelect>
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

export function ProjectVolumeStorageClassSelect({ clusterId, disabled, onChange, projectId, value }: {
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
      <NativeSelect
        className="min-w-0 flex-1"
        disabled={disabled || !clusterId || query.isLoading || query.isError}
        value={value}
        onChange={(event) => {
          const next = event.target.value
          onChange(next, items.find(item => item.name === next))
        }}
      >
        <option value="">{t('common.select')}</option>
        {items.map(item => (
          <option key={item.name} value={item.name}>
            {item.isDefault ? t('projectVolumes.storageClassDefault', { name: item.name }) : item.name}
          </option>
        ))}
      </NativeSelect>
      {query.hasNextPage && (
        <Button disabled={query.isFetchingNextPage} size="sm" type="button" variant="outline" onClick={() => query.fetchNextPage()}>
          {t('projectVolumes.loadMoreStorageClasses')}
        </Button>
      )}
    </div>
  )
}
