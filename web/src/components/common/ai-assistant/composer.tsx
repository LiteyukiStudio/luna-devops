import type { KeyboardEvent, RefObject } from 'react'
import type { AIContextUsage, AIModelOption } from '@/api'
import { Check, ChevronDown, CircleStop, LoaderCircle, Send } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { UsageRing } from '@/components/common/usage-ring'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

export interface AIAssistantComposerProps {
  activeRun: boolean
  canceling: boolean
  canCancel: boolean
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
  maxLength?: number
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
  models = [],
  modelAvailable = true,
  modelChanging = false,
  modelSelectionDisabled = false,
  selectedModelId,
  isNewConversation = false,
  contextUsage,
  draft,
  inputRef,
  maxLength,
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
  return (
    <footer
      className={page
        ? 'shrink-0 border-t border-separator-subtle bg-surface pb-[max(0.75rem,env(safe-area-inset-bottom))] pl-[max(0.75rem,env(safe-area-inset-left))] pr-[max(0.75rem,env(safe-area-inset-right))] pt-3'
        : 'shrink-0 border-t border-separator-subtle bg-surface p-2.5 pb-[max(0.625rem,env(safe-area-inset-bottom))]'}
      data-ai-assistant-surface={surface}
    >
      <div
        className={page
          ? 'flex min-h-24 flex-col gap-2 rounded-container border border-input bg-surface px-3 py-2.5 focus-within:ring-2 focus-within:ring-ring'
          : 'flex min-h-20 flex-col gap-1 rounded-container border border-input bg-surface px-2 py-2 focus-within:ring-2 focus-within:ring-ring'}
      >
        <textarea
          ref={inputRef}
          aria-label={t('aiAssistant.inputLabel')}
          className={page
            ? 'min-h-11 max-h-36 w-full resize-none overflow-y-auto bg-transparent px-1 !text-base leading-6 outline-none placeholder:text-muted-foreground'
            : 'min-h-10 w-full resize-none bg-transparent px-1 !text-base leading-5 outline-none placeholder:text-muted-foreground sm:!text-[13px]'}
          disabled={busy}
          maxLength={maxLength}
          placeholder={waitingInput ? t('aiAssistant.inputRequired') : activeRun ? t('aiAssistant.inputRunning') : t('aiAssistant.inputPlaceholder')}
          value={draft}
          onChange={event => onDraftChange(event.target.value)}
          onKeyDown={(event) => {
            const submitShortcut = page ? event.metaKey || event.ctrlKey : !event.shiftKey
            if (event.key === 'Enter' && submitShortcut && !isConfirmingIME(event) && draft.trim() && canSubmit) {
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
              className={page
                ? '!h-11 w-auto min-w-0 max-w-[65%] gap-1 rounded-full border border-separator-subtle bg-surface-subtle px-3 text-sm shadow-none hover:bg-surface-inset focus:ring-1 focus:ring-ring [&_svg]:hidden'
                : '!h-7 w-auto min-w-0 max-w-[55%] gap-1 rounded-full border border-separator-subtle bg-surface-subtle px-2.5 text-xs shadow-none hover:bg-surface-inset focus:ring-1 focus:ring-ring [&_svg]:hidden'}
            >
              <span className="truncate">
                <SelectValue placeholder={t('aiAssistant.modelEmpty')} />
              </span>
              {modelChanging
                ? <LoaderCircle className="size-3 shrink-0 animate-spin text-muted-foreground motion-reduce:animate-none" />
                : <ChevronDown className="size-3 shrink-0 text-muted-foreground" />}
            </SelectTrigger>
            <SelectContent className={page ? '[&_[data-slot=select-item]]:min-h-11' : undefined}>
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
          <div className={page ? 'grid size-11 shrink-0 place-items-center [&_button]:size-11' : 'contents'}>
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
                  className={page ? 'size-12 shrink-0 rounded-full' : 'size-7 shrink-0 rounded-full'}
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
                  className={page ? 'size-11 shrink-0 rounded-full' : 'size-7 shrink-0 rounded-full'}
                  disabled={!draft.trim() || busy || !modelAvailable}
                  size="icon"
                  onClick={onSubmit}
                >
                  {busy ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" /> : <Send className="size-3.5" />}
                </Button>
              )}
        </div>
      </div>
      {!modelAvailable && <p className={page ? 'mt-2 px-1 text-sm text-destructive' : 'mt-1.5 px-1 text-[10px] text-destructive'}>{t('aiAssistant.modelUnavailable')}</p>}
      <p className={page ? 'mt-2 px-1 text-xs leading-5 text-muted-foreground' : 'mt-1.5 truncate px-1 text-[10px] text-muted-foreground'}>{t('aiAssistant.securityHint')}</p>
    </footer>
  )
}
