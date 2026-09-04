import type { CurrentUser, Project, RuntimeCluster } from '@/api'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { CodeEditor } from '@/components/common/code-editor'
import { FormField as Field } from '@/components/common/form-field'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { ProjectSpaceMultiSelect } from '@/components/common/project-space-select'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { inspectKubeconfig, selectSingleKubeconfigContext } from '@/lib/kubeconfig'
import { clusterResourcePolicySchema } from '../resources/cluster-resource-policy-schema'
import { canInspectClusterKubeconfig, defaultGatewayPublicPort, formatGatewayDomainSuffixes, kubeconfigContextOptionLabel, normalizeFormPort, parseGatewayDomainSuffixes } from './cluster-helpers'

type ClusterForm = Omit<RuntimeCluster, 'gatewayDomainSuffixes' | 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & {
  gatewayDomainSuffixesText: string
  kubeconfig?: string
}

const clusterDefaults: ClusterForm = {
  endpoint: '',
  isDefault: false,
  kubeconfig: '',
  gatewayControllerType: 'traefik',
  gatewayClassName: 'traefik',
  gatewayDefaultRequestHeaders: '',
  gatewayDefaultResponseHeaders: '',
  gatewayExternalTLSMode: 'none',
  gatewayForwardedHeadersMode: 'preserve',
  gatewayName: 'luna-gateway',
  gatewayNamespace: 'kube-system',
  gatewayDomainSuffixesText: 'apps.local',
  gatewayPublicPort: 80,
  gatewayPublicScheme: 'http',
  gatewayHttpListenerName: 'web',
  gatewayHttpListenerPort: 8080,
  gatewayHttpsListenerName: 'websecure',
  gatewayHttpsListenerPort: 8443,
  gatewayTlsSecretName: '',
  gatewayTlsSecretNamespace: '',
  gatewayCertIssuerKind: 'ClusterIssuer',
  gatewayCertIssuerName: '',
  gatewayCertificateNamespace: '',
  gatewayWildcardCertEnabled: false,
  gatewayWildcardCertDomain: '',
  gatewayWildcardCertSecretName: '',
  gatewayTrustedProxyCIDRs: '',
  maxConcurrentBuilds: 4,
  cpuRequestPercent: 10,
  memoryRequestPercent: 25,
  cpuLimitPercent: 100,
  memoryLimitPercent: 100,
  name: '',
  ownerRef: '',
  projectIds: [],
  scope: 'global',
  status: 'unknown',
}

export function ClusterFormDialog({ editingCluster, open, projects, user, onOpenChange, onSaved }: {
  editingCluster: RuntimeCluster | null
  open: boolean
  projects: Project[]
  user?: CurrentUser
  onOpenChange: (open: boolean) => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selectedKubeconfigContext, setSelectedKubeconfigContext] = useState('')
  const form = useForm<ClusterForm>({ defaultValues: editingCluster ? formValuesFromCluster(editingCluster) : clusterDefaults, mode: 'onChange' })
  const policySchema = useMemo(() => clusterResourcePolicySchema(t), [t])
  const policyValues = form.watch(['cpuRequestPercent', 'memoryRequestPercent', 'cpuLimitPercent', 'memoryLimitPercent'])
  const policyValidation = policySchema.safeParse({
    cpuRequestPercent: policyValues[0],
    memoryRequestPercent: policyValues[1],
    cpuLimitPercent: policyValues[2],
    memoryLimitPercent: policyValues[3],
  })
  const policyErrors = policyValidation.success
    ? new Map<string, string>()
    : new Map(policyValidation.error.issues.map(issue => [String(issue.path[0]), issue.message]))
  const scope = form.watch('scope')
  const canEditKubeconfig = !editingCluster || canInspectClusterKubeconfig(editingCluster, user?.id, user?.role)
  const kubeconfigValue = form.watch('kubeconfig') ?? ''
  const kubeconfigInspection = useMemo(() => inspectKubeconfig(kubeconfigValue), [kubeconfigValue])
  const kubeconfigContextSelectionRequired = canEditKubeconfig && kubeconfigInspection.contexts.length > 1
  const effectiveKubeconfigContext = useMemo(() => {
    if (!canEditKubeconfig || kubeconfigInspection.contexts.length === 0)
      return ''
    if (kubeconfigInspection.contexts.some(context => context.name === selectedKubeconfigContext))
      return selectedKubeconfigContext
    return kubeconfigInspection.currentContext || kubeconfigInspection.contexts[0]?.name || ''
  }, [canEditKubeconfig, kubeconfigInspection, selectedKubeconfigContext])

  useEffect(() => {
    if (scope !== 'global')
      form.setValue('isDefault', false, { shouldDirty: true, shouldValidate: true })
    if (scope === 'user')
      form.setValue('ownerRef', '', { shouldDirty: true, shouldValidate: true })
    if (scope !== 'project')
      form.setValue('projectIds', [], { shouldDirty: true, shouldValidate: true })
  }, [form, scope])

  const saveCluster = useMutation({
    mutationFn: (values: ClusterForm) => {
      const { gatewayDomainSuffixesText, ...clusterValues } = values
      const gatewayDomainSuffixes = parseGatewayDomainSuffixes(gatewayDomainSuffixesText)
      const payload = {
        ...clusterValues,
        gatewayDomainSuffixes,
        ownerRef: '',
        projectIds: values.scope === 'project' ? values.projectIds : [],
      }
      return editingCluster ? api.updateRuntimeCluster(editingCluster.id, payload) : api.createRuntimeCluster(payload)
    },
    onSuccess: () => {
      toast.success(t(editingCluster ? 'deploymentsPage.clusterUpdated' : 'deploymentsPage.clusterCreated'))
      form.reset(clusterDefaults)
      onSaved()
      queryClient.invalidateQueries({ queryKey: ['runtime-clusters'] })
    },
    onError: error => toast.error(error.message),
  })

  function submitCluster(values: ClusterForm) {
    const parsedPolicy = policySchema.safeParse(values)
    if (!parsedPolicy.success) {
      toast.error(parsedPolicy.error.issues[0]?.message ?? t('clustersPage.resourcePercentRange'))
      return
    }
    let kubeconfig = values.kubeconfig ?? ''
    if (canEditKubeconfig && kubeconfig.trim() !== '') {
      if (kubeconfigInspection.error) {
        toast.error(t('clustersPage.kubeconfigParseFailed'))
        return
      }
      if (kubeconfigContextSelectionRequired && !effectiveKubeconfigContext) {
        toast.error(t('clustersPage.kubeconfigContextRequired'))
        return
      }
      try {
        kubeconfig = selectSingleKubeconfigContext(kubeconfig, effectiveKubeconfigContext)
      }
      catch {
        toast.error(t('clustersPage.kubeconfigContextInvalid'))
        return
      }
    }
    const maxConcurrentBuilds = Number.isFinite(values.maxConcurrentBuilds) && values.maxConcurrentBuilds > 0
      ? Math.floor(values.maxConcurrentBuilds)
      : 4
    saveCluster.mutate({
      ...values,
      gatewayHttpListenerPort: normalizeFormPort(values.gatewayHttpListenerPort, 8080),
      gatewayHttpsListenerPort: normalizeFormPort(values.gatewayHttpsListenerPort, 8443),
      gatewayPublicPort: normalizeFormPort(values.gatewayPublicPort, defaultGatewayPublicPort(values.gatewayPublicScheme)),
      kubeconfig,
      maxConcurrentBuilds,
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[min(88vh,52rem)] w-[min(92vw,48rem)] max-w-[92vw] min-w-0 flex-col gap-0 overflow-hidden p-0">
        <DialogHeader className="shrink-0 border-b border-border p-5 pb-4">
          <DialogTitle>{editingCluster ? t('deploymentsPage.editCluster') : t('deploymentsPage.createCluster')}</DialogTitle>
          <DialogDescription>{t('clustersPage.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="flex min-h-0 min-w-0 flex-1 flex-col" onSubmit={form.handleSubmit(submitCluster)}>
          <div className="min-h-0 min-w-0 max-w-full flex-1 overflow-y-auto overflow-x-hidden px-5 py-4">
            <div className="grid min-w-0 max-w-full gap-3 overflow-x-hidden">
              <ProgressiveSection defaultOpen description={t('clustersPage.basicClusterConfigDescription')} title={t('clustersPage.basicClusterConfig')}>
                <Field label={t('common.name')} required><Input {...form.register('name', { required: true })} /></Field>
                <Field label={t('common.scope')}>
                  <Select {...form.register('scope')}>
                    <option value="global">{t('codeRepositoriesView.scopeGlobal')}</option>
                    <option value="project">{t('codeRepositoriesView.scopeProject')}</option>
                    <option value="user">{t('codeRepositoriesView.scopeUser')}</option>
                  </Select>
                </Field>
                {scope === 'project' && (
                  <Field label={t('projectSpaces.title')} required>
                    <ProjectSpaceMultiSelect projects={projects} value={form.watch('projectIds')} onChange={value => form.setValue('projectIds', value, { shouldDirty: true, shouldValidate: true })} />
                  </Field>
                )}
                <Field hint={t('clustersPage.maxConcurrentBuildsHint')} label={t('clustersPage.maxConcurrentBuilds')} required>
                  <Input {...form.register('maxConcurrentBuilds', { min: 1, required: true, valueAsNumber: true })} inputMode="numeric" min={1} placeholder={t('clustersPage.maxConcurrentBuildsPlaceholder')} type="number" />
                </Field>
                {scope === 'global' && (
                  <Controller
                    control={form.control}
                    name="isDefault"
                    render={({ field }) => <ControlledCheckboxField field={field}>{t('clustersPage.defaultCluster')}</ControlledCheckboxField>}
                  />
                )}
              </ProgressiveSection>
              <ProgressiveSection defaultOpen description={t('clustersPage.resourcePolicyDescription')} title={t('clustersPage.resourcePolicy')}>
                <div className="grid gap-3 md:grid-cols-2">
                  <Field error={policyErrors.get('cpuRequestPercent')} hint={t('clustersPage.resourcePercentHint')} label={t('clustersPage.cpuRequestPercent')} required>
                    <Input {...form.register('cpuRequestPercent', { valueAsNumber: true })} inputMode="numeric" max={100} min={0} type="number" />
                  </Field>
                  <Field error={policyErrors.get('memoryRequestPercent')} hint={t('clustersPage.resourcePercentHint')} label={t('clustersPage.memoryRequestPercent')} required>
                    <Input {...form.register('memoryRequestPercent', { valueAsNumber: true })} inputMode="numeric" max={100} min={0} type="number" />
                  </Field>
                  <Field error={policyErrors.get('cpuLimitPercent')} hint={t('clustersPage.resourcePercentHint')} label={t('clustersPage.cpuLimitPercent')} required>
                    <Input {...form.register('cpuLimitPercent', { valueAsNumber: true })} inputMode="numeric" max={100} min={0} type="number" />
                  </Field>
                  <Field error={policyErrors.get('memoryLimitPercent')} hint={t('clustersPage.resourcePercentHint')} label={t('clustersPage.memoryLimitPercent')} required>
                    <Input {...form.register('memoryLimitPercent', { valueAsNumber: true })} inputMode="numeric" max={100} min={0} type="number" />
                  </Field>
                </div>
              </ProgressiveSection>
              <ProgressiveSection defaultOpen description={t('clustersPage.gatewayExternalAccessConfigDescription')} title={t('clustersPage.gatewayExternalAccessConfig')}>
                <div className="grid gap-3 md:grid-cols-2">
                  <Field hint={t('clustersPage.gatewayDomainSuffixesHint')} label={t('clustersPage.gatewayDomainSuffixes')} required>
                    <Textarea {...form.register('gatewayDomainSuffixesText', { required: true })} placeholder={t('clustersPage.gatewayDomainSuffixesPlaceholder')} rows={3} />
                  </Field>
                  <Field hint={t('clustersPage.gatewayPublicSchemeHint')} label={t('clustersPage.gatewayPublicScheme')} required>
                    <Select {...form.register('gatewayPublicScheme', { onChange: (event) => {
                      const nextScheme = event.target.value as RuntimeCluster['gatewayPublicScheme']
                      const currentPort = form.getValues('gatewayPublicPort')
                      if (currentPort === 80 || currentPort === 443 || !currentPort)
                        form.setValue('gatewayPublicPort', defaultGatewayPublicPort(nextScheme), { shouldDirty: true, shouldValidate: true })
                    } })}
                    >
                      <option value="http">{t('clustersPage.protocolHttp')}</option>
                      <option value="https">{t('clustersPage.protocolHttps')}</option>
                    </Select>
                  </Field>
                  <Field hint={t('clustersPage.gatewayPublicPortHint')} label={t('clustersPage.gatewayPublicPort')} required>
                    <Input {...form.register('gatewayPublicPort', { min: 1, required: true, valueAsNumber: true })} inputMode="numeric" max={65535} min={1} placeholder={String(defaultGatewayPublicPort(form.watch('gatewayPublicScheme')))} type="number" />
                  </Field>
                  <Field hint={t('clustersPage.gatewayExternalTLSModeHint')} label={t('clustersPage.gatewayExternalTLSMode')}>
                    <Select {...form.register('gatewayExternalTLSMode')}>
                      <option value="none">{t('clustersPage.gatewayTLSNone')}</option>
                      <option value="gateway">{t('clustersPage.gatewayTLSGateway')}</option>
                      <option value="upstream">{t('clustersPage.gatewayTLSUpstream')}</option>
                    </Select>
                  </Field>
                  <Field hint={t('clustersPage.gatewayForwardedHeadersModeHint')} label={t('clustersPage.gatewayForwardedHeadersMode')}>
                    <Select {...form.register('gatewayForwardedHeadersMode')}>
                      <option value="preserve">{t('clustersPage.gatewayForwardedPreserve')}</option>
                      <option value="overwrite">{t('clustersPage.gatewayForwardedOverwrite')}</option>
                      <option value="none">{t('clustersPage.gatewayForwardedNone')}</option>
                    </Select>
                  </Field>
                </div>
              </ProgressiveSection>
              <ProgressiveSection defaultOpen description={t('clustersPage.gatewayInternalConfigDescription')} title={t('clustersPage.gatewayInternalConfig')}>
                <div className="grid gap-3 md:grid-cols-2">
                  <Field hint={t('clustersPage.gatewayControllerTypeHint')} label={t('clustersPage.gatewayControllerType')}>
                    <Select {...form.register('gatewayControllerType')}>
                      <option value="traefik">{t('clustersPage.gatewayControllerTraefik')}</option>
                      <option value="generic">{t('clustersPage.gatewayControllerGeneric')}</option>
                    </Select>
                  </Field>
                  <Field hint={t('clustersPage.gatewayClassNameHint')} label={t('clustersPage.gatewayClassName')}><Input {...form.register('gatewayClassName')} placeholder={t('clustersPage.gatewayClassNamePlaceholder')} /></Field>
                  <Field hint={t('clustersPage.gatewayNameHint')} label={t('clustersPage.gatewayName')}><Input {...form.register('gatewayName')} placeholder={t('clustersPage.gatewayNamePlaceholder')} /></Field>
                  <Field hint={t('clustersPage.gatewayNamespaceHint')} label={t('clustersPage.gatewayNamespace')}><Input {...form.register('gatewayNamespace')} placeholder={t('clustersPage.gatewayNamespacePlaceholder')} /></Field>
                  <Field hint={t('clustersPage.gatewayHttpListenerNameHint')} label={t('clustersPage.gatewayHttpListenerName')}><Input {...form.register('gatewayHttpListenerName')} placeholder={t('clustersPage.gatewayHttpListenerNamePlaceholder')} /></Field>
                  <Field hint={t('clustersPage.gatewayHttpListenerPortHint')} label={t('clustersPage.gatewayHttpListenerPort')} required><Input {...form.register('gatewayHttpListenerPort', { min: 1, required: true, valueAsNumber: true })} inputMode="numeric" max={65535} min={1} placeholder="8080" type="number" /></Field>
                  <Field hint={t('clustersPage.gatewayHttpsListenerNameHint')} label={t('clustersPage.gatewayHttpsListenerName')}><Input {...form.register('gatewayHttpsListenerName')} placeholder={t('clustersPage.gatewayHttpsListenerNamePlaceholder')} /></Field>
                  <Field hint={t('clustersPage.gatewayHttpsListenerPortHint')} label={t('clustersPage.gatewayHttpsListenerPort')} required><Input {...form.register('gatewayHttpsListenerPort', { min: 1, required: true, valueAsNumber: true })} inputMode="numeric" max={65535} min={1} placeholder="8443" type="number" /></Field>
                  <Field hint={t('clustersPage.gatewayTlsSecretNameHint')} label={t('clustersPage.gatewayTlsSecretName')}><Input {...form.register('gatewayTlsSecretName')} placeholder={t('clustersPage.gatewayTlsSecretNamePlaceholder')} /></Field>
                  <Field hint={t('clustersPage.gatewayTlsSecretNamespaceHint')} label={t('clustersPage.gatewayTlsSecretNamespace')}><Input {...form.register('gatewayTlsSecretNamespace')} placeholder={t('clustersPage.gatewayTlsSecretNamespacePlaceholder')} /></Field>
                  <Field hint={t('clustersPage.gatewayCertIssuerKindHint')} label={t('clustersPage.gatewayCertIssuerKind')}>
                    <Select {...form.register('gatewayCertIssuerKind')}>
                      <option value="ClusterIssuer">{t('clustersPage.gatewayClusterIssuer')}</option>
                      <option value="Issuer">{t('clustersPage.gatewayNamespacedIssuer')}</option>
                    </Select>
                  </Field>
                  <Field hint={t('clustersPage.gatewayCertIssuerNameHint')} label={t('clustersPage.gatewayCertIssuerName')}><Input {...form.register('gatewayCertIssuerName')} placeholder={t('clustersPage.gatewayCertIssuerNamePlaceholder')} /></Field>
                  <Field hint={t('clustersPage.gatewayCertificateNamespaceHint')} label={t('clustersPage.gatewayCertificateNamespace')}><Input {...form.register('gatewayCertificateNamespace')} placeholder={t('clustersPage.gatewayCertificateNamespacePlaceholder')} /></Field>
                  <div className="md:col-span-2">
                    <Controller
                      control={form.control}
                      name="gatewayWildcardCertEnabled"
                      render={({ field }) => <ControlledCheckboxField field={field}>{t('clustersPage.gatewayWildcardCertEnabled')}</ControlledCheckboxField>}
                    />
                  </div>
                  <Field hint={t('clustersPage.gatewayWildcardCertDomainHint')} label={t('clustersPage.gatewayWildcardCertDomain')}><Input {...form.register('gatewayWildcardCertDomain')} placeholder={t('clustersPage.gatewayWildcardCertDomainPlaceholder')} /></Field>
                  <Field hint={t('clustersPage.gatewayWildcardCertSecretNameHint')} label={t('clustersPage.gatewayWildcardCertSecretName')}><Input {...form.register('gatewayWildcardCertSecretName')} placeholder={t('clustersPage.gatewayWildcardCertSecretNamePlaceholder')} /></Field>
                </div>
              </ProgressiveSection>
              <ProgressiveSection description={t('clustersPage.gatewayAdvancedDefaultsDescription')} title={t('clustersPage.gatewayAdvancedDefaults')}>
                <Field hint={t('clustersPage.gatewayTrustedProxyCIDRsHint')} label={t('clustersPage.gatewayTrustedProxyCIDRs')}><Textarea {...form.register('gatewayTrustedProxyCIDRs')} placeholder={t('clustersPage.gatewayTrustedProxyCIDRsPlaceholder')} rows={3} /></Field>
                <Field hint={t('clustersPage.gatewayDefaultRequestHeadersHint')} label={t('clustersPage.gatewayDefaultRequestHeaders')}><Textarea {...form.register('gatewayDefaultRequestHeaders')} placeholder={t('gatewayRoutesPage.headersPlaceholder')} rows={4} /></Field>
                <Field hint={t('clustersPage.gatewayDefaultResponseHeadersHint')} label={t('clustersPage.gatewayDefaultResponseHeaders')}><Textarea {...form.register('gatewayDefaultResponseHeaders')} placeholder={t('gatewayRoutesPage.headersPlaceholder')} rows={4} /></Field>
              </ProgressiveSection>
              <ProgressiveSection defaultOpen description={canEditKubeconfig ? t('clustersPage.kubeconfigHint') : t('clustersPage.kubeconfigRestrictedHint')} title={t('deploymentsPage.kubeconfig')}>
                <Field hint={canEditKubeconfig ? t('clustersPage.kubeconfigHint') : t('clustersPage.kubeconfigRestrictedHint')} label={t('deploymentsPage.kubeconfig')} required={!editingCluster}>
                  <Controller
                    control={form.control}
                    name="kubeconfig"
                    rules={{ required: !editingCluster }}
                    render={({ field }) => (
                      <div className="min-w-0 max-w-full overflow-x-hidden">
                        <CodeEditor ariaInvalid={Boolean(form.formState.errors.kubeconfig) || kubeconfigInspection.error} className="w-full" height="22rem" language="yaml" placeholder={editingCluster?.kubeconfigSet ? t('common.secretSetPlaceholder') : t('clustersPage.kubeconfigPlaceholder')} readOnly={!canEditKubeconfig} value={field.value ?? ''} onChange={field.onChange} />
                        {canEditKubeconfig && kubeconfigInspection.error && <p className="mt-2 text-sm text-danger">{t('clustersPage.kubeconfigParseFailed')}</p>}
                        {canEditKubeconfig && kubeconfigInspection.contexts.length === 1 && <p className="mt-2 text-sm text-muted-foreground">{t('clustersPage.kubeconfigSingleContext', { context: kubeconfigInspection.contexts[0].name })}</p>}
                        {kubeconfigContextSelectionRequired && (
                          <div className="mt-3 grid gap-2">
                            <label className="text-sm font-medium text-foreground" htmlFor="cluster-kubeconfig-context">{t('clustersPage.kubeconfigContextLabel')}</label>
                            <Select id="cluster-kubeconfig-context" value={effectiveKubeconfigContext} onChange={event => setSelectedKubeconfigContext(event.target.value)}>
                              {kubeconfigInspection.contexts.map(context => <option key={context.name} value={context.name}>{kubeconfigContextOptionLabel(context)}</option>)}
                            </Select>
                            <p className="text-xs text-muted-foreground">{t('clustersPage.kubeconfigContextHint')}</p>
                          </div>
                        )}
                      </div>
                    )}
                  />
                </Field>
              </ProgressiveSection>
            </div>
          </div>
          <DialogFooter className="shrink-0 border-t border-border p-5 pt-4">
            <Button disabled={!form.formState.isValid || !policyValidation.success || saveCluster.isPending || kubeconfigInspection.error || (kubeconfigContextSelectionRequired && !effectiveKubeconfigContext)} type="submit">{t('common.save')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function formValuesFromCluster(cluster: RuntimeCluster): ClusterForm {
  return {
    endpoint: cluster.endpoint,
    isDefault: cluster.isDefault,
    kubeconfig: '',
    gatewayControllerType: cluster.gatewayControllerType || 'traefik',
    gatewayClassName: cluster.gatewayClassName || 'traefik',
    gatewayDefaultRequestHeaders: cluster.gatewayDefaultRequestHeaders || '',
    gatewayDefaultResponseHeaders: cluster.gatewayDefaultResponseHeaders || '',
    gatewayExternalTLSMode: cluster.gatewayExternalTLSMode || 'none',
    gatewayForwardedHeadersMode: cluster.gatewayForwardedHeadersMode || 'preserve',
    gatewayName: cluster.gatewayName || 'luna-gateway',
    gatewayNamespace: cluster.gatewayNamespace || 'kube-system',
    gatewayDomainSuffixesText: formatGatewayDomainSuffixes(cluster),
    gatewayPublicPort: cluster.gatewayPublicPort || defaultGatewayPublicPort(cluster.gatewayPublicScheme || 'http'),
    gatewayPublicScheme: cluster.gatewayPublicScheme || 'http',
    gatewayHttpListenerName: cluster.gatewayHttpListenerName || 'web',
    gatewayHttpListenerPort: cluster.gatewayHttpListenerPort || 8080,
    gatewayHttpsListenerName: cluster.gatewayHttpsListenerName || 'websecure',
    gatewayHttpsListenerPort: cluster.gatewayHttpsListenerPort || 8443,
    gatewayTlsSecretName: cluster.gatewayTlsSecretName || '',
    gatewayTlsSecretNamespace: cluster.gatewayTlsSecretNamespace || '',
    gatewayCertIssuerKind: cluster.gatewayCertIssuerKind || 'ClusterIssuer',
    gatewayCertIssuerName: cluster.gatewayCertIssuerName || '',
    gatewayCertificateNamespace: cluster.gatewayCertificateNamespace || '',
    gatewayWildcardCertEnabled: cluster.gatewayWildcardCertEnabled || false,
    gatewayWildcardCertDomain: cluster.gatewayWildcardCertDomain || '',
    gatewayWildcardCertSecretName: cluster.gatewayWildcardCertSecretName || '',
    gatewayTrustedProxyCIDRs: cluster.gatewayTrustedProxyCIDRs || '',
    maxConcurrentBuilds: cluster.maxConcurrentBuilds || 4,
    cpuRequestPercent: cluster.cpuRequestPercent ?? 10,
    memoryRequestPercent: cluster.memoryRequestPercent ?? 25,
    cpuLimitPercent: cluster.cpuLimitPercent ?? 100,
    memoryLimitPercent: cluster.memoryLimitPercent ?? 100,
    name: cluster.name,
    ownerRef: cluster.ownerRef,
    projectIds: cluster.projectIds ?? [],
    scope: cluster.scope,
    status: cluster.status,
  }
}
