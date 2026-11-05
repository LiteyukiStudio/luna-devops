import { Copy, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { formatMessageTime } from './message-time'

interface AIMessageMetaProps {
  align: 'start' | 'end'
  copyText: string
  createdAt: string
  onResend?: () => void
  resendDisabled?: boolean
  surface?: 'page' | 'window'
}

export function AIMessageMeta({ align, copyText, createdAt, onResend, resendDisabled, surface = 'window' }: AIMessageMetaProps) {
  const { i18n, t } = useTranslation()
  const page = surface === 'page'
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
  const buttonClassName = cn('text-muted-foreground hover:text-foreground [&_svg]:size-3.5', page ? 'size-11 pointer-fine:size-7' : 'size-7')
  return (
    <div className={cn('flex min-h-7 items-center gap-0.5 text-muted-foreground', page && 'min-h-11 pointer-fine:min-h-7', align === 'end' && 'flex-row-reverse')} data-ai-message-meta data-ai-message-surface={surface}>
      <div className="flex items-center opacity-0 transition-opacity duration-fast group-focus-within/message:opacity-100 group-hover/message:opacity-100 [@media(hover:none)]:opacity-100">
        <Button aria-label={t('aiAssistant.messageActions.copy')} className={buttonClassName} size="icon" title={t('aiAssistant.messageActions.copy')} variant="ghost" onClick={() => void copy()}>
          <Copy />
        </Button>
        {onResend && (
          <Button aria-label={t('aiAssistant.messageActions.resend')} className={buttonClassName} disabled={resendDisabled} size="icon" title={t('aiAssistant.messageActions.resend')} variant="ghost" onClick={onResend}>
            <RotateCcw />
          </Button>
        )}
      </div>
      {time.label && <time className="px-1 text-[11px] leading-none tabular-nums opacity-0 transition-opacity duration-fast group-focus-within/message:opacity-100 group-hover/message:opacity-100 [@media(hover:none)]:opacity-100" dateTime={createdAt} title={time.title}>{time.label}</time>}
    </div>
  )
}
