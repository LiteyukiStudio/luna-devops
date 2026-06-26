import type { UseFormReturn } from 'react-hook-form'
import type { ProjectRuntimeConfigSet, ProjectRuntimeConfigSetPayload } from '@/api'
import { FileCode2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { RuntimeConfigFilesEditor } from '@/components/common/runtime-config-files-editor'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { runtimeConfigDefaults } from './application-deployments-panel-utils'

export function ApplicationRuntimeConfigSetDialog({
  editingSet,
  filesValid,
  form,
  open,
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
      <DialogContent className="max-h-[88vh] max-w-3xl overflow-hidden p-0">
        <DialogHeader className="border-b border-border px-6 py-5">
          <DialogTitle>{editingSet ? t('runtimeConfigSets.editTitle') : t('runtimeConfigSets.createTitle')}</DialogTitle>
          <DialogDescription>{t('runtimeConfigSets.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="grid max-h-[calc(88vh-96px)] grid-rows-[minmax(0,1fr)_auto]" onSubmit={form.handleSubmit(onSubmit)}>
          <div className="grid gap-4 overflow-y-auto px-6 py-5">
            <Field label={t('common.name')} required><Input {...form.register('name', { required: true })} /></Field>
            <Field hint={t('runtimeConfigSets.envVarsHint')} label={t('runtimeConfigSets.envVars')}>
              <Textarea className="min-h-24 font-mono text-sm" {...form.register('envVars')} placeholder={t('runtimeConfigSets.envVarsPlaceholder')} />
            </Field>
            <Field hint={t('runtimeConfigSets.configFilesHint')} label={t('runtimeConfigSets.configFiles')}>
              <RuntimeConfigFilesEditor
                key={`${editingSet?.id ?? 'new'}-target-config-files`}
                initialValue={form.getValues('configFiles') ?? ''}
                onChange={value => form.setValue('configFiles', value, { shouldDirty: true, shouldValidate: true })}
                onValidationChange={setFilesValid}
              />
            </Field>
            <Field hint={editingSet?.secretRefsSet ? t('runtimeConfigSets.secretRefsConfiguredHint') : t('runtimeConfigSets.secretRefsHint')} label={t('runtimeConfigSets.secretRefs')}>
              <Textarea className="min-h-24 font-mono text-sm" {...form.register('secretRefs')} placeholder={t('runtimeConfigSets.secretRefsPlaceholder')} />
            </Field>
            <Field hint={editingSet?.secretFilesSet ? t('runtimeConfigSets.secretFilesConfiguredHint') : t('runtimeConfigSets.secretFilesHint')} label={t('runtimeConfigSets.secretFiles')}>
              <RuntimeConfigFilesEditor
                key={`${editingSet?.id ?? 'new'}-target-secret-files`}
                initialValue={form.getValues('secretFiles') ?? ''}
                onChange={value => form.setValue('secretFiles', value, { shouldDirty: true, shouldValidate: true })}
                onValidationChange={setSecretFilesValid}
              />
            </Field>
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
