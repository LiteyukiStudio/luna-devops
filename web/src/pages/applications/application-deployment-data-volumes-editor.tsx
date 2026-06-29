import type { RuntimeDataVolumeRow } from '@/lib/runtime-data-volumes'
import { Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FormField as Field } from '@/components/common/form-field'
import { UnitInput } from '@/components/common/unit-input'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { emptyRuntimeDataVolumeRow } from '@/lib/runtime-data-volumes'

interface RuntimeDataVolumesEditorProps {
  enabled: boolean
  rows: RuntimeDataVolumeRow[]
  onChange: (rows: RuntimeDataVolumeRow[]) => void
}

export function RuntimeDataVolumesEditor({ enabled, onChange, rows }: RuntimeDataVolumesEditorProps) {
  const { t } = useTranslation()

  return (
    <Field hint={t('deploymentsPage.dataVolumesHint')} label={t('deploymentsPage.dataVolumes')} required={enabled}>
      <div className="grid gap-2 rounded-md border border-input bg-background p-3">
        <div className="hidden gap-2 px-1 text-xs font-medium text-muted-foreground md:grid md:grid-cols-[minmax(7rem,0.7fr)_minmax(0,1.5fr)_minmax(10rem,0.7fr)_auto]">
          <span>{t('deploymentsPage.dataVolumeName')}</span>
          <span>{t('deploymentsPage.dataMountPath')}</span>
          <span>{t('deploymentsPage.dataCapacity')}</span>
          <span className="sr-only">{t('common.actions')}</span>
        </div>
        {rows.map((volume, index) => (
          <div key={volume.id} className="grid gap-2 md:grid-cols-[minmax(7rem,0.7fr)_minmax(0,1.5fr)_minmax(10rem,0.7fr)_auto]">
            <Input
              disabled={!enabled}
              placeholder={t('deploymentsPage.dataVolumeNamePlaceholder')}
              value={volume.name}
              onChange={(event) => {
                const nextRows = [...rows]
                nextRows[index] = { ...volume, name: event.target.value }
                onChange(nextRows)
              }}
            />
            <Input
              disabled={!enabled}
              placeholder={t('deploymentsPage.dataMountPathPlaceholder')}
              value={volume.mountPath}
              onChange={(event) => {
                const nextRows = [...rows]
                nextRows[index] = { ...volume, mountPath: event.target.value }
                onChange(nextRows)
              }}
            />
            <UnitInput
              disabled={!enabled}
              inputProps={{ placeholder: t('deploymentsPage.dataCapacityPlaceholder') }}
              unitSelectLabel={t('deploymentsPage.dataCapacity')}
              units={[
                { label: 'Mi', value: 'Mi' },
                { label: 'Gi', value: 'Gi' },
              ]}
              value={volume.capacity}
              onChange={(value) => {
                const nextRows = [...rows]
                nextRows[index] = { ...volume, capacity: value }
                onChange(nextRows)
              }}
            />
            <Button
              aria-label={t('deploymentsPage.removeDataVolume')}
              disabled={!enabled || rows.length <= 1}
              size="icon"
              type="button"
              variant="ghost"
              onClick={() => onChange(rows.filter(row => row.id !== volume.id))}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        ))}
        <div>
          <Button
            disabled={!enabled}
            size="sm"
            type="button"
            variant="secondary"
            onClick={() => onChange([...rows, emptyRuntimeDataVolumeRow(rows.length)])}
          >
            <Plus className="size-4" />
            {t('deploymentsPage.addDataVolume')}
          </Button>
        </div>
      </div>
    </Field>
  )
}
