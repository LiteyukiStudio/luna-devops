import type { AIUIAction } from '@/api'
import { Check, LoaderCircle } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { isAIUIActionRepeatable, parseAIOptionActions } from './actions'
import { AIOptionLeadingVisual } from './option-visual'

export function AIOptionsBar({ actions, placement = 'floating', sourceKey, onAction }: {
  actions: AIUIAction[]
  placement?: 'floating' | 'inline'
  sourceKey: string
  onAction: (action: AIUIAction) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const reduceMotion = useReducedMotion()
  const [pendingKeys, setPendingKeys] = useState<Set<string>>(() => new Set())
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(() => new Set())
  const pendingKeysRef = useRef(new Set<string>())
  const selectedKeysRef = useRef(new Set<string>())
  const options = useMemo(() => parseAIOptionActions(actions), [actions])
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

  return (
    <section
      aria-label={t('aiAssistant.options.suggested')}
      className={cn(
        'min-w-0 overflow-hidden',
        placement === 'floating' && 'pointer-events-none absolute inset-x-0 bottom-0 z-10 px-3 pb-2',
      )}
      data-ai-options-placement={placement}
    >
      <AnimatePresence initial mode="popLayout">
        <motion.div
          key={sourceKey}
          animate={{ opacity: 1, x: 0 }}
          className={cn(
            'pointer-events-auto flex min-w-0 gap-2',
            placement === 'floating'
              ? 'overflow-x-auto overscroll-x-contain [scrollbar-width:none] [&::-webkit-scrollbar]:hidden'
              : 'flex-wrap py-0.5',
          )}
          exit={reduceMotion ? { opacity: 0 } : { opacity: 0, x: -12 }}
          initial={reduceMotion ? { opacity: 0 } : { opacity: 0, x: 32 }}
          transition={reduceMotion ? { duration: 0.12 } : { type: 'spring', stiffness: 420, damping: 30, mass: 0.75 }}
        >
          {options.map((action, index) => {
            const key = action.id ? `${sourceKey}:id:${action.id}:${index}` : `${sourceKey}:${action.type}-${JSON.stringify(action.payload)}-${index}`
            const selected = selectedKeys.has(key)
            const pending = pendingKeys.has(key)
            const repeatable = isAIUIActionRepeatable(action)
            const variant = action.tone === 'primary' ? 'default' : action.tone === 'danger' ? 'destructive' : 'outline'
            const label = action.label ?? t(`aiAssistant.actions.${action.type}`)
            return (
              <motion.div
                key={key}
                animate={{ opacity: 1, x: 0, scale: 1 }}
                className={placement === 'floating' ? 'shrink-0' : 'min-w-0 max-w-full'}
                initial={reduceMotion ? { opacity: 0 } : { opacity: 0, x: 24, scale: 0.94 }}
                transition={reduceMotion
                  ? { duration: 0.1 }
                  : { type: 'spring', stiffness: 460, damping: 28, mass: 0.7, delay: Math.min(index * 0.045, 0.18) }}
              >
                <Button
                  aria-pressed={selected}
                  className={cn(
                    'shrink-0 rounded-full shadow-none [&_svg]:size-3.5',
                    placement === 'floating'
                      ? 'h-8 max-w-52 px-3 !text-[11px]'
                      : 'h-7 max-w-full px-2.5 !text-xs',
                  )}
                  disabled={pending || (!repeatable && selected)}
                  title={label}
                  variant={variant}
                  onClick={() => void choose(action, key)}
                >
                  {(pending || selected || showVisuals) && (
                    <span className="grid size-4 shrink-0 place-items-center" data-ai-option-visual>
                      {pending && <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" />}
                      {!pending && selected && <Check className="size-3.5" />}
                      {!pending && !selected && showVisuals && action.visual && <AIOptionLeadingVisual visual={action.visual} />}
                    </span>
                  )}
                  <span className="truncate">{label}</span>
                  <span className="sr-only">{t('aiAssistant.options.position', { current: index + 1, total: options.length })}</span>
                </Button>
              </motion.div>
            )
          })}
        </motion.div>
      </AnimatePresence>
    </section>
  )
}
