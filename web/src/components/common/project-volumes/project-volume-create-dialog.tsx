import type { ProjectVolume, ProjectVolumeAccessMode, ProjectVolumeCreateInput, ProjectVolumeMode } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api, createVolumeIdempotencyKey } from '@/api'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { projectVolumeAccessModeOptions, projectVolumeModeLabel, projectVolumeModeOptions } from './project-volume-form-options'
import { ProjectVolumeClusterSelect, ProjectVolumeStorageClassSelect } from './volume-resource-selectors'

type SourceMode = 'blank' | 'existingManaged' | 'existingReferenced' | 'snapshot'

interface CreateVolumeValues {
  accessMode: ProjectVolumeAccessMode
  capacity: string
  claimName: string
  clusterId: string
  displayName: string
  snapshotName: string
  sourceMode: SourceMode
  storageClassName: string
  volumeMode: ProjectVolumeMode
}

const defaults: CreateVolumeValues = {
  accessMode: 'ReadWriteOnce',
  capacity: '10Gi',
  claimName: '',
  clusterId: '',
  displayName: '',
  snapshotName: '',
  sourceMode: 'blank',
  storageClassName: '',
  volumeMode: 'Filesystem',
}

interface ProjectVolumeDeploymentContext {
  clusterId: string
  clusterName: string
  volumeMode: ProjectVolumeMode
}

interface ProjectVolumeCreateDialogProps {
  deploymentContext?: ProjectVolumeDeploymentContext
  onCreated?: (volume: ProjectVolume) => void
  onOpenChange: (open: boolean) => void
  open: boolean
  projectId: string
}

function createDefaults(deploymentContext?: ProjectVolumeDeploymentContext): CreateVolumeValues {
  if (!deploymentContext)
    return defaults
  return {
    ...defaults,
    clusterId: deploymentContext.clusterId,
    sourceMode: 'blank',
    volumeMode: deploymentContext.volumeMode,
  }
}

function createSchema(required: string, nameTooLong: string) {
  return z.object({
    accessMode: z.enum(['ReadWriteOnce', 'ReadWriteOncePod', 'ReadOnlyMany', 'ReadWriteMany']),
    capacity: z.string(),
    claimName: z.string(),
    clusterId: z.string().min(1, required),
    displayName: z.string().trim().min(1, required).max(120, nameTooLong),
    snapshotName: z.string(),
    sourceMode: z.enum(['blank', 'existingManaged', 'existingReferenced', 'snapshot']),
    storageClassName: z.string(),
    volumeMode: z.enum(['Filesystem', 'Block']),
  }).superRefine((value, context) => {
    if (value.sourceMode === 'blank' || value.sourceMode === 'snapshot') {
      if (!value.capacity.trim())
        context.addIssue({ code: 'custom', message: required, path: ['capacity'] })
      if (!value.storageClassName.trim())
        context.addIssue({ code: 'custom', message: required, path: ['storageClassName'] })
    }
    if ((value.sourceMode === 'existingManaged' || value.sourceMode === 'existingReferenced') && !value.claimName.trim())
      context.addIssue({ code: 'custom', message: required, path: ['claimName'] })
    if (value.sourceMode === 'snapshot' && !value.snapshotName.trim())
      context.addIssue({ code: 'custom', message: required, path: ['snapshotName'] })
  })
}

function payloadFromValues(values: CreateVolumeValues): ProjectVolumeCreateInput {
  const base = {
    displayName: values.displayName.trim(),
    clusterId: values.clusterId,
  }
  if (values.sourceMode === 'blank' || values.sourceMode === 'snapshot') {
    const spec = {
      ...base,
      capacity: values.capacity.trim(),
      storageClassName: values.storageClassName,
      accessMode: values.accessMode,
      volumeMode: values.volumeMode,
    }
    return values.sourceMode === 'blank'
      ? { ...spec, source: { type: 'blank' } }
      : { ...spec, source: { type: 'volumeSnapshot', snapshotName: values.snapshotName.trim() } }
  }
  return {
    ...base,
    source: {
      type: 'existingClaim',
      claimName: values.claimName.trim(),
      ownershipMode: values.sourceMode === 'existingManaged' ? 'managed' : 'referenced',
    },
  }
}

export function ProjectVolumeCreateDialog({ deploymentContext, onCreated, onOpenChange, open, projectId }: ProjectVolumeCreateDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const formDefaults = createDefaults(deploymentContext)
  const form = useForm<CreateVolumeValues>({ defaultValues: formDefaults, mode: 'onChange', resolver: zodResolver(createSchema(t('common.required'), t('projectVolumes.nameTooLong'))) })
  const createRequestRef = useRef<{ fingerprint: string, idempotencyKey: string } | null>(null)
  const sourceMode = form.watch('sourceMode')
  const clusterId = form.watch('clusterId')
  const requiresSpec = sourceMode === 'blank' || sourceMode === 'snapshot'
  const create = useMutation({
    mutationFn: (values: CreateVolumeValues) => {
      const payload = payloadFromValues(values)
      const fingerprint = JSON.stringify({ payload, projectId })
      if (createRequestRef.current?.fingerprint !== fingerprint) {
        createRequestRef.current = {
          fingerprint,
          idempotencyKey: createVolumeIdempotencyKey(),
        }
      }
      return api.createProjectVolume(projectId, payload, createRequestRef.current.idempotencyKey)
    },
    onSuccess: (volume) => {
      toast.success(t('projectVolumes.created'))
      createRequestRef.current = null
      form.reset(formDefaults)
      onOpenChange(false)
      onCreated?.(volume)
      queryClient.invalidateQueries({ queryKey: ['project-volumes', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const handleOpenChange = (next: boolean) => {
    if (!next && create.isPending)
      return
    if (!next) {
      createRequestRef.current = null
      form.reset(formDefaults)
    }
    onOpenChange(next)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={handleOpenChange}
    >
      <DialogContent className="max-w-2xl" showCloseButton={!create.isPending}>
        <DialogHeader>
          <DialogTitle>{t('projectVolumes.create')}</DialogTitle>
          <DialogDescription>{t('projectVolumes.createDescription')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-4" onSubmit={form.handleSubmit(values => create.mutate(values))}>
          <div className="grid gap-4 md:grid-cols-2">
            <Field error={form.formState.errors.displayName?.message} label={t('projectVolumes.name')} required>
              <Input {...form.register('displayName')} aria-invalid={Boolean(form.formState.errors.displayName)} placeholder={t('projectVolumes.displayNamePlaceholder')} />
            </Field>
            <Field label={t('projectVolumes.createSource')} required>
              {deploymentContext
                ? (
                    <>
                      <input type="hidden" {...form.register('sourceMode')} />
                      <NativeSelect disabled value="blank">
                        <option value="blank">{t('projectVolumes.sourceBlank')}</option>
                      </NativeSelect>
                    </>
                  )
                : (
                    <NativeSelect {...form.register('sourceMode')}>
                      <option value="blank">{t('projectVolumes.sourceBlank')}</option>
                      <option value="existingReferenced">{t('projectVolumes.sourceExistingReferenced')}</option>
                      <option value="existingManaged">{t('projectVolumes.sourceExistingManaged')}</option>
                      <option value="snapshot">{t('projectVolumes.sourceSnapshot')}</option>
                    </NativeSelect>
                  )}
            </Field>
            <Field error={form.formState.errors.clusterId?.message} label={t('projectVolumes.cluster')} required>
              {deploymentContext
                ? <Input disabled value={deploymentContext.clusterName} />
                : (
                    <ProjectVolumeClusterSelect
                      projectId={projectId}
                      value={clusterId}
                      onChange={(value) => {
                        form.setValue('clusterId', value, { shouldDirty: true, shouldValidate: true })
                        form.setValue('storageClassName', '')
                      }}
                    />
                  )}
            </Field>
            {requiresSpec && (
              <Field error={form.formState.errors.storageClassName?.message} label={t('projectVolumes.storageClass')} required>
                <ProjectVolumeStorageClassSelect
                  clusterId={clusterId}
                  projectId={projectId}
                  value={form.watch('storageClassName')}
                  onChange={value => form.setValue('storageClassName', value, { shouldDirty: true, shouldValidate: true })}
                />
              </Field>
            )}
            {requiresSpec && (
              <Field error={form.formState.errors.capacity?.message} label={t('projectVolumes.capacity')} required>
                <Input {...form.register('capacity')} aria-invalid={Boolean(form.formState.errors.capacity)} placeholder={t('projectVolumes.capacityPlaceholder')} />
              </Field>
            )}
            {requiresSpec && (
              <Field label={t('projectVolumes.accessMode')} required>
                <NativeSelect {...form.register('accessMode')}>
                  {projectVolumeAccessModeOptions(t).map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
                </NativeSelect>
              </Field>
            )}
            {requiresSpec && (
              <Field label={t('projectVolumes.volumeMode')} required>
                {deploymentContext
                  ? (
                      <>
                        <input type="hidden" {...form.register('volumeMode')} />
                        <NativeSelect disabled value={deploymentContext.volumeMode}>
                          <option value={deploymentContext.volumeMode}>{projectVolumeModeLabel(t, deploymentContext.volumeMode)}</option>
                        </NativeSelect>
                      </>
                    )
                  : (
                      <NativeSelect {...form.register('volumeMode')}>
                        {projectVolumeModeOptions(t).map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
                      </NativeSelect>
                    )}
              </Field>
            )}
            {(sourceMode === 'existingManaged' || sourceMode === 'existingReferenced') && (
              <Field error={form.formState.errors.claimName?.message} label={t('projectVolumes.claimName')} required>
                <Input {...form.register('claimName')} aria-invalid={Boolean(form.formState.errors.claimName)} placeholder={t('projectVolumes.claimNamePlaceholder')} />
              </Field>
            )}
            {sourceMode === 'snapshot' && (
              <Field error={form.formState.errors.snapshotName?.message} label={t('projectVolumes.snapshotName')} required>
                <Input {...form.register('snapshotName')} aria-invalid={Boolean(form.formState.errors.snapshotName)} placeholder={t('projectVolumes.snapshotNamePlaceholder')} />
              </Field>
            )}
          </div>
          <DialogFooter>
            <Button disabled={create.isPending} type="button" variant="secondary" onClick={() => handleOpenChange(false)}>{t('common.cancel')}</Button>
            <Button disabled={!form.formState.isValid || create.isPending} type="submit">{t('projectVolumes.create')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
