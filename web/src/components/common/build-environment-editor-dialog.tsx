import type { KeyValueRow } from '@/components/common/key-value-rows-editor'
import { useTranslation } from 'react-i18next'
import { KeyValueRowsEditor } from '@/components/common/key-value-rows-editor'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'

export function BuildEnvironmentEditorDialog({ description, open, pending, secretRows, title, variableRows, onOpenChange, onSave, onSecretRowsChange, onVariableRowsChange }: {
  description: string
  open: boolean
  pending: boolean
  secretRows: KeyValueRow[]
  title: string
  variableRows: KeyValueRow[]
  onOpenChange: (open: boolean) => void
  onSave: () => void
  onSecretRowsChange: (rows: KeyValueRow[]) => void
  onVariableRowsChange: (rows: KeyValueRow[]) => void
}) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <KeyValueRowsEditor
            rows={variableRows}
            title={t('buildsPage.variables')}
            valuePlaceholder={t('buildsPage.variableValuePlaceholder')}
            onChange={onVariableRowsChange}
          />
          <KeyValueRowsEditor
            secret
            rows={secretRows}
            title={t('buildsPage.secrets')}
            valuePlaceholder={t('buildsPage.secretValuePlaceholder')}
            onChange={onSecretRowsChange}
          />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button disabled={pending} type="button" onClick={onSave}>{t('common.save')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
