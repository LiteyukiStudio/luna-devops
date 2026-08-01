import { History } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

interface AIRefreshConversationReturnProps {
  expiresAt: number
  onExpire: () => void
  onReturn: () => void
}

function secondsUntil(expiresAt: number): number {
  return Math.max(0, Math.ceil((expiresAt - Date.now()) / 1_000))
}

export function AIRefreshConversationReturn({ expiresAt, onExpire, onReturn }: AIRefreshConversationReturnProps) {
  const { t } = useTranslation()
  const [remainingSeconds, setRemainingSeconds] = useState(() => secondsUntil(expiresAt))

  useEffect(() => {
    const updateRemaining = () => {
      const remaining = secondsUntil(expiresAt)
      setRemainingSeconds(remaining)
      if (remaining === 0)
        onExpire()
    }
    const interval = window.setInterval(updateRemaining, 250)
    return () => window.clearInterval(interval)
  }, [expiresAt, onExpire])

  if (remainingSeconds === 0)
    return null

  return (
    <div className="sticky top-0 z-10 mb-3 flex min-w-0 items-center gap-2 rounded-control border border-warning-border bg-warning-subtle px-2.5 py-1.5 text-warning shadow-raised" role="status">
      <History className="size-3.5 shrink-0" />
      <span className="min-w-0 flex-1 truncate text-xs">{t('aiAssistant.conversations.refreshStarted')}</span>
      <Button
        aria-label={t('aiAssistant.conversations.returnPreviousLabel', { count: remainingSeconds })}
        className="h-7 shrink-0 px-2 text-xs text-warning hover:bg-warning/10 hover:text-warning"
        size="sm"
        variant="ghost"
        onClick={onReturn}
      >
        {t('aiAssistant.conversations.returnPrevious')}
        <span className="tabular-nums text-[10px] opacity-70">
          {t('aiAssistant.conversations.returnPreviousCountdown', { count: remainingSeconds })}
        </span>
      </Button>
    </div>
  )
}
