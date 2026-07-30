import type { AIUIAction } from '@/api'
import { Check, ChevronRight, LoaderCircle, MessageSquareText, Navigation, Play } from 'lucide-react'
import { useId, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { isAIUIActionRepeatable, parseAIOptionAction } from './actions'

const presentationSchema = z.object({
  title: z.string().trim().min(1).max(120),
  description: z.string().trim().max(300).optional(),
})

export function AIOptionsCard({ actions, arguments: rawArguments, onAction }: {
  actions: AIUIAction[]
  arguments: Record<string, unknown>
  onAction: (action: AIUIAction) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const titleId = useId()
  const [pendingKeys, setPendingKeys] = useState<Set<string>>(() => new Set())
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(() => new Set())
  const pendingKeysRef = useRef(new Set<string>())
  const selectedKeysRef = useRef(new Set<string>())
  const presentation = presentationSchema.safeParse(rawArguments)
  const options = useMemo(() => actions.map(parseAIOptionAction).filter(action => action !== null).slice(0, 5), [actions])

  if (!presentation.success || options.length === 0) {
    return (
      <div className="rounded-container bg-surface px-3 py-2 text-xs text-muted-foreground" role="status">
        {t('aiAssistant.options.unavailable')}
      </div>
    )
  }

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
    <section className="min-w-0 rounded-container bg-primary-subtle/60 p-2.5" aria-labelledby={titleId}>
      <div className="mb-2 px-0.5">
        <p className="text-[10px] font-medium uppercase tracking-wide text-primary-text">{t('aiAssistant.options.suggested')}</p>
        <h3 className="mt-0.5 text-xs font-semibold" id={titleId}>{presentation.data.title}</h3>
        {presentation.data.description && <p className="mt-0.5 text-[11px] leading-4 text-muted-foreground">{presentation.data.description}</p>}
      </div>
      <div className="grid gap-1.5">
        {options.map((action, index) => {
          const key = action.id ? `id:${action.id}:${index}` : `${action.type}-${JSON.stringify(action.payload)}-${index}`
          const selected = selectedKeys.has(key)
          const pending = pendingKeys.has(key)
          const repeatable = isAIUIActionRepeatable(action)
          const Icon = action.type === 'navigate' ? Navigation : action.type === 'send_message' ? MessageSquareText : Play
          const variant = action.tone === 'primary' ? 'default' : action.tone === 'danger' ? 'destructive' : 'outline'
          return (
            <Button
              key={key}
              aria-pressed={selected}
              className="h-auto min-h-8 w-full justify-start whitespace-normal px-2.5 py-1.5 text-left !text-[11px] [&_svg]:size-3.5"
              disabled={pending || (!repeatable && selected)}
              variant={variant}
              onClick={() => void choose(action, key)}
            >
              {pending ? <LoaderCircle className="animate-spin motion-reduce:animate-none" /> : selected ? <Check /> : <Icon />}
              <span className="min-w-0 flex-1">
                <span className="block text-[11px] font-medium leading-4">{action.label ?? t(`aiAssistant.actions.${action.type}`)}</span>
                {action.description && <span className="mt-0.5 block text-[10px] font-normal leading-3.5 opacity-75">{action.description}</span>}
              </span>
              {!pending && !selected && <ChevronRight className="opacity-60" />}
              {selected && <span className="text-[10px] font-normal">{t('aiAssistant.options.selected')}</span>}
              <span className="sr-only">{t('aiAssistant.options.position', { current: index + 1, total: options.length })}</span>
            </Button>
          )
        })}
      </div>
    </section>
  )
}
