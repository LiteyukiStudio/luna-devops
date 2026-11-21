import type { AuthRegistrationSettings } from '@/api/types'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { ErrorState } from '@/components/common/error-state'
import { FloatingFormSaveBar } from '@/components/common/floating-form-save-bar'
import { Surface } from '@/components/common/surface'

type FormValues = AuthRegistrationSettings

const defaultValues: FormValues = {
  allowEmailRegistration: false,
  allowOidcRegistration: true,
  allowExternalIdentityPassword: false,
}

export function AuthRegistrationSettingsPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const settings = useQuery({ queryKey: ['auth-registration-settings'], queryFn: api.getAuthRegistrationSettings })
  const form = useForm<FormValues>({ mode: 'onChange', defaultValues })

  useEffect(() => {
    if (settings.data)
      form.reset(settingsToForm(settings.data))
  }, [form, settings.data])

  const save = useMutation({
    mutationFn: api.updateAuthRegistrationSettings,
    onSuccess: (result) => {
      queryClient.setQueryData(['auth-registration-settings'], result)
      queryClient.invalidateQueries({ queryKey: ['auth-registration-status'] })
      form.reset(settingsToForm(result))
      toast.success(t('settings.registration.saved'))
    },
    onError: error => toast.error(error.message),
  })

  if (settings.isError)
    return <ErrorState title={t('settings.registration.loadFailedTitle')} description={t('settings.registration.loadFailedDescription')} />

  return (
    <form className="grid max-w-3xl gap-5" onSubmit={form.handleSubmit(values => save.mutate(values))}>
      <Surface className="grid gap-4 p-6" variant="bordered">
        <div className="grid gap-4">
          <Controller
            control={form.control}
            name="allowEmailRegistration"
            render={({ field }) => (
              <ControlledCheckboxField description={t('settings.registration.emailRegistrationDescription')} field={field}>
                {t('settings.registration.emailRegistration')}
              </ControlledCheckboxField>
            )}
          />
          <Controller
            control={form.control}
            name="allowOidcRegistration"
            render={({ field }) => (
              <ControlledCheckboxField description={t('settings.registration.oidcRegistrationDescription')} field={field}>
                {t('settings.registration.oidcRegistration')}
              </ControlledCheckboxField>
            )}
          />
          <Controller
            control={form.control}
            name="allowExternalIdentityPassword"
            render={({ field }) => (
              <ControlledCheckboxField description={t('settings.registration.externalPasswordDescription')} field={field}>
                {t('settings.registration.externalPassword')}
              </ControlledCheckboxField>
            )}
          />
        </div>
      </Surface>
      <FloatingFormSaveBar
        open={form.formState.isDirty}
        pending={save.isPending}
        saveDisabled={settings.isLoading || !form.formState.isValid}
        saveLabel={t('settings.registration.save')}
        tone={form.formState.isValid ? 'primary' : 'danger'}
        onDiscard={() => form.reset(settings.data ? settingsToForm(settings.data) : defaultValues)}
      />
    </form>
  )
}

function settingsToForm(settings: AuthRegistrationSettings): FormValues {
  return {
    allowEmailRegistration: settings.allowEmailRegistration,
    allowOidcRegistration: settings.allowOidcRegistration,
    allowExternalIdentityPassword: settings.allowExternalIdentityPassword,
  }
}
