import type { UseFormReturn } from 'react-hook-form'
import type { ProjectRuntimeConfigSet, ProjectRuntimeConfigSetPayload } from '@/api'
import { FileCode2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { KeyValueTextEditor } from '@/components/common/key-value-text-editor'
import { RuntimeConfigFilesEditor } from '@/components/common/runtime-config-files-editor'
import { RuntimeConfigSetSecretsEditor } from '@/components/common/runtime-config-set-secrets-editor'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { runtimeConfigDefaults } from './application-deployments-panel-utils'

export function ApplicationRuntimeConfigSetDialog({
  editingSet,
  filesValid,
  form,
  open,
  projectId,
  canManageRuntimeSecrets,
  pending,
  secretFilesValid,
  setFilesValid,
  setSecretFilesValid,
  onOpenChange,
  onSubmit,
}: {
  editingSet: ProjectRuntimeConfigSet | null
  filesValid: boolean
  form: UseFormReturn<ProjectRuntimeConfigSetPayload>
  open: boolean
  projectId: string
  canManageRuntimeSecrets: boolean
  pending: boolean
  secretFilesValid: boolean
  setFilesValid: (valid: boolean) => void
  setSecretFilesValid: (valid: boolean) => void
  onOpenChange: (open: boolean) => void
  onSubmit: (values: ProjectRuntimeConfigSetPayload) => void
}) {
  const { t } = useTranslation()

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        onOpenChange(nextOpen)
        if (!nextOpen)
          form.reset(runtimeConfigDefaults)
      }}
    >
      <DialogContent className="flex max-h-[88vh] max-w-3xl flex-col overflow-hidden p-0">
        <DialogHeader className="border-b border-border px-6 py-5">
          <DialogTitle>{editingSet ? t('runtimeConfigSets.editTitle') : t('runtimeConfigSets.createTitle')}</DialogTitle>
          <DialogDescription>{t('runtimeConfigSets.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="grid min-h-0 flex-1 grid-rows-[minmax(0,1fr)_auto]" onSubmit={form.handleSubmit(onSubmit)}>
          <div className="grid gap-4 overflow-y-auto px-6 py-5">
            <Field label={t('common.name')} required><Input {...form.register('name', { required: true })} /></Field>
            <Field hint={t('runtimeConfigSets.envVarsHint')} label={t('runtimeConfigSets.envVars')}>
              <KeyValueTextEditor
                initialValue={form.getValues('envVars')}
                onChange={value => form.setValue('envVars', value, { shouldDirty: true, shouldValidate: true })}
              />
            </Field>
            <Field hint={t('runtimeConfigSets.configFilesHint')} label={t('runtimeConfigSets.configFiles')}>
              <RuntimeConfigFilesEditor
                key={`${editingSet?.id ?? 'new'}-target-config-files`}
                initialValue={form.getValues('configFiles') ?? ''}
                onChange={value => form.setValue('configFiles', value, { shouldDirty: true, shouldValidate: true })}
                onValidationChange={setFilesValid}
              />
            </Field>
            <Field hint={editingSet?.secretFilesSet ? t('runtimeConfigSets.secretFilesConfiguredHint') : t('runtimeConfigSets.secretFilesHint')} label={t('runtimeConfigSets.secretFiles')}>
              <RuntimeConfigFilesEditor
                key={`${editingSet?.id ?? 'new'}-target-secret-files`}
                configuredPlaceholder={editingSet?.secretFilesSet ? t('common.secretSetPlaceholder') : undefined}
                initialValue={form.getValues('secretFiles') ?? ''}
                onChange={value => form.setValue('secretFiles', value, { shouldDirty: true, shouldValidate: true })}
                onValidationChange={setSecretFilesValid}
              />
            </Field>
            {canManageRuntimeSecrets && <RuntimeConfigSetSecretsEditor projectId={projectId} set={editingSet} />}
            <CheckboxField {...form.register('enabled')}>
              {t('common.enabled')}
            </CheckboxField>
          </div>
          <DialogFooter className="border-t border-border bg-background px-6 py-4">
            <Button disabled={!filesValid || !secretFilesValid || pending} type="submit">
              <FileCode2 className="size-4" />
              {t('common.save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
