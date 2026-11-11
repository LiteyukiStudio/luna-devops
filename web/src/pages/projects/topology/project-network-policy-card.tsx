import type { ProjectNetworkPolicy } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api, ApiError } from '@/api'
import { ErrorState } from '@/components/common/error-state'
import { FormField } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { NativeSelect } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'

const allowlistLine = /^(?<cidr>[0-9a-f.:]+\/\d{1,3})(?:\s+(?<ports>\d{1,5}(?:,\d{1,5})*))?$/i

/** 出站白名单的格式校验：每行 `CIDR [端口,端口]`。网段是否落在保留范围内由服务端判断。 */
function allowlistFormatValid(value: string) {
  return value.split('\n').map(line => line.trim()).filter(Boolean).every((line) => {
    const match = allowlistLine.exec(line)
    if (!match?.groups || match.groups.cidr.toLowerCase().includes('::ffff:'))
      return false
    return (match.groups.ports ?? '').split(',').filter(Boolean).every(port => Number(port) >= 1 && Number(port) <= 65535)
  })
}

function networkPolicySchema(invalidAllowlist: string) {
  return z.object({
    ingressMode: z.enum(['isolated', 'dependencies']),
    egressMode: z.enum(['internet', 'allowlist', 'none']),
    egressAllowlist: z.string().refine(allowlistFormatValid, invalidAllowlist),
  })
}

type NetworkPolicyForm = z.infer<ReturnType<typeof networkPolicySchema>>

/** 项目空间网络模式：只能比集群默认更严格，保存后由 Worker 重新下发网络策略。 */
export function ProjectNetworkPolicyCard({ canManage, projectId }: { canManage: boolean, projectId: string }) {
  const { t } = useTranslation()
  const policy = useQuery({ queryKey: ['project-network-policy', projectId], queryFn: ({ signal }) => api.getProjectNetworkPolicy(projectId, signal) })
  return (
    <Card className="grid gap-4 p-5">
      <div className="grid gap-1">
        <h2 className="text-base font-semibold">{t('projectTopology.networkPolicy.title')}</h2>
        <p className="text-sm text-muted-foreground">{t('projectTopology.networkPolicy.description')}</p>
      </div>
      {policy.isPending ? <Skeleton className="h-24 w-full" /> : null}
      {policy.isError
        ? <ErrorState title={t('projectTopology.networkPolicy.loadFailed')} retrying={policy.isFetching} onRetry={() => void policy.refetch()} />
        : null}
      {policy.data ? <ProjectNetworkPolicyForm canManage={canManage} initial={policy.data} projectId={projectId} /> : null}
    </Card>
  )
}

function ProjectNetworkPolicyForm({ canManage, initial, projectId }: { canManage: boolean, initial: ProjectNetworkPolicy, projectId: string }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const schema = useMemo(() => networkPolicySchema(t('errors.project.network_egress_allowlist_invalid')), [t])
  const form = useForm<NetworkPolicyForm>({ resolver: zodResolver(schema), defaultValues: initial })
  const save = useMutation({
    mutationFn: (values: ProjectNetworkPolicy) => api.updateProjectNetworkPolicy(projectId, values),
    onSuccess: (saved) => {
      queryClient.setQueryData(['project-network-policy', projectId], saved)
      form.reset(saved)
      toast.success(t('projectTopology.networkPolicy.saved'))
    },
    onError: (error) => {
      if (error instanceof ApiError && error.code === 'project.network_egress_allowlist_invalid')
        form.setError('egressAllowlist', { message: error.message })
      toast.error(error.message)
    },
  })
  const egressMode = form.watch('egressMode')
  const disabled = !canManage
  return (
    <form className="grid gap-4" onSubmit={form.handleSubmit(values => save.mutate(values))}>
      <div className="grid gap-4 md:grid-cols-2">
        <FormField hint={t('projectTopology.networkPolicy.ingressModeHint')} label={t('projectTopology.networkPolicy.ingressMode')}>
          <NativeSelect disabled={disabled} {...form.register('ingressMode')}>
            <option value="isolated">{t('projectTopology.networkPolicy.ingressModes.isolated')}</option>
            <option value="dependencies">{t('projectTopology.networkPolicy.ingressModes.dependencies')}</option>
          </NativeSelect>
        </FormField>
        <FormField hint={t('projectTopology.networkPolicy.egressModeHint')} label={t('projectTopology.networkPolicy.egressMode')}>
          <NativeSelect disabled={disabled} {...form.register('egressMode')}>
            <option value="internet">{t('projectTopology.networkPolicy.egressModes.internet')}</option>
            <option value="allowlist">{t('projectTopology.networkPolicy.egressModes.allowlist')}</option>
            <option value="none">{t('projectTopology.networkPolicy.egressModes.none')}</option>
          </NativeSelect>
        </FormField>
      </div>
      {egressMode === 'allowlist'
        ? (
            <FormField error={form.formState.errors.egressAllowlist?.message} hint={t('projectTopology.networkPolicy.egressAllowlistHint')} label={t('projectTopology.networkPolicy.egressAllowlist')}>
              <Textarea disabled={disabled} rows={4} className="font-mono text-sm" placeholder={t('projectTopology.networkPolicy.egressAllowlistPlaceholder')} {...form.register('egressAllowlist')} />
            </FormField>
          )
        : null}
      {canManage
        ? <Button className="justify-self-start" disabled={save.isPending || !form.formState.isDirty} type="submit">{t('common.save')}</Button>
        : null}
    </form>
  )
}
