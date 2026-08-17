import type { KeyValueTextError } from '@/lib/key-value-text'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Textarea } from '@/components/ui/textarea'
import { formatKeyValueText, parseKeyValueText } from '@/lib/key-value-text'

export function KeyValueTextEditor({ initialValue, onChange, onValidationChange, placeholder }: {
  initialValue: Record<string, string>
  onChange: (value: Record<string, string>) => void
  onValidationChange?: (valid: boolean) => void
  placeholder?: string
}) {
  const { t } = useTranslation()
  const [text, setText] = useState(() => formatKeyValueText(initialValue))
  const [error, setError] = useState<KeyValueTextError | null>(null)

  return (
    <div className="grid gap-2">
      <Textarea
        className="min-h-24 font-mono text-sm"
        value={text}
        placeholder={placeholder}
        onChange={(event) => {
          const nextText = event.target.value
          setText(nextText)
          try {
            onChange(parseKeyValueText(nextText))
            setError(null)
            onValidationChange?.(true)
          }
          catch (nextError) {
            setError(nextError as KeyValueTextError)
            onValidationChange?.(false)
          }
        }}
      />
      {error && <p className="text-sm text-destructive">{t(`common.${error.message}`)}</p>}
    </div>
  )
}
