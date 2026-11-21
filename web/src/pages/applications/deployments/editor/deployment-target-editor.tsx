import type { FieldErrors, Resolver } from 'react-hook-form'
import type { DeploymentTargetFormValues } from './deployment-target-form'
import type { ArtifactRegistry, BuildTemplate, DeploymentTarget, Release, RepositoryBinding, RuntimeCluster } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Rocket, RotateCw } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { FormProvider, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api, ApiError } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { FloatingFormSaveBar, FloatingFormSaveBarSecondaryButton } from '@/components/common/floating-form-save-bar'
import { Button } from '@/components/ui/button'
import { deploymentReleaseKey } from '@/pages/applications/application-config-utils'
import { latestReleaseByTarget, redeployReleasePayload } from '@/pages/applications/deployments/application-deployments-panel-utils'
import { advancedItemConfiguredCount, visibleAdvancedItems } from './advanced/advanced-items'
import { AdvancedSection } from './advanced/advanced-section'
import { DeploymentTargetEditorNav } from './deployment-target-editor-nav'
import { preferredScrollBehavior, scrollToDeploymentTargetSection, sectionErrorCount } from './deployment-target-editor-utils'
import {
  createDeploymentTargetFormSchema,
  deploymentTargetErrorFields,
  deploymentTargetFormToPayload,
  deploymentTargetNeedsRedeploy,
  deploymentTargetSectionOrder,
  deploymentTargetToFormValues,
  pendingSecretOperations,
} from './deployment-target-form'
import { targetImageTemplateQuery } from './deployment-target-queries'
import { BasicsSection } from './sections/basics-section'
import { RuntimeSection } from './sections/runtime-section'
import { SourceSection } from './sections/source-section'
import { useUnsavedChangesGuard } from './use-unsaved-changes-guard'

export interface DeploymentTargetEditorProps {
  applicationId: string
  buildEnvironmentAvailable: boolean
  buildEnvironmentRetrying?: boolean
  canManageRuntimeSecrets: boolean
  deploymentTargets: DeploymentTarget[]
  initialValues: DeploymentTargetFormValues
  onRetryBuildEnvironment?: () => void
  projectId: string
  registries: ArtifactRegistry[]
  releases: Release[]
  repositoryBindings: RepositoryBinding[]
  runtimeClusters: RuntimeCluster[]
  target: DeploymentTarget | null
}

interface EditorResolverContext {
  buildTemplates: readonly BuildTemplate[] | undefined
  targetImageRefTouchedByUser: boolean
}

/** 部署配置独立编辑页的表单主体：4 个均衡主分区 + 6+1 折叠高级子项 + 底部粘性操作栏。 */
export function DeploymentTargetEditor({
  applicationId,
  buildEnvironmentAvailable,
  buildEnvironmentRetrying,
  canManageRuntimeSecrets,
  deploymentTargets,
  initialValues,
  onRetryBuildEnvironment,
  projectId,
  registries,
  releases,
  repositoryBindings,
  runtimeClusters,
  target,
}: DeploymentTargetEditorProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const formRef = useRef<HTMLFormElement>(null)
  const [targetImageRefTouchedByUser, setTargetImageRefTouchedByUser] = useState(false)
  const buildTemplates = useQuery({
    queryKey: ['build-templates'],
    queryFn: () => api.listBuildTemplates(),
    enabled: initialValues.sourceType === 'repository' || !target,
    staleTime: 5 * 60 * 1000,
  })

  const resolver = useMemo<Resolver<DeploymentTargetFormValues, EditorResolverContext>>(() => {
    return async (formValues, resolverContext, resolverOptions) => {
      let candidate = formValues
      if (!target && formValues.sourceType === 'repository' && !resolverContext?.targetImageRefTouchedByUser && formValues.targetRegistryId) {
        const templateOptions = targetImageTemplateQuery({
          applicationId,
          projectId,
          registryId: formValues.targetRegistryId,
          stage: formValues.stage,
          targetName: formValues.name,
        })
        const cached = queryClient.getQueryData<{ targetImageRef?: string }>(templateOptions.queryKey)
        if (cached?.targetImageRef) {
          candidate = { ...formValues, targetImageRef: cached.targetImageRef }
        }
        else if (!formValues.targetImageRef.trim()) {
          const fetched = await queryClient.fetchQuery(templateOptions).catch(() => undefined)
          if (fetched?.targetImageRef)
            candidate = { ...formValues, targetImageRef: fetched.targetImageRef }
        }
      }
      const currentSchema = createDeploymentTargetFormSchema(t, { buildTemplates: resolverContext?.buildTemplates })
      return zodResolver(currentSchema)(candidate, resolverContext, resolverOptions)
    }
  }, [applicationId, projectId, queryClient, t, target])

  const form = useForm<DeploymentTargetFormValues, EditorResolverContext>({
    context: {
      buildTemplates: buildTemplates.data,
      targetImageRefTouchedByUser,
    },
    defaultValues: initialValues,
    mode: 'onTouched',
    resolver,
    shouldFocusError: false,
  })
  // 已有非默认高级配置时默认展开，避免把已生效的配置藏起来。
  const [advancedOpen, setAdvancedOpen] = useState(() => visibleAdvancedItems(initialValues.sourceType)
    .some(item => advancedItemConfiguredCount(item, initialValues, target) > 0))
  const values = form.watch()
  const { dirtyFields, errors, isDirty, isSubmitted } = form.formState

  const followsImageTemplate = !target && values.sourceType === 'repository' && !targetImageRefTouchedByUser
  const imageTemplate = useQuery({
    ...targetImageTemplateQuery({
      applicationId,
      projectId,
      registryId: values.targetRegistryId,
      stage: values.stage,
      targetName: values.name,
    }),
    enabled: followsImageTemplate && Boolean(values.targetRegistryId),
  })

  const backTo = `/projects/${projectId}/apps/${applicationId}?tab=deployments`
  const latestRelease = target ? latestReleaseByTarget(releases)[deploymentReleaseKey(target.id)] : undefined
  const baselinePayload = useMemo(
    () => target ? deploymentTargetFormToPayload(initialValues, { includeBuildEnvironment: false }) : null,
    [initialValues, target],
  )
  const needsRedeploy = baselinePayload ? safeNeedsRedeploy(baselinePayload, values) : false
  const canRedeploy = Boolean(target && values.enabled && (values.sourceType === 'image' ? values.imageRef.trim() : latestRelease?.imageRef.trim()))
  const pendingSecrets = pendingSecretOperations(values.environmentVariables, {
    secretFiles: values.secretFiles,
    secretFilesSet: target?.secretFilesSet,
  })
  const pendingSecretCount = pendingSecrets.cleared + pendingSecrets.generated + pendingSecrets.set
  const totalErrors = deploymentTargetSectionOrder.reduce((count, section) => count + sectionErrorCount(errors, section, values.sourceType), 0)
  // 配置已保存但重新部署失败时留在页面，单独反馈并允许只重试部署。
  const [redeployFailure, setRedeployFailure] = useState<{ message: string, target: DeploymentTarget } | null>(null)
  const { allowNextNavigation, blocker } = useUnsavedChangesGuard(isDirty)
  // 保存后若继续修改，回到常规保存流程；未再修改时只提供“重试部署”。
  const showRedeployRetry = Boolean(redeployFailure) && !isDirty
  const leaveEditor = () => {
    allowNextNavigation()
    navigate(backTo)
  }
  const discardChanges = () => {
    form.reset()
    setTargetImageRefTouchedByUser(false)
  }

  const save = useMutation({
    mutationFn: async ({ formValues, redeploy }: { formValues: DeploymentTargetFormValues, redeploy: boolean }) => {
      const resolvedValues = await withTemplateImageRef(formValues)
      const payload = deploymentTargetFormToPayload(resolvedValues, {
        includeBuildEnvironment: buildEnvironmentAvailable && Boolean(dirtyFields.buildVariables || dirtyFields.buildSecrets),
      })
      const saved = target
        ? await api.updateDeploymentTarget(projectId, applicationId, target.id, payload)
        : await api.createDeploymentTarget(projectId, applicationId, payload)
      if (!redeploy)
        return { redeploy, redeployError: null, resolvedValues, saved }
      // 配置更新与创建发布是两次独立请求：发布失败不能让已成功的保存被当作整体失败。
      try {
        await createRedeployRelease(saved)
        return { redeploy, redeployError: null, resolvedValues, saved }
      }
      catch (error) {
        return { redeploy, redeployError: error instanceof Error ? error.message : t('errors.request.failed'), resolvedValues, saved }
      }
    },
    onSuccess: async ({ redeploy, redeployError, resolvedValues, saved }) => {
      void queryClient.invalidateQueries({ queryKey: ['build-environment-config', 'deployment', projectId, applicationId] })
      if (redeployError !== null) {
        // 表单基线更新为已保存的值，离开时不再提示未保存；保留“需要重新部署”的提示与重试入口。
        form.reset({ ...deploymentTargetToFormValues(saved), buildSecrets: resolvedValues.buildSecrets, buildVariables: resolvedValues.buildVariables })
        setRedeployFailure({ message: redeployError, target: saved })
        toast.error(t('deploymentTargetEditor.savedButRedeployFailed', { message: redeployError }))
        void queryClient.invalidateQueries({ queryKey: ['deployment-targets', projectId, applicationId] })
        return
      }
      toast.success(t(redeploy ? 'deploymentsPage.targetUpdatedAndRedeployQueued' : target ? 'deploymentsPage.targetUpdated' : 'deploymentsPage.targetCreated'))
      await queryClient.invalidateQueries({ queryKey: ['deployment-targets', projectId, applicationId] })
      void queryClient.invalidateQueries({ queryKey: ['application-runtime', projectId, applicationId] })
      if (redeploy)
        void queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
      leaveEditor()
    },
    onError: (error) => {
      const field = error instanceof ApiError ? deploymentTargetErrorFields[error.code] : undefined
      if (!field) {
        toast.error(error.message)
        return
      }
      form.setError(field, { message: error.message, type: 'server' })
      focusFirstError({ [field]: { message: error.message, type: 'server' } })
    },
  })

  const retryRedeploy = useMutation({
    mutationFn: (saved: DeploymentTarget) => createRedeployRelease(saved),
    onSuccess: () => {
      setRedeployFailure(null)
      toast.success(t('deploymentsPage.targetUpdatedAndRedeployQueued'))
      void queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
      leaveEditor()
    },
    onError: (error, saved) => setRedeployFailure({ message: error.message, target: saved }),
  })

  async function createRedeployRelease(saved: DeploymentTarget) {
    const releasePayload = redeployReleasePayload(saved, latestRelease)
    if (!releasePayload)
      throw new Error(t('deploymentsPage.redeployUnavailable'))
    return api.createRelease(projectId, releasePayload)
  }

  async function withTemplateImageRef(formValues: DeploymentTargetFormValues) {
    // 新建且未手动修改目标镜像时，以镜像站模板为准，与界面上派生展示的值保持一致。
    if (target || formValues.sourceType !== 'repository' || targetImageRefTouchedByUser || !formValues.targetRegistryId)
      return formValues
    const template = await queryClient.fetchQuery(targetImageTemplateQuery({
      applicationId,
      projectId,
      registryId: formValues.targetRegistryId,
      stage: formValues.stage,
      targetName: formValues.name,
    })).catch(() => undefined)
    return template?.targetImageRef ? { ...formValues, targetImageRef: template.targetImageRef } : formValues
  }

  function focusFirstError(fieldErrors: FieldErrors<DeploymentTargetFormValues>) {
    const currentSourceType = form.getValues('sourceType')
    if (sectionErrorCount(fieldErrors, 'advanced', currentSourceType) > 0)
      setAdvancedOpen(true)
    // 等错误与自动展开的高级选项渲染后，按页面顺序定位第一个无效控件；找不到控件时退回到所在分区。
    window.requestAnimationFrame(() => window.requestAnimationFrame(() => {
      const invalid = formRef.current?.querySelector<HTMLElement>('[aria-invalid="true"]')
      if (invalid) {
        invalid.scrollIntoView({ behavior: preferredScrollBehavior(), block: 'center' })
        invalid.focus({ preventScroll: true })
        return
      }
      const section = deploymentTargetSectionOrder.find(item => sectionErrorCount(fieldErrors, item, currentSourceType) > 0)
      if (section)
        scrollToDeploymentTargetSection(section)
    }))
  }

  const submit = (redeploy: boolean) => form.handleSubmit(
    formValues => save.mutate({ formValues, redeploy }),
    focusFirstError,
  )

  const hasVisibleErrors = isSubmitted && totalErrors > 0
  const showActionBar = !target || isDirty || showRedeployRetry || hasVisibleErrors || pendingSecretCount > 0 || needsRedeploy
  const statusTone = (redeployFailure && showRedeployRetry) || hasVisibleErrors
    ? 'danger'
    : needsRedeploy
      ? 'warning'
      : 'primary'

  return (
    <FormProvider {...form}>
      <form
        ref={formRef}
        noValidate
        aria-busy={save.isPending}
        className="grid min-w-0 gap-5"
        onSubmit={(event) => {
          // 弹窗通过 Portal 渲染，但 React 事件仍沿组件树冒泡；只处理本表单自身的提交。
          if (event.target !== event.currentTarget)
            return
          void submit(false)(event)
        }}
      >
        <div className="grid min-w-0 gap-5 @2xl/workspace:grid-cols-[14.5rem_minmax(0,1fr)]">
          <aside className="min-w-0 @2xl/workspace:sticky @2xl/workspace:top-0 @2xl/workspace:self-start">
            <DeploymentTargetEditorNav
              repositoryBindings={repositoryBindings}
              target={target}
              onNavigate={(section) => {
                if (section === 'advanced')
                  setAdvancedOpen(true)
              }}
            />
          </aside>
          <div className="grid min-w-0 gap-5">
            <BasicsSection
              editing={Boolean(target)}
              projectId={projectId}
              runtimeClusters={runtimeClusters}
            />
            <SourceSection
              applicationId={applicationId}
              editing={Boolean(target)}
              followsImageTemplate={followsImageTemplate}
              projectId={projectId}
              registries={registries}
              repositoryBindings={repositoryBindings}
              resolvedTemplateImageRef={imageTemplate.data?.targetImageRef}
              onExpandAdvanced={() => setAdvancedOpen(true)}
              onTargetImageRefManualChange={() => setTargetImageRefTouchedByUser(true)}
            />
            <RuntimeSection
              canManageRuntimeSecrets={canManageRuntimeSecrets}
              deploymentTargets={deploymentTargets}
              projectId={projectId}
              releases={releases}
              runtimeClusters={runtimeClusters}
            />
            <AdvancedSection
              buildEnvironmentAvailable={buildEnvironmentAvailable}
              buildEnvironmentRetrying={buildEnvironmentRetrying}
              open={advancedOpen}
              projectId={projectId}
              repositoryBindings={repositoryBindings}
              target={target}
              onOpenChange={setAdvancedOpen}
              onRetryBuildEnvironment={onRetryBuildEnvironment}
            />
            <FloatingFormSaveBar
              actions={(
                <>
                  {target && !showRedeployRetry
                    ? (
                        <FloatingFormSaveBarSecondaryButton
                          disabled={save.isPending || retryRedeploy.isPending}
                          type="button"
                          onClick={discardChanges}
                        >
                          {t('common.discardChanges')}
                        </FloatingFormSaveBarSecondaryButton>
                      )
                    : (
                        <FloatingFormSaveBarSecondaryButton
                          disabled={save.isPending || retryRedeploy.isPending}
                          type="button"
                          onClick={leaveEditor}
                        >
                          {t('common.cancel')}
                        </FloatingFormSaveBarSecondaryButton>
                      )}
                  {redeployFailure && showRedeployRetry && (
                    <Button className="rounded-full px-5" disabled={retryRedeploy.isPending || save.isPending} type="button" onClick={() => retryRedeploy.mutate(redeployFailure.target)}>
                      <RotateCw className="size-4" />
                      {t('deploymentTargetEditor.retryRedeploy')}
                    </Button>
                  )}
                  {needsRedeploy && !showRedeployRetry && (
                    <Button className="rounded-full px-4" disabled={save.isPending} type="submit" variant="outline">
                      {t('deploymentsPage.saveOnly')}
                    </Button>
                  )}
                  {showRedeployRetry
                    ? null
                    : needsRedeploy
                      ? (
                          <Button className="rounded-full px-5" disabled={save.isPending || !canRedeploy} type="button" onClick={() => void submit(true)()}>
                            <Rocket className="size-4" />
                            {t('deploymentsPage.saveAndRedeploy')}
                          </Button>
                        )
                      : (
                          <Button className="rounded-full px-5" disabled={save.isPending} type="submit">
                            {target ? t('common.save') : t('deploymentTargetEditor.create')}
                          </Button>
                        )}
                </>
              )}
              open={showActionBar}
              slot="editor-action-bar"
              statusContent={redeployFailure && showRedeployRetry
                ? (
                    <p className="m-0 min-w-0 text-danger" data-slot="redeploy-failure">
                      {t('deploymentTargetEditor.savedButRedeployFailed', { message: redeployFailure.message })}
                    </p>
                  )
                : hasVisibleErrors
                  ? (
                      <button className="min-w-0 text-left font-medium text-danger underline-offset-4 hover:underline focus-visible:underline" type="button" onClick={() => focusFirstError(errors)}>
                        {t('deploymentTargetEditor.fixErrors', { count: totalErrors })}
                      </button>
                    )
                  : pendingSecretCount > 0
                    ? (
                        <p className="m-0 min-w-0 text-foreground">
                          {t('deploymentTargetEditor.pendingSecretSummary', pendingSecrets)}
                        </p>
                      )
                    : needsRedeploy
                      ? (
                          <p className="m-0 min-w-0 text-warning">
                            {t(canRedeploy ? 'deploymentsPage.runtimeChangesNeedRedeployInline' : 'deploymentsPage.runtimeChangesNeedRedeployUnavailableInline')}
                          </p>
                        )
                      : (
                          <p className="m-0 truncate text-foreground">
                            {t(target ? 'common.unsavedChanges' : 'deploymentTargetEditor.readyToCreate')}
                          </p>
                        )}
              tone={statusTone}
            />
          </div>
        </div>
      </form>
      <ConfirmDialog
        cancelText={t('deploymentTargetEditor.leaveGuard.stay')}
        closeOnConfirm={false}
        confirmText={t('deploymentTargetEditor.leaveGuard.leave')}
        description={t('deploymentTargetEditor.leaveGuard.description')}
        open={blocker.state === 'blocked'}
        title={t('deploymentTargetEditor.leaveGuard.title')}
        onConfirm={() => blocker.proceed?.()}
        onOpenChange={(open) => {
          if (!open && blocker.state === 'blocked')
            blocker.reset()
        }}
      />
    </FormProvider>
  )
}

function safeNeedsRedeploy(baseline: ReturnType<typeof deploymentTargetFormToPayload>, values: DeploymentTargetFormValues) {
  try {
    return deploymentTargetNeedsRedeploy(baseline, deploymentTargetFormToPayload(values, { includeBuildEnvironment: false }))
  }
  catch {
    // 环境变量文本暂未通过格式校验时无法得到载荷；保守地视为需要重新部署。
    return true
  }
}
