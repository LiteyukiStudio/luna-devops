import type { DeploymentTargetFormValues, RuntimeEnvironmentRow } from '../deployment-target-form'
import { ClipboardPaste, KeyRound, MoreHorizontal, Plus, RefreshCw, RotateCcw, Trash2, X } from 'lucide-react'
import { useState } from 'react'
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { useFieldError } from '../deployment-target-editor-utils'
import { maxRuntimeEnvironmentVariables } from '../deployment-target-form'
import { DotEnvPasteDialog } from './dotenv-paste-dialog'

const secretMask = '••••••'

/**
 * 普通配置与加密配置的统一键值表。名称在两种模式之间唯一；
 * 加密行的替换、重新生成与清除都只暂存在表单里，随部署配置一起保存。
 */
export function RuntimeEnvironmentTable() {
  const { t } = useTranslation()
  const { control, getValues, trigger, formState } = useFormContext<DeploymentTargetFormValues>()
  const { append, fields, remove, update } = useFieldArray({ control, name: 'environmentVariables' })
  const rows = useWatch({ control, name: 'environmentVariables' })
  const [pasteOpen, setPasteOpen] = useState(false)

  const updateRow = (index: number, patch: Partial<RuntimeEnvironmentRow>) => {
    update(index, { ...getValues(`environmentVariables.${index}`), ...patch })
    if (formState.isSubmitted)
      void trigger('environmentVariables')
  }

  const importEntries = (entries: Array<{ key: string, value: string }>, overwrite: boolean) => {
    const current = getValues('environmentVariables')
    for (const entry of entries) {
      const index = current.findIndex(row => row.key === entry.key && !(row.mode === 'secret' && row.secretAction === 'clear'))
      if (index < 0)
        append({ key: entry.key, mode: 'public', secretAction: 'keep', stored: 'none', value: entry.value })
      else if (overwrite && current[index]?.mode === 'public')
        update(index, { ...current[index], value: entry.value })
    }
  }

  return (
    <div className="grid min-w-0 gap-3">
      <div className="flex min-w-0 flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h4 className="text-sm font-medium text-foreground">{t('deploymentTargetEditor.environmentTableTitle')}</h4>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">{t('deploymentsPage.runtimeEnvVarsHint')}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" type="button" variant="outline" onClick={() => setPasteOpen(true)}>
            <ClipboardPaste className="size-4" />
            {t('deploymentTargetEditor.pasteDotEnv')}
          </Button>
          <Button
            disabled={fields.length >= maxRuntimeEnvironmentVariables}
            size="sm"
            type="button"
            variant="outline"
            onClick={() => append({ key: '', mode: 'public', secretAction: 'keep', stored: 'none', value: '' })}
          >
            <Plus className="size-4" />
            {t('deploymentTargetEditor.addEnvironmentVariable')}
          </Button>
        </div>
      </div>
      {fields.length === 0
        ? <p className="rounded-control border border-dashed border-border px-3 py-4 text-sm text-muted-foreground">{t('deploymentTargetEditor.environmentEmpty')}</p>
        : (
            <div className="grid gap-2">
              <div aria-hidden="true" className="hidden gap-2 px-1 text-xs font-medium text-muted-foreground xl:grid xl:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_4.5rem_2.25rem]">
                <span>{t('deploymentTargetEditor.environmentName')}</span>
                <span>{t('deploymentTargetEditor.environmentValue')}</span>
                <span>{t('deploymentTargetEditor.environmentEncrypted')}</span>
                <span />
              </div>
              {fields.map((field, index) => (
                <EnvironmentRow
                  key={field.id}
                  index={index}
                  row={rows[index] ?? field}
                  onRemove={() => remove(index)}
                  onUpdate={patch => updateRow(index, patch)}
                />
              ))}
            </div>
          )}
      <DotEnvPasteDialog open={pasteOpen} onImport={importEntries} onOpenChange={setPasteOpen} />
    </div>
  )
}

function EnvironmentRow({ index, onRemove, onUpdate, row }: {
  index: number
  row: RuntimeEnvironmentRow
  onRemove: () => void
  onUpdate: (patch: Partial<RuntimeEnvironmentRow>) => void
}) {
  const { t } = useTranslation()
  const { register } = useFormContext<DeploymentTargetFormValues>()
  const keyError = useFieldError(`environmentVariables.${index}.key`)
  const valueError = useFieldError(`environmentVariables.${index}.value`)
  const storedSecret = row.stored === 'secret'
  const secret = row.mode === 'secret'
  const clearing = secret && row.secretAction === 'clear'
  const name = row.key || t('deploymentTargetEditor.environmentRowLabel', { index: index + 1 })
  const errorId = `environment-row-${index}-error`
  const hintId = `environment-row-${index}-hint`
  const errors = [keyError, valueError].filter(Boolean)
  const reenterHint = row.mode === 'public' && storedSecret

  const toggleSecret = (checked: boolean) => {
    if (checked) {
      // 普通转加密：当前明文作为新的密钥值；已保存的密钥直接恢复为保持不变。
      onUpdate(storedSecret
        ? { mode: 'secret', secretAction: 'keep', value: '' }
        : { mode: 'secret', secretAction: 'set' })
      return
    }
    // 加密转普通：已保存的密钥无法读取原值，必须重新填写。
    onUpdate(storedSecret
      ? { mode: 'public', secretAction: 'keep', value: '' }
      : { mode: 'public', secretAction: 'keep' })
  }

  return (
    <div className={cn('grid gap-1 rounded-control', clearing && 'opacity-70')}>
      <div className="grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-2 rounded-control bg-surface-inset/60 p-2 xl:bg-transparent xl:p-0 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_4.5rem_2.25rem]">
        <Input
          {...register(`environmentVariables.${index}.key`)}
          aria-describedby={errors.length ? errorId : undefined}
          aria-invalid={keyError ? true : undefined}
          aria-label={t('deploymentTargetEditor.environmentNameFor', { index: index + 1 })}
          autoComplete="off"
          className={cn('order-1 font-mono', clearing && 'line-through')}
          disabled={storedSecret}
          placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: 'LOG_LEVEL' })}
          spellCheck={false}
        />
        <EnvironmentValueCell
          className="order-4 col-span-3 xl:order-2 xl:col-span-1"
          describedBy={[errors.length ? errorId : '', reenterHint ? hintId : ''].filter(Boolean).join(' ') || undefined}
          index={index}
          invalid={Boolean(valueError)}
          name={name}
          row={row}
          onUpdate={onUpdate}
        />
        <div className="order-2 flex items-center gap-2 xl:order-3">
          <Switch
            aria-label={t('deploymentTargetEditor.environmentEncryptFor', { name })}
            checked={secret}
            disabled={clearing}
            onCheckedChange={toggleSecret}
          />
        </div>
        <div className="order-3 flex justify-end xl:order-4">
          {storedSecret && secret
            ? (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button aria-label={t('deploymentTargetEditor.secretActionsFor', { name })} size="icon" type="button" variant="ghost">
                      <MoreHorizontal className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    {row.secretAction === 'keep'
                      ? (
                          <>
                            <DropdownMenuItem onSelect={() => onUpdate({ secretAction: 'set', value: '' })}>
                              <KeyRound className="size-4" />
                              {t('deploymentsPage.runtimeSecretReplace')}
                            </DropdownMenuItem>
                            <DropdownMenuItem onSelect={() => onUpdate({ secretAction: 'generate', value: '' })}>
                              <RefreshCw className="size-4" />
                              {t('deploymentTargetEditor.secretRegenerate')}
                            </DropdownMenuItem>
                            <DropdownMenuItem variant="destructive" onSelect={() => onUpdate({ secretAction: 'clear', value: '' })}>
                              <Trash2 className="size-4" />
                              {t('deploymentsPage.runtimeSecretClear')}
                            </DropdownMenuItem>
                          </>
                        )
                      : (
                          <DropdownMenuItem onSelect={() => onUpdate({ secretAction: 'keep', value: '' })}>
                            <RotateCcw className="size-4" />
                            {t('deploymentTargetEditor.secretUndo')}
                          </DropdownMenuItem>
                        )}
                  </DropdownMenuContent>
                </DropdownMenu>
              )
            : (
                <Button aria-label={t('deploymentTargetEditor.environmentRemoveFor', { name })} size="icon" type="button" variant="ghost" onClick={onRemove}>
                  <X className="size-4" />
                </Button>
              )}
        </div>
      </div>
      {reenterHint && <p className="text-xs text-warning" id={hintId}>{t('deploymentTargetEditor.secretToPublicHint')}</p>}
      {errors.length > 0 && <p className="text-xs text-danger" id={errorId} role="alert">{errors.join(' ')}</p>}
    </div>
  )
}

function EnvironmentValueCell({ className, describedBy, index, invalid, name, onUpdate, row }: {
  className?: string
  describedBy?: string
  index: number
  invalid: boolean
  name: string
  row: RuntimeEnvironmentRow
  onUpdate: (patch: Partial<RuntimeEnvironmentRow>) => void
}) {
  const { t } = useTranslation()
  const { register } = useFormContext<DeploymentTargetFormValues>()
  const valueLabel = t('deploymentTargetEditor.environmentValueFor', { name })

  if (row.mode === 'secret' && (row.secretAction === 'keep' || row.secretAction === 'generate' || row.secretAction === 'clear')) {
    const status = row.secretAction === 'generate'
      ? t('deploymentTargetEditor.secretPendingGenerate')
      : row.secretAction === 'clear'
        ? t('deploymentTargetEditor.secretPendingClear')
        : null
    return (
      <div className={cn('flex h-9 min-w-0 items-center gap-2 rounded-md border border-dashed border-border px-3', className)} aria-label={valueLabel} role="group">
        <span aria-hidden="true" className={cn('font-mono text-sm text-muted-foreground', row.secretAction === 'clear' && 'line-through')}>{secretMask}</span>
        <span className="sr-only">{t('common.secretValueMasked')}</span>
        {status && <StatusBadge tone={row.secretAction === 'clear' ? 'danger' : 'info'}>{status}</StatusBadge>}
        {row.secretAction === 'generate' && row.stored !== 'secret' && (
          <Button className="ml-auto" size="sm" type="button" variant="ghost" onClick={() => onUpdate({ secretAction: 'set' })}>
            {t('common.cancel')}
          </Button>
        )}
      </div>
    )
  }

  const secretInput = row.mode === 'secret'
  return (
    <div className={cn('flex min-w-0 items-center gap-1', className)}>
      <Input
        {...register(`environmentVariables.${index}.value`)}
        aria-describedby={describedBy}
        aria-invalid={invalid ? true : undefined}
        aria-label={valueLabel}
        autoComplete={secretInput ? 'new-password' : 'off'}
        className="font-mono"
        placeholder={secretInput ? t('deploymentTargetEditor.secretValuePlaceholder') : t('deploymentTargetEditor.environmentValuePlaceholder')}
        spellCheck={false}
        type={secretInput ? 'password' : 'text'}
      />
      {secretInput && row.stored !== 'secret' && (
        <Button aria-label={t('deploymentTargetEditor.secretGenerateFor', { name })} size="icon" type="button" variant="ghost" onClick={() => onUpdate({ secretAction: 'generate', value: '' })}>
          <RefreshCw className="size-4" />
        </Button>
      )}
      {secretInput && row.stored === 'secret' && (
        <Button size="sm" type="button" variant="ghost" onClick={() => onUpdate({ secretAction: 'keep', value: '' })}>
          {t('common.cancel')}
        </Button>
      )}
    </div>
  )
}
