import type { CSSProperties, ReactNode, UIEvent } from 'react'
import type { LogLine } from './log-lines'
import { ArrowDownToLine, CircleAlert, Copy, Download, ListOrdered, LoaderCircle, RotateCw, ScrollText, WrapText } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { parseLogLines, stripAnsi } from './log-lines'
import { TerminalPlaceholder, TerminalPlaceholderButton, ViewerToolbar, ViewerToolbarButton } from './terminal-chrome'
import { downloadTextFile } from './text-file-actions'

const bottomThreshold = 32

export type LogViewerState = 'error' | 'loading' | 'ready'

/**
 * 统一的只读日志查看器：浅色工具栏 + 深色日志画布。
 * live 时显示“跟随最新”开关；用户向上滚动会暂停跟随，滚回底部或点击开关恢复。
 */
export function LogViewer({
  className,
  content,
  downloadName,
  emptyLabel,
  errorLabel,
  live = false,
  loadingLabel,
  meta,
  resetKey,
  state,
  onRetry,
}: {
  className?: string
  content?: string
  downloadName: string
  emptyLabel: string
  errorLabel?: string
  live?: boolean
  loadingLabel?: string
  meta?: ReactNode
  resetKey: string
  state: LogViewerState
  onRetry?: () => void
}) {
  const { t } = useTranslation()
  const viewportRef = useRef<HTMLDivElement>(null)
  const [followState, setFollowState] = useState({ following: true, resetKey })
  const following = followState.resetKey === resetKey ? followState.following : true
  const [wrap, setWrap] = useState(false)
  const [lineNumbers, setLineNumbers] = useState(true)
  const visibleContent = state === 'error' ? '' : content ?? ''
  const lines = useMemo(() => parseLogLines(visibleContent), [visibleContent])
  const hasContent = visibleContent.length > 0

  const scrollToBottom = () => {
    const viewport = viewportRef.current
    if (viewport)
      viewport.scrollTop = viewport.scrollHeight
  }

  // 切换日志对象时回到底部；跟随开启时新输出到达后滚动到底部。
  useEffect(() => {
    if (!following)
      return
    const frame = requestAnimationFrame(() => {
      const viewport = viewportRef.current
      if (viewport)
        viewport.scrollTop = viewport.scrollHeight
    })
    return () => cancelAnimationFrame(frame)
  }, [following, resetKey, visibleContent])

  const handleScroll = (event: UIEvent<HTMLDivElement>) => {
    const viewport = event.currentTarget
    const atBottom = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight <= bottomThreshold
    if (atBottom !== following)
      setFollowState({ following: atBottom, resetKey })
  }

  const toggleFollow = () => {
    const next = !following
    setFollowState({ following: next, resetKey })
    if (next)
      scrollToBottom()
  }

  const copyLogs = async () => {
    try {
      await navigator.clipboard.writeText(stripAnsi(visibleContent))
      toast.success(t('common.copied'))
    }
    catch {
      toast.error(t('common.copyFailed'))
    }
  }

  const body = (() => {
    if (state === 'error') {
      return (
        <TerminalPlaceholder
          action={onRetry && (
            <TerminalPlaceholderButton icon={<RotateCw aria-hidden="true" />} onClick={onRetry}>
              {t('common.retry')}
            </TerminalPlaceholderButton>
          )}
          icon={<CircleAlert />}
          label={errorLabel ?? t('common.unavailable')}
          tone="danger"
        />
      )
    }
    if (!hasContent && state === 'loading')
      return <TerminalPlaceholder icon={<LoaderCircle className="motion-safe:animate-spin" />} label={loadingLabel ?? t('common.loading')} />
    if (!hasContent)
      return <TerminalPlaceholder icon={<ScrollText />} label={emptyLabel} />
    return (
      <div
        ref={viewportRef}
        aria-label={t('common.logViewer.label')}
        className="h-full min-h-0 overflow-auto bg-terminal py-3 font-mono text-xs leading-5 text-terminal-foreground selection:bg-terminal-selection focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary"
        role="log"
        tabIndex={0}
        onScroll={handleScroll}
      >
        <div className={cn(wrap ? 'w-full' : 'w-max min-w-full')} style={{ '--log-gutter': `${String(lines.length).length + 3}ch` } as CSSProperties}>
          {lines.map((line, index) => (
            // 日志行没有稳定 ID，追加式输出下行号即身份。
            // eslint-disable-next-line react/no-array-index-key
            <LogRow key={index} line={line} lineNumber={lineNumbers ? index + 1 : undefined} wrap={wrap} />
          ))}
        </div>
      </div>
    )
  })()

  return (
    <div className={cn('flex min-h-0 flex-col', className)}>
      <ViewerToolbar
        actions={(
          <>
            {live && (
              <ViewerToolbarButton
                icon={<ArrowDownToLine aria-hidden="true" />}
                label={t('common.logViewer.follow')}
                pressed={following}
                onClick={toggleFollow}
              />
            )}
            <ViewerToolbarButton icon={<WrapText aria-hidden="true" />} label={t('common.logViewer.wrap')} pressed={wrap} onClick={() => setWrap(value => !value)} />
            <ViewerToolbarButton icon={<ListOrdered aria-hidden="true" />} label={t('common.logViewer.lineNumbers')} pressed={lineNumbers} onClick={() => setLineNumbers(value => !value)} />
            <ViewerToolbarButton disabled={!hasContent} icon={<Copy aria-hidden="true" />} label={t('common.logViewer.copy')} onClick={() => void copyLogs()} />
            <ViewerToolbarButton disabled={!hasContent} icon={<Download aria-hidden="true" />} label={t('common.logViewer.download')} onClick={() => downloadTextFile(downloadName, stripAnsi(visibleContent))} />
          </>
        )}
      >
        {meta}
      </ViewerToolbar>
      <div className="min-h-0 flex-1 bg-terminal">{body}</div>
    </div>
  )
}

const toneClassName = {
  error: 'text-terminal-error',
  info: 'text-terminal-info',
  muted: 'text-terminal-muted',
  success: 'text-terminal-success',
  warning: 'text-terminal-warning',
} as const

function LogRow({ line, lineNumber, wrap }: { line: LogLine, lineNumber?: number, wrap: boolean }) {
  return (
    <div
      className={cn(
        'flex border-l-2 border-transparent pr-4',
        line.severity === 'error' && 'border-terminal-error bg-terminal-error/10 text-terminal-error',
        line.severity === 'warning' && 'border-terminal-warning bg-terminal-warning/10 text-terminal-warning',
        lineNumber === undefined && 'pl-4',
      )}
    >
      {lineNumber !== undefined && (
        <span aria-hidden="true" className="sticky left-0 w-(--log-gutter) shrink-0 select-none bg-terminal pr-3 text-right tabular-nums text-terminal-muted/70">
          {lineNumber}
        </span>
      )}
      <span className={cn('min-w-0', wrap ? 'whitespace-pre-wrap break-all' : 'whitespace-pre')}>
        {line.segments.length === 1 && !line.segments[0].tone && !line.segments[0].bold
          ? line.segments[0].text || ' '
          : line.segments.map((segment, index) => (
              // eslint-disable-next-line react/no-array-index-key
              <span key={index} className={cn(segment.bold && 'font-semibold', segment.tone && toneClassName[segment.tone])}>{segment.text}</span>
            ))}
      </span>
    </div>
  )
}
