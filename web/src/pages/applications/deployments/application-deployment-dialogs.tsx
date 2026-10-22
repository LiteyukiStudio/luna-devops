import type { ComponentProps } from 'react'
import { lazy } from 'react'
import { LazyDialogBoundary } from '@/components/common/lazy-dialog-boundary'

const CreateReleaseDialog = lazy(() =>
  import('./application-create-release-dialog').then(module => ({ default: module.ApplicationCreateReleaseDialog })),
)
const DeploymentBundleImportDialog = lazy(() =>
  import('./application-deployment-bundle-import-dialog').then(module => ({ default: module.ApplicationDeploymentBundleImportDialog })),
)
const RepositoryBindingDialog = lazy(() =>
  import('@/pages/applications/deployments/editor/source/application-repository-binding-dialog').then(module => ({ default: module.ApplicationRepositoryBindingDialog })),
)
const RuntimeConfigSetDialog = lazy(() =>
  import('@/components/common/runtime-config-set-dialog').then(module => ({ default: module.RuntimeConfigSetDialog })),
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
