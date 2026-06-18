import type { UseFormRegisterReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField as Field } from '@/components/common/form-field'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'

export function GatewayRouteFormFields({
  applicationIdField,
  applications = [],
  deploymentTargetIdField,
  deploymentTargets,
  enabledField,
  hostField,
  pathField,
  servicePortField,
  showApplication = false,
  tlsModeField,
}: {
  applicationIdField?: UseFormRegisterReturn<'applicationId'>
  applications?: Array<{ id: string, name: string }>
  deploymentTargetIdField: UseFormRegisterReturn<'deploymentTargetId'>
  deploymentTargets: Array<{ id: string, label: string }>
  enabledField: UseFormRegisterReturn<'enabled'>
  hostField: UseFormRegisterReturn<'host'>
  pathField: UseFormRegisterReturn<'path'>
  servicePortField: UseFormRegisterReturn<'servicePort'>
  showApplication?: boolean
  tlsModeField: UseFormRegisterReturn<'tlsMode'>
}) {
  const { t } = useTranslation()

  return (
    <>
      {showApplication && (
        <Field label={t('apps.title')} required>
          <Select {...applicationIdField}>
            <option value="">{t('common.select')}</option>
            {applications.map(app => <option key={app.id} value={app.id}>{app.name}</option>)}
          </Select>
        </Field>
      )}
      <Field label={t('gatewayRoutesPage.deploymentTarget')} required>
        <Select {...deploymentTargetIdField}>
          <option value="">{t('common.select')}</option>
          {deploymentTargets.map(target => <option key={target.id} value={target.id}>{target.label}</option>)}
        </Select>
      </Field>
      <Field hint={t('gatewayRoutesPage.hostHint')} label={t('gatewayRoutesPage.host')}>
        <Input {...hostField} />
      </Field>
      <Field label={t('gatewayRoutesPage.path')}>
        <Input {...pathField} />
      </Field>
      <Field label={t('gatewayRoutesPage.servicePort')}>
        <Input {...servicePortField} type="number" />
      </Field>
      <Field label={t('gatewayRoutesPage.tlsMode')}>
        <Select {...tlsModeField}>
          <option value="http-only">{t('gatewayRoutesPage.tlsHttpOnly')}</option>
          <option value="http-challenge">{t('gatewayRoutesPage.tlsHttpChallenge')}</option>
          <option value="manual-cert">{t('gatewayRoutesPage.tlsManualCert')}</option>
        </Select>
      </Field>
      <label className="flex items-start gap-3 rounded-md border border-border bg-muted/20 px-3 py-2 text-sm">
        <input className="mt-1 size-4 accent-primary" type="checkbox" {...enabledField} />
        <span className="grid gap-1">
          <span className="font-medium text-foreground">{t('gatewayRoutesPage.enabled')}</span>
          <span className="text-muted-foreground">{t('gatewayRoutesPage.enabledHint')}</span>
        </span>
      </label>
    </>
  )
}
