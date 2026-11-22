import type { ComponentProps } from 'react'
import type { Control, FieldErrors, UseFormRegisterReturn } from 'react-hook-form'
import type { GatewayRoute } from '@/api'
import { Controller } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
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
  domainSuffixError = false,
  domainSuffixLoading = false,
  domainSuffixOptions = [],
  domainSuffixRetrying = false,
  enabledField,
  errors,
  hostField,
  hostPlaceholder,
  onDeploymentTargetChange,
  onRetryDomainSuffix,
  pathField,
  servicePortOptions = [],
  showApplication = false,
}: {
  applications?: Array<{ id: string, name: string }>
  control: Control<GatewayRouteFormValues>
  deploymentTargets: Array<{ id: string, label: string }>
  domainSuffixError?: boolean
  domainSuffixLoading?: boolean
  domainSuffixOptions?: Array<{ label: string, value: string }>
  domainSuffixRetrying?: boolean
  enabledField: ComponentProps<typeof ControlledCheckboxField>['field']
  errors?: FieldErrors<Partial<GatewayRoute>>
  hostField: UseFormRegisterReturn<'host'>
  hostPlaceholder?: string
  onDeploymentTargetChange?: (deploymentTargetId: string) => void
  onRetryDomainSuffix?: () => void
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
                  <SelectValue placeholder={deploymentTargets.length === 0 ? t('common.noOptions') : t('common.select')} />
                </SelectTrigger>
                <SelectContent>
                  {deploymentTargets.length === 0
                    ? <SelectItem disabled value="__empty__">{t('common.noOptions')}</SelectItem>
                    : deploymentTargets.map(target => (
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
          <div className="grid gap-1.5">
            <Controller
              control={control}
              name="domainSuffix"
              rules={{ required: t('common.required') }}
              render={({ field }) => {
                const selectOptions = !domainSuffixLoading && !domainSuffixError && field.value && !domainSuffixOptions.some(option => option.value === field.value)
                  ? [{ label: field.value, value: field.value }, ...domainSuffixOptions]
                  : domainSuffixOptions
                return (
                  <Select
                    disabled={domainSuffixLoading || (domainSuffixError && selectOptions.length === 0)}
                    value={!domainSuffixLoading && selectOptions.some(option => option.value === field.value) ? field.value : ''}
                    onValueChange={(next) => {
                      if (next)
                        field.onChange(next)
                    }}
                  >
                    <SelectTrigger {...fieldControl} className="w-full">
                      <SelectValue
                        placeholder={
                          domainSuffixLoading
                            ? t('common.loading')
                            : domainSuffixError
                              ? t('common.unavailable')
                              : selectOptions.length === 0
                                ? t('common.noOptions')
                                : t('common.select')
                        }
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {domainSuffixLoading
                        ? <SelectItem disabled value="__loading__">{t('common.loading')}</SelectItem>
                        : selectOptions.length === 0
                          ? <SelectItem disabled value="__empty__">{t('common.noOptions')}</SelectItem>
                          : selectOptions.map(option => (
                              <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                            ))}
                    </SelectContent>
                  </Select>
                )
              }}
            />
            {domainSuffixError && (
              <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-danger" role="alert">
                <span>{t('common.unavailable')}</span>
                {onRetryDomainSuffix && (
                  <Button disabled={domainSuffixRetrying} size="sm" type="button" variant="outline" onClick={onRetryDomainSuffix}>
                    {t('common.retry')}
                  </Button>
                )}
              </div>
            )}
          </div>
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
