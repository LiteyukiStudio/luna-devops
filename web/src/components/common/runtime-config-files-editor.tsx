import type { RuntimeConfigFileRow } from '@/lib/runtime-config-files'
import { FilePlus2, Trash2 } from 'lucide-react'
import { useEffect, useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { duplicateRuntimeConfigPathRowIds, emptyRuntimeConfigFileRow, parseRuntimeConfigFiles, serializeRuntimeConfigFiles } from '@/lib/runtime-config-files'

export function RuntimeConfigFilesEditor({
  ariaDescribedBy,
  ariaInvalid,
  ariaLabelledBy,
  configuredPlaceholder,
  initialValue,
  onChange,
  onValidationChange,
  value,
}: {
  ariaDescribedBy?: string
  ariaInvalid?: boolean
  ariaLabelledBy?: string
  configuredPlaceholder?: string
  initialValue: string
  onChange: (value: string) => void
  onValidationChange?: (valid: boolean) => void
  value?: string
}) {
  const { t } = useTranslation()
  const baseId = useId()
  const controlledValue = value ?? initialValue
  const [draft, setDraft] = useState(() => ({
    rows: parseRuntimeConfigFiles(controlledValue),
    serialized: controlledValue,
  }))
  const rows = useMemo(
    () => draft.serialized === controlledValue ? draft.rows : parseRuntimeConfigFiles(controlledValue),
    [controlledValue, draft],
  )

  const duplicatePathRowIds = useMemo(() => duplicateRuntimeConfigPathRowIds(rows), [rows])
  const invalidPathRowIds = useMemo(() => {
    const invalid = new Set<string>()
    for (const row of rows) {
      if ((row.path.trim() || row.content.trim()) && !row.path.trim().startsWith('/'))
        invalid.add(row.id)
    }
    return invalid
  }, [rows])
  const valid = duplicatePathRowIds.size === 0 && invalidPathRowIds.size === 0

  const updateRows = (nextRows: RuntimeConfigFileRow[]) => {
    const serialized = serializeRuntimeConfigFiles(nextRows)
    setDraft({ rows: nextRows, serialized })
    onChange(serialized)
  }
  const addFile = () => {
    updateRows([...rows, emptyRuntimeConfigFileRow()])
  }
  const updateFile = (rowId: string, patch: Partial<RuntimeConfigFileRow>) => {
    updateRows(rows.map(row => row.id === rowId ? { ...row, ...patch } : row))
  }
  const removeFile = (rowId: string) => {
    updateRows(rows.filter(row => row.id !== rowId))
  }

  useEffect(() => {
    onValidationChange?.(valid)
  }, [onValidationChange, valid])

  return (
    <div aria-labelledby={ariaLabelledBy} className="grid gap-3" role={ariaLabelledBy ? 'group' : undefined}>
      {rows.length === 0
        ? (
            <div className="grid justify-items-start gap-3 rounded-container border border-border bg-surface-subtle px-3 py-4">
              <p className="text-sm text-muted-foreground">{configuredPlaceholder || t('runtimeConfigFilesEditor.empty')}</p>
              <Button size="sm" type="button" variant="secondary" onClick={addFile}>
                <FilePlus2 className="size-4" />
                {t('runtimeConfigFilesEditor.addFile')}
              </Button>
            </div>
          )
        : (
            <div className="grid gap-3">
              {rows.map((row, index) => {
                const isDuplicate = duplicatePathRowIds.has(row.id)
                const isPathFormatInvalid = invalidPathRowIds.has(row.id)
                const rowPathInvalid = isDuplicate || isPathFormatInvalid || Boolean(ariaInvalid && index === 0 && invalidPathRowIds.size === 0 && duplicatePathRowIds.size === 0)
                const duplicateErrorId = `${baseId}-duplicate-${row.id}`
                const describedBy = [isDuplicate ? duplicateErrorId : '', rowPathInvalid && ariaDescribedBy ? ariaDescribedBy : ''].filter(Boolean).join(' ') || undefined

                return (
                  <div key={row.id} className="grid gap-2 rounded-container border border-border bg-surface p-3">
                    <div className="flex items-center justify-between gap-3">
                      <span className="text-sm font-medium">{t('runtimeConfigFilesEditor.fileTitle', { index: index + 1 })}</span>
                      <Button size="sm" type="button" variant="ghost" onClick={() => removeFile(row.id)}>
                        <Trash2 className="size-4" />
                        {t('common.remove')}
                      </Button>
                    </div>
                    <Input
                      aria-describedby={describedBy}
                      aria-invalid={rowPathInvalid ? true : undefined}
                      aria-label={t('runtimeConfigFilesEditor.pathLabel')}
                      className="font-mono text-sm"
                      placeholder={t('runtimeConfigFilesEditor.pathPlaceholder')}
                      value={row.path}
                      onChange={event => updateFile(row.id, { path: event.target.value })}
                    />
                    {isDuplicate && (
                      <p className="text-xs text-danger" id={duplicateErrorId} role="alert">{t('runtimeConfigFilesEditor.duplicatePath')}</p>
                    )}
                    <Textarea
                      aria-label={t('runtimeConfigFilesEditor.contentLabel')}
                      className="min-h-36 font-mono text-sm"
                      placeholder={t('runtimeConfigFilesEditor.contentPlaceholder')}
                      value={row.content}
                      onChange={event => updateFile(row.id, { content: event.target.value })}
                    />
                  </div>
                )
              })}
              <Button className="justify-self-start" size="sm" type="button" variant="secondary" onClick={addFile}>
                <FilePlus2 className="size-4" />
                {t('runtimeConfigFilesEditor.addFile')}
              </Button>
            </div>
          )}
    </div>
  )
}
