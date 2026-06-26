import type { BuildJob, BuildRun } from '@/api'
import { CalendarClock, CircleCheck, CircleX, Clock3, LoaderCircle, MoreHorizontal, Package, RotateCcw, ScrollText, Square, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { buildRunImageRef } from '@/components/common/deployment-build-runs'
import { HoverText } from '@/components/common/hover-text'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatElapsedDuration, formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { shortBuildId } from './application-config-utils'

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

const buildFailedStatuses = new Set(['failed', 'lost', 'timeout'])

export function ApplicationBuildRunRow({ binding, deploymentTargetName, canceling, deleting, jobs, latestJob, onCancel, onDelete, onOpenLogs, onRetry, retrying, run }: {
  binding: { cloneUrl?: string, defaultBranch: string, gitAccountId: string, owner: string, repo: string }
  deploymentTargetName?: string
  canceling: boolean
  deleting: boolean
  jobs: BuildJob[]
  latestJob?: BuildJob
  onCancel: () => void
  onDelete: () => void
  onOpenLogs: (job: BuildJob) => void
  onRetry: () => void
  retrying: boolean
  run: BuildRun
}) {
  const { t } = useTranslation()
  const branch = run.sourceBranch || run.sourceTag || binding.defaultBranch || 'main'
  const targetImage = buildRunImageRef(run)
  const commit = shortCommit(run.sourceCommit)
  const triggerActor = buildRunTriggerActor(run)
  const sourceAuthor = buildRunSourceAuthor(run)
  const imageReady = run.status === 'succeeded' && Boolean(targetImage)
  const liveState = buildRunLiveState(run, latestJob, t)
  const failureMessage = buildRunFailureMessage(run, latestJob, t)
  const duration = formatBuildDuration(run, t)
  const commitUrl = buildCommitUrl(binding, run.sourceCommit)
  const authorUrl = buildAuthorUrl(binding, run)
  const canCancel = run.status === 'queued' || run.status === 'running'
  const canDelete = ['succeeded', 'failed', 'canceled', 'lost', 'timeout'].includes(run.status)
  const copyImageRef = () => {
    if (!targetImage)
      return
    navigator.clipboard.writeText(targetImage)
      .then(() => toast.success(t('buildsPage.imageRefCopied')))
      .catch(error => toast.error(error.message))
  }
  return (
    <div className="grid gap-2 px-4 py-3 transition-colors hover:bg-muted/35 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
      <div className="flex min-w-0 gap-2.5">
        <BuildRunStatusIcon status={run.status} />
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-1">
            <h3 className="truncate text-sm font-semibold text-foreground" title={buildRunTitle(run, t, deploymentTargetName)}>
              {buildRunTitle(run, t, deploymentTargetName)}
            </h3>
            <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={run.status} />
          </div>
          <div className="mt-1.5 flex min-w-0 flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
            <span className="inline-flex max-w-full min-w-0 items-center gap-1.5 rounded-md border border-border bg-muted/60 px-2 py-1">
              <span className="shrink-0 font-mono font-medium text-primary">{branch}</span>
              {commitUrl
                ? (
                    <a className="min-w-0 truncate font-medium text-foreground/80 transition-colors hover:text-primary" href={commitUrl} rel="noreferrer" target="_blank" title={`${binding.owner}/${binding.repo}`}>
                      {binding.owner}
                      /
                      {binding.repo}
                    </a>
                  )
                : (
                    <span className="min-w-0 truncate font-medium text-foreground/80" title={`${binding.owner}/${binding.repo}`}>
                      {binding.owner}
                      /
                      {binding.repo}
                    </span>
                  )}
              <span className="shrink-0 font-mono text-muted-foreground">
                #
                {shortBuildId(run.id)}
              </span>
              <span className="shrink-0">{t('buildsPage.triggeredBy', { actor: triggerActor })}</span>
              {commit && (
                <>
                  {sourceAuthor && <span className="shrink-0">·</span>}
                  {sourceAuthor && (
                    authorUrl
                      ? <a className="shrink-0 transition-colors hover:text-primary" href={authorUrl} rel="noreferrer" target="_blank">{t('buildsPage.committedBy', { actor: sourceAuthor })}</a>
                      : <span className="shrink-0">{t('buildsPage.committedBy', { actor: sourceAuthor })}</span>
                  )}
                  <span className="shrink-0">{t('buildsPage.commitAction')}</span>
                  {commitUrl
                    ? <a className="shrink-0 font-mono text-foreground/70 transition-colors hover:text-primary" href={commitUrl} rel="noreferrer" target="_blank">{commit}</a>
                    : <span className="shrink-0 font-mono text-foreground/70">{commit}</span>}
                </>
              )}
            </span>
            <button
              className="inline-flex max-w-full min-w-0 items-center gap-1.5 rounded-md border border-border bg-muted/60 px-2 py-1 text-left transition-colors hover:border-primary/50 hover:text-primary disabled:hover:border-border disabled:hover:text-muted-foreground"
              disabled={!imageReady}
              title={imageReady ? targetImage : failureMessage ? undefined : liveState}
              type="button"
              onClick={copyImageRef}
            >
              {failureMessage
                ? (
                    <HoverText className="max-w-full" value={failureMessage}>
                      <span className="inline-flex min-w-0 items-center gap-1.5">
                        <BuildRunStatusIcon compact status={run.status} />
                        <span className="min-w-0 truncate font-mono">{liveState}</span>
                      </span>
                    </HoverText>
                  )
                : (
                    <>
                      {imageReady
                        ? <Package className="size-3.5 shrink-0 text-muted-foreground" />
                        : <BuildRunStatusIcon compact status={run.status} />}
                      <span className="min-w-0 truncate font-mono">
                        {imageReady ? targetImage : liveState}
                      </span>
                    </>
                  )}
            </button>
          </div>
        </div>
      </div>
      <div className="flex min-w-0 items-start justify-between gap-2 lg:min-w-72">
        <div className="grid min-w-0 gap-1 text-sm text-muted-foreground lg:justify-items-start">
          <span className="inline-flex min-w-0 items-center gap-2">
            <CalendarClock className="size-4 shrink-0" />
            <span className="truncate">
              {formatBuildDate(run, t)}
              {duration && (
                <>
                  {' '}
                  ·
                  {' '}
                  {duration}
                </>
              )}
            </span>
          </span>
          <span className="inline-flex min-w-0 items-center gap-2">
            <Clock3 className="size-4 shrink-0" />
            <span className="truncate">
              {latestJob
                ? t('buildsPage.latestJobSummary', { attempts: latestJob.attempts, id: shortBuildId(latestJob.id) })
                : t('buildsPage.noBuildJob')}
            </span>
          </span>
          {jobs.length > 1 && <span className="text-xs">{t('buildsPage.jobCount', { count: jobs.length })}</span>}
        </div>
        <Popover>
          <PopoverTrigger asChild>
            <Button aria-label={t('buildsPage.runActions')} className="shrink-0" size="icon" variant="ghost">
              <MoreHorizontal className="size-4" />
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" className="w-64 max-w-[calc(100vw-2rem)] p-1">
            <Button className="h-auto w-full justify-start gap-2 whitespace-normal text-left" disabled={retrying} variant="ghost" onClick={onRetry}>
              <RotateCcw className="size-4 shrink-0" />
              <span className="min-w-0">{t('buildsPage.retry')}</span>
            </Button>
            <Button className="h-auto w-full justify-start gap-2 whitespace-normal text-left" disabled={!latestJob} variant="ghost" onClick={() => latestJob && onOpenLogs(latestJob)}>
              <ScrollText className="size-4 shrink-0" />
              <span className="min-w-0">{t('buildsPage.viewLogsStream')}</span>
            </Button>
            {canCancel && (
              <ConfirmDialog
                confirmText={t('buildsPage.cancelBuildConfirm')}
                description={t('buildsPage.cancelBuildDescription')}
                pending={canceling}
                title={t('buildsPage.cancelBuildTitle')}
                onConfirm={onCancel}
              >
                <Button className="h-auto w-full justify-start gap-2 whitespace-normal text-left text-danger hover:text-danger" disabled={canceling} variant="ghost">
                  <Square className="size-4 shrink-0" />
                  <span className="min-w-0">{t('buildsPage.cancelBuild')}</span>
                </Button>
              </ConfirmDialog>
            )}
            {canDelete && (
              <ConfirmDialog
                confirmText={t('common.delete')}
                description={t('buildsPage.deleteBuildDescription')}
                pending={deleting}
                title={t('buildsPage.deleteBuildTitle')}
                onConfirm={onDelete}
              >
                <Button className="h-auto w-full justify-start gap-2 whitespace-normal text-left text-danger hover:text-danger" disabled={deleting} variant="ghost">
                  <Trash2 className="size-4 shrink-0" />
                  <span className="min-w-0">{t('buildsPage.deleteBuild')}</span>
                </Button>
              </ConfirmDialog>
            )}
          </PopoverContent>
        </Popover>
      </div>
    </div>
  )
}

function BuildRunStatusIcon({ compact = false, status }: { compact?: boolean, status: string }) {
  const className = compact ? 'size-3.5 shrink-0' : 'mt-0.5 size-5 shrink-0'
  if (status === 'succeeded')
    return <CircleCheck className={`${className} text-emerald-600`} />
  if (status === 'failed' || status === 'lost' || status === 'timeout')
    return <CircleX className={`${className} text-rose-600`} />
  if (status === 'running')
    return <LoaderCircle className={`${className} animate-spin text-primary`} />
  return <Clock3 className={`${className} text-muted-foreground`} />
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

function buildRunLiveState(run: BuildRun, latestJob: BuildJob | undefined, t: ReturnType<typeof useTranslation>['t']) {
  const progress = buildJobProgressLabel(latestJob?.message, t)
  if (latestJob?.status === 'running' && progress)
    return progress
  return t(`buildsPage.statuses.${run.status}`)
}

function buildRunFailureMessage(run: BuildRun, latestJob: BuildJob | undefined, t: ReturnType<typeof useTranslation>['t']) {
  if (!buildFailedStatuses.has(run.status) && !buildFailedStatuses.has(latestJob?.status ?? ''))
    return ''
  const message = latestJob?.message?.trim()
  if (message && !buildJobProgressKeys.has(message))
    return message
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
