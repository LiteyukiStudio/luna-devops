import type {
  Application,
  CreateKubeCredentialResponse,
  KubeCredential,
  Project,
  RuntimeCluster,
} from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, Download, Shield, Sparkles } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { CheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { SearchSelect } from '@/components/common/search-select'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatAbsoluteDateTime } from '@/components/common/time-format'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { downloadKubeconfigFile } from '@/lib/kubeconfig-file'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'

const KUBE_SCOPE_VALUES = ['kube:read', 'kube:write', 'kube:connect'] as const

function createSchema(t: (key: string, options?: Record<string, unknown>) => string) {
  return z.object({
    applicationId: z.string(),
    expiresInDays: z.union([z.literal(1), z.literal(7), z.literal(30)]),
    name: z.string().trim().min(1, t('kubectlAccess.form.nameRequired')).max(64, t('kubectlAccess.form.nameTooLong')),
    runtimeClusterId: z.string().min(1, t('kubectlAccess.form.runtimeClusterRequired')),
    scopes: z.array(z.enum(KUBE_SCOPE_VALUES)).min(1, t('kubectlAccess.form.scopesRequired')),
  })
}

interface KubeconfigCreateFormInput {
  applicationId: string
  expiresInDays: 1 | 7 | 30
  name: string
  runtimeClusterId: string
  scopes: Array<typeof KUBE_SCOPE_VALUES[number]>
}

function defaultFormValues(runtimeClusters: RuntimeCluster[]): KubeconfigCreateFormInput {
  return {
    applicationId: '',
    expiresInDays: 7,
    name: '',
    runtimeClusterId: runtimeClusters[0]?.id ?? '',
    scopes: ['kube:read'],
  }
}

export function KubeconfigCreateDialog({
  applications,
  open,
  project,
  runtimeClusters,
  onOpenChange,
}: {
  applications: Application[]
  open: boolean
  project?: Project
  runtimeClusters: RuntimeCluster[]
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const schema = useMemo(() => createSchema(t), [t])
  const [createdResult, setCreatedResult] = useState<CreateKubeCredentialResponse | null>(null)
  const form = useForm<KubeconfigCreateFormInput>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: defaultFormValues(runtimeClusters),
  })
  const selectedClusterId = form.watch('runtimeClusterId')
  const selectedScopes = form.watch('scopes') ?? []
  const selectedCluster = runtimeClusters.find(cluster => cluster.id === selectedClusterId)
  const gatewayStatus = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['runtime-clusters', selectedClusterId, 'kube-gateway'],
    queryFn: () => api.getRuntimeClusterKubeGateway(selectedClusterId),
    enabled: open && Boolean(selectedClusterId),
  })
  const gatewayReady = gatewayStatus.data?.status === 'ready'
  const clusterOptions = runtimeClusters.map(cluster => ({
    description: cluster.gatewayDomainSuffixes?.join(', ') || cluster.gatewayRootDomain,
    label: cluster.name,
    value: cluster.id,
  }))
  const applicationOptions = [
    {
      description: t('kubectlAccess.form.applicationAllDescription'),
      label: t('kubectlAccess.form.applicationAll'),
      value: '',
    },
    ...applications.map(application => ({
      description: application.identifier,
      label: application.name,
      value: application.id,
    })),
  ]

  useEffect(() => {
    if (!open)
      return

    const currentValues = form.getValues()
    form.reset({
      ...defaultFormValues(runtimeClusters),
      ...currentValues,
      runtimeClusterId: currentValues.runtimeClusterId && runtimeClusters.some(cluster => cluster.id === currentValues.runtimeClusterId)
        ? currentValues.runtimeClusterId
        : runtimeClusters[0]?.id ?? '',
    })
  }, [form, open, runtimeClusters])

  const createCredential = useMutation({
    mutationFn: async (values: KubeconfigCreateFormInput) => {
      const result = await api.createKubeCredential({
        name: values.name.trim(),
        expiresInDays: values.expiresInDays,
        scopes: normalizeKubeScopes(values.scopes),
        contexts: [{
          projectId: project?.id ?? '',
          runtimeClusterId: values.runtimeClusterId,
          ...(values.applicationId ? { applicationId: values.applicationId } : {}),
        }],
      })
      downloadKubeconfigFile(result.credential.name, result.kubeconfig)
      return result
    },
    onSuccess: (result) => {
      setCreatedResult(result)
      toast.success(t('kubectlAccess.created'))
      queryClient.invalidateQueries({ queryKey: ['kube-credentials'] })
    },
    onError: error => toast.error(error.message),
  })

  const submitDisabled = createCredential.isPending
    || !form.formState.isValid
    || !project?.id
    || !selectedCluster
    || (gatewayStatus.data ? !gatewayReady : false)

  const closeDialog = (nextOpen: boolean) => {
    if (!nextOpen) {
      form.reset(defaultFormValues(runtimeClusters))
      setCreatedResult(null)
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={closeDialog}>
      <DialogContent className="flex max-h-[min(88vh,54rem)] w-[min(92vw,44rem)] max-w-[92vw] min-w-0 flex-col gap-0 overflow-hidden p-0">
        <DialogHeader className="shrink-0 border-b border-border p-5 pb-4">
          <DialogTitle>{t('kubectlAccess.createDialog.title')}</DialogTitle>
          <DialogDescription>{t('kubectlAccess.createDialog.description')}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 overflow-y-auto px-5 py-4">
          {createdResult
            ? (
                <CreatedKubeconfigState
                  credential={createdResult.credential}
                  kubeconfig={createdResult.kubeconfig}
                  namespace={project?.kubernetesNamespace ?? ''}
                  onDownloadAgain={() => downloadKubeconfigFile(createdResult.credential.name, createdResult.kubeconfig)}
                />
              )
            : (
                <form className="grid gap-4" onSubmit={form.handleSubmit(values => createCredential.mutate(values))}>
                  <Alert>
                    <Shield className="size-4" />
                    <AlertTitle>{t('kubectlAccess.oneTimeTitle')}</AlertTitle>
                    <AlertDescription>
                      <p>{t('kubectlAccess.oneTimeDescription')}</p>
                      <p>{t('kubectlAccess.platformOverrideHint')}</p>
                    </AlertDescription>
                  </Alert>

                  <Field error={form.formState.errors.name?.message} hint={t('kubectlAccess.form.nameHint')} label={t('kubectlAccess.form.name')} required>
                    <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder={t('kubectlAccess.form.namePlaceholder')} />
                  </Field>

                  <Field error={form.formState.errors.runtimeClusterId?.message} hint={t('kubectlAccess.form.runtimeClusterHint')} label={t('kubectlAccess.form.runtimeCluster')} required>
                    <Controller
                      control={form.control}
                      name="runtimeClusterId"
                      render={({ field }) => (
                        <SearchSelect
                          ariaLabel={t('kubectlAccess.form.runtimeCluster')}
                          emptyLabel={t('kubectlAccess.noActiveClustersTitle')}
                          options={clusterOptions}
                          placeholder={t('kubectlAccess.form.runtimeClusterPlaceholder')}
                          value={field.value}
                          onValueChange={field.onChange}
                        />
                      )}
                    />
                  </Field>

                  {selectedCluster && (
                    <Alert variant={gatewayReady ? 'default' : 'warning'}>
                      {gatewayReady ? <Sparkles className="size-4" /> : <AlertCircle className="size-4" />}
                      <AlertTitle>{t('kubectlAccess.gatewayStatusLabel')}</AlertTitle>
                      <AlertDescription>
                        <div className="flex flex-wrap items-center gap-2">
                          <StatusValueBadge labelKeyPrefix="kubectlAccess.gatewayStatuses" value={gatewayStatus.data?.status ?? 'unavailable'} />
                          {gatewayStatus.data?.lastCheckedAt && (
                            <span>{t('kubectlAccess.gatewayCheckedAt', { time: formatAbsoluteDateTime(gatewayStatus.data.lastCheckedAt) })}</span>
                          )}
                        </div>
                        {gatewayStatus.data?.observationCode && <p>{gatewayStatus.data.observationCode}</p>}
                        {!gatewayReady && <p>{t('kubectlAccess.gatewayReadyRequired')}</p>}
                      </AlertDescription>
                    </Alert>
                  )}

                  <Field hint={t('kubectlAccess.form.applicationHint')} label={t('kubectlAccess.form.application')}>
                    <Controller
                      control={form.control}
                      name="applicationId"
                      render={({ field }) => (
                        <SearchSelect
                          ariaLabel={t('kubectlAccess.form.application')}
                          emptyLabel={t('common.noOptions')}
                          options={applicationOptions}
                          placeholder={t('kubectlAccess.form.applicationPlaceholder')}
                          value={field.value}
                          onValueChange={field.onChange}
                        />
                      )}
                    />
                  </Field>

                  <Field error={form.formState.errors.scopes?.message} hint={t('kubectlAccess.form.scopesHint')} label={t('kubectlAccess.form.scopes')} required>
                    <div className="grid gap-3 rounded-container border border-border bg-surface-raised p-4">
                      {KUBE_SCOPE_VALUES.map(scope => (
                        <CheckboxField
                          key={scope}
                          checked={selectedScopes.includes(scope)}
                          description={t(`kubectlAccess.scopeDescriptions.${scope}`)}
                          onCheckedChange={(checked) => {
                            form.setValue('scopes', normalizeKubeScopes(toggleKubeScope(selectedScopes, scope, checked === true)), { shouldDirty: true, shouldValidate: true })
                          }}
                        >
                          {t(`kubectlAccess.scopeLabels.${scope}`)}
                        </CheckboxField>
                      ))}
                    </div>
                  </Field>

                  <Field error={form.formState.errors.expiresInDays?.message} hint={t('kubectlAccess.form.expiresInDaysHint')} label={t('kubectlAccess.form.expiresInDays')} required>
                    <Select {...form.register('expiresInDays', { setValueAs: value => Number(value) as 1 | 7 | 30 })} aria-invalid={Boolean(form.formState.errors.expiresInDays)}>
                      <option value={1}>{t('kubectlAccess.expiresIn1Day')}</option>
                      <option value={7}>{t('kubectlAccess.expiresIn7Days')}</option>
                      <option value={30}>{t('kubectlAccess.expiresIn30Days')}</option>
                    </Select>
                  </Field>

                  <DialogFooter>
                    <Button type="button" variant="secondary" onClick={() => closeDialog(false)}>
                      {t('common.cancel')}
                    </Button>
                    <Button disabled={submitDisabled} type="submit">
                      <Download className="size-4" />
                      {createCredential.isPending ? t('kubectlAccess.createDialog.submitting') : t('kubectlAccess.createDialog.submit')}
                    </Button>
                  </DialogFooter>
                </form>
              )}
        </div>
        {createdResult && (
          <DialogFooter className="shrink-0 border-t border-border px-5 py-4">
            <Button type="button" variant="secondary" onClick={() => downloadKubeconfigFile(createdResult.credential.name, createdResult.kubeconfig)}>
              <Download className="size-4" />
              {t('kubectlAccess.downloadAgain')}
            </Button>
            <Button type="button" onClick={() => closeDialog(false)}>
              {t('common.close')}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  )
}

function CreatedKubeconfigState({
  credential,
  kubeconfig,
  namespace,
  onDownloadAgain,
}: {
  credential: KubeCredential
  kubeconfig: string
  namespace: string
  onDownloadAgain: () => void
}) {
  const { t } = useTranslation()

  return (
    <div className="grid gap-4">
      <Alert>
        <Download className="size-4" />
        <AlertTitle>{t('kubectlAccess.createDialog.createdTitle')}</AlertTitle>
        <AlertDescription>
          <p>{t('kubectlAccess.createDialog.createdDescription')}</p>
          <p>{t('kubectlAccess.gatewayNamespaceHint', { namespace })}</p>
        </AlertDescription>
      </Alert>

      <div className="grid gap-3 rounded-container border border-border bg-surface-raised p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="font-medium">{credential.name}</p>
            <p className="text-sm text-muted-foreground">{t('kubectlAccess.bindingCount', { count: credential.bindingCount })}</p>
          </div>
          <StatusValueBadge labelKeyPrefix="kubectlAccess.credentialStatuses" value={credential.status} />
        </div>
        <div className="flex flex-wrap gap-2">
          {credential.scopes.map(scope => (
            <StatusBadge key={scope}>{t(`kubectlAccess.scopeLabels.${scope}`)}</StatusBadge>
          ))}
        </div>
        <div className="grid gap-1 text-sm text-muted-foreground">
          <p>{t('kubectlAccess.createdAt', { time: formatAbsoluteDateTime(credential.createdAt) })}</p>
          <p>{t('kubectlAccess.expiresAt', { time: formatAbsoluteDateTime(credential.expiresAt) })}</p>
        </div>
      </div>

      <div className="grid gap-2 rounded-container border border-border bg-surface-inset p-4">
        <p className="text-sm font-medium">{t('kubectlAccess.kubeconfigPreviewTitle')}</p>
        <pre className="max-h-56 overflow-auto whitespace-pre-wrap break-all rounded-md bg-card p-3 text-xs text-muted-foreground">{kubeconfig}</pre>
      </div>

      <div className="flex justify-end">
        <Button type="button" variant="outline" onClick={onDownloadAgain}>
          <Download className="size-4" />
          {t('kubectlAccess.downloadAgain')}
        </Button>
      </div>
    </div>
  )
}

function normalizeKubeScopes(scopes: string[]) {
  const normalized = new Set(scopes)
  if (normalized.has('kube:write') || normalized.has('kube:connect'))
    normalized.add('kube:read')

  return KUBE_SCOPE_VALUES.filter(scope => normalized.has(scope))
}

function toggleKubeScope(scopes: string[], scope: string, checked: boolean) {
  if (checked)
    return [...scopes, scope]

  return scopes.filter(item => item !== scope)
}
