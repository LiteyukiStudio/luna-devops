import type { DeploymentTargetRuntimePod, Release } from '@/api'
import type { SearchSelectOption } from '@/components/common/search-select'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { SearchSelect } from '@/components/common/search-select'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'

/** 发布运行日志与终端共用的副本选择器；空值表示由平台自动选择（优先当前发布）。 */
export function RuntimePodSelect({
  projectId,
  release,
  value,
  onValueChange,
}: {
  projectId: string
  release: Release
  value: string
  onValueChange: (pod: string) => void
}) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const pods = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['deployment-target-runtime-pods', projectId, release.applicationId, release.deploymentTargetId, release.id, search],
    queryFn: ({ signal }) => api.listDeploymentTargetRuntimePods(projectId, release.applicationId, release.deploymentTargetId, { releaseId: release.id, search }, signal),
    enabled: Boolean(projectId && release.applicationId && release.deploymentTargetId),
    refetchInterval: WORKFLOW_STATUS_REFETCH_INTERVAL_MS,
  })
  const page = pods.data
  const observed = page?.status === 'ready'
  const items = observed ? page.items : []
  const options: SearchSelectOption[] = [
    { value: '', label: t('deploymentsPage.runtimePodAuto'), description: t('deploymentsPage.runtimePodAutoDescription') },
    ...items.map(pod => ({ value: pod.name, label: pod.name, description: runtimePodDescription(pod, release.id, t) })),
  ]
  // 搜索时列表只是子集，不能据此判断所选副本已消失。
  const selectedMissing = Boolean(value && observed && !search.trim() && !items.some(pod => pod.name === value))
  const unavailable = pods.isError || page?.status === 'unavailable'

  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2 text-xs text-muted-foreground">
      <span>{t('deploymentsPage.runtimePod')}</span>
      <SearchSelect
        ariaLabel={t('deploymentsPage.runtimePod')}
        className="w-64 font-mono"
        limited={Boolean(page && page.total > page.items.length)}
        loading={pods.isLoading}
        options={selectedMissing ? [...options, { value, label: value }] : options}
        search={search}
        searchPlaceholder={t('deploymentsPage.runtimePodSearchPlaceholder')}
        size="sm"
        value={value}
        onSearchChange={setSearch}
        onValueChange={onValueChange}
      />
      {selectedMissing && <span className="text-warning" role="status">{t('deploymentsPage.runtimePodMissing')}</span>}
      {unavailable && <span role="status">{t('deploymentsPage.runtimePodsUnavailable')}</span>}
    </div>
  )
}

function runtimePodDescription(pod: DeploymentTargetRuntimePod, releaseId: string, t: (key: string, options?: Record<string, unknown>) => string) {
  return [
    pod.terminating
      ? t('deploymentsPage.runtimePodTerminating')
      : t(pod.ready ? 'deploymentsPage.runtimePodReady' : 'deploymentsPage.runtimePodNotReady'),
    t('deploymentsPage.runtimePodRestarts', { count: pod.restartCount }),
    t(pod.releaseId === releaseId ? 'deploymentsPage.runtimePodCurrentRelease' : 'deploymentsPage.runtimePodOtherRelease'),
  ].join(' · ')
}
