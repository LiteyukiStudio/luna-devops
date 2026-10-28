import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckboxField } from '@/components/common/checkbox-field'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Textarea } from '@/components/ui/textarea'
import { parseDotEnv } from '@/lib/dotenv'

/** 把 .env 文本批量解析为普通配置行；只追加或覆盖普通行，加密行保持不变。 */
export function DotEnvPasteDialog({ onImport, onOpenChange, open }: {
  open: boolean
  onImport: (entries: Array<{ key: string, value: string }>, overwrite: boolean) => void
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const textId = useId()
  const [text, setText] = useState('')
  const [overwrite, setOverwrite] = useState(false)
  const parsed = parseDotEnv(text)

  const close = () => {
    setText('')
    setOverwrite(false)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={next => next ? onOpenChange(true) : close()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('deploymentTargetEditor.pasteDotEnvTitle')}</DialogTitle>
          <DialogDescription>{t('deploymentTargetEditor.pasteDotEnvDescription')}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <label className="sr-only" htmlFor={textId}>{t('deploymentTargetEditor.pasteDotEnvTitle')}</label>
          <Textarea
            className="min-h-48 font-mono text-sm"
            id={textId}
            placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: 'LOG_LEVEL=info' })}
            spellCheck={false}
            value={text}
            onChange={event => setText(event.target.value)}
          />
          <p aria-live="polite" className="text-xs text-muted-foreground">
            {t('deploymentTargetEditor.pasteDotEnvSummary', { count: parsed.entries.length })}
            {parsed.invalidLines.length > 0 && ` ${t('deploymentTargetEditor.pasteDotEnvInvalid', { lines: parsed.invalidLines.join(', ') })}`}
          </p>
          <CheckboxField checked={overwrite} description={t('deploymentTargetEditor.pasteDotEnvOverwriteHint')} onCheckedChange={checked => setOverwrite(checked === true)}>
            {t('deploymentTargetEditor.pasteDotEnvOverwrite')}
          </CheckboxField>
        </div>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={close}>{t('common.cancel')}</Button>
          <Button
            disabled={parsed.entries.length === 0}
            type="button"
            onClick={() => {
              onImport(parsed.entries, overwrite)
              close()
            }}
          >
            {t('deploymentTargetEditor.pasteDotEnvImport')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
