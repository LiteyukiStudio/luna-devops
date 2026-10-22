import type { FieldErrors } from 'react-hook-form'
import type { DeploymentTargetFormValues } from './deployment-target-form'
import type { ArtifactRegistry, DeploymentTarget, Release, RepositoryBinding, RuntimeCluster } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Rocket, Save } from 'lucide-react'
import { useMemo, useRef } from 'react'
import { FormProvider, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api, ApiError } from '@/api'
import { Button } from '@/components/ui/button'
import { deploymentReleaseKey } from '@/pages/applications/application-config-utils'
import { latestReleaseByTarget, redeployReleasePayload } from '@/pages/applications/deployments/application-deployments-panel-utils'
import { DeploymentTargetEditorNav } from './deployment-target-editor-nav'
import { preferredScrollBehavior, scrollToDeploymentTargetSection, sectionErrorCount } from './deployment-target-editor-utils'
import {
  createDeploymentTargetFormSchema,
  deploymentTargetErrorFields,
  deploymentTargetFormToPayload,
  deploymentTargetNeedsRedeploy,
  deploymentTargetSectionOrder,
} from './deployment-target-form'
import { targetImageTemplateQuery } from './deployment-target-queries'
import { BasicsSection } from './sections/basics-section'
import { HooksSection } from './sections/hooks-section'
import { ReleaseSection } from './sections/release-section'
import { RuntimeSection } from './sections/runtime-section'
import { SecuritySection } from './sections/security-section'
import { SourceBuildSection } from './sections/source-build-section'

export interface DeploymentTargetEditorProps {
  applicationId: string
  buildEnvironmentAvailable: boolean
  canManageRuntimeSecrets: boolean
  deploymentTargets: DeploymentTarget[]
  initialValues: DeploymentTargetFormValues
  projectId: string
  registries: ArtifactRegistry[]
  releases: Release[]
  repositoryBindings: RepositoryBinding[]
  runtimeClusters: RuntimeCluster[]
  target: DeploymentTarget | null
}

/** 部署配置独立编辑页的表单主体：分区导航 + 分区表单 + 固定操作栏。 */
export function DeploymentTargetEditor({
  applicationId,
  buildEnvironmentAvailable,
  canManageRuntimeSecrets,
  deploymentTargets,
  initialValues,
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
  const schema = useMemo(() => createDeploymentTargetFormSchema(t), [t])
  const form = useForm<DeploymentTargetFormValues>({
    defaultValues: initialValues,
    mode: 'onTouched',
    resolver: zodResolver(schema),
    shouldFocusError: false,
  })
  const values = form.watch()
  const { dirtyFields, errors, isSubmitted } = form.formState
  const backTo = `/projects/${projectId}/apps/${applicationId}?tab=deployments`
  const latestRelease = target ? latestReleaseByTarget(releases)[deploymentReleaseKey(target.id)] : undefined
  const baselinePayload = useMemo(
    () => target ? deploymentTargetFormToPayload(initialValues, { includeBuildEnvironment: false }) : null,
    [initialValues, target],
  )
  const needsRedeploy = baselinePayload ? safeNeedsRedeploy(baselinePayload, values) : false
  const canRedeploy = Boolean(target && values.enabled && (values.sourceType === 'image' ? values.imageRef.trim() : latestRelease?.imageRef.trim()))
  const totalErrors = deploymentTargetSectionOrder.reduce((count, section) => count + sectionErrorCount(errors, section), 0)

  const save = useMutation({
    mutationFn: async ({ formValues, redeploy }: { formValues: DeploymentTargetFormValues, redeploy: boolean }) => {
      const resolvedValues = await withTemplateImageRef(formValues)
      const payload = deploymentTargetFormToPayload(resolvedValues, {
        includeBuildEnvironment: buildEnvironmentAvailable && Boolean(dirtyFields.buildVariables || dirtyFields.buildSecrets),
      })
      const saved = target
        ? await api.updateDeploymentTarget(projectId, applicationId, target.id, payload)
        : await api.createDeploymentTarget(projectId, applicationId, payload)
      if (redeploy) {
        const releasePayload = redeployReleasePayload(saved, latestRelease)
        if (!releasePayload)
          throw new Error(t('deploymentsPage.redeployUnavailable'))
        await api.createRelease(projectId, releasePayload)
      }
      return { redeploy }
    },
    onSuccess: async ({ redeploy }) => {
      toast.success(t(redeploy ? 'deploymentsPage.targetUpdatedAndRedeployQueued' : target ? 'deploymentsPage.targetUpdated' : 'deploymentsPage.targetCreated'))
      await queryClient.invalidateQueries({ queryKey: ['deployment-targets', projectId, applicationId] })
      void queryClient.invalidateQueries({ queryKey: ['build-environment-config', 'deployment', projectId, applicationId] })
      if (redeploy)
        void queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
      navigate(backTo)
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

  async function withTemplateImageRef(formValues: DeploymentTargetFormValues) {
    // 新建且未手动修改目标镜像时，以镜像站模板为准，与界面上派生展示的值保持一致。
    if (target || formValues.sourceType !== 'repository' || dirtyFields.targetImageRef || !formValues.targetRegistryId)
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
    // 等错误与自动展开的高级选项渲染后，按页面顺序定位第一个无效控件；找不到控件时退回到所在分区。
    window.requestAnimationFrame(() => window.requestAnimationFrame(() => {
      const invalid = formRef.current?.querySelector<HTMLElement>('[aria-invalid="true"]')
      if (invalid) {
        invalid.scrollIntoView({ behavior: preferredScrollBehavior(), block: 'center' })
        invalid.focus({ preventScroll: true })
        return
      }
      const section = deploymentTargetSectionOrder.find(item => sectionErrorCount(fieldErrors, item) > 0)
      if (section)
        scrollToDeploymentTargetSection(section)
    }))
  }

  const submit = (redeploy: boolean) => form.handleSubmit(
    formValues => save.mutate({ formValues, redeploy }),
    focusFirstError,
  )

  return (
    <FormProvider {...form}>
      <form
        ref={formRef}
        noValidate
        aria-busy={save.isPending}
        className="grid min-w-0 gap-6"
        onSubmit={(event) => {
          // 弹窗通过 Portal 渲染，但 React 事件仍沿组件树冒泡；只处理本表单自身的提交。
          if (event.target !== event.currentTarget)
            return
          void submit(false)(event)
        }}
      >
        <div className="grid min-w-0 gap-6 lg:grid-cols-[15rem_minmax(0,1fr)]">
          <aside className="min-w-0 lg:sticky lg:top-0 lg:self-start">
            <DeploymentTargetEditorNav repositoryBindings={repositoryBindings} runtimeClusters={runtimeClusters} />
          </aside>
          <div className="grid min-w-0 gap-6">
            <BasicsSection editing={Boolean(target)} projectId={projectId} runtimeClusters={runtimeClusters} />
            <SourceBuildSection
              applicationId={applicationId}
              buildEnvironmentAvailable={buildEnvironmentAvailable}
              editing={Boolean(target)}
              projectId={projectId}
              registries={registries}
              repositoryBindings={repositoryBindings}
            />
            <RuntimeSection
              applicationId={applicationId}
              canManageRuntimeSecrets={canManageRuntimeSecrets}
              deploymentTargets={deploymentTargets}
              projectId={projectId}
              releases={releases}
              runtimeClusters={runtimeClusters}
              target={target}
            />
            <ReleaseSection />
            <HooksSection projectId={projectId} />
            <SecuritySection />
          </div>
        </div>
        <div className="sticky bottom-0 z-10 flex flex-col gap-3 rounded-container border border-border bg-surface-raised px-4 py-3 shadow-raised sm:flex-row sm:items-center sm:justify-between">
          <div aria-live="polite" className="min-w-0 text-sm empty:hidden">
            {isSubmitted && totalErrors > 0 && (
              <button className="text-left text-danger underline-offset-4 hover:underline focus-visible:underline" type="button" onClick={() => focusFirstError(errors)}>
                {t('deploymentTargetEditor.fixErrors', { count: totalErrors })}
              </button>
            )}
            {!(isSubmitted && totalErrors > 0) && needsRedeploy && (
              <p className="text-warning">
                {t(canRedeploy ? 'deploymentsPage.runtimeChangesNeedRedeployInline' : 'deploymentsPage.runtimeChangesNeedRedeployUnavailableInline')}
              </p>
            )}
          </div>
          <div className="flex shrink-0 flex-wrap justify-end gap-2">
            <Button asChild type="button" variant="ghost">
              <Link to={backTo}>{t('common.cancel')}</Link>
            </Button>
            {needsRedeploy && (
              <Button disabled={save.isPending} type="submit" variant="outline">
                <Save className="size-4" />
                {t('deploymentsPage.saveOnly')}
              </Button>
            )}
            {needsRedeploy
              ? (
                  <Button disabled={save.isPending || !canRedeploy} type="button" onClick={() => void submit(true)()}>
                    <Rocket className="size-4" />
                    {t('deploymentsPage.saveAndRedeploy')}
                  </Button>
                )
              : (
                  <Button disabled={save.isPending} type="submit">
                    <Save className="size-4" />
                    {target ? t('common.save') : t('deploymentTargetEditor.create')}
                  </Button>
                )}
          </div>
        </div>
      </form>
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
