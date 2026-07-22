import { Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { emptyKeyValueRow } from '@/lib/build-variables'

export interface KeyValueRow {
  id: string
  key: string
  value: string
  existing?: boolean
}

export function KeyValueRowsEditor({ onChange, rows, secret = false, title, valuePlaceholder }: {
  rows: KeyValueRow[]
  secret?: boolean
  title: string
  valuePlaceholder: string
  onChange: (rows: KeyValueRow[]) => void
}) {
  const { t } = useTranslation()
  const updateRow = (rowId: string, patch: Partial<KeyValueRow>) => {
    onChange(rows.map(row => row.id === rowId ? { ...row, ...patch } : row))
  }
  const removeRow = (rowId: string) => {
    const nextRows = rows.filter(row => row.id !== rowId)
    onChange(nextRows.length ? nextRows : [emptyKeyValueRow()])
  }
  return (
    <div className="grid gap-2 rounded-lg border border-border p-3">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium">{title}</h3>
        <Button size="sm" type="button" variant="secondary" onClick={() => onChange([...rows, emptyKeyValueRow()])}>
          <Plus className="size-4" />
          {t('buildsPage.addKeyValueRow')}
        </Button>
      </div>
      <div className="grid gap-2">
        {rows.map(row => (
          <div key={row.id} className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_auto]">
            <Input placeholder={t('buildsPage.variableKeyPlaceholder')} value={row.key} onChange={event => updateRow(row.id, { key: event.target.value })} />
            <Input
              placeholder={row.existing && secret ? t('common.secretSetPlaceholder') : valuePlaceholder}
              type={secret ? 'password' : 'text'}
              value={row.value}
              onChange={event => updateRow(row.id, { value: event.target.value })}
            />
            <Button aria-label={t('common.delete')} size="icon" type="button" variant="ghost" onClick={() => removeRow(row.id)}>
              <Trash2 className="size-4" />
            </Button>
          </div>
        ))}
      </div>
      {secret && <p className="text-xs leading-5 text-muted-foreground">{t('buildsPage.secretEditHint')}</p>}
    </div>
  )
}
