import type { ComponentProps } from 'react'
import { lazy } from 'react'
import { LazyDialogBoundary } from '@/components/common/lazy-dialog-boundary'

const CreateReleaseDialog = lazy(() =>
  import('./application-create-release-dialog').then(module => ({ default: module.ApplicationCreateReleaseDialog })),
)
const DeploymentTargetDialog = lazy(() =>
  import('@/pages/applications/deployments/editor/application-deployment-target-dialog').then(module => ({ default: module.ApplicationDeploymentTargetDialog })),
)
const DeploymentBundleImportDialog = lazy(() =>
  import('./application-deployment-bundle-import-dialog').then(module => ({ default: module.ApplicationDeploymentBundleImportDialog })),
)
const ReleaseLogsDialog = lazy(() =>
  import('./application-release-logs-dialog').then(module => ({ default: module.ApplicationReleaseLogsDialog })),
)
const RepositoryBindingDialog = lazy(() =>
  import('@/pages/applications/deployments/editor/source/application-repository-binding-dialog').then(module => ({ default: module.ApplicationRepositoryBindingDialog })),
)
const RuntimeConfigSetDialog = lazy(() =>
  import('@/components/common/runtime-config-set-dialog').then(module => ({ default: module.RuntimeConfigSetDialog })),
)
const WebConsoleDialog = lazy(() =>
  import('@/pages/applications/runtime/application-web-console-dialog').then(module => ({ default: module.ApplicationWebConsoleDialog })),
)

export function DeferredCreateReleaseDialog(props: ComponentProps<typeof CreateReleaseDialog>) {
  if (!props.open)
    return null
  return (
    <LazyDialogBoundary resetKey={`release-${props.projectId}-${props.applicationId}`} onOpenChange={props.onOpenChange}>
      <CreateReleaseDialog {...props} />
    </LazyDialogBoundary>
  )
}

export function DeferredDeploymentTargetDialog(props: ComponentProps<typeof DeploymentTargetDialog>) {
  if (!props.open)
    return null
  return (
    <LazyDialogBoundary resetKey={`target-${props.editingTarget?.id ?? 'new'}`} onOpenChange={props.onOpenChange}>
      <DeploymentTargetDialog {...props} />
    </LazyDialogBoundary>
  )
}

export function DeferredDeploymentBundleImportDialog(props: ComponentProps<typeof DeploymentBundleImportDialog>) {
  if (!props.open)
    return null
  return (
    <LazyDialogBoundary resetKey={`deployment-bundle-${props.projectId}-${props.applicationId}`} onOpenChange={props.onOpenChange}>
      <DeploymentBundleImportDialog {...props} />
    </LazyDialogBoundary>
  )
}

export function DeferredRepositoryBindingDialog(props: ComponentProps<typeof RepositoryBindingDialog>) {
  if (!props.open)
    return null
  return (
    <LazyDialogBoundary resetKey="repository-binding" onOpenChange={props.onOpenChange}>
      <RepositoryBindingDialog {...props} />
    </LazyDialogBoundary>
  )
}

export function DeferredRuntimeConfigSetDialog(props: ComponentProps<typeof RuntimeConfigSetDialog>) {
  if (!props.open)
    return null
  return (
    <LazyDialogBoundary resetKey={`runtime-config-${props.editingSet?.id ?? 'new'}`} onOpenChange={props.onOpenChange}>
      <RuntimeConfigSetDialog {...props} />
    </LazyDialogBoundary>
  )
}

export function DeferredReleaseLogsDialog(props: ComponentProps<typeof ReleaseLogsDialog>) {
  if (!props.release)
    return null
  return (
    <LazyDialogBoundary resetKey={`release-logs-${props.release.id}`} onOpenChange={props.onOpenChange}>
      <ReleaseLogsDialog {...props} />
    </LazyDialogBoundary>
  )
}

export function DeferredWebConsoleDialog(props: ComponentProps<typeof WebConsoleDialog>) {
  if (!props.release)
    return null
  return (
    <LazyDialogBoundary resetKey={`web-console-${props.release.id}`} onOpenChange={props.onOpenChange}>
      <WebConsoleDialog {...props} />
    </LazyDialogBoundary>
  )
}
