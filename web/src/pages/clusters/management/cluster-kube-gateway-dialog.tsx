import type { ClusterKubeGatewayFormValues } from './cluster-kube-gateway-form'
import type { RuntimeCluster, RuntimeClusterKubeGateway } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, LoaderCircle, Shield } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { SettingsSkeleton } from '@/components/common/loading-states'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatAbsoluteDateTime } from '@/components/common/time-format'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { ClusterKubeGatewayRulesEditor } from './cluster-kube-gateway-rules-editor'

function createSchema(t: (key: string, options?: Record<string, unknown>) => string) {
  return z.object({
    enabled: z.boolean(),
    extraResourceRules: z.array(z.object({
      action: z.string().trim().min(1, t('kubectlAccess.gatewayRules.actionRequired')),
      apiGroup: z.string().trim().min(1, t('kubectlAccess.gatewayRules.apiGroupRequired')),
      apiVersion: z.string().trim().min(1, t('kubectlAccess.gatewayRules.apiVersionRequired')),
      resource: z.string().trim().min(1, t('kubectlAccess.gatewayRules.resourceRequired')),
      subresourcesText: z.string(),
      verbs: z.array(z.string()).min(1, t('kubectlAccess.gatewayRules.verbsRequired')),
    })).max(50, t('kubectlAccess.gatewayRules.maxRules')),
  })
}

function rulesToFormValues(cluster: RuntimeCluster): ClusterKubeGatewayFormValues {
  return {
    enabled: cluster.kubeGatewayEnabled ?? false,
    extraResourceRules: [],
  }
}

function gatewayQueryToFormValues(gateway: RuntimeClusterKubeGateway): ClusterKubeGatewayFormValues {
  return {
    enabled: gateway.enabled,
    extraResourceRules: (gateway.extraResourceRules ?? []).map(rule => ({
      action: rule.action,
      apiGroup: rule.apiGroup,
      apiVersion: rule.apiVersion,
      resource: rule.resource,
      subresourcesText: (rule.subresources ?? []).join(', '),
      verbs: rule.verbs,
    })),
  }
}

export function ClusterKubeGatewayDialog({
  cluster,
  open,
  onOpenChange,
}: {
  cluster: RuntimeCluster | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const schema = useMemo(() => createSchema(t), [t])
  const form = useForm<ClusterKubeGatewayFormValues>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: cluster ? rulesToFormValues(cluster) : { enabled: false, extraResourceRules: [] },
  })
  const gateway = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['runtime-clusters', cluster?.id, 'kube-gateway'],
    queryFn: () => api.getRuntimeClusterKubeGateway(cluster?.id ?? ''),
    enabled: open && Boolean(cluster?.id),
  })

  useEffect(() => {
    if (!cluster)
      return

    if (gateway.data) {
      form.reset(gatewayQueryToFormValues(gateway.data))
      return
    }

    form.reset(rulesToFormValues(cluster))
  }, [cluster, form, gateway.data])

  const saveGateway = useMutation({
    mutationFn: (values: ClusterKubeGatewayFormValues) => api.updateRuntimeClusterKubeGateway(cluster?.id ?? '', {
      enabled: values.enabled,
      extraResourceRules: values.extraResourceRules.map(rule => ({
        action: rule.action.trim(),
        apiGroup: rule.apiGroup.trim(),
        apiVersion: rule.apiVersion.trim(),
        resource: rule.resource.trim(),
        subresources: normalizeCommaSeparatedList(rule.subresourcesText),
        verbs: rule.verbs.map(item => item.trim()).filter(Boolean),
      })),
    }),
    onSuccess: (result) => {
      toast.success(t('kubectlAccess.gatewaySaved'))
      queryClient.setQueryData(['runtime-clusters', cluster?.id, 'kube-gateway'], result)
      queryClient.invalidateQueries({ queryKey: ['runtime-clusters'] })
      queryClient.invalidateQueries({ queryKey: ['runtime-clusters', cluster?.id, 'kube-gateway'] })
    },
    onError: error => toast.error(error.message),
  })
  const gatewayEnabled = form.watch('enabled')
  let body = null

  if (cluster) {
    if (gateway.isLoading && !gateway.data) {
      body = <SettingsSkeleton />
    }
    else if (gateway.isError && !gateway.data) {
      body = <ErrorState description={t('kubectlAccess.gatewayLoadFailedDescription')} title={t('kubectlAccess.gatewayLoadFailedTitle')} />
    }
    else {
      body = (
        <form className="grid gap-4" onSubmit={form.handleSubmit(values => saveGateway.mutate(values))}>
          <Alert>
            <Shield className="size-4" />
            <AlertTitle>{t('kubectlAccess.gatewayStatusLabel')}</AlertTitle>
            <AlertDescription>
              <div className="flex flex-wrap items-center gap-2">
                <StatusValueBadge labelKeyPrefix="kubectlAccess.gatewayStatuses" value={gateway.data?.status ?? 'unavailable'} />
                {gateway.data?.lastCheckedAt && (
                  <span>{t('kubectlAccess.gatewayCheckedAt', { time: formatAbsoluteDateTime(gateway.data.lastCheckedAt) })}</span>
                )}
              </div>
              {gateway.data?.observationCode && <p>{gateway.data.observationCode}</p>}
            </AlertDescription>
          </Alert>

          <Field hint={t('kubectlAccess.gatewayEnabledHint')} label={t('kubectlAccess.gatewayEnabled')}>
            <Controller
              control={form.control}
              name="enabled"
              render={({ field }) => (
                <ControlledCheckboxField field={field}>{t('kubectlAccess.gatewayEnabled')}</ControlledCheckboxField>
              )}
            />
          </Field>

          {!gatewayEnabled && (
            <Alert variant="warning">
              <AlertCircle className="size-4" />
              <AlertTitle>{t('kubectlAccess.gatewayDisabledTitle')}</AlertTitle>
              <AlertDescription>{t('kubectlAccess.gatewayDisabledDescription')}</AlertDescription>
            </Alert>
          )}

          <Field hint={t('kubectlAccess.gatewayRulesHint')} label={t('kubectlAccess.gatewayRules.title')}>
            <ClusterKubeGatewayRulesEditor control={form.control} disabled={saveGateway.isPending} t={t} />
          </Field>

          <DialogFooter>
            <Button type="button" variant="secondary" onClick={() => onOpenChange(false)}>
              {t('common.close')}
            </Button>
            <Button disabled={saveGateway.isPending || !form.formState.isValid} type="submit">
              {saveGateway.isPending && <LoaderCircle className="size-4 animate-spin" />}
              {t('common.save')}
            </Button>
          </DialogFooter>
        </form>
      )
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[min(88vh,56rem)] w-[min(94vw,54rem)] max-w-[94vw] min-w-0 flex-col gap-0 overflow-hidden p-0">
        <DialogHeader className="shrink-0 border-b border-border p-5 pb-4">
          <DialogTitle>{t('kubectlAccess.gatewayDialog.title', { name: cluster?.name ?? '' })}</DialogTitle>
          <DialogDescription>{t('kubectlAccess.gatewayDialog.description')}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 overflow-y-auto px-5 py-4">{body}</div>
      </DialogContent>
    </Dialog>
  )
}

function normalizeCommaSeparatedList(value: string) {
  return value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean)
}
