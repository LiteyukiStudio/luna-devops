import type { AIUIAction } from '@/api'
import { ArrowUpRight, Check, LoaderCircle } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { isAIUIActionRepeatable, parseAIOptionActions } from './actions'
import { AIOptionLeadingVisual } from './option-visual'

/**
 * 助手给出的下一步选项。
 * inline：回复内的紧凑按钮组；welcome：空会话欢迎页里按页面上下文给出的建议问题卡片。
 */
export function AIOptionsBar({ actions, placement = 'inline', sourceKey, surface = 'window', onAction }: {
  actions: AIUIAction[]
  placement?: 'inline' | 'welcome'
  sourceKey: string
  surface?: 'page' | 'window'
  onAction: (action: AIUIAction) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const reduceMotion = useReducedMotion()
  const [pendingKeys, setPendingKeys] = useState<Set<string>>(() => new Set())
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(() => new Set())
  const pendingKeysRef = useRef(new Set<string>())
  const selectedKeysRef = useRef(new Set<string>())
  const options = useMemo(() => parseAIOptionActions(actions), [actions])
  const page = surface === 'page'
  const visualType = options[0]?.visual?.type
  const showVisuals = Boolean(visualType && options.every(option => option.visual?.type === visualType))

  if (options.length === 0)
    return null

  const choose = async (action: AIUIAction, key: string) => {
    const repeatable = isAIUIActionRepeatable(action)
    if (pendingKeysRef.current.has(key) || (!repeatable && selectedKeysRef.current.has(key)))
      return
    try {
      pendingKeysRef.current.add(key)
      setPendingKeys(current => new Set(current).add(key))
      const success = await onAction(action)
      if (!success) {
        toast.error(t('aiAssistant.actions.unavailable'))
        return
      }
      if (!repeatable) {
        selectedKeysRef.current.add(key)
        setSelectedKeys(current => new Set(current).add(key))
      }
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('aiAssistant.actions.unavailable'))
    }
    finally {
      pendingKeysRef.current.delete(key)
      setPendingKeys((current) => {
        const next = new Set(current)
        next.delete(key)
        return next
      })
    }
  }

  const welcome = placement === 'welcome'
  return (
    <section
      aria-label={t('aiAssistant.options.suggested')}
      className="min-w-0"
      data-ai-options-placement={placement}
      data-ai-options-surface={surface}
    >
      <AnimatePresence initial mode="popLayout">
        <motion.div
          key={sourceKey}
          animate={{ opacity: 1, y: 0 }}
          className={cn('min-w-0', welcome ? 'grid gap-2 sm:grid-cols-2' : 'flex flex-wrap gap-2 py-0.5')}
          exit={{ opacity: 0 }}
          initial={reduceMotion ? { opacity: 0 } : { opacity: 0, y: 6 }}
          transition={{ duration: reduceMotion ? 0.1 : 0.18, ease: [0.2, 0, 0, 1] }}
        >
          {options.map((action, index) => {
            const key = action.id ? `${sourceKey}:id:${action.id}:${index}` : `${sourceKey}:${action.type}-${JSON.stringify(action.payload)}-${index}`
            const selected = selectedKeys.has(key)
            const pending = pendingKeys.has(key)
            const repeatable = isAIUIActionRepeatable(action)
            const label = action.label ?? t(`aiAssistant.actions.${action.type}`)
            const visual = (pending || selected || showVisuals) && (
              <span className="grid size-4 shrink-0 place-items-center" data-ai-option-visual>
                {pending && <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" />}
                {!pending && selected && <Check className="size-3.5" />}
                {!pending && !selected && showVisuals && action.visual && <AIOptionLeadingVisual visual={action.visual} />}
              </span>
            )
            const position = <span className="sr-only">{t('aiAssistant.options.position', { current: index + 1, total: options.length })}</span>
            if (welcome) {
              const detail = action.type === 'send_message' ? action.payload.message : action.description
              return (
                <Button
                  key={key}
                  aria-pressed={selected}
                  className={cn(
                    'h-auto min-h-11 w-full min-w-0 items-start justify-start gap-2.5 whitespace-normal rounded-container border border-border bg-surface px-3.5 py-3 text-left font-normal text-foreground shadow-xs hover:border-primary-border hover:bg-primary-subtle/40 [&_svg]:size-3.5',
                    page && 'pointer-coarse:min-h-14',
                  )}
                  disabled={pending || (!repeatable && selected)}
                  variant="ghost"
                  onClick={() => void choose(action, key)}
                >
                  {visual || <ArrowUpRight aria-hidden="true" className="mt-0.5 text-muted-foreground" />}
                  <span className="grid min-w-0 flex-1 gap-0.5">
                    <span className="truncate text-sm font-medium">{label}</span>
                    {detail && <span className="line-clamp-2 text-xs leading-5 text-muted-foreground">{detail}</span>}
                  </span>
                  {position}
                </Button>
              )
            }
            const variant = action.tone === 'primary' ? 'secondary' : action.tone === 'danger' ? 'destructive' : 'outline'
            return (
              <Button
                key={key}
                aria-pressed={selected}
                className={cn(
                  'min-w-0 max-w-full shrink-0 shadow-none [&_svg]:size-3.5',
                  page ? 'min-h-11 px-3 text-sm pointer-fine:h-8 pointer-fine:min-h-0 pointer-fine:text-xs' : 'h-8 px-3 text-xs',
                )}
                disabled={pending || (!repeatable && selected)}
                title={label}
                variant={variant}
                onClick={() => void choose(action, key)}
              >
                {visual}
                <span className="truncate">{label}</span>
                {position}
              </Button>
            )
          })}
        </motion.div>
      </AnimatePresence>
    </section>
  )
}
