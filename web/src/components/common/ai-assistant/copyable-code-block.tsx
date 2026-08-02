import type { ReactNode } from 'react'
import { Copy } from 'lucide-react'
import { isValidElement } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

function nodeText(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number')
    return String(node)
  if (Array.isArray(node))
    return node.map(nodeText).join('')
  if (isValidElement<{ children?: ReactNode }>(node))
    return nodeText(node.props.children)
  return ''
}

export function CopyableCodeBlock({ children, className, dataSlot, value }: { children: ReactNode, className?: string, dataSlot?: string, value?: string }) {
  const { t } = useTranslation()
  const content = value ?? nodeText(children)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(content)
      toast.success(t('common.copied'))
    }
    catch {
      toast.error(t('common.copyFailed'))
    }
  }

  return (
    <div className="group relative min-w-0">
      <pre className={cn('m-0 max-w-full overflow-x-auto overflow-y-auto rounded-control bg-surface-inset p-2 pr-9 font-mono text-[11px] leading-4 text-foreground', className)} data-slot={dataSlot}>
        {children}
      </pre>
      <Button
        aria-label={t('common.copy')}
        className="absolute right-1.5 top-1.5 size-6 bg-surface/90 text-muted-foreground opacity-0 shadow-sm transition-opacity hover:bg-surface hover:text-foreground focus-visible:opacity-100 group-hover:opacity-100 group-focus-within:opacity-100"
        size="icon"
        title={t('common.copy')}
        type="button"
        variant="ghost"
        onClick={() => void copy()}
      >
        <Copy className="size-3" />
      </Button>
    </div>
  )
}
