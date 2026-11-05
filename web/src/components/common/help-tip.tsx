import type { ReactNode } from 'react'
import i18next from 'i18next'
import { CircleHelp } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

/**
 * 标题、标签旁的问号说明。
 * 用于把字段、分组、开关的解释收进 tooltip，降低表单第一眼的信息量；说明同时以隐藏文本形式
 * 通过 id 提供给 aria-describedby。问号按钮不进入 Tab 序列，避免长表单出现大量额外焦点。
 */
export function HelpTip({ children, id, label }: { children: ReactNode, id?: string, label: string }) {
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            aria-label={`${label}${i18next.t('common.helpSuffix')}`}
            className="size-auto shrink-0 rounded-sm p-0 align-middle text-muted-foreground hover:bg-transparent hover:text-primary-text focus:text-primary-text focus-visible:ring-0 [&_svg]:size-3.5"
            size="icon"
            tabIndex={-1}
            type="button"
            variant="ghost"
          >
            <CircleHelp className="size-3.5 transition" />
          </Button>
        </TooltipTrigger>
        <TooltipContent className="max-w-80 leading-5" side="top">
          {children}
        </TooltipContent>
      </Tooltip>
      {id && <span className="sr-only" id={id}>{children}</span>}
    </>
  )
}
