import type { NotificationChannelMode } from './notification-models'
import type { NotificationChannel, NotificationChannelPayload, NotificationPreset, Project } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { FlaskConical } from 'lucide-react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { ProjectSpaceMultiSelect } from '@/components/common/project-space-select'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import {
  buildChannelDraftTestPayload,
  buildScopedChannelPayload,
  defaultChannelFormValues,
  notificationChannelFormSchema,
  parseStringHeadersJson,
  presetSecretFieldLabel,
} from './notification-models'

export function NotificationChannelDialog({
  allowGlobalScope = false,
  channel,
  defaultMode = 'preset',
  defaultProjectIds = [],
  defaultScope = 'project',
  manageableProjects,
  open,
  presets,
  saving,
  onClose,
  onSave,
}: {
  allowGlobalScope?: boolean
  channel?: NotificationChannel
  defaultMode?: NotificationChannelMode
  defaultProjectIds?: string[]
  defaultScope?: 'project' | 'global'
  manageableProjects: Project[]
  open: boolean
  presets: NotificationPreset[]
  saving: boolean
  onClose: () => void
  onSave: (payload: NotificationChannelPayload, channelId?: string) => void
}) {
  if (!open)
    return null

  const dialogKey = channel ? `edit-${channel.id}` : `new-${defaultMode}-${defaultScope}-${defaultProjectIds.join(',')}`
  return (
    <Dialog open onOpenChange={nextOpen => !nextOpen && onClose()}>
      <NotificationChannelDialogBody
        key={dialogKey}
        allowGlobalScope={allowGlobalScope}
        channel={channel}
        defaultMode={defaultMode}
        defaultProjectIds={defaultProjectIds}
        defaultScope={defaultScope}
        manageableProjects={manageableProjects}
        presets={presets}
        saving={saving}
        onClose={onClose}
        onSave={onSave}
      />
    </Dialog>
  )
}

function NotificationChannelDialogBody({
  allowGlobalScope,
  channel,
  defaultMode,
  defaultProjectIds,
  defaultScope,
  manageableProjects,
  presets,
  saving,
  onClose,
  onSave,
}: {
  allowGlobalScope: boolean
  channel?: NotificationChannel
  defaultMode: NotificationChannelMode
  defaultProjectIds: string[]
  defaultScope: 'project' | 'global'
  manageableProjects: Project[]
  presets: NotificationPreset[]
  saving: boolean
  onClose: () => void
  onSave: (payload: NotificationChannelPayload, channelId?: string) => void
}) {
  const { t } = useTranslation()
  const form = useForm({
    defaultValues: defaultChannelFormValues({
      channel,
      defaultMode,
      defaultProjectIds,
      defaultScope,
      presets,
    }),
    mode: 'onChange',
    resolver: zodResolver(notificationChannelFormSchema),
  })

  const values = form.watch()
  const selectedPreset = presets.find(item => item.id === values.presetId) ?? presets[0]
  const headersValid = values.mode !== 'webhook' || parseStringHeadersJson(values.webhookHeadersJson) !== null
  const payload = buildScopedChannelPayload(values, channel, presets)
  const draftTestPayload = buildChannelDraftTestPayload(values, channel, presets)
  const existingSecrets = channel?.secretSet ?? {}

  const draftTestMutation = useMutation({
    mutationFn: api.testDraftScopedNotificationChannel,
    onSuccess: () => toast.success(t('notificationsPage.testSent')),
    onError: error => toast.error(error.message),
  })

  const submit = form.handleSubmit((submittedValues) => {
    const nextPayload = buildScopedChannelPayload(submittedValues, channel, presets)
    if (!nextPayload) {
      toast.error(t('notificationsPage.channelInvalid'))
      return
    }
    onSave(nextPayload, channel?.id)
  })

  return (
    <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>{t('notificationsPage.channelDialog')}</DialogTitle>
        <DialogDescription>{t('notificationsPage.channelDialogDescription')}</DialogDescription>
      </DialogHeader>
      <form className="grid gap-4" onSubmit={submit}>
        <div className="grid gap-4 md:grid-cols-2">
          <Field label={t('notificationsPage.channelMode')}>
            <Select
              disabled={Boolean(channel)}
              value={values.mode}
              onChange={(event) => {
                const nextMode = event.target.value as NotificationChannelMode
                form.setValue('mode', nextMode, { shouldDirty: true, shouldValidate: true })
              }}
            >
              {presets.length > 0 && <option value="preset">{t('notificationsPage.modePreset')}</option>}
              <option value="webhook">{t('notificationsPage.modeWebhook')}</option>
              <option value="smtp">{t('notificationsPage.modeSmtp')}</option>
            </Select>
          </Field>

          {allowGlobalScope
            ? (
                <Field label={t('notificationsPage.scope')}>
                  <Select
                    value={values.scope}
                    onChange={(event) => {
                      const nextScope = event.target.value === 'global' ? 'global' : 'project'
                      form.setValue('scope', nextScope, { shouldDirty: true, shouldValidate: true })
                    }}
                  >
                    <option value="global">{t('notificationsPage.scopeGlobal')}</option>
                    <option value="project">{t('notificationsPage.scopeProject')}</option>
                  </Select>
                </Field>
              )
            : (
                <Field label={t('notificationsPage.name')} required={values.mode !== 'preset'}>
                  <Input
                    {...form.register('name')}
                    placeholder={values.mode === 'preset' ? selectedPreset?.name : undefined}
                  />
                </Field>
              )}

          {allowGlobalScope && (
            <Field className="md:col-span-2" label={t('notificationsPage.name')} required={values.mode !== 'preset'}>
              <Input
                {...form.register('name')}
                placeholder={values.mode === 'preset' ? selectedPreset?.name : undefined}
              />
            </Field>
          )}

          {values.scope === 'project' && (
            <Field
              className="md:col-span-2"
              error={values.projectIds.length === 0 ? t('notificationsPage.ruleProjectsRequired') : undefined}
              hint={t('notificationsPage.boundProjectsHint')}
              label={t('notificationsPage.boundProjects')}
              required
            >
              <Controller
                control={form.control}
                name="projectIds"
                render={({ field }) => (
                  <ProjectSpaceMultiSelect
                    projects={manageableProjects}
                    value={field.value}
                    onChange={field.onChange}
                  />
                )}
              />
            </Field>
          )}

          {values.mode === 'preset' && selectedPreset && (
            <>
              <Field className="md:col-span-2" label={t('notificationsPage.preset')} required>
                <Select
                  disabled={Boolean(channel)}
                  value={selectedPreset.id}
                  onChange={(event) => {
                    form.setValue('presetId', event.target.value, { shouldDirty: true, shouldValidate: true })
                    form.setValue('presetSecrets', {}, { shouldDirty: true })
                  }}
                >
                  {presets.map(item => (
                    <option key={item.id} value={item.id}>
                      {`${item.name} — ${item.description}`}
                    </option>
                  ))}
                </Select>
              </Field>
              {selectedPreset.secretFields.map(secretField => (
                <Field
                  key={secretField}
                  className="md:col-span-2"
                  hint={existingSecrets[secretField] ? t('notificationsPage.secretKeepHint') : undefined}
                  label={presetSecretFieldLabel(secretField, t)}
                  required={!existingSecrets[secretField]}
                >
                  <Input
                    autoComplete="off"
                    placeholder={secretField === 'webhookUrl' || secretField === 'appUrl' ? 'https://' : undefined}
                    type={secretField === 'token' ? 'password' : 'text'}
                    value={values.presetSecrets[secretField] ?? ''}
                    onChange={(event) => {
                      form.setValue(`presetSecrets.${secretField}`, event.target.value, { shouldDirty: true })
                    }}
                  />
                </Field>
              ))}
              {(selectedPreset.optionalSecretFields ?? []).map(secretField => (
                <Field
                  key={secretField}
                  className="md:col-span-2"
                  hint={existingSecrets[secretField]
                    ? t('notificationsPage.secretKeepHint')
                    : (secretField === 'signSecret' ? t('notificationsPage.signSecretHint') : undefined)}
                  label={presetSecretFieldLabel(secretField, t)}
                >
                  <Input
                    autoComplete="new-password"
                    type="password"
                    value={values.presetSecrets[secretField] ?? ''}
                    onChange={(event) => {
                      form.setValue(`presetSecrets.${secretField}`, event.target.value, { shouldDirty: true })
                    }}
                  />
                </Field>
              ))}
            </>
          )}

          {values.mode === 'webhook' && (
            <>
              <Field label={t('notificationsPage.webhookMethod')}>
                <Select
                  value={values.webhookMethod}
                  onChange={event => form.setValue('webhookMethod', event.target.value === 'PUT' ? 'PUT' : 'POST', { shouldDirty: true })}
                >
                  <option value="POST">POST</option>
                  <option value="PUT">PUT</option>
                </Select>
              </Field>
              <Field label={t('notificationsPage.webhookUrl')} required>
                <Input {...form.register('webhookUrlCustom')} placeholder="https://example.com/webhook" type="url" />
              </Field>
              <Field
                className="md:col-span-2"
                error={headersValid ? undefined : t('notificationsPage.headersJsonInvalid')}
                label={t('notificationsPage.headersJson')}
              >
                <Textarea {...form.register('webhookHeadersJson')} className="min-h-24 font-mono text-xs" />
              </Field>
              <div className="md:col-span-2">
                <ProgressiveSection
                  defaultOpen={Boolean(values.webhookBodyTemplate.trim() || values.extraSecretsText.trim())}
                  hint={t('notificationsPage.customBodyHint')}
                  title={t('notificationsPage.jsonBodyTemplate')}
                >
                  <div className="grid gap-3">
                    <Field label={t('notificationsPage.jsonBodyTemplate')}>
                      <Textarea {...form.register('webhookBodyTemplate')} className="min-h-28 font-mono text-xs" />
                    </Field>
                    <Field
                      hint={Object.keys(existingSecrets).some(key => existingSecrets[key]) ? t('notificationsPage.secretKeepHint') : undefined}
                      label={t('notificationsPage.secretLines')}
                    >
                      <Textarea
                        {...form.register('extraSecretsText')}
                        className="min-h-20 font-mono text-xs"
                        placeholder={t('notificationsPage.secretPlaceholder')}
                      />
                    </Field>
                  </div>
                </ProgressiveSection>
              </div>
            </>
          )}

          {values.mode === 'smtp' && (
            <>
              <Field label={t('notificationsPage.smtpHost')} required>
                <Input {...form.register('smtpHost')} placeholder="smtp.example.com" />
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field label={t('notificationsPage.smtpPort')} required>
                  <Input
                    {...form.register('smtpPort', { valueAsNumber: true })}
                    max={65535}
                    min={1}
                    type="number"
                  />
                </Field>
                <Field label={t('notificationsPage.smtpSecurity')}>
                  <Select
                    value={values.smtpSecurity}
                    onChange={(event) => {
                      const security = event.target.value as 'starttls' | 'tls' | 'none'
                      form.setValue('smtpSecurity', security, { shouldDirty: true })
                      if (security === 'tls' && values.smtpPort === 587)
                        form.setValue('smtpPort', 465, { shouldDirty: true })
                      else if (security === 'starttls' && values.smtpPort === 465)
                        form.setValue('smtpPort', 587, { shouldDirty: true })
                    }}
                  >
                    <option value="starttls">{t('notificationsPage.smtpSecurityStarttls')}</option>
                    <option value="tls">{t('notificationsPage.smtpSecurityTls')}</option>
                    <option value="none">{t('notificationsPage.smtpSecurityNone')}</option>
                  </Select>
                </Field>
              </div>
              <Field label={t('notificationsPage.smtpUsername')}>
                <Input {...form.register('smtpUsername')} autoComplete="off" placeholder="alerts@example.com" />
              </Field>
              <Field
                hint={existingSecrets.password ? t('notificationsPage.secretKeepHint') : undefined}
                label={t('notificationsPage.smtpPassword')}
              >
                <Input {...form.register('smtpPassword')} autoComplete="new-password" type="password" />
              </Field>
              <Field className="md:col-span-2" label={t('notificationsPage.smtpFrom')} required>
                <Input {...form.register('smtpFrom')} placeholder="Luna DevOps <alerts@example.com>" />
              </Field>
              <Field
                className="md:col-span-2"
                hint={t('notificationsPage.smtpDefaultToHint')}
                label={t('notificationsPage.smtpDefaultTo')}
              >
                <Textarea {...form.register('smtpToText')} className="min-h-16 text-sm" placeholder="oncall@example.com, team@example.com" />
              </Field>
              <Field className="md:col-span-2" label={t('notificationsPage.smtpDefaultCc')}>
                <Input {...form.register('smtpCcText')} placeholder="lead@example.com" />
              </Field>
              <Field className="md:col-span-2" label={t('notificationsPage.testRecipientLabel')}>
                <Input
                  {...form.register('testRecipient')}
                  placeholder={t('notificationsPage.testRecipientPlaceholder')}
                  type="email"
                />
              </Field>
            </>
          )}
        </div>

        <Controller
          control={form.control}
          name="enabled"
          render={({ field }) => (
            <ControlledCheckboxField field={field}>
              {t('notificationsPage.channelEnabled')}
            </ControlledCheckboxField>
          )}
        />

        <DialogFooter className="gap-2 sm:justify-between">
          <Button
            disabled={saving || draftTestMutation.isPending || !draftTestPayload}
            type="button"
            variant="outline"
            onClick={() => draftTestPayload && draftTestMutation.mutate(draftTestPayload)}
          >
            <FlaskConical className="size-4" />
            {t('notificationsPage.testDraft')}
          </Button>
          <div className="flex items-center justify-end gap-2">
            <Button disabled={saving || draftTestMutation.isPending} type="button" variant="outline" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button disabled={saving || draftTestMutation.isPending || !payload} type="submit">
              {channel ? t('common.save') : t('notificationsPage.createChannel')}
            </Button>
          </div>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
