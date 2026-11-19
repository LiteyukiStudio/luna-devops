import type { Application, DeploymentTarget, NotificationChannel, NotificationRule, NotificationRulePayload, NotificationTemplate, Project } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMemo } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { ProjectSpaceMultiSelect } from '@/components/common/project-space-select'
import { SearchMultiSelect } from '@/components/common/search-select'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import {
  ALL_NOTIFICATION_EVENTS,
  buildNotificationRulePayload,
  channelHasDefaultSmtpRecipients,
  COMMON_TIME_ZONES,
  defaultRuleFormValues,
  hasAdvancedFilter,
  NOTIFICATION_EVENT_GROUPS,
  notificationEventGroupLabel,
  notificationEventLabel,
  notificationRuleFormSchema,
  parseNotificationRuleAdvancedFilter,
  RECOMMENDED_NOTIFICATION_EVENTS,
  smtpChannelMissingRuleRecipient,
} from './notification-models'

export function NotificationRuleDialog({
  applications,
  applicationsLoading,
  channels,
  contextMode,
  deploymentTargets,
  deploymentTargetsLoading,
  fixedProjectId,
  open,
  projects,
  projectsLoading,
  rule,
  saving,
  templates,
  onClose,
  onSave,
}: {
  applications?: Application[]
  applicationsLoading?: boolean
  channels: NotificationChannel[]
  contextMode: 'project' | 'global'
  deploymentTargets?: DeploymentTarget[]
  deploymentTargetsLoading?: boolean
  fixedProjectId?: string
  open: boolean
  projects?: Project[]
  projectsLoading?: boolean
  rule?: NotificationRule
  saving: boolean
  templates?: NotificationTemplate[]
  onClose: () => void
  onSave: (payload: NotificationRulePayload, ruleId?: string) => void
}) {
  if (!open)
    return null

  const dialogKey = rule ? `edit-rule-${rule.id}` : `new-rule-${contextMode}-${fixedProjectId ?? 'global'}`
  return (
    <Dialog open onOpenChange={nextOpen => !nextOpen && onClose()}>
      <NotificationRuleDialogBody
        key={dialogKey}
        applications={applications}
        applicationsLoading={applicationsLoading}
        channels={channels}
        contextMode={contextMode}
        deploymentTargets={deploymentTargets}
        deploymentTargetsLoading={deploymentTargetsLoading}
        fixedProjectId={fixedProjectId}
        projects={projects}
        projectsLoading={projectsLoading}
        rule={rule}
        saving={saving}
        templates={templates}
        onClose={onClose}
        onSave={onSave}
      />
    </Dialog>
  )
}

function NotificationRuleDialogBody({
  applications,
  applicationsLoading,
  channels,
  contextMode,
  deploymentTargets,
  deploymentTargetsLoading,
  fixedProjectId,
  projects,
  projectsLoading,
  rule,
  saving,
  templates,
  onClose,
  onSave,
}: {
  applications?: Application[]
  applicationsLoading?: boolean
  channels: NotificationChannel[]
  contextMode: 'project' | 'global'
  deploymentTargets?: DeploymentTarget[]
  deploymentTargetsLoading?: boolean
  fixedProjectId?: string
  projects?: Project[]
  projectsLoading?: boolean
  rule?: NotificationRule
  saving: boolean
  templates?: NotificationTemplate[]
  onClose: () => void
  onSave: (payload: NotificationRulePayload, ruleId?: string) => void
}) {
  const { t } = useTranslation()
  const resolvedApplications = useMemo(() => applications ?? [], [applications])
  const resolvedDeploymentTargets = useMemo(() => deploymentTargets ?? [], [deploymentTargets])
  const resolvedProjects = useMemo(() => projects ?? [], [projects])
  const resolvedTemplates = useMemo(() => templates ?? [], [templates])
  const form = useForm({
    defaultValues: defaultRuleFormValues({
      contextMode,
      fixedProjectId,
      rule,
    }),
    mode: 'onChange',
    resolver: zodResolver(notificationRuleFormSchema),
  })

  const values = form.watch()
  const timeZoneOptions = useMemo(() => {
    const current = values.timeZone.trim()
    return current && !COMMON_TIME_ZONES.includes(current)
      ? [current, ...COMMON_TIME_ZONES]
      : [...COMMON_TIME_ZONES]
  }, [values.timeZone])

  const applicationOptions = useMemo(() => resolvedApplications.map(app => ({
    description: app.identifier,
    label: app.name,
    value: app.id,
  })), [resolvedApplications])

  const applicationNameById = useMemo(
    () => new Map(resolvedApplications.map(app => [app.id, app.name])),
    [resolvedApplications],
  )

  const activeApplicationIds = useMemo(() => {
    if (!applications)
      return values.applicationIds
    const allowed = new Set(applications.map(app => app.id))
    return values.applicationIds.filter(id => allowed.has(id))
  }, [applications, values.applicationIds])

  const deploymentTargetOptions = useMemo(() => resolvedDeploymentTargets
    .filter(target => activeApplicationIds.length === 0 || activeApplicationIds.includes(target.applicationId))
    .map(target => ({
      description: applicationNameById.get(target.applicationId) ?? target.stage,
      label: target.name,
      value: target.id,
    })), [activeApplicationIds, applicationNameById, resolvedDeploymentTargets])

  const activeDeploymentTargetIds = useMemo(() => {
    if (!deploymentTargets)
      return values.deploymentTargetIds
    const allowed = new Set(
      deploymentTargets
        .filter(target => activeApplicationIds.length === 0 || activeApplicationIds.includes(target.applicationId))
        .map(target => target.id),
    )
    return values.deploymentTargetIds.filter(id => allowed.has(id))
  }, [activeApplicationIds, deploymentTargets, values.deploymentTargetIds])

  const staleApplicationsExhausted = applications !== undefined
    && !applicationsLoading
    && values.applicationIds.length > 0
    && activeApplicationIds.length === 0

  const staleDeploymentTargetsExhausted = deploymentTargets !== undefined
    && !deploymentTargetsLoading
    && values.deploymentTargetIds.length > 0
    && activeDeploymentTargetIds.length === 0

  const selectedActiveProjectCount = useMemo(() => {
    if (!projects)
      return values.projectIds.length
    const validProjectIds = new Set(projects.map(project => project.id))
    return values.projectIds.filter(projectId => validProjectIds.has(projectId)).length
  }, [projects, values.projectIds])

  const filterOptionsLoading = (contextMode === 'project'
    && ((values.applicationIds.length > 0 && Boolean(applicationsLoading))
      || (values.deploymentTargetIds.length > 0 && Boolean(deploymentTargetsLoading))))
    || (contextMode === 'global' && values.scope === 'projects' && values.projectIds.length > 0 && Boolean(projectsLoading))

  const advancedFilterValid = contextMode !== 'global' || parseNotificationRuleAdvancedFilter(values.filterText) !== null
  const payload = buildNotificationRulePayload({
    availableApplications: applications,
    availableChannels: channels,
    availableDeploymentTargets: deploymentTargets,
    availableProjects: projects,
    availableTemplates: templates,
    contextMode,
    fixedProjectId,
    values,
  })
  const canSave = payload !== null && !filterOptionsLoading

  const toggleEvent = (eventType: string, checked: boolean) => {
    const next = checked
      ? [...values.eventTypes, eventType]
      : values.eventTypes.filter(item => item !== eventType)
    form.setValue('eventTypes', next, { shouldDirty: true, shouldValidate: true })
  }

  const toggleChannel = (channelId: string, checked: boolean) => {
    const activeChannelIds = new Set(channels.map(channel => channel.id))
    const base = values.channelIds.filter(item => activeChannelIds.has(item))
    const next = checked
      ? [...base, channelId]
      : base.filter(item => item !== channelId)
    form.setValue('channelIds', next, { shouldDirty: true, shouldValidate: true })
  }

  const submit = form.handleSubmit((submittedValues) => {
    const nextPayload = buildNotificationRulePayload({
      availableApplications: applications,
      availableChannels: channels,
      availableDeploymentTargets: deploymentTargets,
      availableProjects: projects,
      availableTemplates: templates,
      contextMode,
      fixedProjectId,
      values: submittedValues,
    })
    if (!nextPayload) {
      toast.error(t('notificationsPage.ruleInvalid'))
      return
    }
    onSave(nextPayload, rule?.id)
  })

  return (
    <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-3xl">
      <DialogHeader>
        <DialogTitle>{t('notificationsPage.ruleDialog')}</DialogTitle>
        <DialogDescription>{t('notificationsPage.ruleDialogDescription')}</DialogDescription>
      </DialogHeader>
      <form className="grid gap-4" onSubmit={submit}>
        <div className="grid gap-4 md:grid-cols-2">
          <Field className={contextMode === 'project' ? 'md:col-span-2' : undefined} label={t('notificationsPage.name')} required>
            <Input {...form.register('name')} />
          </Field>

          {contextMode === 'global' && (
            <Field label={t('notificationsPage.template')}>
              <Select
                value={resolvedTemplates.some(item => item.id === values.templateId) ? values.templateId : ''}
                onChange={event => form.setValue('templateId', event.target.value, { shouldDirty: true })}
              >
                <option value="">{t('notificationsPage.defaultTemplate')}</option>
                {resolvedTemplates.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}
              </Select>
            </Field>
          )}

          {contextMode === 'global' && (
            <>
              <Field className="md:col-span-2" label={t('notificationsPage.ruleScope')}>
                <Select
                  value={values.scope}
                  onChange={(event) => {
                    const nextScope = event.target.value === 'all' ? 'all' : 'projects'
                    form.setValue('scope', nextScope, { shouldDirty: true, shouldValidate: true })
                    if (nextScope === 'all')
                      form.setValue('projectIds', [], { shouldDirty: true, shouldValidate: true })
                  }}
                >
                  <option value="projects">{t('notificationsPage.ruleScopeProjects')}</option>
                  <option value="all">{t('notificationsPage.ruleScopeAll')}</option>
                </Select>
              </Field>

              {values.scope === 'projects'
                ? (
                    <Field
                      className="md:col-span-2"
                      error={!projectsLoading && selectedActiveProjectCount === 0 ? t('notificationsPage.ruleProjectsRequired') : undefined}
                      hint={t('notificationsPage.ruleProjectsHint')}
                      label={t('notificationsPage.ruleProjects')}
                      required
                    >
                      <ProjectSpaceMultiSelect
                        loading={projectsLoading}
                        projects={resolvedProjects}
                        pruneUnknownValues
                        value={values.projectIds}
                        onChange={projectIds => form.setValue('projectIds', projectIds, { shouldDirty: true, shouldValidate: true })}
                      />
                    </Field>
                  )
                : (
                    <div className="rounded-container bg-warning-subtle p-group text-sm text-warning md:col-span-2" role="status">
                      {t('notificationsPage.ruleScopeAllWarning')}
                    </div>
                  )}
            </>
          )}

          {contextMode === 'project' && (
            <>
              <Field
                error={staleApplicationsExhausted ? t('errors.notification.rule_application_not_found') : undefined}
                label={t('notificationsPage.filterApplications')}
              >
                <SearchMultiSelect
                  loading={applicationsLoading}
                  options={applicationOptions}
                  placeholder={t('notificationsPage.filterApplicationsPlaceholder')}
                  pruneUnknownValues
                  searchPlaceholder={t('notificationsPage.filterApplicationsSearch')}
                  selectedLabel={selected => selected.map(item => item.label).join(', ')}
                  value={values.applicationIds}
                  onValueChange={(next) => {
                    form.setValue('applicationIds', next, { shouldDirty: true, shouldValidate: true })
                    if (deploymentTargets && next.length > 0) {
                      const targetById = new Map(deploymentTargets.map(target => [target.id, target]))
                      const prunedTargets = values.deploymentTargetIds.filter((id) => {
                        const target = targetById.get(id)
                        return !target || next.includes(target.applicationId)
                      })
                      if (prunedTargets.length !== values.deploymentTargetIds.length)
                        form.setValue('deploymentTargetIds', prunedTargets, { shouldDirty: true, shouldValidate: true })
                    }
                  }}
                />
              </Field>
              <Field
                error={staleDeploymentTargetsExhausted ? t('errors.notification.rule_deployment_target_not_found') : undefined}
                label={t('notificationsPage.filterDeploymentTargets')}
              >
                <SearchMultiSelect
                  loading={deploymentTargetsLoading}
                  options={deploymentTargetOptions}
                  placeholder={t('notificationsPage.filterDeploymentTargetsPlaceholder')}
                  pruneUnknownValues
                  searchPlaceholder={t('notificationsPage.filterDeploymentTargetsSearch')}
                  selectedLabel={selected => selected.map(item => item.label).join(', ')}
                  value={values.deploymentTargetIds}
                  onValueChange={next => form.setValue('deploymentTargetIds', next, { shouldDirty: true, shouldValidate: true })}
                />
              </Field>
            </>
          )}
        </div>

        <div className="grid gap-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-sm font-medium">{t('notificationsPage.eventTypes')}</span>
            <div className="flex flex-wrap items-center gap-1.5">
              <Button
                size="sm"
                type="button"
                variant="outline"
                onClick={() => form.setValue('eventTypes', [...RECOMMENDED_NOTIFICATION_EVENTS], { shouldDirty: true, shouldValidate: true })}
              >
                {t('notificationsPage.selectRecommendedEvents')}
              </Button>
              <Button
                size="sm"
                type="button"
                variant="ghost"
                onClick={() => form.setValue('eventTypes', [...ALL_NOTIFICATION_EVENTS], { shouldDirty: true, shouldValidate: true })}
              >
                {t('notificationsPage.selectAllEvents')}
              </Button>
            </div>
          </div>

          <div className="grid gap-3 rounded-container border border-border p-3 md:grid-cols-2">
            {NOTIFICATION_EVENT_GROUPS.map(group => (
              <fieldset key={group.key} className="grid content-start gap-2 rounded-control bg-surface-subtle/60 p-2.5">
                <legend className="mb-1 text-xs font-semibold text-muted-foreground">
                  {notificationEventGroupLabel(group.key, t)}
                </legend>
                {group.events.map(eventType => (
                  <label key={eventType} className="flex items-start gap-2 text-sm">
                    <Checkbox
                      checked={values.eventTypes.includes(eventType)}
                      className="mt-0.5"
                      onCheckedChange={checked => toggleEvent(eventType, checked === true)}
                    />
                    <span className="leading-snug">{notificationEventLabel(eventType, t)}</span>
                  </label>
                ))}
              </fieldset>
            ))}
          </div>
        </div>

        <Field label={t('notificationsPage.channels')} required>
          <div className="grid gap-2.5 rounded-container border border-border p-3">
            {channels.length === 0 && (
              <p className="text-sm text-muted-foreground">{t('notificationsPage.emptyChannels')}</p>
            )}
            {channels.map((channel) => {
              const checked = values.channelIds.includes(channel.id)
              const targetDraft = values.channelTargets[channel.id]
              const missingSmtpRecipient = checked && smtpChannelMissingRuleRecipient(channel, targetDraft)
              const hasDefaultRecipients = channelHasDefaultSmtpRecipients(channel)
              return (
                <div key={channel.id} className="grid gap-2">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <label className="flex items-center gap-2 text-sm font-medium">
                      <Checkbox
                        checked={checked}
                        onCheckedChange={next => toggleChannel(channel.id, next === true)}
                      />
                      <span>{channel.name}</span>
                    </label>
                    <div className="flex items-center gap-1.5">
                      <StatusBadge tone="neutral">
                        {channel.adapterKind === 'smtp' ? t('notificationsPage.adapters.smtp') : t('notificationsPage.adapters.webhook')}
                      </StatusBadge>
                      {channel.scope === 'global' && (
                        <StatusBadge tone="info">{t('notificationsPage.scopeGlobal')}</StatusBadge>
                      )}
                    </div>
                  </div>

                  {checked && channel.adapterKind === 'smtp' && (
                    <div className="ml-6 grid gap-3 rounded-control border border-border bg-surface-subtle/70 p-3 md:grid-cols-2">
                      <p className="text-xs font-medium text-foreground md:col-span-2">
                        {t('notificationsPage.smtpOverrideTitle')}
                      </p>
                      <Field
                        error={missingSmtpRecipient ? t('notificationsPage.smtpRecipientRequiredError') : undefined}
                        hint={hasDefaultRecipients || channel.canManage === false
                          ? t('notificationsPage.smtpOverrideOptionalHint')
                          : t('notificationsPage.smtpOverrideRequiredHint')}
                        label={t('notificationsPage.smtpOverrideTo')}
                        required={!hasDefaultRecipients && channel.canManage !== false}
                      >
                        <Input
                          placeholder="oncall@example.com, dev@example.com"
                          value={targetDraft?.toText ?? ''}
                          onChange={(event) => {
                            form.setValue(`channelTargets.${channel.id}`, {
                              toText: event.target.value,
                              ccText: targetDraft?.ccText ?? '',
                            }, { shouldDirty: true, shouldValidate: true })
                          }}
                        />
                      </Field>
                      <Field label={t('notificationsPage.smtpOverrideCc')}>
                        <Input
                          placeholder="lead@example.com"
                          value={targetDraft?.ccText ?? ''}
                          onChange={(event) => {
                            form.setValue(`channelTargets.${channel.id}`, {
                              toText: targetDraft?.toText ?? '',
                              ccText: event.target.value,
                            }, { shouldDirty: true })
                          }}
                        />
                      </Field>
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </Field>

        <div className="grid gap-3 rounded-container border border-border p-3 md:grid-cols-3">
          <p className="text-sm font-medium md:col-span-3">{t('notificationsPage.throttlingSectionTitle')}</p>
          <Field hint={t('notificationsPage.remindIntervalHint')} label={t('notificationsPage.remindIntervalMinutes')}>
            <Input
              {...form.register('remindIntervalMinutes', { valueAsNumber: true })}
              max={10080}
              min={0}
              type="number"
            />
          </Field>
          <Field hint={t('notificationsPage.maxRemindersHint')} label={t('notificationsPage.maxReminders')}>
            <Input
              {...form.register('maxReminders', { valueAsNumber: true })}
              max={100}
              min={0}
              type="number"
            />
          </Field>
          <Field label={t('notificationsPage.timeZone')}>
            <Select
              value={values.timeZone}
              onChange={event => form.setValue('timeZone', event.target.value, { shouldDirty: true })}
            >
              {timeZoneOptions.map(zone => <option key={zone} value={zone}>{zone}</option>)}
            </Select>
          </Field>
          <div className="md:col-span-3">
            <Controller
              control={form.control}
              name="notifyOnRecovery"
              render={({ field }) => (
                <ControlledCheckboxField description={t('notificationsPage.notifyOnRecoveryHint')} field={field}>
                  {t('notificationsPage.notifyOnRecovery')}
                </ControlledCheckboxField>
              )}
            />
          </div>
        </div>

        {contextMode === 'global' && (
          <ProgressiveSection
            defaultOpen={!advancedFilterValid || hasAdvancedFilter(values.filterText)}
            hint={t('notificationsPage.filterJsonHint')}
            summary={!advancedFilterValid
              ? t('notificationsPage.filterSummaryInvalid')
              : hasAdvancedFilter(values.filterText) ? t('notificationsPage.filterSummarySet') : t('notificationsPage.filterSummaryNone')}
            title={t('notificationsPage.advancedFilter')}
          >
            <Field error={advancedFilterValid ? undefined : t('notificationsPage.filterJsonInvalid')} label={t('notificationsPage.filterJson')}>
              <Textarea {...form.register('filterText')} className="min-h-28 font-mono text-xs" />
            </Field>
          </ProgressiveSection>
        )}

        <Controller
          control={form.control}
          name="enabled"
          render={({ field }) => (
            <ControlledCheckboxField field={field}>
              {t('notificationsPage.ruleEnabled')}
            </ControlledCheckboxField>
          )}
        />

        <DialogFooter>
          <Button disabled={saving} type="button" variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button disabled={saving || !canSave} type="submit">
            {rule ? t('common.save') : t('notificationsPage.createRule')}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
