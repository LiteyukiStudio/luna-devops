import { Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Surface } from '@/components/common/surface'
import { Button } from '@/components/ui/button'

/** 创建或轮换后只展示一次的令牌/密钥，提供复制入口；关闭页面后不可再次查看。 */
export function OneTimeSecret({ label, value }: { label: string, value: string }) {
  const { t } = useTranslation()
  const copy = () => navigator.clipboard.writeText(value)
    .then(() => toast.success(t('common.copied')))
    .catch((error: Error) => toast.error(error.message))

  return (
    <Surface className="grid gap-2 p-4" variant="inset">
      <p className="text-sm font-medium text-foreground">{label}</p>
      <div className="flex min-w-0 items-center gap-2">
        <code className="min-w-0 flex-1 truncate rounded-control border border-border bg-surface-raised px-2 py-1.5 font-mono text-xs">{value}</code>
        <Button aria-label={t('common.copy')} size="icon" type="button" variant="outline" onClick={copy}>
          <Copy className="size-4" />
        </Button>
      </div>
    </Surface>
  )
}
