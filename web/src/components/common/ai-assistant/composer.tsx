import type { KeyboardEvent, RefObject } from 'react'
import type { AIContextUsage, AIModelOption } from '@/api'
import { ArrowUp, Check, ChevronDown, LoaderCircle, MapPin, Square } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { UsageRing } from '@/components/common/usage-ring'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { isAIInputWithinLimit } from './input-size'

export interface AIAssistantComposerProps {
  activeRun: boolean
  canceling: boolean
  canCancel: boolean
  /** 当前页面上下文提示，例如“当前页面：/projects/…”。 */
  contextLabel?: string
  models?: AIModelOption[]
  modelAvailable?: boolean
  modelChanging?: boolean
  modelSelectionDisabled?: boolean
  selectedModelId?: string
  /** 尚未发起过模型调用的新会话可以安全地展示为 0% 上下文用量。 */
  isNewConversation?: boolean
  /** 会话最近一次已确认的上下文大小；新 Run 启动时保持不变。 */
  contextUsage?: AIContextUsage
  draft: string
  inputRef: RefObject<HTMLTextAreaElement | null>
  maxInputBytes?: number
  sending: boolean
  submitting: boolean
  surface?: 'page' | 'window'
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
    <UsageRing
      ariaLabel={contextLabel}
      ratio={ratio}
      tooltip={contextLabel}
    />
  )
}

function UnavailableContextUsageRing() {
  const { t } = useTranslation()
  const unavailableLabel = t('aiAssistant.contextUsageUnavailable')
  return (
    <UsageRing
      ariaLabel={unavailableLabel}
      ratio={0}
      tooltip={unavailableLabel}
    />
  )
}

export function AIAssistantComposer({
  activeRun,
  canceling,
  canCancel,
  contextLabel,
  models = [],
  modelAvailable = true,
  modelChanging = false,
  modelSelectionDisabled = false,
  selectedModelId,
  isNewConversation = false,
  contextUsage,
  draft,
  inputRef,
  maxInputBytes,
  sending,
  submitting,
  surface = 'window',
  waitingInput,
  onCancel,
  onDraftChange,
  onModelChange,
  onSubmit,
}: AIAssistantComposerProps) {
  const { t } = useTranslation()
  const page = surface === 'page'
  const busy = sending || submitting
  const canSubmit = modelAvailable && (!activeRun || waitingInput)
  const normalizedDraft = draft.trim()
  const inputTooLarge = !isAIInputWithinLimit(normalizedDraft, maxInputBytes)
  const inputLimitLabel = maxInputBytes === undefined
    ? ''
    : maxInputBytes % 1024 === 0
      ? `${maxInputBytes / 1024} KiB`
      : `${maxInputBytes} B`
  const submitEnabled = Boolean(normalizedDraft) && !busy && canSubmit && !inputTooLarge
  const selectedModel = models.find(model => model.id === selectedModelId)
  const hasReportedUsage = contextUsage?.status === 'reported'
    && contextUsage.modelId === selectedModel?.id
    && typeof contextUsage.usedTokens === 'number'
    && typeof contextUsage.maxContextTokensSnapshot === 'number'
    && contextUsage.maxContextTokensSnapshot > 0
  const contextTotal = hasReportedUsage ? contextUsage.maxContextTokensSnapshot : 0
  const contextUsed = hasReportedUsage ? contextUsage.usedTokens : 0
  const contextRatio = hasReportedUsage ? contextUsed / contextTotal : 0
  const hasInitialContextUsage = isNewConversation
    && typeof selectedModel?.maxContextTokens === 'number'
    && selectedModel.maxContextTokens > 0
  const modelValue = (
    <>
      <span className="truncate">
        <SelectValue placeholder={t('aiAssistant.modelEmpty')} />
      </span>
      {modelChanging
        ? <LoaderCircle className="size-3 shrink-0 animate-spin motion-reduce:animate-none" />
        : <ChevronDown className="size-3 shrink-0 opacity-70" />}
    </>
  )
  return (
    <footer
      className={page
        ? 'shrink-0 bg-surface pb-[max(0.75rem,env(safe-area-inset-bottom))] pl-[max(0.75rem,env(safe-area-inset-left))] pr-[max(0.75rem,env(safe-area-inset-right))] pt-1'
        : 'shrink-0 bg-surface px-3 pb-2.5 pt-1'}
      data-ai-assistant-surface={surface}
    >
      <div className="mx-auto w-full max-w-3xl">
        <div
          className={cn(
            'flex flex-col rounded-container border border-input bg-surface shadow-xs transition-[border-color,box-shadow] duration-fast focus-within:border-primary-border focus-within:ring-3 focus-within:ring-primary/15',
            inputTooLarge && 'border-danger-border focus-within:border-danger-border focus-within:ring-danger/15',
            busy && 'bg-surface-subtle',
          )}
          data-ai-composer-state={inputTooLarge ? 'invalid' : busy ? 'sending' : activeRun && !waitingInput ? 'running' : 'idle'}
        >
          <textarea
            ref={inputRef}
            aria-describedby={inputTooLarge ? 'ai-assistant-input-size-error' : undefined}
            aria-invalid={inputTooLarge}
            aria-label={t('aiAssistant.inputLabel')}
            className={page
              ? 'min-h-12 max-h-40 w-full resize-none overflow-y-auto bg-transparent px-3.5 pt-3 !text-base leading-6 outline-none [field-sizing:content] placeholder:text-muted-foreground disabled:cursor-not-allowed'
              : 'min-h-11 max-h-36 w-full resize-none overflow-y-auto bg-transparent px-3 pt-2.5 !text-base leading-5.5 outline-none [field-sizing:content] placeholder:text-muted-foreground disabled:cursor-not-allowed sm:!text-[13px]'}
            disabled={busy}
            placeholder={waitingInput ? t('aiAssistant.inputRequired') : activeRun ? t('aiAssistant.inputRunning') : t('aiAssistant.inputPlaceholder')}
            rows={1}
            value={draft}
            onChange={event => onDraftChange(event.target.value)}
            onKeyDown={(event) => {
              const submitShortcut = page ? event.metaKey || event.ctrlKey : !event.shiftKey
              if (event.key === 'Enter' && submitShortcut && !isConfirmingIME(event) && submitEnabled) {
                event.preventDefault()
                onSubmit()
              }
            }}
          />
          {inputTooLarge && (
            <p
              id="ai-assistant-input-size-error"
              className={cn('px-3.5 pt-1 text-danger', page ? 'text-sm' : 'text-xs')}
              role="alert"
            >
              {t('aiAssistant.inputTooLarge', { limit: inputLimitLabel })}
            </p>
          )}
          <div className={cn('flex min-w-0 items-center gap-1', page ? 'px-1.5 pb-1 pointer-fine:px-2 pointer-fine:pb-2' : 'px-2 pb-2')}>
            <Select
              disabled={modelSelectionDisabled || modelChanging || models.length === 0}
              value={selectedModelId ?? ''}
              onValueChange={onModelChange}
            >
              <SelectTrigger
                aria-label={t('aiAssistant.modelLabel')}
                className={page
                  ? 'group/model !h-11 w-auto min-w-0 max-w-[45%] shrink rounded-control border-0 bg-transparent p-0 shadow-none hover:bg-transparent focus-visible:ring-2 focus-visible:ring-ring pointer-fine:!h-7 [&>svg]:hidden'
                  : '!h-7 w-auto min-w-0 max-w-[45%] shrink gap-1 rounded-control border-0 bg-transparent px-2 text-xs text-muted-foreground shadow-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring [&>svg]:hidden'}
              >
                {page
                  ? (
                      <span className="flex h-7 min-w-0 max-w-full items-center gap-1 rounded-control px-2.5 text-xs text-muted-foreground transition-colors group-hover/model:bg-muted group-hover/model:text-foreground" data-ai-composer-model-visual>
                        {modelValue}
                      </span>
                    )
                  : modelValue}
              </SelectTrigger>
              <SelectContent className={page ? '[&_[data-slot=select-item]]:min-h-11 pointer-fine:[&_[data-slot=select-item]]:min-h-0' : undefined}>
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
            {contextLabel
              ? (
                  <span className="flex min-w-0 flex-1 items-center gap-1 px-1 text-[11px] text-muted-foreground" title={contextLabel}>
                    <MapPin aria-hidden="true" className="size-3 shrink-0" />
                    <span className="truncate">{contextLabel}</span>
                  </span>
                )
              : <span className="flex-1" />}
            {normalizedDraft && (
              <span className={cn('shrink-0 px-1 text-[11px] tabular-nums', inputTooLarge ? 'text-danger' : 'text-muted-foreground')}>
                {t('aiAssistant.inputLength', { count: draft.length })}
              </span>
            )}
            <div className={page ? 'grid size-11 shrink-0 place-items-center pointer-fine:size-8 [&_button]:size-11 pointer-fine:[&_button]:size-8' : 'contents'}>
              {hasReportedUsage
                ? (
                    <ContextUsageRing
                      ratio={contextRatio}
                      total={contextTotal}
                      used={contextUsed}
                    />
                  )
                : hasInitialContextUsage
                  ? (
                      <ContextUsageRing
                        ratio={0}
                        total={selectedModel.maxContextTokens}
                        used={0}
                      />
                    )
                  : <UnavailableContextUsageRing />}
            </div>
            {activeRun && !waitingInput
              ? (
                  <Button
                    aria-label={t('aiAssistant.stop')}
                    className={page ? 'group size-11 shrink-0 rounded-full p-0 hover:bg-transparent pointer-fine:size-8' : 'size-8 shrink-0 rounded-full border border-border bg-surface text-foreground hover:bg-muted'}
                    disabled={canceling || !canCancel}
                    size="icon"
                    title={t('aiAssistant.stop')}
                    variant="ghost"
                    onClick={onCancel}
                  >
                    {page
                      ? (
                          <span className="grid size-7 place-items-center rounded-full border border-border bg-surface text-foreground transition-colors group-hover:bg-muted" data-ai-composer-stop-visual>
                            {canceling ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" /> : <Square className="size-3 fill-current" />}
                          </span>
                        )
                      : canceling ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" /> : <Square className="size-3 fill-current" />}
                  </Button>
                )
              : (
                  <Button
                    aria-label={waitingInput ? t('aiAssistant.continue') : t('aiAssistant.send')}
                    className={page ? 'group size-11 shrink-0 rounded-full p-0 hover:bg-transparent pointer-fine:size-8' : 'size-8 shrink-0 rounded-full disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100'}
                    disabled={!submitEnabled}
                    size="icon"
                    title={waitingInput ? t('aiAssistant.continue') : t('aiAssistant.send')}
                    variant={page ? 'ghost' : 'default'}
                    onClick={onSubmit}
                  >
                    {page
                      ? (
                          <span className="grid size-7 place-items-center rounded-full bg-primary text-primary-foreground transition-colors group-hover:bg-primary-hover group-disabled:bg-muted group-disabled:text-muted-foreground" data-ai-composer-submit-visual>
                            {busy ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" /> : <ArrowUp className="size-4" />}
                          </span>
                        )
                      : busy ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" /> : <ArrowUp className="size-4" />}
                  </Button>
                )}
          </div>
        </div>
        {!modelAvailable && <p className={cn('mt-1.5 px-1 text-danger', page ? 'text-sm' : 'text-xs')}>{t('aiAssistant.modelUnavailable')}</p>}
        <p className={cn('mt-1.5 truncate px-1 text-center text-muted-foreground', page ? 'text-xs' : 'text-[11px]')}>{t('aiAssistant.securityHint')}</p>
      </div>
    </footer>
  )
}
