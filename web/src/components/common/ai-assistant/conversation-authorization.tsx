import { ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

export function AIConversationAuthorizationNotice({ expiresAt, revoking, onRevoke }: {
  expiresAt: string
  revoking: boolean
  onRevoke: () => void
}) {
  const { i18n, t } = useTranslation()
  const expiry = new Intl.DateTimeFormat(i18n.language, { hour: '2-digit', minute: '2-digit' }).format(new Date(expiresAt))
  return (
    <div className="flex items-center gap-2 rounded-control bg-success-subtle px-3 py-2 text-xs text-success" data-ai-conversation-authorization>
      <ShieldCheck aria-hidden="true" className="size-4 shrink-0" />
      <span className="min-w-0 flex-1">{t('aiAssistant.approval.conversationActive', { expiry })}</span>
      <Button className="h-7 px-2.5 !text-[11px]" disabled={revoking} size="sm" variant="ghost" onClick={onRevoke}>
        {t('aiAssistant.approval.revokeConversation')}
      </Button>
    </div>
  )
}
