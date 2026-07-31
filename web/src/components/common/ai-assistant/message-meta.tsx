import { Copy, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface AIMessageMetaProps {
  align: 'start' | 'end'
  copyText: string
  createdAt: string
  onResend?: () => void
  resendDisabled?: boolean
}

function formatMessageTime(value: string, locale: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return { label: '', title: '' }
  return {
    label: new Intl.DateTimeFormat(locale, { hour: '2-digit', minute: '2-digit' }).format(date),
    title: new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(date),
  }
}

export function AIMessageMeta({ align, copyText, createdAt, onResend, resendDisabled }: AIMessageMetaProps) {
  const { i18n, t } = useTranslation()
  const time = formatMessageTime(createdAt, i18n.language)
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(copyText)
      toast.success(t('aiAssistant.messageActions.copied'))
    }
    catch {
      toast.error(t('aiAssistant.messageActions.copyFailed'))
    }
  }
  return (
    <div className={cn('flex h-6 items-center gap-0.5 px-1 text-muted-foreground', align === 'end' && 'flex-row-reverse')} data-ai-message-meta>
      {time.label && <time className="px-1 text-[10px] leading-none tabular-nums" dateTime={createdAt} title={time.title}>{time.label}</time>}
      <div className="flex items-center opacity-0 transition-opacity group-focus-within/message:opacity-100 group-hover/message:opacity-100 [@media(hover:none)]:opacity-100">
        <Button aria-label={t('aiAssistant.messageActions.copy')} className="size-6 text-muted-foreground" size="icon" title={t('aiAssistant.messageActions.copy')} variant="ghost" onClick={() => void copy()}>
          <Copy className="size-3.5" />
        </Button>
        {onResend && (
          <Button aria-label={t('aiAssistant.messageActions.resend')} className="size-6 text-muted-foreground" disabled={resendDisabled} size="icon" title={t('aiAssistant.messageActions.resend')} variant="ghost" onClick={onResend}>
            <RotateCcw className="size-3.5" />
          </Button>
        )}
      </div>
    </div>
  )
}
