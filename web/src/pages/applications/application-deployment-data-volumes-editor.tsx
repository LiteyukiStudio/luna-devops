import type { RuntimeDataVolumeRow } from '@/lib/runtime-data-volumes'
import { useQuery } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { api } from '@/api'
import { FormField as Field } from '@/components/common/form-field'
import { UnitInput } from '@/components/common/unit-input'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { emptyRuntimeDataVolumeRow } from '@/lib/runtime-data-volumes'

interface RuntimeDataVolumesEditorProps {
  enabled: boolean
  rows: RuntimeDataVolumeRow[]
  onChange: (rows: RuntimeDataVolumeRow[]) => void
}

export function RuntimeDataVolumesEditor({ enabled, onChange, rows }: RuntimeDataVolumesEditorProps) {
  const { t } = useTranslation()
  const { projectId = '' } = useParams()
  const retainedVolumes = useQuery({
    queryKey: ['retained-volumes', projectId],
    queryFn: () => api.listRetainedVolumes(projectId),
    enabled: enabled && Boolean(projectId),
  })

  return (
    <Field hint={t('deploymentsPage.dataVolumesHint')} label={t('deploymentsPage.dataVolumes')} required={enabled}>
      <div className="grid gap-2 rounded-md border border-input bg-background p-3">
        <div className="hidden gap-2 px-1 text-xs font-medium text-muted-foreground md:grid md:grid-cols-[minmax(7rem,0.7fr)_minmax(0,1fr)_minmax(0,1.4fr)_minmax(10rem,0.8fr)_auto]">
          <span>{t('deploymentsPage.dataVolumeName')}</span>
          <span>{t('deploymentsPage.dataVolumeSourceType')}</span>
          <span>{t('deploymentsPage.dataMountPath')}</span>
          <span>{t('deploymentsPage.dataVolumeSourceDetail')}</span>
          <span className="sr-only">{t('common.actions')}</span>
        </div>
        {rows.map((volume, index) => (
          <div key={volume.id} className="grid gap-2 md:grid-cols-[minmax(7rem,0.7fr)_minmax(0,1fr)_minmax(0,1.4fr)_minmax(10rem,0.8fr)_auto]">
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
            <Select
              disabled={!enabled}
              value={volume.sourceType}
              onChange={(event) => {
                const nextRows = [...rows]
                nextRows[index] = { ...volume, sourceType: event.target.value as RuntimeDataVolumeRow['sourceType'] }
                onChange(nextRows)
              }}
            >
              <option value="managed">{t('deploymentsPage.dataVolumeSourceManaged')}</option>
              <option value="existingClaim">{t('deploymentsPage.dataVolumeSourceExistingClaim')}</option>
              <option value="retainedClaim">{t('deploymentsPage.dataVolumeSourceRetainedClaim')}</option>
              <option value="emptyDir">{t('deploymentsPage.dataVolumeSourceEmptyDir')}</option>
            </Select>
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
            {volume.sourceType === 'retainedClaim'
              ? (
                  <Select
                    disabled={!enabled || retainedVolumes.isLoading}
                    value={volume.retainedVolumeId}
                    onChange={(event) => {
                      const retained = retainedVolumes.data?.find(item => item.id === event.target.value)
                      const nextRows = [...rows]
                      nextRows[index] = { ...volume, retainedVolumeId: retained?.id ?? '', existingClaimName: retained?.claimName ?? '', capacity: retained?.capacity ?? '' }
                      onChange(nextRows)
                    }}
                  >
                    <option value="">{t('deploymentsPage.dataRetainedVolumePlaceholder')}</option>
                    {retainedVolumes.data?.filter(item => item.status === 'retained').map(item => (
                      <option key={item.id} value={item.id}>
                        {item.sourceApplicationName}
                        {' '}
                        ·
                        {' '}
                        {item.claimName}
                        {' '}
                        ·
                        {' '}
                        {item.capacity}
                      </option>
                    ))}
                  </Select>
                )
              : volume.sourceType === 'existingClaim'
                ? (
                    <Input
                      disabled={!enabled}
                      placeholder={t('deploymentsPage.dataExistingClaimNamePlaceholder')}
                      value={volume.existingClaimName}
                      onChange={(event) => {
                        const nextRows = [...rows]
                        nextRows[index] = { ...volume, existingClaimName: event.target.value }
                        onChange(nextRows)
                      }}
                    />
                  )
                : volume.sourceType === 'emptyDir'
                  ? (
                      <div className="grid gap-2 md:grid-cols-2">
                        <Select
                          disabled={!enabled}
                          value={volume.emptyDirMedium}
                          onChange={(event) => {
                            const nextRows = [...rows]
                            nextRows[index] = { ...volume, emptyDirMedium: event.target.value }
                            onChange(nextRows)
                          }}
                        >
                          <option value="">{t('deploymentsPage.emptyDirMediumDefault')}</option>
                          <option value="Memory">{t('deploymentsPage.emptyDirMediumMemory')}</option>
                        </Select>
                        <Input
                          disabled={!enabled}
                          placeholder={t('deploymentsPage.emptyDirSizeLimitPlaceholder')}
                          value={volume.emptyDirSizeLimit}
                          onChange={(event) => {
                            const nextRows = [...rows]
                            nextRows[index] = { ...volume, emptyDirSizeLimit: event.target.value }
                            onChange(nextRows)
                          }}
                        />
                      </div>
                    )
                  : (
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
                    )}
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
