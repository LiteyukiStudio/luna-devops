import type { ReactNode } from 'react'
import type { AIBlock } from './state'
import { useTranslation } from 'react-i18next'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { formatMillisecondsDuration } from '@/components/common/time-format'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { CopyableCodeBlock } from './copyable-code-block'

type ToolCallBlock = Extract<AIBlock, { type: 'tool_call' }>

const SECRET_FIELD = /authorization|cookie|password|secret|token|credential/i

function displayValue(value: unknown): string {
  if (value === null || value === undefined)
    return '—'
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean')
    return String(value)
  return displayJSON(value)
}

function displayJSON(value: unknown): string {
  if (typeof value === 'string')
    return value
  try {
    return JSON.stringify(value, null, 2)
  }
  catch {
    return String(value)
  }
}

function isStructured(value: unknown): boolean {
  return typeof value === 'object' && value !== null
}

function DetailHeading({ children }: { children: ReactNode }) {
  return <h3 className="text-[11px] font-semibold text-muted-foreground">{children}</h3>
}

function MetadataItem({ copyable = false, label, value, mono = true }: { copyable?: boolean, label: string, value: string, mono?: boolean }) {
  const { t } = useTranslation()
  return (
    <div className="min-w-0 rounded-control bg-surface px-2.5 py-2">
      <dt className="text-[10px] leading-4 text-muted-foreground">{label}</dt>
      <dd className="m-0 mt-0.5 min-w-0">
        {copyable
          ? (
              <CopyableHoverText
                className={cn('break-all text-[11px] leading-4 text-foreground', mono && 'font-mono')}
                copyLabel={`${t('common.copy')} ${label}`}
                truncate={false}
                value={value}
              />
            )
          : <span className={cn('break-all text-[11px] leading-4 text-foreground', mono && 'font-mono')}>{value}</span>}
      </dd>
    </div>
  )
}

function ValueView({ value }: { value: unknown }) {
  if (!isStructured(value))
    return <span className="break-words font-medium text-foreground">{displayValue(value)}</span>
  const content = displayJSON(value)
  return (
    <CopyableCodeBlock className="max-h-48" value={content}>
      <code>{content}</code>
    </CopyableCodeBlock>
  )
}

export function AIToolCallDetails({ block, errorCode, summary }: { block: ToolCallBlock, errorCode?: string, summary: string }) {
  const { t, i18n } = useTranslation()
  const entries = Object.entries(block.arguments).filter(([key]) => !SECRET_FIELD.test(key)).slice(0, 20)
  const metadata = [
    { label: t('aiAssistant.toolIdentifier'), value: block.operationId, copyable: true },
    { label: t('aiAssistant.toolCallId'), value: block.toolCallId, copyable: true },
    { label: t('aiAssistant.runId'), value: block.runId, copyable: true },
    ...(block.traceId ? [{ label: t('aiAssistant.traceId'), value: block.traceId, copyable: true }] : []),
    ...(block.durationMs !== undefined
      ? [{ label: t('aiAssistant.duration'), value: formatMillisecondsDuration(block.durationMs, i18n.language), mono: false }]
      : []),
    ...(block.result?.requestId ? [{ label: t('aiAssistant.requestId'), value: block.result.requestId, copyable: true }] : []),
  ]

  return (
    <div className="border-t border-separator-subtle bg-surface-subtle/40 px-3 pb-3">
      <section className="mt-3 grid gap-2" aria-labelledby={`${block.id}-identifiers`}>
        <DetailHeading><span id={`${block.id}-identifiers`}>{t('aiAssistant.identifiers')}</span></DetailHeading>
        <dl className="m-0 grid min-w-0 grid-cols-1 gap-1.5 sm:grid-cols-2">
          {metadata.map(item => <MetadataItem key={item.label} {...item} />)}
        </dl>
      </section>

      <section className="mt-3 grid gap-2" aria-labelledby={`${block.id}-arguments`}>
        <DetailHeading><span id={`${block.id}-arguments`}>{t('aiAssistant.arguments')}</span></DetailHeading>
        {entries.length
          ? (
              <dl className="m-0 grid gap-0.5 rounded-control bg-surface px-2.5 py-1.5 text-xs">
                {entries.map(([key, value]) => (
                  <div key={key} className="grid min-w-0 grid-cols-[minmax(5rem,35%)_minmax(0,1fr)] gap-2 border-b border-separator-subtle py-1.5 last:border-b-0">
                    <dt className="min-w-0 truncate text-muted-foreground">{key}</dt>
                    <dd className="m-0 min-w-0"><ValueView value={value} /></dd>
                  </div>
                ))}
              </dl>
            )
          : <p className="m-0 rounded-control bg-surface px-2.5 py-2 text-xs text-muted-foreground">{t('aiAssistant.noArguments')}</p>}
      </section>

      <section className="mt-3 grid gap-2" aria-labelledby={`${block.id}-result`}>
        <DetailHeading><span id={`${block.id}-result`}>{t('aiAssistant.returnValue')}</span></DetailHeading>
        {block.result
          ? (
              <div className="grid gap-2 rounded-control bg-surface px-2.5 py-2 text-xs">
                <div className="grid gap-0.5">
                  <span className="text-[10px] text-muted-foreground">{t('aiAssistant.summary')}</span>
                  <p className="m-0 text-foreground">{summary}</p>
                </div>
                {errorCode && (
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">{t('aiAssistant.errorCode')}</span>
                    <code className="break-all text-right text-[11px]">{errorCode}</code>
                  </div>
                )}
                {block.result.errorMessage && (
                  <p className="m-0 break-words rounded-control bg-danger-subtle px-2 py-1.5 text-danger">
                    {block.result.errorMessage}
                  </p>
                )}
                {block.result.fields?.map(field => (
                  <div key={field.labelKey} className="flex justify-between gap-3">
                    <span className="text-muted-foreground">{i18n.exists(field.labelKey) ? t(field.labelKey) : field.labelKey}</span>
                    <strong className="break-all text-right">{displayValue(field.value)}</strong>
                  </div>
                ))}
                {block.result.issues && block.result.issues.length > 0 && (
                  <div className="grid gap-1.5 rounded-control bg-danger-subtle px-2 py-2">
                    <strong className="text-[11px] text-danger">{t('aiAssistant.validationDetails')}</strong>
                    <ul className="m-0 grid list-none gap-1 p-0">
                      {block.result.issues.map(issue => (
                        <li key={`${issue.path}-${issue.code}-${issue.message}`} className="grid gap-0.5 text-[11px]">
                          <code className="break-all font-semibold text-danger">{issue.path || t('aiAssistant.rootField')}</code>
                          <span className="break-words text-muted-foreground">{issue.message}</span>
                          {issue.expected && (
                            <span className="break-words text-muted-foreground">
                              {t('aiAssistant.expectedValue')}
                              :
                              {' '}
                              <code>{issue.expected}</code>
                            </span>
                          )}
                          {issue.received && (
                            <span className="break-words text-muted-foreground">
                              {t('aiAssistant.receivedValue')}
                              :
                              {' '}
                              <code>{issue.received}</code>
                            </span>
                          )}
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
                {block.result.data !== undefined && (
                  <div className="grid min-w-0 gap-1">
                    <strong className="text-[11px] text-muted-foreground">{t('aiAssistant.responseData')}</strong>
                    <ValueView value={block.result.data} />
                  </div>
                )}
              </div>
            )
          : (
              <div className="grid gap-2 rounded-control bg-surface px-2.5 py-2" aria-label={t('aiAssistant.resultPending')}>
                <Skeleton className="h-2.5 w-full" />
                <Skeleton className="h-2.5 w-2/3" />
              </div>
            )}
      </section>
    </div>
  )
}
