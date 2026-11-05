import type { InstallFormValues } from './install-form'
import type { AppTemplate, AppTemplateInstallPayload, AppTemplateSummary } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, PackagePlus } from 'lucide-react'
import { useId, useState } from 'react'
import { Controller, useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { ProjectSpaceSelect } from '@/components/common/project-space-select'
import { StatusBadge } from '@/components/common/status-badge'
import { UnitInput } from '@/components/common/unit-input'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { APPLICATION_IDENTIFIER_MIN_LENGTH } from '@/lib/identifier-limits'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { projectVolumeCapabilities } from '@/lib/project-volume-capabilities'
import { isPlatformAdmin } from '@/lib/roles'
import { cn } from '@/lib/utils'
import {
  applicationIdentifierBaseMaxLength,
  createInstallFormSchema,
  defaultInstallFormValues,
  emptyComponentProjectVolumeIds,
  hasAdvancedError,
  hasProjectVolume,
  INSTALL_STAGES,
  isEssentialTemplateValue,
  isMultiComponentTemplate,
  isSystemComponentTemplate,
  normalizeIdentifierInput,
  projectInstallPayload,
  projectVolumeModeOf,
} from './install-form'
import { TemplateIcon, TemplateSourceLinks } from './template-card'
import { TemplateProjectVolumePicker } from './template-project-volume-picker'
import { templateCategoryLabel, templateDescription } from './template-text'

interface SystemInstallRequest {
  templateId: string
  clusterId: string
  apiBaseUrl: string
  traefikMetricsUrl?: string
  image?: string
  provisionAccess?: boolean
}

/** 模板详情与安装抽屉：头部展示模板信息，正文首屏只问项目空间、名称和环境，其余参数默认折叠。 */
export function InstallTemplateSheet({ summary, onClose }: { summary: AppTemplateSummary | null, onClose: () => void }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const templateDetail = useQuery({
    queryKey: ['app-template', summary?.id],
    queryFn: () => api.getAppTemplate(summary!.id),
    enabled: Boolean(summary?.id),
  })

  const installTemplate = useMutation({
    mutationFn: (payload: AppTemplateInstallPayload & { templateId: string, projectId: string }) =>
      api.installAppTemplate(payload.projectId, payload.templateId, payload),
    onSuccess: async (result) => {
      toast.success(t('appTemplatesPage.installStarted'))
      onClose()
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['projects'] }),
        queryClient.invalidateQueries({ queryKey: ['applications', result.application.projectId] }),
      ])
      navigate(`/projects/${result.application.projectId}/apps/${result.application.id}?tab=deployments`)
    },
    onError: error => toast.error(error.message),
  })

  const installSystemTemplate = useMutation({
    mutationFn: (payload: SystemInstallRequest) =>
      api.installSystemAppTemplate(payload.templateId, {
        apiBaseUrl: payload.apiBaseUrl,
        clusterId: payload.clusterId,
        image: payload.image,
        mode: 'traefik-metrics',
        provisionAccess: payload.provisionAccess,
        traefikMetricsUrl: payload.traefikMetricsUrl,
      }),
    onSuccess: async (result) => {
      toast.success(t('appTemplatesPage.systemInstallStarted'))
      onClose()
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['projects'] }),
        queryClient.invalidateQueries({ queryKey: ['system-components'] }),
        queryClient.invalidateQueries({ queryKey: ['billing', 'gateway-traffic-status'] }),
      ])
      if (result.application)
        navigate(`/projects/${result.application.projectId}/apps/${result.application.id}?tab=deployments`)
    },
    onError: error => toast.error(error.message),
  })

  const installing = installTemplate.isPending || installSystemTemplate.isPending
  const template = templateDetail.data?.id === summary?.id ? templateDetail.data : undefined

  return (
    <Sheet
      open={Boolean(summary)}
      onOpenChange={(open) => {
        if (!open && !installing)
          onClose()
      }}
    >
      <SheetContent className="w-full gap-0 p-0 sm:max-w-lg">
        {summary && (
          <>
            <SheetHeader className="shrink-0 gap-3 border-b border-border px-5 py-4 pr-14">
              <div className="flex min-w-0 items-start gap-3">
                <TemplateIcon icon={summary.icon} />
                <div className="min-w-0 flex-1">
                  <SheetTitle className="truncate text-base">{t('appTemplatesPage.installDialogTitle', { name: summary.name })}</SheetTitle>
                  <div className="mt-1 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                    <span>{templateCategoryLabel(summary.category, t)}</span>
                    {summary.version && <span className="font-mono">{summary.version}</span>}
                    {isMultiComponentTemplate(summary) && <span>{t('appTemplatesPage.componentCount', { count: summary.components.length })}</span>}
                    {isSystemComponentTemplate(summary) && <StatusBadge tone="neutral">{t('appTemplatesPage.platformComponent')}</StatusBadge>}
                  </div>
                </div>
              </div>
              <SheetDescription className="leading-5">{templateDescription(summary, t)}</SheetDescription>
              <TemplateSourceLinks template={summary} />
            </SheetHeader>
            {templateDetail.isError && (
              <div className="p-5">
                <ErrorState title={templateDetail.error.message} />
              </div>
            )}
            {!templateDetail.isError && !template && <InstallFormSkeleton />}
            {template && (
              <InstallTemplateForm
                key={template.id}
                installing={installing}
                template={template}
                onCancel={onClose}
                onInstallProject={payload => installTemplate.mutate(payload)}
                onInstallSystem={payload => installSystemTemplate.mutate(payload)}
              />
            )}
          </>
        )}
      </SheetContent>
    </Sheet>
  )
}

function InstallFormSkeleton() {
  return (
    <div aria-busy="true" className="grid gap-4 p-5">
      {Array.from({ length: 3 }, (_, index) => (
        <div key={index} className="grid gap-2">
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-8 w-full" />
        </div>
      ))}
    </div>
  )
}

function InstallTemplateForm({ installing, template, onCancel, onInstallProject, onInstallSystem }: {
  installing: boolean
  template: AppTemplate
  onCancel: () => void
  onInstallProject: (payload: AppTemplateInstallPayload & { templateId: string, projectId: string }) => void
  onInstallSystem: (payload: SystemInstallRequest) => void
}) {
  const { t } = useTranslation()
  const { user } = useSession()
  const advancedTitleId = useId()
  const advancedSummaryId = useId()
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [defaultValues] = useState(() => defaultInstallFormValues(template))
  const form = useForm<InstallFormValues>({
    defaultValues,
    resolver: zodResolver(createInstallFormSchema(template, t)),
  })
  const { control, formState: { errors }, register, setValue } = form
  const [selectedProjectId, clusterId, applicationIdentifier, imageRef, replicas, cpuRequest, memoryRequest] = useWatch({
    control,
    name: ['projectId', 'clusterId', 'applicationIdentifier', 'imageRef', 'replicas', 'cpuRequest', 'memoryRequest'],
  })

  const systemComponent = isSystemComponentTemplate(template)
  const multiComponent = isMultiComponentTemplate(template)
  const components = template.components
  const requiresProjectVolume = hasProjectVolume(template.dataVolumes)
  const projectVolumeMode = projectVolumeModeOf(template.dataVolumes)
  const identifierMaxLength = applicationIdentifierBaseMaxLength(template)
  const essentialValues = template.values.filter(isEssentialTemplateValue)
  const optionalValues = template.values.filter(value => !isEssentialTemplateValue(value))
  const volumeCount = multiComponent
    ? components.filter(component => hasProjectVolume(component.dataVolumes)).length
    : requiresProjectVolume ? 1 : 0

  const projects = useQuery({ queryKey: ['projects'], queryFn: () => api.listProjects(), enabled: !systemComponent })
  const projectItems = projects.data ?? []
  const projectId = projectItems.some(project => project.id === selectedProjectId)
    ? selectedProjectId
    : projectItems[0]?.id ?? ''
  const projectDetail = useQuery({
    queryKey: ['project', projectId],
    queryFn: () => api.getProject(projectId),
    enabled: !systemComponent && Boolean(projectId),
  })
  const clusters = useQuery({
    queryKey: ['runtime-clusters', systemComponent ? 'system' : projectId],
    queryFn: () => api.listRuntimeClusters(systemComponent ? undefined : projectId),
    enabled: systemComponent || Boolean(projectId),
    ...liveObservationQueryPolicy,
  })
  const clusterItems = clusters.data ?? []
  const effectiveCluster = clusterItems.find(cluster => cluster.id === clusterId)
    ?? clusterItems.find(cluster => cluster.isDefault)
    ?? clusterItems[0]
  const effectiveClusterId = effectiveCluster?.id ?? ''
  const effectiveClusterName = effectiveCluster?.name ?? effectiveClusterId

  const canInstallSystemComponent = isPlatformAdmin(user?.role)
  const canWriteProject = projectVolumeCapabilities(user?.role, projectDetail.data?.currentUserRole, user?.id).canWrite
  const canInstall = systemComponent ? canInstallSystemComponent : Boolean(projectId) && canWriteProject
  const permissionNotice = systemComponent
    ? (!canInstallSystemComponent ? t('appTemplatesPage.systemInstallAdminOnly') : '')
    : (projectDetail.isSuccess && !canWriteProject ? t('appTemplatesPage.projectWriteRequired') : '')

  function resetVolumes() {
    setValue('projectVolumeId', '')
    setValue('componentProjectVolumeIds', emptyComponentProjectVolumeIds(template))
  }

  const submit = form.handleSubmit((value) => {
    if (!canInstall)
      return
    if (systemComponent) {
      onInstallSystem({
        apiBaseUrl: value.values.apiBaseUrl ?? '',
        clusterId: value.clusterId,
        image: value.imageRef.trim(),
        provisionAccess: value.provisionAccess,
        templateId: template.id,
        traefikMetricsUrl: value.values.traefikMetricsUrl ?? '',
      })
      return
    }
    onInstallProject({ ...projectInstallPayload(template, value), projectId, templateId: template.id })
  }, (invalid) => {
    if (hasAdvancedError(template, invalid))
      setAdvancedOpen(true)
  })

  const clusterOptions = clusterItems.map(cluster => (
    <option key={cluster.id} value={cluster.id}>
      {cluster.name}
      {cluster.isDefault ? ` (${t('common.default')})` : ''}
    </option>
  ))

  const templateValueField = (value: AppTemplate['values'][number]) => {
    const label = t(`appTemplatesPage.valueLabels.${value.key}`, { defaultValue: value.label || value.key })
    const required = value.required && !value.autoGenerate
    return (
      <Field
        key={value.key}
        error={errors.values?.[value.key]?.message}
        hint={templateValueHint(template.id, value.key, t)}
        label={label}
        required={required}
      >
        <Input
          aria-label={label}
          autoComplete={value.secret ? 'new-password' : undefined}
          placeholder={templateValuePlaceholder(template.id, value.key, value.autoGenerate, value.default, t)}
          required={required}
          type={value.secret ? 'password' : 'text'}
          {...register(`values.${value.key}`)}
        />
      </Field>
    )
  }

  return (
    <form className="flex min-h-0 flex-1 flex-col" noValidate onSubmit={submit}>
      <div className="grid min-h-0 flex-1 content-start gap-4 overflow-y-auto px-5 py-4">
        {permissionNotice && (
          <Alert variant="warning">
            <AlertDescription className="text-foreground">{permissionNotice}</AlertDescription>
          </Alert>
        )}
        <p className="text-xs leading-5 text-muted-foreground">
          {systemComponent ? t('appTemplatesPage.systemInstallDialogDescription') : t('appTemplatesPage.installDialogDescription')}
        </p>

        {systemComponent
          ? (
              <Field error={errors.clusterId?.message} label={t('appTemplatesPage.runtimeCluster')} required>
                <Select disabled={clusters.isLoading || installing} {...register('clusterId')}>
                  <option value="">{t('appTemplatesPage.selectRuntimeCluster')}</option>
                  {clusterOptions}
                </Select>
              </Field>
            )
          : (
              <>
                <Field label={t('projectSpaces.title')} required>
                  <ProjectSpaceSelect
                    disabled={projectItems.length === 0 || installing}
                    projects={projectItems}
                    value={projectId}
                    onChange={(value) => {
                      setValue('projectId', value)
                      setValue('clusterId', '')
                      resetVolumes()
                    }}
                  />
                </Field>
                <Field error={errors.applicationName?.message} label={t('appTemplatesPage.applicationName')} required>
                  <Input {...register('applicationName')} />
                </Field>
                <Field
                  error={errors.applicationIdentifier?.message}
                  hint={multiComponent
                    ? t('appTemplatesPage.componentIdentifierHint', {
                        identifiers: components.map(component => `${applicationIdentifier}-${component.key}`).join(', '),
                      })
                    : undefined}
                  label={t('appTemplatesPage.applicationIdentifier')}
                  required
                >
                  {fieldControl => (
                    <Controller
                      control={control}
                      name="applicationIdentifier"
                      render={({ field }) => (
                        <Input
                          {...fieldControl}
                          {...field}
                          maxLength={identifierMaxLength}
                          minLength={APPLICATION_IDENTIFIER_MIN_LENGTH}
                          onChange={event => field.onChange(normalizeIdentifierInput(event.target.value))}
                        />
                      )}
                    />
                  )}
                </Field>
                <Field label={t('appTemplatesPage.stage')}>
                  <Select {...register('stage')}>
                    {INSTALL_STAGES.map(stage => (
                      <option key={stage} value={stage}>{t(`appTemplatesPage.stageOptions.${stage}`)}</option>
                    ))}
                  </Select>
                </Field>
              </>
            )}

        {essentialValues.map(templateValueField)}

        <Collapsible className="min-w-0 rounded-container border border-border" open={advancedOpen} onOpenChange={setAdvancedOpen}>
          <CollapsibleTrigger asChild>
            <button
              aria-describedby={advancedSummaryId}
              aria-labelledby={advancedTitleId}
              className="flex w-full min-w-0 items-start justify-between gap-3 rounded-container px-3 py-2.5 text-left transition-colors hover:bg-surface-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              type="button"
            >
              <span className="min-w-0">
                <span className="block text-sm font-medium text-foreground" id={advancedTitleId}>{t('appTemplatesPage.customParameters')}</span>
                <span className="mt-0.5 flex min-w-0 flex-wrap gap-x-3 gap-y-0.5 text-xs text-muted-foreground" id={advancedSummaryId}>
                  {(systemComponent || !multiComponent) && imageRef && <span className="max-w-full truncate font-mono">{imageRef}</span>}
                  {!systemComponent && !multiComponent && (
                    <span>{t('appTemplatesPage.resourceSummary', { cpu: cpuRequest, memory: memoryRequest, replicas })}</span>
                  )}
                  {multiComponent && <span>{t('appTemplatesPage.componentCount', { count: components.length })}</span>}
                  {volumeCount > 0 && <span>{t('appTemplatesPage.volumeRequiredSummary', { count: volumeCount })}</span>}
                </span>
              </span>
              <ChevronDown aria-hidden="true" className={cn('mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform', advancedOpen && 'rotate-180')} />
            </button>
          </CollapsibleTrigger>
          <CollapsibleContent className="grid min-w-0 gap-4 border-t border-border px-3 py-4">
            {systemComponent
              ? (
                  <>
                    <Field label={t('appTemplatesPage.componentNamespace')}>
                      <Input disabled value="luna-system" />
                    </Field>
                    <Field hint={t('appTemplatesPage.imageRefHint')} label={t('appTemplatesPage.imageRef')}>
                      <Input {...register('imageRef')} />
                    </Field>
                    <Controller
                      control={control}
                      name="provisionAccess"
                      render={({ field }) => (
                        <ControlledCheckboxField
                          description={t('appTemplatesPage.provisionAccessDescription')}
                          disabled={installing}
                          field={field}
                        >
                          {t('appTemplatesPage.provisionAccess')}
                        </ControlledCheckboxField>
                      )}
                    />
                  </>
                )
              : (
                  <>
                    <div className="grid gap-4 sm:grid-cols-2">
                      <Field label={t('appTemplatesPage.runtimeCluster')}>
                        <Select
                          disabled={clusters.isLoading || installing}
                          {...register('clusterId', { onChange: resetVolumes })}
                        >
                          <option value="">{t('appTemplatesPage.defaultCluster')}</option>
                          {clusterOptions}
                        </Select>
                      </Field>
                      <Field label={t('appTemplatesPage.deploymentName')}>
                        <Input {...register('deploymentName')} />
                      </Field>
                    </div>
                    {multiComponent
                      ? (
                          <div className="grid min-w-0 gap-2">
                            <div>
                              <h3 className="text-sm font-medium">{t('appTemplatesPage.components')}</h3>
                              <p className="mt-0.5 text-xs leading-5 text-muted-foreground">{t('appTemplatesPage.componentsDescription')}</p>
                            </div>
                            <ul className="divide-y divide-border border-y border-border">
                              {components.map((component) => {
                                const mode = projectVolumeModeOf(component.dataVolumes)
                                return (
                                  <li key={component.key} className="grid min-w-0 gap-2 py-3">
                                    <div className="flex min-w-0 flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
                                      <span className="text-sm font-medium">{component.name}</span>
                                      <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">{component.image}</span>
                                    </div>
                                    <p className="text-xs text-muted-foreground">
                                      {t('appTemplatesPage.componentSpec', {
                                        port: component.servicePort,
                                        replicas: component.defaultReplicas || 1,
                                        cpu: component.defaultCPU,
                                        memory: component.defaultMemory,
                                      })}
                                    </p>
                                    {hasProjectVolume(component.dataVolumes) && (
                                      <Field
                                        error={errors.componentProjectVolumeIds?.[component.key]?.message}
                                        label={t('appTemplatesPage.componentProjectVolume', { name: component.name })}
                                        required
                                      >
                                        {fieldControl => (
                                          <Controller
                                            control={control}
                                            name={`componentProjectVolumeIds.${component.key}`}
                                            render={({ field }) => (
                                              <TemplateProjectVolumePicker
                                                key={`${projectId}:${effectiveClusterId}:${mode}:${component.key}`}
                                                canCreate={canWriteProject}
                                                clusterId={effectiveClusterId}
                                                clusterName={effectiveClusterName}
                                                control={fieldControl}
                                                disabled={installing}
                                                mode={mode}
                                                projectId={projectId}
                                                value={field.value ?? ''}
                                                onChange={(value) => {
                                                  if (value)
                                                    setValue('clusterId', effectiveClusterId)
                                                  field.onChange(value)
                                                }}
                                              />
                                            )}
                                          />
                                        )}
                                      </Field>
                                    )}
                                  </li>
                                )
                              })}
                            </ul>
                          </div>
                        )
                      : (
                          <>
                            <Field error={errors.imageRef?.message} hint={t('appTemplatesPage.imageRefHint')} label={t('appTemplatesPage.imageRef')} required>
                              <Input {...register('imageRef')} />
                            </Field>
                            <div className="grid gap-4 sm:grid-cols-3">
                              <Field error={errors.replicas?.message} label={t('appTemplatesPage.replicas')}>
                                <Input min={1} type="number" {...register('replicas', { valueAsNumber: true })} />
                              </Field>
                              <Field label={t('appTemplatesPage.cpu')}>
                                {fieldControl => (
                                  <Controller
                                    control={control}
                                    name="cpuRequest"
                                    render={({ field }) => (
                                      <UnitInput
                                        className="min-w-0"
                                        inputProps={fieldControl}
                                        unitSelectLabel={t('appTemplatesPage.cpu')}
                                        units={[
                                          { label: 'm', value: 'm' },
                                          { label: t('deploymentsPage.cpuUnits.core'), value: '' },
                                        ]}
                                        value={field.value}
                                        onChange={field.onChange}
                                      />
                                    )}
                                  />
                                )}
                              </Field>
                              <Field label={t('appTemplatesPage.memory')}>
                                {fieldControl => (
                                  <Controller
                                    control={control}
                                    name="memoryRequest"
                                    render={({ field }) => (
                                      <UnitInput
                                        className="min-w-0"
                                        inputProps={fieldControl}
                                        unitSelectLabel={t('appTemplatesPage.memory')}
                                        units={[
                                          { label: 'Mi', value: 'Mi' },
                                          { label: 'Gi', value: 'Gi' },
                                        ]}
                                        value={field.value}
                                        onChange={field.onChange}
                                      />
                                    )}
                                  />
                                )}
                              </Field>
                            </div>
                            {requiresProjectVolume && (
                              <Field error={errors.projectVolumeId?.message} label={t('appTemplatesPage.projectVolume')} required>
                                {fieldControl => (
                                  <Controller
                                    control={control}
                                    name="projectVolumeId"
                                    render={({ field }) => (
                                      <TemplateProjectVolumePicker
                                        key={`${projectId}:${effectiveClusterId}:${projectVolumeMode}:${template.id}`}
                                        canCreate={canWriteProject}
                                        clusterId={effectiveClusterId}
                                        clusterName={effectiveClusterName}
                                        control={fieldControl}
                                        disabled={installing}
                                        mode={projectVolumeMode}
                                        projectId={projectId}
                                        value={field.value}
                                        onChange={(value) => {
                                          if (value)
                                            setValue('clusterId', effectiveClusterId)
                                          field.onChange(value)
                                        }}
                                      />
                                    )}
                                  />
                                )}
                              </Field>
                            )}
                          </>
                        )}
                  </>
                )}

            {optionalValues.length > 0 && (
              <div className="grid min-w-0 gap-4 border-t border-border pt-4">
                <div>
                  <h3 className="text-sm font-medium">{t('appTemplatesPage.templateParameters')}</h3>
                  <p className="mt-0.5 text-xs leading-5 text-muted-foreground">{t('appTemplatesPage.templateParametersDescription')}</p>
                </div>
                {optionalValues.map(templateValueField)}
              </div>
            )}

            {!systemComponent && (
              <Controller
                control={control}
                name="installNow"
                render={({ field }) => (
                  <ControlledCheckboxField
                    className="border-t border-border pt-4"
                    description={t('appTemplatesPage.installNowDescription')}
                    disabled={installing}
                    field={field}
                  >
                    {t('appTemplatesPage.installNow')}
                  </ControlledCheckboxField>
                )}
              />
            )}
          </CollapsibleContent>
        </Collapsible>
      </div>
      <div className="flex shrink-0 flex-col-reverse gap-2 border-t border-border px-5 py-3 sm:flex-row sm:justify-end">
        <Button disabled={installing} type="button" variant="outline" onClick={onCancel}>{t('common.cancel')}</Button>
        <Button disabled={!canInstall || installing} type="submit">
          <PackagePlus className={cn('size-4', installing && 'motion-safe:animate-pulse')} />
          {installing ? t('appTemplatesPage.installing') : t('appTemplatesPage.install')}
        </Button>
      </div>
    </form>
  )
}

type Translate = ReturnType<typeof useTranslation>['t']

function templateValueHint(templateId: string, key: string, t: Translate) {
  if (templateId === 'redis' && key === 'password')
    return t('appTemplatesPage.valueHints.redisPassword')
  if (key === 'apiBaseUrl')
    return t('appTemplatesPage.valueHints.apiBaseUrl')
  if (key === 'traefikMetricsUrl')
    return t('appTemplatesPage.valueHints.traefikMetricsUrl')
}

function templateValuePlaceholder(templateId: string, key: string, autoGenerate: boolean, defaultValue: string, t: Translate) {
  if (templateId === 'redis' && key === 'password')
    return t('appTemplatesPage.valuePlaceholders.redisPassword')
  if (key === 'apiBaseUrl')
    return t('appTemplatesPage.valuePlaceholders.apiBaseUrl')
  if (key === 'traefikMetricsUrl')
    return t('appTemplatesPage.valuePlaceholders.traefikMetricsUrl')
  if (autoGenerate)
    return t('appTemplatesPage.autoGeneratePlaceholder')
  return defaultValue
}
