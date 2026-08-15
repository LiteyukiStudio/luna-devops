import type { ProjectVolumeAccessMode, ProjectVolumeCreateInput, ProjectVolumeMode } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
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
  const source = values.sourceMode === 'blank'
    ? { type: 'blank' as const }
    : values.sourceMode === 'snapshot'
      ? { type: 'volumeSnapshot' as const, snapshotName: values.snapshotName.trim() }
      : {
          type: 'existingClaim' as const,
          claimName: values.claimName.trim(),
          ownershipMode: values.sourceMode === 'existingManaged' ? 'managed' as const : 'referenced' as const,
        }
  const needsSpec = values.sourceMode === 'blank' || values.sourceMode === 'snapshot'
  return {
    displayName: values.displayName.trim(),
    clusterId: values.clusterId,
    ...(needsSpec
      ? {
          capacity: values.capacity.trim(),
          storageClassName: values.storageClassName,
          accessMode: values.accessMode,
          volumeMode: values.volumeMode,
        }
      : {}),
    source,
  }
}

export function ProjectVolumeCreateDialog({ onOpenChange, open, projectId }: { onOpenChange: (open: boolean) => void, open: boolean, projectId: string }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const form = useForm<CreateVolumeValues>({ defaultValues: defaults, mode: 'onChange', resolver: zodResolver(createSchema(t('common.required'), t('projectVolumes.nameTooLong'))) })
  const sourceMode = form.watch('sourceMode')
  const clusterId = form.watch('clusterId')
  const requiresSpec = sourceMode === 'blank' || sourceMode === 'snapshot'
  const create = useMutation({
    mutationFn: (values: CreateVolumeValues) => api.createProjectVolume(projectId, payloadFromValues(values)),
    onSuccess: () => {
      toast.success(t('projectVolumes.created'))
      form.reset(defaults)
      onOpenChange(false)
      queryClient.invalidateQueries({ queryKey: ['project-volumes', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !create.isPending)
          form.reset(defaults)
        onOpenChange(next)
      }}
    >
      <DialogContent className="max-w-2xl">
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
              <NativeSelect {...form.register('sourceMode')}>
                <option value="blank">{t('projectVolumes.sourceBlank')}</option>
                <option value="existingReferenced">{t('projectVolumes.sourceExistingReferenced')}</option>
                <option value="existingManaged">{t('projectVolumes.sourceExistingManaged')}</option>
                <option value="snapshot">{t('projectVolumes.sourceSnapshot')}</option>
              </NativeSelect>
            </Field>
            <Field error={form.formState.errors.clusterId?.message} label={t('projectVolumes.cluster')} required>
              <ProjectVolumeClusterSelect
                projectId={projectId}
                value={clusterId}
                onChange={(value) => {
                  form.setValue('clusterId', value, { shouldDirty: true, shouldValidate: true })
                  form.setValue('storageClassName', '')
                }}
              />
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
                  <option value="ReadWriteOnce">ReadWriteOnce</option>
                  <option value="ReadWriteOncePod">ReadWriteOncePod</option>
                  <option value="ReadOnlyMany">ReadOnlyMany</option>
                  <option value="ReadWriteMany">ReadWriteMany</option>
                </NativeSelect>
              </Field>
            )}
            {requiresSpec && (
              <Field label={t('projectVolumes.volumeMode')} required>
                <NativeSelect {...form.register('volumeMode')}>
                  <option value="Filesystem">Filesystem</option>
                  <option value="Block">Block</option>
                </NativeSelect>
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
            <Button type="button" variant="secondary" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
            <Button disabled={!form.formState.isValid || create.isPending} type="submit">{t('projectVolumes.create')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
