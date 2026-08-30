import type { UseFormReturn } from 'react-hook-form'
import type { ProjectRuntimeConfigSet, ProjectRuntimeConfigSetPayload } from '@/api'
import { FileCode2 } from 'lucide-react'
import { Controller } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { KeyValueTextEditor } from '@/components/common/key-value-text-editor'
import { RuntimeConfigFilesEditor } from '@/components/common/runtime-config-files-editor'
import { RuntimeConfigSetSecretsEditor } from '@/components/common/runtime-config-set-secrets-editor'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { publicRuntimeEnvironmentInputs, publicRuntimeEnvironmentRecord } from '@/lib/runtime-environment'

interface RuntimeConfigSetDialogProps {
  canManageSecrets: boolean
  configFilesValid: boolean
  editingSet: ProjectRuntimeConfigSet | null
  form: UseFormReturn<ProjectRuntimeConfigSetPayload>
  open: boolean
  pending: boolean
  projectId: string
  secretFilesValid: boolean
  onConfigFilesValidityChange: (valid: boolean) => void
  onOpenChange: (open: boolean) => void
  onSecretFilesValidityChange: (valid: boolean) => void
  onSubmit: (values: ProjectRuntimeConfigSetPayload) => void
}

/** Shared create/edit dialog for runtime config sets, regardless of the entry page. */
export function RuntimeConfigSetDialog({
  canManageSecrets,
  configFilesValid,
  editingSet,
  form,
  open,
  pending,
  projectId,
  secretFilesValid,
  onConfigFilesValidityChange,
  onOpenChange,
  onSecretFilesValidityChange,
  onSubmit,
}: RuntimeConfigSetDialogProps) {
  const { t } = useTranslation()
  const editorKey = editingSet?.id ?? 'new'
  const nameError = form.formState.errors.name?.message

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[88vh] max-w-3xl flex-col overflow-hidden p-0">
        <DialogHeader className="border-b border-border px-6 py-5">
          <DialogTitle>{editingSet ? t('runtimeConfigSets.editTitle') : t('runtimeConfigSets.createTitle')}</DialogTitle>
          <DialogDescription>{t('runtimeConfigSets.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <form aria-busy={pending} className="grid min-h-0 flex-1 grid-rows-[minmax(0,1fr)_auto]" onSubmit={form.handleSubmit(onSubmit)}>
          <div className="grid gap-4 overflow-y-auto px-6 py-5">
            <Field error={nameError} label={t('common.name')} required>
              <Input
                aria-invalid={Boolean(nameError)}
                aria-label={t('common.name')}
                {...form.register('name', { required: t('common.required') })}
              />
            </Field>
            <Field hint={t('runtimeConfigSets.envVarsHint')} label={t('runtimeConfigSets.envVars')}>
              <KeyValueTextEditor
                initialValue={publicRuntimeEnvironmentRecord(form.getValues('environmentVariables'))}
                onChange={value => form.setValue('environmentVariables', publicRuntimeEnvironmentInputs(value), { shouldDirty: true, shouldValidate: true })}
              />
            </Field>
            <Field hint={t('runtimeConfigSets.configFilesHint')} label={t('runtimeConfigSets.configFiles')}>
              <RuntimeConfigFilesEditor
                key={`${editorKey}-config-files`}
                initialValue={form.getValues('configFiles') ?? ''}
                onChange={value => form.setValue('configFiles', value, { shouldDirty: true, shouldValidate: true })}
                onValidationChange={onConfigFilesValidityChange}
              />
            </Field>
            {canManageSecrets && <RuntimeConfigSetSecretsEditor projectId={projectId} set={editingSet} />}
            <Field hint={editingSet?.secretFilesSet ? t('runtimeConfigSets.secretFilesConfiguredHint') : t('runtimeConfigSets.secretFilesHint')} label={t('runtimeConfigSets.secretFiles')}>
              <RuntimeConfigFilesEditor
                key={`${editorKey}-secret-files`}
                configuredPlaceholder={editingSet?.secretFilesSet ? t('common.secretSetPlaceholder') : undefined}
                initialValue={form.getValues('secretFiles') ?? ''}
                onChange={value => form.setValue('secretFiles', value, { shouldDirty: true, shouldValidate: true })}
                onValidationChange={onSecretFilesValidityChange}
              />
            </Field>
            <Controller
              control={form.control}
              name="enabled"
              render={({ field }) => (
                <ControlledCheckboxField field={field}>
                  {t('common.enabled')}
                </ControlledCheckboxField>
              )}
            />
          </div>
          <DialogFooter className="border-t border-border bg-background px-6 py-4">
            <Button disabled={!form.formState.isValid || !configFilesValid || !secretFilesValid || pending} type="submit">
              <FileCode2 className="size-4" />
              {t('common.save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
