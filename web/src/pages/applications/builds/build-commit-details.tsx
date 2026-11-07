import type { BuildRun } from '@/api'
import { ExternalLink } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useMediaQuery } from '@/lib/use-media-query'

/** 能悬停的精确指针设备用悬停卡片；触屏设备没有 hover，改为点按弹出。 */
const hoverCapableQuery = '(hover: hover) and (pointer: fine)'

/**
 * 构建列表中的提交短 SHA：桌面端悬停查看提交消息、作者和完整 SHA，点击仍跳转 Git 平台；
 * 触屏端点按打开同样的详情，并在详情中提供跳转链接。
 */
export function BuildCommitDetails({ author, commitUrl, run }: { author: string, commitUrl: string, run: Pick<BuildRun, 'sourceCommit' | 'sourceCommitMessage'> }) {
  const { t } = useTranslation()
  const canHover = useMediaQuery(hoverCapableQuery)
  const shortSha = run.sourceCommit.slice(0, 7)
  const details = <CommitDetailsContent author={author} commitUrl={canHover ? '' : commitUrl} run={run} />
  const contentClassName = 'grid w-80 max-w-[calc(100vw-2rem)] gap-2 p-3 text-xs'

  if (canHover) {
    return (
      <HoverCard closeDelay={100} openDelay={300}>
        <HoverCardTrigger asChild>
          {commitUrl
            ? <a className="font-mono text-primary-text hover:underline" href={commitUrl} rel="noopener noreferrer" target="_blank">{shortSha}</a>
            : <span className="font-mono">{shortSha}</span>}
        </HoverCardTrigger>
        <HoverCardContent align="start" className={contentClassName}>{details}</HoverCardContent>
      </HoverCard>
    )
  }
  return (
    <Popover>
      <PopoverTrigger aria-label={t('buildsPage.commitDetailsLabel', { commit: shortSha })} className="rounded-sm font-mono text-primary-text underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
        {shortSha}
      </PopoverTrigger>
      <PopoverContent align="start" className={contentClassName}>{details}</PopoverContent>
    </Popover>
  )
}

function CommitDetailsContent({ author, commitUrl, run }: { author: string, commitUrl: string, run: Pick<BuildRun, 'sourceCommit' | 'sourceCommitMessage'> }) {
  const { t } = useTranslation()
  const [subject = '', ...bodyLines] = run.sourceCommitMessage.split('\n')
  const body = bodyLines.join('\n').trim()
  return (
    <>
      {subject
        ? (
            <div className="grid gap-1">
              <p className="break-words text-sm font-medium text-foreground">{subject}</p>
              {body && <p className="max-h-40 overflow-y-auto whitespace-pre-wrap break-words text-muted-foreground">{body}</p>}
            </div>
          )
        : <p className="text-muted-foreground">{t('buildsPage.commitMessageUnavailable')}</p>}
      {author && <p className="text-muted-foreground">{t('buildsPage.committedBy', { actor: author })}</p>}
      <p className="select-all break-all font-mono text-muted-foreground">{run.sourceCommit}</p>
      {commitUrl && (
        <a className="inline-flex w-fit items-center gap-1 text-primary-text hover:underline" href={commitUrl} rel="noopener noreferrer" target="_blank">
          {t('buildsPage.viewCommitOnGit')}
          <ExternalLink aria-hidden="true" className="size-3" />
        </a>
      )}
    </>
  )
}
