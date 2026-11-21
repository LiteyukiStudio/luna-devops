import type { ComponentProps } from 'react'
import type { Control, FieldErrors, UseFormRegisterReturn } from 'react-hook-form'
import type { GatewayRoute } from '@/api'
import { Controller } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

export type GatewayRouteFormValues = Omit<
  GatewayRoute,
  'id' | 'projectId' | 'createdBy' | 'createdAt' | 'certificateStatus' | 'certificateMessage' | 'certificateNotAfter' | 'certificateIssuerKind' | 'certificateIssuerName' | 'cnameName' | 'cnameTarget' | 'accessUrl' | 'deleteStatus' | 'deleteMessage' | 'deleteStartedAt' | 'deleteFinishedAt' | 'routeSummary' | 'conditions'
>

export function GatewayRouteFormFields({
  applications = [],
  control,
  deploymentTargets,
  domainSuffixOptions = [],
  enabledField,
  errors,
  hostField,
  hostPlaceholder,
  onDeploymentTargetChange,
  pathField,
  servicePortOptions = [],
  showApplication = false,
}: {
  applications?: Array<{ id: string, name: string }>
  control: Control<GatewayRouteFormValues>
  deploymentTargets: Array<{ id: string, label: string }>
  domainSuffixOptions?: Array<{ label: string, value: string }>
  enabledField: ComponentProps<typeof ControlledCheckboxField>['field']
  errors?: FieldErrors<Partial<GatewayRoute>>
  hostField: UseFormRegisterReturn<'host'>
  hostPlaceholder?: string
  onDeploymentTargetChange?: (deploymentTargetId: string) => void
  pathField: UseFormRegisterReturn<'path'>
  servicePortOptions?: Array<{ label: string, value: number }>
  showApplication?: boolean
}) {
  const { t } = useTranslation()

  return (
    <>
      {showApplication && (
        <Field error={errors?.applicationId?.message} label={t('apps.title')} required>
          {fieldControl => (
            <Controller
              control={control}
              name="applicationId"
              rules={{ required: t('common.required') }}
              render={({ field }) => (
                <Select value={field.value || undefined} onValueChange={field.onChange}>
                  <SelectTrigger {...fieldControl} className="w-full">
                    <SelectValue placeholder={t('common.select')} />
                  </SelectTrigger>
                  <SelectContent>
                    {applications.map(app => (
                      <SelectItem key={app.id} value={app.id}>{app.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          )}
        </Field>
      )}
      <Field error={errors?.deploymentTargetId?.message} label={t('gatewayRoutesPage.deploymentTarget')} required>
        {fieldControl => (
          <Controller
            control={control}
            name="deploymentTargetId"
            rules={{ required: t('common.required') }}
            render={({ field }) => (
              <Select
                value={field.value || undefined}
                onValueChange={(next) => {
                  field.onChange(next)
                  onDeploymentTargetChange?.(next)
                }}
              >
                <SelectTrigger {...fieldControl} className="w-full">
                  <SelectValue placeholder={t('common.select')} />
                </SelectTrigger>
                <SelectContent>
                  {deploymentTargets.map(target => (
                    <SelectItem key={target.id} value={target.id}>{target.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        )}
      </Field>
      <Field error={errors?.host?.message} hint={t('gatewayRoutesPage.hostHint')} label={t('gatewayRoutesPage.host')}>
        <Input {...hostField} aria-invalid={Boolean(errors?.host)} placeholder={hostPlaceholder || t('gatewayRoutesPage.hostPlaceholder')} />
      </Field>
      <Field error={errors?.domainSuffix?.message} hint={t('gatewayRoutesPage.domainSuffixHint')} label={t('gatewayRoutesPage.domainSuffix')} required>
        {fieldControl => (
          <Controller
            control={control}
            name="domainSuffix"
            rules={{ required: t('common.required') }}
            render={({ field }) => (
              <Select value={field.value || undefined} onValueChange={field.onChange}>
                <SelectTrigger {...fieldControl} className="w-full">
                  <SelectValue placeholder={t('common.select')} />
                </SelectTrigger>
                <SelectContent>
                  {domainSuffixOptions.map(option => (
                    <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        )}
      </Field>
      <Field error={errors?.path?.message} label={t('gatewayRoutesPage.path')}>
        <Input {...pathField} aria-invalid={Boolean(errors?.path)} />
      </Field>
      <Field error={errors?.servicePort?.message} label={t('gatewayRoutesPage.servicePort')}>
        {fieldControl => (
          <Controller
            control={control}
            name="servicePort"
            render={({ field }) => (
              <Select
                value={field.value ? String(field.value) : undefined}
                onValueChange={next => field.onChange(Number(next))}
              >
                <SelectTrigger {...fieldControl} className="w-full">
                  <SelectValue placeholder={t('common.select')} />
                </SelectTrigger>
                <SelectContent>
                  {servicePortOptions.map(option => (
                    <SelectItem key={option.value} value={String(option.value)}>{option.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        )}
      </Field>
      <Field error={errors?.tlsMode?.message} label={t('gatewayRoutesPage.tlsMode')}>
        {fieldControl => (
          <Controller
            control={control}
            name="tlsMode"
            render={({ field }) => (
              <Select value={field.value || undefined} onValueChange={field.onChange}>
                <SelectTrigger {...fieldControl} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="http-only">{t('gatewayRoutesPage.tlsHttpOnly')}</SelectItem>
                  <SelectItem value="http-challenge">{t('gatewayRoutesPage.tlsHttpChallenge')}</SelectItem>
                  <SelectItem value="manual-cert">{t('gatewayRoutesPage.tlsManualCert')}</SelectItem>
                </SelectContent>
              </Select>
            )}
          />
        )}
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
