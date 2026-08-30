import type { ComponentProps } from 'react'
import type { UseFormRegisterReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'

export function GatewayRouteFormFields({
  applicationIdField,
  applications = [],
  deploymentTargetIdField,
  deploymentTargets,
  domainSuffixField,
  domainSuffixOptions = [],
  enabledField,
  hostField,
  hostPlaceholder,
  pathField,
  servicePortField,
  servicePortOptions = [],
  showApplication = false,
  tlsModeField,
}: {
  applicationIdField?: UseFormRegisterReturn<'applicationId'>
  applications?: Array<{ id: string, name: string }>
  deploymentTargetIdField: UseFormRegisterReturn<'deploymentTargetId'>
  deploymentTargets: Array<{ id: string, label: string }>
  domainSuffixField: UseFormRegisterReturn<'domainSuffix'>
  domainSuffixOptions?: Array<{ label: string, value: string }>
  enabledField: ComponentProps<typeof ControlledCheckboxField>['field']
  hostField: UseFormRegisterReturn<'host'>
  hostPlaceholder?: string
  pathField: UseFormRegisterReturn<'path'>
  servicePortField: UseFormRegisterReturn<'servicePort'>
  servicePortOptions?: Array<{ label: string, value: number }>
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
        <Input {...hostField} placeholder={hostPlaceholder || t('gatewayRoutesPage.hostPlaceholder')} />
      </Field>
      <Field hint={t('gatewayRoutesPage.domainSuffixHint')} label={t('gatewayRoutesPage.domainSuffix')} required>
        <Select {...domainSuffixField}>
          {domainSuffixOptions.length === 0 && <option value="">{t('common.select')}</option>}
          {domainSuffixOptions.map(option => (
            <option key={option.value} value={option.value}>{option.label}</option>
          ))}
        </Select>
      </Field>
      <Field label={t('gatewayRoutesPage.path')}>
        <Input {...pathField} />
      </Field>
      <Field label={t('gatewayRoutesPage.servicePort')}>
        <Select {...servicePortField}>
          {servicePortOptions.length === 0 && <option value="">{t('common.select')}</option>}
          {servicePortOptions.map(option => (
            <option key={option.value} value={option.value}>{option.label}</option>
          ))}
        </Select>
      </Field>
      <Field label={t('gatewayRoutesPage.tlsMode')}>
        <Select {...tlsModeField}>
          <option value="http-only">{t('gatewayRoutesPage.tlsHttpOnly')}</option>
          <option value="http-challenge">{t('gatewayRoutesPage.tlsHttpChallenge')}</option>
          <option value="manual-cert">{t('gatewayRoutesPage.tlsManualCert')}</option>
        </Select>
      </Field>
      <ControlledCheckboxField
        className="rounded-md border border-border bg-muted/20 px-3 py-2"
        description={t('gatewayRoutesPage.enabledHint')}
        field={enabledField}
      >
        {t('gatewayRoutesPage.enabled')}
      </ControlledCheckboxField>
    </>
  )
}
