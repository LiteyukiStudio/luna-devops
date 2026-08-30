import type {
  Application,
  DeploymentServicePort,
  DeploymentTarget,
  ProjectTopologyManualEdge,
  ProjectTopologyManualEdgePayload,
  ServiceBinding,
  ServiceBindingPayload,
} from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useRef } from 'react'
import { Controller, useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { SearchSelect } from '@/components/common/search-select'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Textarea } from '@/components/ui/textarea'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { cn } from '@/lib/utils'
import { projectTopologyKeys } from './project-topology-query'

const envNamePattern = /^[A-Z_][A-Z0-9_]*$/
const applicationTargetValue = '__application__'

export type RelationDialogMode = 'service_binding' | 'manual'

interface RelationFormValues {
  mode: RelationDialogMode
  sourceApplicationId: string
  sourceDeploymentTargetId: string
  targetApplicationId: string
  targetDeploymentTargetId: string
  targetPortName: string
  protocol: 'http' | 'https' | 'tcp'
  path: string
  injectionMode: 'url' | 'host_port'
  urlEnvVar: string
  hostEnvVar: string
  portEnvVar: string
  enabled: boolean
  relationType: 'depends_on' | 'calls' | 'reads_writes' | 'publishes_to' | 'consumes_from'
  manualPort: string
  description: string
}

const relationTypes: RelationFormValues['relationType'][] = ['depends_on', 'calls', 'reads_writes', 'publishes_to', 'consumes_from']

export interface RelationSavedResult {
  requiresRedeploy: boolean
  sourceApplicationId: string
}

interface ProjectTopologyRelationDialogProps {
  applications: Application[]
  editingManualEdge?: ProjectTopologyManualEdge
  editingServiceBinding?: ServiceBinding
  initialMode?: RelationDialogMode
  projectId: string
  onOpenChange: (open: boolean) => void
  onSaved: (result: RelationSavedResult) => void
}

export function ProjectTopologyRelationDialog({
  applications,
  editingManualEdge,
  editingServiceBinding,
  initialMode = 'service_binding',
  projectId,
  onOpenChange,
  onSaved,
}: ProjectTopologyRelationDialogProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const releaseAfterSaveRef = useRef(false)
  const queryClient = useQueryClient()
  const editing = editingServiceBinding ?? editingManualEdge
  const form = useForm<RelationFormValues>({
    resolver: zodResolver(createRelationSchema(t)),
    mode: 'onChange',
    defaultValues: relationDefaultValues(initialMode, editingServiceBinding, editingManualEdge),
  })
  const mode = useWatch({ control: form.control, name: 'mode' })
  const protocol = useWatch({ control: form.control, name: 'protocol' })
  const injectionMode = useWatch({ control: form.control, name: 'injectionMode' })
  const sourceApplicationId = useWatch({ control: form.control, name: 'sourceApplicationId' })
  const sourceDeploymentTargetId = useWatch({ control: form.control, name: 'sourceDeploymentTargetId' })
  const targetApplicationId = useWatch({ control: form.control, name: 'targetApplicationId' })
  const targetDeploymentTargetId = useWatch({ control: form.control, name: 'targetDeploymentTargetId' })

  const sourceTargets = useQuery({
    queryKey: ['deployment-targets', projectId, sourceApplicationId],
    queryFn: () => api.listDeploymentTargets(projectId, sourceApplicationId),
    enabled: Boolean(sourceApplicationId),
    ...liveObservationQueryPolicy,
  })
  const targetTargets = useQuery({
    queryKey: ['deployment-targets', projectId, targetApplicationId],
    queryFn: () => api.listDeploymentTargets(projectId, targetApplicationId),
    enabled: Boolean(targetApplicationId),
    ...liveObservationQueryPolicy,
  })
  const targetDeployment = targetTargets.data?.find(target => target.id === targetDeploymentTargetId)
  const targetPorts = useMemo(() => normalizedServicePorts(targetDeployment), [targetDeployment])
  const applicationOptions = useMemo(() => applications.map(application => ({
    label: application.name,
    description: application.identifier,
    keywords: application.identifier,
    value: application.id,
  })), [applications])
  const sourceTargetOptions = useMemo(() => deploymentTargetOptions(sourceTargets.data ?? []), [sourceTargets.data])
  const targetTargetOptions = useMemo(() => deploymentTargetOptions(targetTargets.data ?? []), [targetTargets.data])
  const optionalSourceTargetOptions = useMemo(() => optionalDeploymentTargetOptions(sourceTargets.data ?? [], t('projectTopology.applicationLevel')), [sourceTargets.data, t])
  const optionalTargetTargetOptions = useMemo(() => optionalDeploymentTargetOptions(targetTargets.data ?? [], t('projectTopology.applicationLevel')), [targetTargets.data, t])

  const saveRelation = useMutation({
    mutationFn: async (values: RelationFormValues) => {
      if (values.mode === 'service_binding') {
        const payload = serviceBindingPayload(values)
        const result = editingServiceBinding
          ? await api.updateServiceBinding(projectId, editingServiceBinding.id, payload)
          : await api.createServiceBinding(projectId, payload)
        return { result, sourceApplicationId: values.sourceApplicationId }
      }
      const payload = manualEdgePayload(values)
      if (editingManualEdge)
        await api.updateProjectTopologyEdge(projectId, editingManualEdge.id, payload)
      else
        await api.createProjectTopologyEdge(projectId, payload)
      return { result: undefined, sourceApplicationId: values.sourceApplicationId }
    },
    onSuccess: ({ result, sourceApplicationId }) => {
      toast.success(t(editing ? 'projectTopology.updated' : 'projectTopology.created'))
      if (result?.requiresRedeploy)
        toast.warning(t('projectTopology.savedNeedsRelease'))
      void queryClient.invalidateQueries({ queryKey: projectTopologyKeys.all(projectId) })
      onSaved({ requiresRedeploy: Boolean(result?.requiresRedeploy), sourceApplicationId })
      onOpenChange(false)
      if (result?.requiresRedeploy && releaseAfterSaveRef.current)
        void navigate(`/projects/${projectId}/apps/${sourceApplicationId}?tab=deployments`)
    },
    onError: error => toast.error(error.message),
  })

  const sourceTargetField = mode === 'service_binding'
    ? (
        <Field error={form.formState.errors.sourceDeploymentTargetId?.message} label={t('projectTopology.form.sourceTarget')} required>
          <SearchSelect
            emptyLabel={t('projectTopology.form.noTargets')}
            loading={sourceTargets.isLoading}
            options={sourceTargetOptions}
            placeholder={t('projectTopology.form.selectDeploymentTarget')}
            searchPlaceholder={t('projectTopology.form.searchTargets')}
            value={sourceDeploymentTargetId}
            onValueChange={value => form.setValue('sourceDeploymentTargetId', value, { shouldDirty: true, shouldValidate: true })}
          />
        </Field>
      )
    : null
  const targetTargetField = mode === 'service_binding'
    ? (
        <Field error={form.formState.errors.targetDeploymentTargetId?.message} label={t('projectTopology.form.targetTarget')} required>
          <SearchSelect
            emptyLabel={t('projectTopology.form.noTargets')}
            loading={targetTargets.isLoading}
            options={targetTargetOptions}
            placeholder={t('projectTopology.form.selectDeploymentTarget')}
            searchPlaceholder={t('projectTopology.form.searchTargets')}
            value={targetDeploymentTargetId}
            onValueChange={(value) => {
              form.setValue('targetDeploymentTargetId', value, { shouldDirty: true, shouldValidate: true })
              form.setValue('targetPortName', '', { shouldDirty: true, shouldValidate: true })
            }}
          />
        </Field>
      )
    : null

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t(editing ? 'projectTopology.form.editTitle' : 'projectTopology.form.createTitle')}</DialogTitle>
          <DialogDescription>{t('projectTopology.form.description')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-4" onSubmit={form.handleSubmit(values => saveRelation.mutate(values))}>
          <Field hint={t('projectTopology.form.modeHint')} label={t('projectTopology.form.mode')} required>
            <RadioGroup
              className="grid gap-3 sm:grid-cols-2"
              disabled={Boolean(editing)}
              value={mode}
              onValueChange={(value) => {
                const nextMode = value as RelationDialogMode
                form.setValue('mode', nextMode, { shouldDirty: true, shouldValidate: true })
                form.setValue('sourceDeploymentTargetId', nextMode === 'manual' ? applicationTargetValue : '', { shouldDirty: true, shouldValidate: true })
                form.setValue('targetDeploymentTargetId', nextMode === 'manual' ? applicationTargetValue : '', { shouldDirty: true, shouldValidate: true })
                form.setValue('targetPortName', '', { shouldDirty: true, shouldValidate: true })
              }}
            >
              <ModeOption
                description={t('projectTopology.form.serviceBindingOptionDescription')}
                title={t('projectTopology.form.serviceBindingOptionTitle')}
                value="service_binding"
              />
              <ModeOption
                description={t('projectTopology.form.manualOptionDescription')}
                hint={t('projectTopology.form.manualOptionHint')}
                title={t('projectTopology.form.manualOptionTitle')}
                value="manual"
              />
            </RadioGroup>
          </Field>

          <ProgressiveSection defaultOpen description={t('projectTopology.form.basicSectionDescription')} title={t('projectTopology.form.basicSection')}>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field error={form.formState.errors.sourceApplicationId?.message} label={t('projectTopology.form.sourceApplication')} required>
                <SearchSelect
                  emptyLabel={t('projectTopology.form.noApplications')}
                  options={applicationOptions}
                  placeholder={t('projectTopology.form.selectApplication')}
                  searchPlaceholder={t('projectTopology.form.searchApplications')}
                  value={sourceApplicationId}
                  onValueChange={(value) => {
                    form.setValue('sourceApplicationId', value, { shouldDirty: true, shouldValidate: true })
                    form.setValue('sourceDeploymentTargetId', mode === 'manual' ? applicationTargetValue : '', { shouldDirty: true, shouldValidate: true })
                  }}
                />
              </Field>
              <Field error={form.formState.errors.targetApplicationId?.message} label={t('projectTopology.form.targetApplication')} required>
                <SearchSelect
                  emptyLabel={t('projectTopology.form.noApplications')}
                  options={applicationOptions}
                  placeholder={t('projectTopology.form.selectApplication')}
                  searchPlaceholder={t('projectTopology.form.searchApplications')}
                  value={targetApplicationId}
                  onValueChange={(value) => {
                    form.setValue('targetApplicationId', value, { shouldDirty: true, shouldValidate: true })
                    form.setValue('targetDeploymentTargetId', mode === 'manual' ? applicationTargetValue : '', { shouldDirty: true, shouldValidate: true })
                    form.setValue('targetPortName', '', { shouldDirty: true, shouldValidate: true })
                  }}
                />
              </Field>
              {sourceTargetField}
              {targetTargetField}
            </div>
            {mode === 'manual' && (
              <Field label={t('projectTopology.form.relationType')} required>
                <NativeSelect {...form.register('relationType')}>
                  {relationTypes.map(type => <option key={type} value={type}>{t(`projectTopology.relationTypes.${type}`)}</option>)}
                </NativeSelect>
              </Field>
            )}
          </ProgressiveSection>

          {mode === 'service_binding' && (
            <ProgressiveSection defaultOpen description={t('projectTopology.form.runtimeSectionDescription')} title={t('projectTopology.form.runtimeSection')}>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field error={form.formState.errors.targetPortName?.message} hint={t('projectTopology.form.targetPortHint')} label={t('projectTopology.form.targetPort')} required>
                  <NativeSelect {...form.register('targetPortName')} disabled={!targetDeploymentTargetId}>
                    <option value="">{t('projectTopology.form.selectPort')}</option>
                    {targetPorts.map(port => <option key={port.name} value={port.name}>{`${port.name} · ${port.port}`}</option>)}
                  </NativeSelect>
                </Field>
                <Field label={t('projectTopology.form.injectionMode')} required>
                  <NativeSelect {...form.register('injectionMode')}>
                    <option value="url" disabled={protocol === 'tcp'}>{t('projectTopology.form.urlMode')}</option>
                    <option value="host_port">{t('projectTopology.form.hostPortMode')}</option>
                  </NativeSelect>
                </Field>
                {injectionMode === 'url'
                  ? (
                      <Field error={form.formState.errors.urlEnvVar?.message} hint={t('projectTopology.form.envHint')} label={t('projectTopology.form.urlEnvVar')} required>
                        <Input placeholder="SERVICE_URL" {...form.register('urlEnvVar')} />
                      </Field>
                    )
                  : (
                      <>
                        <Field error={form.formState.errors.hostEnvVar?.message} hint={t('projectTopology.form.envHint')} label={t('projectTopology.form.hostEnvVar')} required>
                          <Input placeholder="SERVICE_HOST" {...form.register('hostEnvVar')} />
                        </Field>
                        <Field error={form.formState.errors.portEnvVar?.message} label={t('projectTopology.form.portEnvVar')} required>
                          <Input placeholder="SERVICE_PORT" {...form.register('portEnvVar')} />
                        </Field>
                      </>
                    )}
              </div>
            </ProgressiveSection>
          )}

          <ProgressiveSection summary={t('projectTopology.form.advancedSummary')} title={t('projectTopology.form.advancedSection')}>
            <div className="grid gap-4 sm:grid-cols-2">
              {mode === 'manual' && (
                <>
                  <Field label={t('projectTopology.form.sourceTarget')}>
                    <SearchSelect
                      loading={sourceTargets.isLoading}
                      options={optionalSourceTargetOptions}
                      placeholder={t('projectTopology.applicationLevel')}
                      searchPlaceholder={t('projectTopology.form.searchTargets')}
                      value={sourceDeploymentTargetId || applicationTargetValue}
                      onValueChange={value => form.setValue('sourceDeploymentTargetId', value, { shouldDirty: true, shouldValidate: true })}
                    />
                  </Field>
                  <Field label={t('projectTopology.form.targetTarget')}>
                    <SearchSelect
                      loading={targetTargets.isLoading}
                      options={optionalTargetTargetOptions}
                      placeholder={t('projectTopology.applicationLevel')}
                      searchPlaceholder={t('projectTopology.form.searchTargets')}
                      value={targetDeploymentTargetId || applicationTargetValue}
                      onValueChange={value => form.setValue('targetDeploymentTargetId', value, { shouldDirty: true, shouldValidate: true })}
                    />
                  </Field>
                </>
              )}
              <Field label={t('projectTopology.form.protocol')} required={mode === 'service_binding'}>
                <NativeSelect
                  {...form.register('protocol')}
                  onChange={(event) => {
                    const value = event.target.value as RelationFormValues['protocol']
                    form.setValue('protocol', value, { shouldDirty: true, shouldValidate: true })
                    if (value === 'tcp')
                      form.setValue('injectionMode', 'host_port', { shouldDirty: true, shouldValidate: true })
                  }}
                >
                  <option value="http">{t('projectTopology.form.protocols.http')}</option>
                  <option value="https">{t('projectTopology.form.protocols.https')}</option>
                  <option value="tcp">{t('projectTopology.form.protocols.tcp')}</option>
                </NativeSelect>
              </Field>
              {mode === 'service_binding' && protocol !== 'tcp' && (
                <Field error={form.formState.errors.path?.message} label={t('projectTopology.form.path')}>
                  <Input placeholder={t('projectTopology.form.pathPlaceholder')} {...form.register('path')} />
                </Field>
              )}
              {mode === 'manual' && (
                <Field error={form.formState.errors.manualPort?.message} label={t('projectTopology.form.manualPort')}>
                  <Input inputMode="numeric" placeholder={t('projectTopology.form.manualPortPlaceholder')} {...form.register('manualPort')} />
                </Field>
              )}
            </div>
            {mode === 'manual' && (
              <Field error={form.formState.errors.description?.message} label={t('projectTopology.form.manualDescriptionLabel')}>
                <Textarea maxLength={500} placeholder={t('projectTopology.form.manualDescriptionPlaceholder')} {...form.register('description')} />
              </Field>
            )}
            {mode === 'service_binding' && (
              <Controller
                control={form.control}
                name="enabled"
                render={({ field }) => (
                  <ControlledCheckboxField description={t('projectTopology.form.enabledDescription')} field={field}>
                    {t('projectTopology.form.enabled')}
                  </ControlledCheckboxField>
                )}
              />
            )}
          </ProgressiveSection>

          <DialogFooter>
            <Button type="button" variant="secondary" onClick={() => onOpenChange(false)}>{t('cancel')}</Button>
            {mode === 'service_binding' && (
              <Button
                disabled={!form.formState.isValid || saveRelation.isPending}
                type="button"
                variant="outline"
                onClick={form.handleSubmit((values) => {
                  releaseAfterSaveRef.current = true
                  saveRelation.mutate(values)
                })}
              >
                {t('projectTopology.form.saveAndRelease')}
              </Button>
            )}
            <Button disabled={!form.formState.isValid || saveRelation.isPending} type="submit">
              {t(editing ? 'projectTopology.form.save' : 'projectTopology.form.create')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function relationDefaultValues(
  initialMode: RelationDialogMode,
  service?: ServiceBinding,
  manual?: ProjectTopologyManualEdge,
): RelationFormValues {
  if (service) {
    return {
      mode: 'service_binding',
      sourceApplicationId: service.sourceApplicationId,
      sourceDeploymentTargetId: service.sourceDeploymentTargetId,
      targetApplicationId: service.targetApplicationId,
      targetDeploymentTargetId: service.targetDeploymentTargetId,
      targetPortName: service.targetPortName,
      protocol: service.protocol,
      path: service.path,
      injectionMode: service.injectionMode,
      urlEnvVar: service.urlEnvVar,
      hostEnvVar: service.hostEnvVar,
      portEnvVar: service.portEnvVar,
      enabled: service.enabled,
      relationType: 'calls',
      manualPort: '',
      description: '',
    }
  }
  if (manual) {
    return {
      mode: 'manual',
      sourceApplicationId: manual.sourceApplicationId,
      sourceDeploymentTargetId: manual.sourceDeploymentTargetId || applicationTargetValue,
      targetApplicationId: manual.targetApplicationId,
      targetDeploymentTargetId: manual.targetDeploymentTargetId || applicationTargetValue,
      targetPortName: '',
      protocol: normalizeProtocol(manual.protocol),
      path: '',
      injectionMode: 'url',
      urlEnvVar: '',
      hostEnvVar: '',
      portEnvVar: '',
      enabled: true,
      relationType: manual.relationType,
      manualPort: manual.port ? String(manual.port) : '',
      description: manual.description,
    }
  }
  return {
    mode: initialMode,
    sourceApplicationId: '',
    sourceDeploymentTargetId: initialMode === 'manual' ? applicationTargetValue : '',
    targetApplicationId: '',
    targetDeploymentTargetId: initialMode === 'manual' ? applicationTargetValue : '',
    targetPortName: '',
    protocol: 'http',
    path: '',
    injectionMode: 'url',
    urlEnvVar: '',
    hostEnvVar: '',
    portEnvVar: '',
    enabled: true,
    relationType: 'depends_on',
    manualPort: '',
    description: '',
  }
}

function createRelationSchema(t: (key: string) => string) {
  return z.object({
    mode: z.enum(['service_binding', 'manual']),
    sourceApplicationId: z.string().min(1, t('projectTopology.form.sourceApplicationRequired')),
    sourceDeploymentTargetId: z.string(),
    targetApplicationId: z.string().min(1, t('projectTopology.form.targetApplicationRequired')),
    targetDeploymentTargetId: z.string(),
    targetPortName: z.string(),
    protocol: z.enum(['http', 'https', 'tcp']),
    path: z.string(),
    injectionMode: z.enum(['url', 'host_port']),
    urlEnvVar: z.string(),
    hostEnvVar: z.string(),
    portEnvVar: z.string(),
    enabled: z.boolean(),
    relationType: z.enum(['depends_on', 'calls', 'reads_writes', 'publishes_to', 'consumes_from']),
    manualPort: z.string(),
    description: z.string().max(500, t('projectTopology.form.descriptionTooLong')),
  }).superRefine((values, context) => {
    if (values.mode === 'manual') {
      if (values.sourceApplicationId && values.sourceApplicationId === values.targetApplicationId)
        context.addIssue({ code: 'custom', path: ['targetApplicationId'], message: t('projectTopology.form.manualApplicationSame') })
      if (values.manualPort && (!/^\d+$/.test(values.manualPort) || Number(values.manualPort) < 1 || Number(values.manualPort) > 65535))
        context.addIssue({ code: 'custom', path: ['manualPort'], message: t('projectTopology.form.portInvalid') })
      return
    }
    if (!values.sourceDeploymentTargetId)
      context.addIssue({ code: 'custom', path: ['sourceDeploymentTargetId'], message: t('projectTopology.form.sourceTargetRequired') })
    if (!values.targetDeploymentTargetId)
      context.addIssue({ code: 'custom', path: ['targetDeploymentTargetId'], message: t('projectTopology.form.targetTargetRequired') })
    if (values.sourceDeploymentTargetId && values.sourceDeploymentTargetId === values.targetDeploymentTargetId)
      context.addIssue({ code: 'custom', path: ['targetDeploymentTargetId'], message: t('projectTopology.form.sourceTargetSame') })
    if (!values.targetPortName)
      context.addIssue({ code: 'custom', path: ['targetPortName'], message: t('projectTopology.form.targetPortRequired') })
    if (values.injectionMode === 'url') {
      validateEnvName(values.urlEnvVar, 'urlEnvVar', context, t)
    }
    else {
      validateEnvName(values.hostEnvVar, 'hostEnvVar', context, t)
      validateEnvName(values.portEnvVar, 'portEnvVar', context, t)
      if (values.hostEnvVar && values.hostEnvVar === values.portEnvVar)
        context.addIssue({ code: 'custom', path: ['portEnvVar'], message: t('projectTopology.form.envDuplicated') })
    }
    if (values.path && (values.protocol === 'tcp' || !values.path.startsWith('/') || /[?#]/.test(values.path) || values.path.startsWith('//')))
      context.addIssue({ code: 'custom', path: ['path'], message: t('projectTopology.form.pathInvalid') })
  })
}

function validateEnvName(value: string, path: keyof RelationFormValues, context: z.RefinementCtx, t: (key: string) => string) {
  if (!value) {
    context.addIssue({ code: 'custom', path: [path], message: t('projectTopology.form.envRequired') })
    return
  }
  if (!envNamePattern.test(value))
    context.addIssue({ code: 'custom', path: [path], message: t('projectTopology.form.envInvalid') })
}

function normalizedServicePorts(target?: DeploymentTarget): DeploymentServicePort[] {
  if (!target)
    return []
  return (target.servicePorts ?? []).filter(port => port.name && port.port > 0)
}

function deploymentTargetOptions(targets: DeploymentTarget[]) {
  return targets.map(target => ({
    label: target.name,
    description: `${target.stage} · ${target.clusterId || '-'}`,
    keywords: `${target.stage} ${target.clusterId}`,
    value: target.id,
  }))
}

function optionalDeploymentTargetOptions(targets: DeploymentTarget[], applicationLevelLabel: string) {
  return [
    { label: applicationLevelLabel, value: applicationTargetValue },
    ...deploymentTargetOptions(targets),
  ]
}

function serviceBindingPayload(values: RelationFormValues): ServiceBindingPayload {
  return {
    sourceApplicationId: values.sourceApplicationId,
    sourceDeploymentTargetId: values.sourceDeploymentTargetId,
    targetApplicationId: values.targetApplicationId,
    targetDeploymentTargetId: values.targetDeploymentTargetId,
    targetPortName: values.targetPortName,
    protocol: values.protocol,
    path: values.protocol === 'tcp' ? '' : values.path.trim(),
    injectionMode: values.injectionMode,
    urlEnvVar: values.injectionMode === 'url' ? values.urlEnvVar.trim() : '',
    hostEnvVar: values.injectionMode === 'host_port' ? values.hostEnvVar.trim() : '',
    portEnvVar: values.injectionMode === 'host_port' ? values.portEnvVar.trim() : '',
    enabled: values.enabled,
  }
}

function manualEdgePayload(values: RelationFormValues): ProjectTopologyManualEdgePayload {
  return {
    sourceApplicationId: values.sourceApplicationId,
    sourceDeploymentTargetId: optionalTargetId(values.sourceDeploymentTargetId),
    targetApplicationId: values.targetApplicationId,
    targetDeploymentTargetId: optionalTargetId(values.targetDeploymentTargetId),
    relationType: values.relationType,
    protocol: values.protocol,
    port: values.manualPort ? Number(values.manualPort) : null,
    description: values.description.trim(),
  }
}

function optionalTargetId(value: string) {
  return value && value !== applicationTargetValue ? value : null
}

/** 关系模式选项卡片：带说明的单选项，编辑态禁用切换 */
function ModeOption({ description, hint, title, value }: { description: string, hint?: string, title: string, value: RelationDialogMode }) {
  return (
    <label
      className={cn(
        'group/mode relative grid cursor-pointer gap-1 rounded-container border border-border bg-surface p-3 transition-colors',
        'hover:border-separator-strong',
        'has-[:checked]:border-primary has-[:checked]:bg-primary-subtle/40',
        'has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-60',
      )}
    >
      <span className="flex items-center gap-2">
        <RadioGroupItem aria-label={title} value={value} />
        <span className="text-sm font-medium">{title}</span>
      </span>
      <span className="pl-6 text-xs leading-5 text-muted-foreground">{description}</span>
      {hint && (
        <span className="mt-1 ml-6 rounded-control border border-border bg-muted/40 px-2 py-1 text-[11px] leading-4 text-muted-foreground">
          {hint}
        </span>
      )}
    </label>
  )
}

function normalizeProtocol(value: string): RelationFormValues['protocol'] {
  if (value === 'https' || value === 'tcp')
    return value
  return 'http'
}
