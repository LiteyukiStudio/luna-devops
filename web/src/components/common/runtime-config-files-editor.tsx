import type { RuntimeConfigFileRow } from '@/lib/runtime-config-files'
import { FilePlus2, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { duplicateRuntimeConfigPathRowIds, emptyRuntimeConfigFileRow, parseRuntimeConfigFiles, serializeRuntimeConfigFiles } from '@/lib/runtime-config-files'

export function RuntimeConfigFilesEditor({ configuredPlaceholder, initialValue, onChange, onValidationChange }: {
  configuredPlaceholder?: string
  initialValue: string
  onChange: (value: string) => void
  onValidationChange?: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const [rows, setRows] = useState<RuntimeConfigFileRow[]>(() => parseRuntimeConfigFiles(initialValue))
  const duplicatePathRowIds = useMemo(() => duplicateRuntimeConfigPathRowIds(rows), [rows])
  const valid = duplicatePathRowIds.size === 0

  const updateRows = (nextRows: RuntimeConfigFileRow[]) => {
    setRows(nextRows)
    onChange(serializeRuntimeConfigFiles(nextRows))
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
    <div className="grid gap-3">
      {rows.length === 0
        ? (
            <div className="grid justify-items-start gap-3 rounded-md border border-dashed border-border px-3 py-4">
              <p className="text-sm text-muted-foreground">{configuredPlaceholder || t('runtimeConfigFilesEditor.empty')}</p>
              <Button size="sm" type="button" variant="secondary" onClick={addFile}>
                <FilePlus2 className="size-4" />
                {t('runtimeConfigFilesEditor.addFile')}
              </Button>
            </div>
          )
        : (
            <div className="grid gap-3">
              {rows.map((row, index) => (
                <div key={row.id} className="grid gap-2 rounded-md border border-border bg-background p-3">
                  <div className="flex items-center justify-between gap-3">
                    <span className="text-sm font-medium">{t('runtimeConfigFilesEditor.fileTitle', { index: index + 1 })}</span>
                    <Button size="sm" type="button" variant="ghost" onClick={() => removeFile(row.id)}>
                      <Trash2 className="size-4" />
                      {t('common.remove')}
                    </Button>
                  </div>
                  <Input
                    aria-label={t('runtimeConfigFilesEditor.pathLabel')}
                    aria-invalid={duplicatePathRowIds.has(row.id)}
                    placeholder={t('runtimeConfigFilesEditor.pathPlaceholder')}
                    value={row.path}
                    onChange={event => updateFile(row.id, { path: event.target.value })}
                  />
                  {duplicatePathRowIds.has(row.id) && (
                    <p className="text-xs text-destructive">{t('runtimeConfigFilesEditor.duplicatePath')}</p>
                  )}
                  <Textarea
                    aria-label={t('runtimeConfigFilesEditor.contentLabel')}
                    className="min-h-36 font-mono text-sm"
                    placeholder={t('runtimeConfigFilesEditor.contentPlaceholder')}
                    value={row.content}
                    onChange={event => updateFile(row.id, { content: event.target.value })}
                  />
                </div>
              ))}
              <Button className="justify-self-start" size="sm" type="button" variant="secondary" onClick={addFile}>
                <FilePlus2 className="size-4" />
                {t('runtimeConfigFilesEditor.addFile')}
              </Button>
            </div>
          )}
    </div>
  )
}
