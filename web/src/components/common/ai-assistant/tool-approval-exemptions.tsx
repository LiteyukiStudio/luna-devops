import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle, ShieldCheck, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { toolDisplayName } from './tool-display-name'

const approvalExemptionsQueryKey = ['ai', 'tool-approval-exemptions'] as const

export function AIToolApprovalExemptionsDialog() {
  const { i18n, t } = useTranslation()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const exemptions = useQuery({
    queryKey: approvalExemptionsQueryKey,
    queryFn: api.listAIToolApprovalExemptions,
    enabled: open,
    staleTime: 0,
    retry: false,
  })
  const revoke = useMutation({
    mutationFn: api.revokeAIToolApprovalExemption,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: approvalExemptionsQueryKey })
      toast.success(t('aiAssistant.approvalExemptions.revokeSuccess'))
    },
    onError: () => toast.error(t('aiAssistant.approvalExemptions.revokeError')),
  })
  const items = [...(exemptions.data?.items ?? [])].sort((left, right) => left.operationId.localeCompare(right.operationId))

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          aria-label={t('aiAssistant.approvalExemptions.trigger')}
          size="icon"
          title={t('aiAssistant.approvalExemptions.trigger')}
          variant="ghost"
        >
          <ShieldCheck className="size-4" />
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('aiAssistant.approvalExemptions.title')}</DialogTitle>
          <DialogDescription>{t('aiAssistant.approvalExemptions.description')}</DialogDescription>
        </DialogHeader>
        {exemptions.isLoading && (
          <div className="flex min-h-24 items-center justify-center gap-2 text-sm text-muted-foreground" role="status">
            <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" />
            <span>{t('aiAssistant.approvalExemptions.loading')}</span>
          </div>
        )}
        {exemptions.isError && (
          <div className="grid min-h-24 place-content-center gap-2 text-center">
            <p className="text-sm text-danger">{t('aiAssistant.approvalExemptions.loadError')}</p>
            <Button size="sm" variant="outline" onClick={() => void exemptions.refetch()}>{t('common.retry')}</Button>
          </div>
        )}
        {exemptions.isSuccess && items.length === 0 && (
          <p className="py-8 text-center text-sm text-muted-foreground">{t('aiAssistant.approvalExemptions.empty')}</p>
        )}
        {items.length > 0 && (
          <ul className="grid gap-2">
            {items.map(item => (
              <li key={item.operationId} className="flex min-w-0 items-center gap-3 rounded-control bg-surface-subtle p-3">
                <div className="min-w-0 flex-1">
                  <strong className="block truncate text-sm font-medium">{toolDisplayName(t, item.operationId)}</strong>
                  <code className="block truncate text-xs text-muted-foreground">{item.operationId}</code>
                  <time className="block text-[11px] text-muted-foreground" dateTime={item.createdAt}>
                    {t('aiAssistant.approvalExemptions.createdAt', {
                      date: new Intl.DateTimeFormat(i18n.language, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(item.createdAt)),
                    })}
                  </time>
                </div>
                <Button
                  aria-label={t('aiAssistant.approvalExemptions.revokeOperation', { operationId: item.operationId })}
                  disabled={revoke.isPending}
                  size="sm"
                  variant="outline"
                  onClick={() => revoke.mutate(item.operationId)}
                >
                  {revoke.isPending && revoke.variables === item.operationId
                    ? <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" />
                    : <Trash2 className="size-4" />}
                  {t('aiAssistant.approvalExemptions.revoke')}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </DialogContent>
    </Dialog>
  )
}
