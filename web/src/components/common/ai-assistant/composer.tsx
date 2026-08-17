import type { KeyboardEvent, RefObject } from 'react'
import { CircleStop, LoaderCircle, Send } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

export interface AIAssistantComposerProps {
  activeRun: boolean
  canceling: boolean
  canCancel: boolean
  modelAvailable?: boolean
  draft: string
  inputRef: RefObject<HTMLTextAreaElement | null>
  maxLength?: number
  sending: boolean
  submitting: boolean
  waitingInput: boolean
  onCancel: () => void
  onDraftChange: (value: string) => void
  onSubmit: () => void
}

function isConfirmingIME(event: KeyboardEvent<HTMLTextAreaElement>): boolean {
  return event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229
}

export function AIAssistantComposer({
  activeRun,
  canceling,
  canCancel,
  modelAvailable = true,
  draft,
  inputRef,
  maxLength,
  sending,
  submitting,
  waitingInput,
  onCancel,
  onDraftChange,
  onSubmit,
}: AIAssistantComposerProps) {
  const { t } = useTranslation()
  const busy = sending || submitting
  const canSubmit = modelAvailable && (!activeRun || waitingInput)
  return (
    <footer className="shrink-0 border-t border-separator-subtle bg-surface p-2.5 pb-[max(0.625rem,env(safe-area-inset-bottom))]">
      <div className="flex min-h-16 gap-2 rounded-container border border-input bg-surface px-3 py-2 focus-within:ring-2 focus-within:ring-ring">
        <textarea
          ref={inputRef}
          aria-label={t('aiAssistant.inputLabel')}
          className="min-h-10 min-w-0 flex-1 resize-none bg-transparent !text-base leading-5 outline-none placeholder:text-muted-foreground sm:!text-[13px]"
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
        {activeRun && !waitingInput
          ? (
              <Button
                aria-label={t('aiAssistant.stop')}
                className="self-end"
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
                className="self-end"
                disabled={!draft.trim() || busy || !modelAvailable}
                size="icon"
                onClick={onSubmit}
              >
                {busy ? <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" /> : <Send className="size-4" />}
              </Button>
            )}
      </div>
      {!modelAvailable && <p className="mt-1.5 px-1 text-[10px] text-destructive">{t('aiAssistant.modelUnavailable')}</p>}
      <p className="mt-1.5 truncate px-1 text-[10px] text-muted-foreground">{t('aiAssistant.securityHint')}</p>
    </footer>
  )
}
