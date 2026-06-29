import type { DeploymentTargetPayload } from '@/api'
import { Plus, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

interface ServicePortsEditorProps {
  ports: NonNullable<DeploymentTargetPayload['servicePorts']>
  onChange: (ports: NonNullable<DeploymentTargetPayload['servicePorts']>) => void
}

export function ServicePortsEditor({ onChange, ports }: ServicePortsEditorProps) {
  const { t } = useTranslation()

  return (
    <Field hint={t('deploymentsPage.servicePortsHint')} label={t('deploymentsPage.servicePorts')} required>
      <div className="grid gap-2">
        {ports.map((item, index) => (
          <div key={`${item.name || 'port'}-${item.port || 'empty'}`} className="grid gap-2 md:grid-cols-[1fr_180px_auto]">
            <Input
              aria-label={t('deploymentsPage.servicePortName')}
              placeholder={index === 0 ? 'http' : 'metrics'}
              value={item.name}
              onChange={event => onChange(ports.map((row, rowIndex) => rowIndex === index ? { ...row, name: event.target.value } : row))}
            />
            <Input
              aria-label={t('deploymentsPage.servicePortNumber')}
              max={65535}
              min={1}
              type="number"
              value={item.port}
              onChange={event => onChange(ports.map((row, rowIndex) => rowIndex === index ? { ...row, port: Number(event.target.value) } : row))}
            />
            <Button
              aria-label={t('common.delete')}
              disabled={ports.length <= 1}
              size="icon"
              type="button"
              variant="ghost"
              onClick={() => onChange(ports.filter((_, rowIndex) => rowIndex !== index))}
            >
              <X className="size-4" />
            </Button>
          </div>
        ))}
        <Button
          className="w-fit"
          size="sm"
          type="button"
          variant="outline"
          onClick={() => onChange([...ports, { name: `port-${ports.length + 1}`, port: 9001 }])}
        >
          <Plus className="size-4" />
          {t('deploymentsPage.addServicePort')}
        </Button>
      </div>
    </Field>
  )
}
