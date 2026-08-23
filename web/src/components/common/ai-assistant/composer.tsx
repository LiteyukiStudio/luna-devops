import type { KeyboardEvent, ReactNode, RefObject } from 'react'
import type { AIRunUsage } from './state'
import type { AIModelOption } from '@/api'
import { Check, ChevronDown, CircleStop, LoaderCircle, Send } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

export interface AIAssistantComposerProps {
  activeRun: boolean
  canceling: boolean
  canCancel: boolean
  models?: AIModelOption[]
  modelAvailable?: boolean
  modelChanging?: boolean
  modelSelectionDisabled?: boolean
  selectedModelId?: string
  /** 最近一次模型调用的官方 Provider 用量；非 reported 状态不展示百分比。 */
  providerUsage?: AIRunUsage
  draft: string
  inputRef: RefObject<HTMLTextAreaElement | null>
  maxLength?: number
  sending: boolean
  submitting: boolean
  waitingInput: boolean
  onCancel: () => void
  onDraftChange: (value: string) => void
  onModelChange?: (modelId: string) => void
  onSubmit: () => void
}

function isConfirmingIME(event: KeyboardEvent<HTMLTextAreaElement>): boolean {
  return event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229
}

function formatTokenRatio(used: number, total: number): { used: string, total: string } {
  if (total < 1000)
    return { used: String(used), total: String(total) }

  const formatKValue = (value: number) => {
    const k = value / 1000
    return k >= 100 ? String(Math.round(k)) : k.toFixed(1).replace(/\.0$/, '')
  }
  return { used: formatKValue(used), total: `${formatKValue(total)}k` }
}

function TokenRing({ ratio, ariaLabel, tooltip }: { ratio: number, ariaLabel: string, tooltip: ReactNode }) {
  const clamped = Math.min(1, Math.max(0, ratio))
  // 圆环几何：r=7，周长≈43.98
  const radius = 7
  const circumference = 2 * Math.PI * radius
  const filled = clamped * circumference
  const over = ratio >= 1
  const warn = !over && ratio >= 0.8
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={ariaLabel}
            className="inline-flex size-7 shrink-0 items-center justify-center rounded-full outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <svg className="size-5 -rotate-90" viewBox="0 0 18 18" role="img">
              <circle
                className="text-separator-subtle"
                cx="9"
                cy="9"
                fill="none"
                r={radius}
                stroke="currentColor"
                strokeWidth="2"
              />
              <circle
                className={cn(
                  'transition-[stroke-dashoffset] duration-200',
                  over ? 'text-destructive' : warn ? 'text-warning' : 'text-primary',
                )}
                cx="9"
                cy="9"
                fill="none"
                r={radius}
                stroke="currentColor"
                strokeDasharray={circumference}
                strokeDashoffset={circumference - filled}
                strokeLinecap="round"
                strokeWidth="2"
              />
            </svg>
          </button>
        </TooltipTrigger>
        <TooltipContent side="top">
          {tooltip}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

function ContextUsageRing({
  ratio,
  used,
  total,
}: {
  ratio: number
  used: number
  total: number
}) {
  const { t } = useTranslation()
  const percent = Math.round(Math.min(1, Math.max(0, ratio)) * 100)
  const contextTokens = formatTokenRatio(used, total)
  const contextLabel = t('aiAssistant.contextUsage', { ...contextTokens, percent })
  return (
    <TokenRing
      ariaLabel={contextLabel}
      ratio={ratio}
      tooltip={contextLabel}
    />
  )
}

export function AIAssistantComposer({
  activeRun,
  canceling,
  canCancel,
  models = [],
  modelAvailable = true,
  modelChanging = false,
  modelSelectionDisabled = false,
  selectedModelId,
  providerUsage,
  draft,
  inputRef,
  maxLength,
  sending,
  submitting,
  waitingInput,
  onCancel,
  onDraftChange,
  onModelChange,
  onSubmit,
}: AIAssistantComposerProps) {
  const { t } = useTranslation()
  const busy = sending || submitting
  const canSubmit = modelAvailable && (!activeRun || waitingInput)
  const selectedModel = models.find(model => model.id === selectedModelId)
  const hasReportedUsage = providerUsage?.status === 'reported'
    && providerUsage.modelId === selectedModel?.id
    && typeof providerUsage.promptTokens === 'number'
    && typeof providerUsage.maxContextTokensSnapshot === 'number'
    && providerUsage.maxContextTokensSnapshot > 0
  const contextTotal = hasReportedUsage ? providerUsage.maxContextTokensSnapshot! : 0
  const contextUsed = hasReportedUsage ? providerUsage.promptTokens! : 0
  const contextRatio = hasReportedUsage ? contextUsed / contextTotal : 0
  return (
    <footer className="shrink-0 border-t border-separator-subtle bg-surface p-2.5 pb-[max(0.625rem,env(safe-area-inset-bottom))]">
      <div className="flex min-h-20 flex-col gap-1 rounded-container border border-input bg-surface px-2 py-2 focus-within:ring-2 focus-within:ring-ring">
        <textarea
          ref={inputRef}
          aria-label={t('aiAssistant.inputLabel')}
          className="min-h-10 w-full resize-none bg-transparent px-1 !text-base leading-5 outline-none placeholder:text-muted-foreground sm:!text-[13px]"
          disabled={busy}
          maxLength={maxLength}
          placeholder={waitingInput ? t('aiAssistant.inputRequired') : activeRun ? t('aiAssistant.inputRunning') : t('aiAssistant.inputPlaceholder')}
          value={draft}
          onChange={event => onDraftChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !event.shiftKey && !isConfirmingIME(event) && draft.trim() && canSubmit) {
              event.preventDefault()
              onSubmit()
            }
          }}
        />
        <div className="flex items-center justify-end gap-1.5">
          <Select
            disabled={modelSelectionDisabled || modelChanging || models.length === 0}
            value={selectedModelId ?? ''}
            onValueChange={onModelChange}
          >
            <SelectTrigger
              aria-label={t('aiAssistant.modelLabel')}
              className="!h-7 w-auto min-w-0 max-w-[55%] gap-1 rounded-full border border-separator-subtle bg-surface-subtle px-2.5 text-xs shadow-none hover:bg-surface-inset focus:ring-1 focus:ring-ring [&_svg]:hidden"
            >
              <span className="truncate">
                <SelectValue placeholder={t('aiAssistant.modelEmpty')} />
              </span>
              {modelChanging
                ? <LoaderCircle className="size-3 shrink-0 animate-spin text-muted-foreground motion-reduce:animate-none" />
                : <ChevronDown className="size-3 shrink-0 text-muted-foreground" />}
            </SelectTrigger>
            <SelectContent>
              {models.map(model => (
                <SelectItem key={model.id} value={model.id}>
                  <span className="flex items-center gap-1.5">
                    {model.id === selectedModelId && <Check className="size-3.5" />}
                    <span className={model.id === selectedModelId ? '' : 'pl-5'}>{model.name}</span>
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {hasReportedUsage
            ? (
                <ContextUsageRing
                  ratio={contextRatio}
                  total={contextTotal}
                  used={contextUsed}
                />
              )
            : <span className="px-1 text-xs text-muted-foreground">{t('aiAssistant.contextUsageUnavailable')}</span>}
          {activeRun && !waitingInput
            ? (
                <Button
                  aria-label={t('aiAssistant.stop')}
                  className="size-7 shrink-0 rounded-full"
                  disabled={canceling || !canCancel}
                  size="icon"
                  variant="outline"
                  onClick={onCancel}
                >
                  {canceling ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" /> : <CircleStop className="size-3.5" />}
                </Button>
              )
            : (
                <Button
                  aria-label={waitingInput ? t('aiAssistant.continue') : t('aiAssistant.send')}
                  className="size-7 shrink-0 rounded-full"
                  disabled={!draft.trim() || busy || !modelAvailable}
                  size="icon"
                  onClick={onSubmit}
                >
                  {busy ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" /> : <Send className="size-3.5" />}
                </Button>
              )}
        </div>
      </div>
      {!modelAvailable && <p className="mt-1.5 px-1 text-[10px] text-destructive">{t('aiAssistant.modelUnavailable')}</p>}
      <p className="mt-1.5 truncate px-1 text-[10px] text-muted-foreground">{t('aiAssistant.securityHint')}</p>
    </footer>
  )
}
