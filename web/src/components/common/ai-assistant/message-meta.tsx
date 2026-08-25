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
  return (
    <div className={cn(page ? 'flex min-h-11 items-center gap-2 px-0.5 text-muted-foreground' : 'flex h-6 items-center gap-0.5 px-1 text-muted-foreground', align === 'end' && 'flex-row-reverse')} data-ai-message-meta data-ai-message-surface={surface}>
      {time.label && <time className={page ? 'px-1 text-xs leading-none tabular-nums' : 'px-1 text-[10px] leading-none tabular-nums'} dateTime={createdAt} title={time.title}>{time.label}</time>}
      <div className={page ? 'flex items-center gap-2' : 'flex items-center opacity-0 transition-opacity group-focus-within/message:opacity-100 group-hover/message:opacity-100 [@media(hover:none)]:opacity-100'}>
        <Button aria-label={t('aiAssistant.messageActions.copy')} className={page ? 'size-11 text-muted-foreground' : 'size-5 text-muted-foreground'} size="icon" title={t('aiAssistant.messageActions.copy')} variant="ghost" onClick={() => void copy()}>
          <Copy className="size-3" />
        </Button>
        {onResend && (
          <Button aria-label={t('aiAssistant.messageActions.resend')} className={page ? 'size-11 text-muted-foreground' : 'size-5 text-muted-foreground'} disabled={resendDisabled} size="icon" title={t('aiAssistant.messageActions.resend')} variant="ghost" onClick={onResend}>
            <RotateCcw className="size-3" />
          </Button>
        )}
      </div>
    </div>
  )
}
