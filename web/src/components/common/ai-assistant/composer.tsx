import type { KeyboardEvent, RefObject } from 'react'
import type { AIModelOption } from '@/api'
import { CircleStop, LoaderCircle, Send } from 'lucide-react'
import { useTranslation } from 'react-i18next'
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

export function AIAssistantComposer({
  activeRun,
  canceling,
  canCancel,
  models = [],
  modelAvailable = true,
  modelChanging = false,
  modelSelectionDisabled = false,
  selectedModelId,
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
        <div className="flex items-end justify-between gap-2">
          <Select
            disabled={modelSelectionDisabled || modelChanging || models.length === 0}
            value={selectedModelId ?? ''}
            onValueChange={onModelChange}
          >
            <SelectTrigger
              aria-label={t('aiAssistant.modelLabel')}
              className="!h-11 min-w-0 max-w-[70%] border-0 bg-transparent px-2 text-sm shadow-none hover:bg-surface-subtle focus:ring-0 sm:!h-9 sm:text-xs"
            >
              <SelectValue placeholder={t('aiAssistant.modelEmpty')} />
            </SelectTrigger>
            <SelectContent>
              {models.map(model => <SelectItem key={model.id} value={model.id}>{model.name}</SelectItem>)}
            </SelectContent>
          </Select>
          {activeRun && !waitingInput
            ? (
                <Button
                  aria-label={t('aiAssistant.stop')}
                  className="size-11 shrink-0 rounded-full sm:size-9"
                  disabled={canceling || !canCancel}
                  size="icon"
                  variant="outline"
                  onClick={onCancel}
                >
                  {canceling ? <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" /> : <CircleStop className="size-4" />}
                </Button>
              )
            : (
                <Button
                  aria-label={waitingInput ? t('aiAssistant.continue') : t('aiAssistant.send')}
                  className="size-11 shrink-0 rounded-full sm:size-9"
                  disabled={!draft.trim() || busy || !modelAvailable}
                  size="icon"
                  onClick={onSubmit}
                >
                  {busy ? <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" /> : <Send className="size-4" />}
                </Button>
              )}
        </div>
      </div>
      {!modelAvailable && <p className="mt-1.5 px-1 text-[10px] text-destructive">{t('aiAssistant.modelUnavailable')}</p>}
      <p className="mt-1.5 truncate px-1 text-[10px] text-muted-foreground">{t('aiAssistant.securityHint')}</p>
    </footer>
  )
}
