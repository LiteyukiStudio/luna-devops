import type { UseFormRegisterReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { ApplicationIconPicker } from '@/components/common/application-icon-picker'
import { FormField as Field } from '@/components/common/form-field'
import { Input } from '@/components/ui/input'
import { APPLICATION_IDENTIFIER_MAX_LENGTH, APPLICATION_IDENTIFIER_MIN_LENGTH } from '@/lib/identifier-limits'

export function ApplicationBasicFields({
  compact = false,
  icon,
  identifierError,
  identifierField,
  identifierMaxLength,
  identifierReadOnly = false,
  nameError,
  nameField,
  onIconChange,
}: {
  compact?: boolean
  icon?: string
  identifierError?: string
  identifierField: UseFormRegisterReturn<'identifier'>
  identifierMaxLength?: number
  identifierReadOnly?: boolean
  nameError?: string
  nameField: UseFormRegisterReturn<'name'>
  onIconChange: (icon: string) => void
}) {
  const { t } = useTranslation()
  const normalizedIdentifierMaxLength = identifierMaxLength ?? APPLICATION_IDENTIFIER_MAX_LENGTH

  if (compact) {
    return (
      <div className="grid gap-3">
        <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-3">
          <Field hint={t('apps.iconHint')} label={t('apps.icon')}>
            <ApplicationIconPicker compact value={icon} onChange={onIconChange} />
          </Field>
          <Field error={nameError} hint={t('apps.nameHint')} label={t('apps.name')} required>
            <Input {...nameField} aria-invalid={Boolean(nameError)} placeholder={t('apps.namePlaceholder')} />
          </Field>
        </div>
        <Field error={identifierError} hint={t('apps.identifierHint', { min: APPLICATION_IDENTIFIER_MIN_LENGTH, max: normalizedIdentifierMaxLength })} label={t('apps.identifier')} required>
          <Input
            {...identifierField}
            aria-invalid={Boolean(identifierError)}
            maxLength={normalizedIdentifierMaxLength}
            minLength={APPLICATION_IDENTIFIER_MIN_LENGTH}
            placeholder={t('apps.identifierPlaceholder')}
            readOnly={identifierReadOnly}
          />
        </Field>
      </div>
    )
  }

  return (
    <>
      <div className="grid gap-3 md:grid-cols-2">
        <Field error={nameError} hint={t('apps.nameHint')} label={t('apps.name')} required>
          <Input {...nameField} aria-invalid={Boolean(nameError)} placeholder={t('apps.namePlaceholder')} />
        </Field>
        <Field error={identifierError} hint={t('apps.identifierHint', { min: APPLICATION_IDENTIFIER_MIN_LENGTH, max: normalizedIdentifierMaxLength })} label={t('apps.identifier')} required>
          <Input
            {...identifierField}
            aria-invalid={Boolean(identifierError)}
            maxLength={normalizedIdentifierMaxLength}
            minLength={APPLICATION_IDENTIFIER_MIN_LENGTH}
            placeholder={t('apps.identifierPlaceholder')}
            readOnly={identifierReadOnly}
          />
        </Field>
      </div>
      <Field hint={t('apps.iconHint')} label={t('apps.icon')}>
        <ApplicationIconPicker value={icon} onChange={onIconChange} />
      </Field>
    </>
  )
}
