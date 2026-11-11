import type { EgressException, EgressExceptionPayload, Project, RuntimeCluster } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Plus, X } from 'lucide-react'
import { useMemo } from 'react'
import { Controller, FormProvider, useFieldArray, useForm, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api, ApiError } from '@/api'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { FormField } from '@/components/common/form-field'
import { SearchMultiSelect, SearchSelect } from '@/components/common/search-select'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'

function exceptionSchema(message: { required: string, cidr: string, ports: string, project: string }) {
  return z.object({
    name: z.string().trim().min(1, message.required).max(64),
    cidr: z.string().trim().regex(/^[0-9a-f.:]+\/\d{1,3}$/i, message.cidr).refine(value => !value.toLowerCase().includes('::ffff:'), message.cidr),
    ports: z.string().trim().refine(value => value === '' || value.split(',').every(port => /^\d{1,5}$/.test(port.trim()) && Number(port) >= 1 && Number(port) <= 65535), message.ports),
    protocol: z.enum(['tcp', 'udp', 'any']),
    reason: z.string().trim().min(1, message.required).max(500),
    grants: z.array(z.object({
      projectId: z.string().min(1, message.project),
      deploymentTargetIds: z.array(z.string()),
      includeBuilds: z.boolean(),
    })),
  })
}

type ExceptionForm = EgressExceptionPayload

const errorFields: Partial<Record<string, keyof ExceptionForm>> = {
  'runtime_cluster.egress_exception_invalid': 'cidr',
  'runtime_cluster.egress_exception_grant_invalid': 'grants',
}

export function ClusterEgressExceptionFormDialog({ cluster, exception, projects, onOpenChange, onSaved }: {
  cluster: RuntimeCluster
  exception: EgressException | null
  projects: Project[]
  onOpenChange: (open: boolean) => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const schema = useMemo(() => exceptionSchema({
    required: t('clustersPage.egressExceptions.required'),
    cidr: t('errors.runtime_cluster.egress_exception_invalid'),
    ports: t('clustersPage.egressExceptions.portsInvalid'),
    project: t('clustersPage.egressExceptions.projectRequired'),
  }), [t])
  const form = useForm<ExceptionForm>({
    resolver: zodResolver(schema),
    defaultValues: exception
      ? { name: exception.name, cidr: exception.cidr, ports: exception.ports, protocol: exception.protocol, reason: exception.reason, grants: exception.grants }
      : { name: '', cidr: '', ports: '', protocol: 'tcp', reason: '', grants: [] },
  })
  const grants = useFieldArray({ control: form.control, name: 'grants' })
  const save = useMutation({
    mutationFn: (values: ExceptionForm) => exception
      ? api.updateRuntimeClusterEgressException(cluster.id, exception.id, values)
      : api.createRuntimeClusterEgressException(cluster.id, values),
    onSuccess: () => {
      toast.success(t('clustersPage.egressExceptions.saved'))
      onSaved()
    },
    onError: (error) => {
      const field = error instanceof ApiError ? errorFields[error.code] : undefined
      if (field)
        form.setError(field, { message: error.message })
      toast.error(error.message)
    },
  })
  const errors = form.formState.errors

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <FormProvider {...form}>
          <form className="grid gap-4" onSubmit={form.handleSubmit(values => save.mutate(values))}>
            <DialogHeader>
              <DialogTitle>{t(exception ? 'clustersPage.egressExceptions.editTitle' : 'clustersPage.egressExceptions.createTitle')}</DialogTitle>
              <DialogDescription>{t('clustersPage.egressExceptions.formDescription')}</DialogDescription>
            </DialogHeader>
            <div className="grid gap-4 md:grid-cols-2">
              <FormField error={errors.name?.message} label={t('clustersPage.egressExceptions.name')} required><Input {...form.register('name')} /></FormField>
              <FormField error={errors.cidr?.message} hint={t('clustersPage.egressExceptions.cidrHint')} label={t('clustersPage.egressExceptions.cidr')} required><Input className="font-mono" placeholder="192.168.31.10/32" {...form.register('cidr')} /></FormField>
              <FormField label={t('clustersPage.egressExceptions.protocol')}>
                <NativeSelect {...form.register('protocol')}>
                  <option value="tcp">{t('clustersPage.egressExceptions.protocols.tcp')}</option>
                  <option value="udp">{t('clustersPage.egressExceptions.protocols.udp')}</option>
                  <option value="any">{t('clustersPage.egressExceptions.protocols.any')}</option>
                </NativeSelect>
              </FormField>
              <FormField error={errors.ports?.message} hint={t('clustersPage.egressExceptions.portsHint')} label={t('clustersPage.egressExceptions.ports')}><Input placeholder="3306" {...form.register('ports')} /></FormField>
            </div>
            <FormField error={errors.reason?.message} hint={t('clustersPage.egressExceptions.reasonHint')} label={t('clustersPage.egressExceptions.reason')} required><Textarea rows={2} {...form.register('reason')} /></FormField>
            <div className="grid gap-2">
              <div className="flex items-center justify-between gap-2">
                <span className="text-sm font-medium">{t('clustersPage.egressExceptions.grants')}</span>
                <Button size="sm" type="button" variant="outline" onClick={() => grants.append({ projectId: '', deploymentTargetIds: [], includeBuilds: false })}>
                  <Plus className="size-4" />
                  {t('clustersPage.egressExceptions.addGrant')}
                </Button>
              </div>
              {errors.grants?.message ? <p className="text-sm text-destructive">{errors.grants.message}</p> : null}
              {grants.fields.length === 0 ? <p className="text-sm text-muted-foreground">{t('clustersPage.egressExceptions.noGrantsHint')}</p> : null}
              {grants.fields.map((field, index) => (
                <GrantRow key={field.id} index={index} projects={projects} onRemove={() => grants.remove(index)} />
              ))}
            </div>
            <DialogFooter>
              <DialogClose asChild><Button disabled={save.isPending} type="button" variant="outline">{t('common.cancel')}</Button></DialogClose>
              <Button disabled={save.isPending} type="submit">{t('common.save')}</Button>
            </DialogFooter>
          </form>
        </FormProvider>
      </DialogContent>
    </Dialog>
  )
}

/** 一条授权：项目空间、可选的部署配置收窄与是否包含构建 Pod。部署配置候选来自项目拓扑。 */
function GrantRow({ index, projects, onRemove }: { index: number, projects: Project[], onRemove: () => void }) {
  const { t } = useTranslation()
  const { control, formState, watch } = useFormContext<ExceptionForm>()
  const projectId = watch(`grants.${index}.projectId`)
  const topology = useQuery({
    queryKey: ['project-topology', projectId, 'egress-exception-targets'],
    queryFn: () => api.getProjectTopology(projectId),
    enabled: Boolean(projectId),
  })
  const targetOptions = (topology.data?.nodes ?? []).flatMap(node => node.deploymentTargets.map(target => ({
    label: `${node.name} / ${target.name}`,
    value: target.id,
    description: target.stage,
  })))
  const projectError = formState.errors.grants?.[index]?.projectId?.message
  return (
    <div className="grid gap-3 rounded-control border border-border p-3 md:grid-cols-[1fr_1fr_auto] md:items-end">
      <FormField error={projectError} label={t('clustersPage.egressExceptions.project')}>
        <Controller
          control={control}
          name={`grants.${index}.projectId`}
          render={({ field }) => (
            <SearchSelect
              ariaLabel={t('clustersPage.egressExceptions.project')}
              options={projects.map(project => ({ label: project.name, value: project.id, description: project.identifier }))}
              value={field.value}
              onValueChange={field.onChange}
            />
          )}
        />
      </FormField>
      <FormField hint={t('clustersPage.egressExceptions.targetsHint')} label={t('clustersPage.egressExceptions.targets')}>
        <Controller
          control={control}
          name={`grants.${index}.deploymentTargetIds`}
          render={({ field }) => (
            <SearchMultiSelect
              ariaLabel={t('clustersPage.egressExceptions.targets')}
              disabled={!projectId}
              loading={topology.isFetching}
              options={targetOptions}
              placeholder={t('clustersPage.egressExceptions.wholeProject')}
              value={field.value}
              onValueChange={field.onChange}
            />
          )}
        />
      </FormField>
      <div className="flex items-center gap-2">
        <Controller
          control={control}
          name={`grants.${index}.includeBuilds`}
          render={({ field }) => <ControlledCheckboxField field={field}>{t('clustersPage.egressExceptions.includeBuilds')}</ControlledCheckboxField>}
        />
        <Button aria-label={t('clustersPage.egressExceptions.removeGrant')} size="icon" type="button" variant="ghost" onClick={onRemove}>
          <X className="size-4" />
        </Button>
      </div>
    </div>
  )
}
