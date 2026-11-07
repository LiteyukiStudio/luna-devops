import type { BuildJob, BuildRun } from '@/api'
import { MoreHorizontal, Package, RotateCcw, ScrollText, Square, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { buildRunImageRef } from '@/components/common/deployment-build-runs'
import { HoverText } from '@/components/common/hover-text'
import { StatusIndicator } from '@/components/common/status-indicator'
import { statusIndicatorStateFor } from '@/components/common/status-tone'
import { formatElapsedDuration, formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { shortBuildId } from '@/pages/applications/application-config-utils'
import { BuildCommitDetails } from './build-commit-details'

const buildJobProgressKeys = new Set([
  'claimed',
  'clone_repository',
  'load_dockerfile',
  'pull_image_metadata',
  'pull_base_image',
  'upload_build_context',
  'run_command',
  'export_image',
  'push_image_layers',
  'push_image_manifest',
  'registry_auth',
])

const buildFailedStatuses = new Set(['failed', 'timeout'])

export interface BuildRunBinding { cloneUrl?: string, defaultBranch: string, gitAccountId: string, owner: string, repo: string }

/** 构建状态：执行中显示“构建中 n/m”，悬停查看当前阶段和步骤进度；失败、超时或取消时直接显示原因摘要，悬停查看任务诊断信息。 */
export function BuildRunStatusCell({ latestJob, run }: { latestJob?: BuildJob, run: BuildRun }) {
  const { t } = useTranslation()
  const building = latestJob?.status === 'running'
  const steps = building && latestJob.stepsTotal ? `${latestJob.stepsCompleted}/${latestJob.stepsTotal}` : undefined
  const failureMessage = buildRunFailureMessage(run, latestJob, t)
  const failureReason = run.failureCode ? t(`buildsPage.failureCodes.${run.failureCode}`) : ''
  const status = <StatusIndicator detail={steps} label={building ? t('buildsPage.building') : t(`buildsPage.statuses.${run.status}`)} state={statusIndicatorStateFor(run.status)} />
  const hint = failureMessage || (building ? buildJobLiveHint(latestJob, t) : '')
  return (
    <div className="grid min-w-0 max-w-full gap-0.5">
      {hint
        ? (
            <HoverText className="max-w-full" value={hint}>
              {status}
            </HoverText>
          )
        : status}
      {failureReason && <span className="truncate text-xs text-muted-foreground" title={failureReason}>{failureReason}</span>}
    </div>
  )
}

/** 构建来源：触发方式与部署配置，下方为分支、仓库、commit 和触发人。 */
export function BuildRunSourceCell({ binding, deploymentTargetName, run }: { binding?: BuildRunBinding, deploymentTargetName?: string, run: BuildRun }) {
  const { t } = useTranslation()
  const branch = run.sourceBranch || run.sourceTag || binding?.defaultBranch || 'main'
  const commit = shortCommit(run.sourceCommit)
  const triggerActor = buildRunTriggerActor(run)
  const sourceAuthor = buildRunSourceAuthor(run)
  const commitUrl = binding ? buildCommitUrl(binding, run.sourceCommit) : ''
  const authorUrl = binding ? buildAuthorUrl(binding, run) : ''
  const title = buildRunTitle(run, t, deploymentTargetName)
  return (
    <div className="grid min-w-0 max-w-full gap-0.5">
      <HoverText className="text-sm font-medium text-foreground" value={title} />
      <div className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs text-muted-foreground">
        <span className="font-mono text-foreground">{branch}</span>
        {commit && <BuildCommitDetails author={sourceAuthor} commitUrl={commitUrl} run={run} />}
        {binding && <span className="min-w-0 truncate font-mono" title={`${binding.owner}/${binding.repo}`}>{`${binding.owner}/${binding.repo}`}</span>}
        <span className="font-mono">{`#${shortBuildId(run.id)}`}</span>
        <span>{t('buildsPage.triggeredBy', { actor: triggerActor })}</span>
        {sourceAuthor && commit && (
          authorUrl
            ? <a className="text-primary-text hover:underline" href={authorUrl} rel="noopener noreferrer" target="_blank">{t('buildsPage.committedBy', { actor: sourceAuthor })}</a>
            : <span>{t('buildsPage.committedBy', { actor: sourceAuthor })}</span>
        )}
      </div>
    </div>
  )
}

/** 构建产物：成功后可复制镜像引用。 */
export function BuildRunImageCell({ run }: { run: BuildRun }) {
  const { t } = useTranslation()
  const targetImage = buildRunImageRef(run)
  if (run.status !== 'succeeded' || !targetImage)
    return <span className="text-sm text-muted-foreground">-</span>
  const copyImageRef = () => {
    navigator.clipboard.writeText(targetImage)
      .then(() => toast.success(t('buildsPage.imageRefCopied')))
      .catch(error => toast.error(error.message))
  }
  return (
    <button
      aria-label={t('buildsPage.copyImageRef', { image: targetImage })}
      className="inline-flex min-w-0 max-w-full items-center gap-1.5 rounded-sm text-left font-mono text-xs text-foreground hover:text-primary-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      title={targetImage}
      type="button"
      onClick={copyImageRef}
    >
      <Package aria-hidden="true" className="size-3.5 shrink-0 text-muted-foreground" />
      <span className="min-w-0 truncate">{targetImage}</span>
    </button>
  )
}

/** 构建时间与耗时；多次尝试时附带任务次数。 */
export function BuildRunTimeCell({ jobs, run }: { jobs: BuildJob[], run: BuildRun }) {
  const { t } = useTranslation()
  const duration = formatBuildDuration(run, t)
  return (
    <div className="grid gap-0.5 text-xs text-muted-foreground">
      <span className="whitespace-nowrap text-sm text-foreground">{formatBuildDate(run, t)}</span>
      <span className="whitespace-nowrap">
        {[duration, jobs.length > 1 ? t('buildsPage.jobCount', { count: jobs.length }) : ''].filter(Boolean).join(' · ') || '-'}
      </span>
    </div>
  )
}

/** 行操作：可见主操作“查看日志”，其余收进菜单；取消和删除需要确认。 */
export function BuildRunActions({ canRetry, canceling, deleting, latestJob, onCancel, onDelete, onOpenLogs, onRetry, retrying, run }: {
  canRetry: boolean
  canceling: boolean
  deleting: boolean
  latestJob?: BuildJob
  onCancel: () => void
  onDelete: () => void
  onOpenLogs: (job: BuildJob) => void
  onRetry: () => void
  retrying: boolean
  run: BuildRun
}) {
  const { t } = useTranslation()
  const [confirming, setConfirming] = useState<'cancel' | 'delete' | null>(null)
  const canCancel = run.status === 'queued' || run.status === 'running'
  const canDelete = ['succeeded', 'failed', 'canceled', 'timeout'].includes(run.status)
  return (
    <div className="flex items-center justify-end gap-1">
      <Button disabled={!latestJob} size="sm" variant="outline" onClick={() => latestJob && onOpenLogs(latestJob)}>
        <ScrollText className="size-4" />
        {t('buildsPage.viewLogs')}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button aria-label={t('buildsPage.runActions')} size="icon" variant="ghost">
            <MoreHorizontal className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem disabled={!canRetry || retrying} onSelect={onRetry}>
            <RotateCcw className="size-4" />
            {t('buildsPage.retry')}
          </DropdownMenuItem>
          {(canCancel || canDelete) && <DropdownMenuSeparator />}
          {canCancel && (
            <DropdownMenuItem disabled={canceling} variant="destructive" onSelect={() => setConfirming('cancel')}>
              <Square className="size-4" />
              {t('buildsPage.cancelBuild')}
            </DropdownMenuItem>
          )}
          {canDelete && (
            <DropdownMenuItem disabled={deleting} variant="destructive" onSelect={() => setConfirming('delete')}>
              <Trash2 className="size-4" />
              {t('buildsPage.deleteBuild')}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      <ConfirmDialog
        confirmText={t('buildsPage.cancelBuildConfirm')}
        description={t('buildsPage.cancelBuildDescription')}
        open={confirming === 'cancel'}
        pending={canceling}
        title={t('buildsPage.cancelBuildTitle')}
        onConfirm={onCancel}
        onOpenChange={open => !open && setConfirming(null)}
      />
      <ConfirmDialog
        confirmText={t('common.delete')}
        description={t('buildsPage.deleteBuildDescription')}
        open={confirming === 'delete'}
        pending={deleting}
        title={t('buildsPage.deleteBuildTitle')}
        onConfirm={onDelete}
        onOpenChange={open => !open && setConfirming(null)}
      />
    </div>
  )
}

function buildRunTitle(run: BuildRun, t: ReturnType<typeof useTranslation>['t'], deploymentTargetName?: string) {
  let title = t('buildsPage.runTitleManual')
  if (run.triggerType === 'webhook' || run.triggerType === 'push')
    title = t('buildsPage.runTitlePush')
  else if (run.triggerType === 'tag')
    title = t('buildsPage.runTitleTag')
  else if (run.triggerType === 'api')
    title = t('buildsPage.runTitleApi')
  else if (run.triggerType === 'retry')
    title = t('buildsPage.runTitleRetry')
  return deploymentTargetName ? t('buildsPage.runTitleWithConfig', { config: deploymentTargetName, title }) : title
}

function shortCommit(value: string) {
  return value ? value.slice(0, 7) : ''
}

function shortActorLabel(value: string) {
  if (!value)
    return '-'
  const index = value.indexOf('_')
  if (index >= 0)
    return value.slice(index + 1, index + 9)
  return value.length > 12 ? value.slice(0, 12) : value
}

function buildRunTriggerActor(run: BuildRun) {
  return run.triggeredByName || run.triggeredByEmail || shortActorLabel(run.createdBy)
}

function buildRunSourceAuthor(run: BuildRun) {
  return run.sourceAuthorName || run.sourceAuthorEmail
}

function buildCommitUrl(binding: { cloneUrl?: string, owner: string, repo: string }, commit: string) {
  if (!commit)
    return ''
  const repositoryUrl = repositoryBrowserUrl(binding)
  return repositoryUrl ? `${repositoryUrl}/commit/${encodeURIComponent(commit)}` : ''
}

function buildAuthorUrl(binding: { cloneUrl?: string }, run: BuildRun) {
  const username = gitAuthorUsername(run)
  if (!username)
    return ''
  const host = repositoryHostUrl(binding)
  return host ? `${host}/${encodeURIComponent(username)}` : ''
}

function gitAuthorUsername(run: BuildRun) {
  const name = run.sourceAuthorName?.trim()
  if (name && /^[\w.-]+$/.test(name))
    return name
  const emailPrefix = run.sourceAuthorEmail?.split('@')[0]?.trim()
  if (emailPrefix && /^[\w.-]+$/.test(emailPrefix))
    return emailPrefix
  return ''
}

function repositoryBrowserUrl(binding: { cloneUrl?: string, owner: string, repo: string }) {
  const host = repositoryHostUrl(binding)
  if (!host)
    return ''
  return `${host}/${encodeURIComponent(binding.owner)}/${encodeURIComponent(binding.repo)}`
}

function repositoryHostUrl(binding: { cloneUrl?: string }) {
  const cloneUrl = binding.cloneUrl?.trim()
  if (!cloneUrl)
    return ''
  const httpsUrl = cloneUrl
    .replace(/^git@([^:]+):(.+)$/, 'https://$1/$2')
    .replace(/\.git$/, '')
  try {
    const url = new URL(httpsUrl)
    return `${url.protocol}//${url.host}`
  }
  catch {
    return ''
  }
}

/** 执行中构建的悬停提示：第一行为当前阶段（如“运行构建命令”），第二行为 Dockerfile 指令步骤进度。 */
function buildJobLiveHint(job: BuildJob, t: ReturnType<typeof useTranslation>['t']) {
  const steps = job.stepsTotal ? t('buildsPage.stepProgress', { completed: job.stepsCompleted, total: job.stepsTotal }) : ''
  return [buildJobProgressLabel(job.message, t), steps].filter(Boolean).join('\n')
}

function buildRunFailureMessage(run: BuildRun, latestJob: BuildJob | undefined, t: ReturnType<typeof useTranslation>['t']) {
  if (!buildFailedStatuses.has(run.status) && !buildFailedStatuses.has(latestJob?.status ?? ''))
    return ''
  const message = latestJob?.message?.trim()
  if (message && !buildJobProgressKeys.has(message) && message !== run.failureCode)
    return message
  if (run.failureCode)
    return ''
  return latestJob ? t('buildsPage.failureReasonUnavailable') : t('buildsPage.failureReasonNoJob')
}

function buildJobProgressLabel(message: string | undefined, t: ReturnType<typeof useTranslation>['t']) {
  const key = message?.trim()
  if (!key || !buildJobProgressKeys.has(key))
    return ''
  return t(`buildsPage.progress.${key}`)
}

function formatBuildDate(run: BuildRun, t: ReturnType<typeof useTranslation>['t']) {
  return formatSmartDateTime(run.createdAt, t)
}

function formatBuildDuration(run: BuildRun, t: ReturnType<typeof useTranslation>['t']) {
  return formatElapsedDuration(run.startedAt, run.finishedAt, run.status === 'running', t)
}
