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

/** 代码、参数与返回数据统一使用终端 token 的深色块：顶部语言标签 + 常驻复制按钮。 */
export function CopyableCodeBlock({ children, className, dataSlot, language, value }: {
  children: ReactNode
  className?: string
  dataSlot?: string
  language?: string
  value?: string
}) {
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
    <div className="min-w-0 overflow-hidden rounded-control border border-terminal-border bg-terminal text-terminal-foreground" data-ai-code-block>
      <div className="flex h-8 items-center gap-2 border-b border-terminal-border pl-3 pr-1">
        <span className="min-w-0 flex-1 truncate font-mono text-[11px] lowercase text-terminal-muted">{language}</span>
        <Button
          aria-label={t('common.copy')}
          className="size-6 text-terminal-muted hover:bg-terminal-selection hover:text-terminal-foreground [&_svg]:size-3.5"
          size="icon"
          title={t('common.copy')}
          type="button"
          variant="ghost"
          onClick={() => void copy()}
        >
          <Copy />
        </Button>
      </div>
      <pre className={cn('m-0 max-w-full overflow-auto p-3 font-mono text-xs leading-5 selection:bg-terminal-selection', className)} data-slot={dataSlot}>
        {children}
      </pre>
    </div>
  )
}
