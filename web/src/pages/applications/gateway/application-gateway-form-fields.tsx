import type { ComponentProps } from 'react'
import type { FieldErrors, UseFormRegisterReturn } from 'react-hook-form'
import type { GatewayRoute } from '@/api'
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
  errors,
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
  errors?: FieldErrors<Partial<GatewayRoute>>
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
        <Field error={errors?.applicationId?.message} label={t('apps.title')} required>
          <Select {...applicationIdField} aria-invalid={Boolean(errors?.applicationId)}>
            <option value="">{t('common.select')}</option>
            {applications.map(app => <option key={app.id} value={app.id}>{app.name}</option>)}
          </Select>
        </Field>
      )}
      <Field error={errors?.deploymentTargetId?.message} label={t('gatewayRoutesPage.deploymentTarget')} required>
        <Select {...deploymentTargetIdField} aria-invalid={Boolean(errors?.deploymentTargetId)}>
          <option value="">{t('common.select')}</option>
          {deploymentTargets.map(target => <option key={target.id} value={target.id}>{target.label}</option>)}
        </Select>
      </Field>
      <Field error={errors?.host?.message} hint={t('gatewayRoutesPage.hostHint')} label={t('gatewayRoutesPage.host')}>
        <Input {...hostField} aria-invalid={Boolean(errors?.host)} placeholder={hostPlaceholder || t('gatewayRoutesPage.hostPlaceholder')} />
      </Field>
      <Field error={errors?.domainSuffix?.message} hint={t('gatewayRoutesPage.domainSuffixHint')} label={t('gatewayRoutesPage.domainSuffix')} required>
        <Select {...domainSuffixField} aria-invalid={Boolean(errors?.domainSuffix)}>
          {domainSuffixOptions.length === 0 && <option value="">{t('common.select')}</option>}
          {domainSuffixOptions.map(option => (
            <option key={option.value} value={option.value}>{option.label}</option>
          ))}
        </Select>
      </Field>
      <Field error={errors?.path?.message} label={t('gatewayRoutesPage.path')}>
        <Input {...pathField} aria-invalid={Boolean(errors?.path)} />
      </Field>
      <Field error={errors?.servicePort?.message} label={t('gatewayRoutesPage.servicePort')}>
        <Select {...servicePortField} aria-invalid={Boolean(errors?.servicePort)}>
          {servicePortOptions.length === 0 && <option value="">{t('common.select')}</option>}
          {servicePortOptions.map(option => (
            <option key={option.value} value={option.value}>{option.label}</option>
          ))}
        </Select>
      </Field>
      <Field error={errors?.tlsMode?.message} label={t('gatewayRoutesPage.tlsMode')}>
        <Select {...tlsModeField} aria-invalid={Boolean(errors?.tlsMode)}>
          <option value="http-only">{t('gatewayRoutesPage.tlsHttpOnly')}</option>
          <option value="http-challenge">{t('gatewayRoutesPage.tlsHttpChallenge')}</option>
          <option value="manual-cert">{t('gatewayRoutesPage.tlsManualCert')}</option>
        </Select>
      </Field>
      <ControlledCheckboxField
        description={t('gatewayRoutesPage.enabledHint')}
        field={enabledField}
      >
        {t('gatewayRoutesPage.enabled')}
      </ControlledCheckboxField>
    </>
  )
}
