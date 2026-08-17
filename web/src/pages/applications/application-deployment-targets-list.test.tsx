import type { DeploymentTargetRow } from './application-deployment-targets-list'
import type { DeploymentTarget, Release } from '@/api'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ApplicationDeploymentTargetsList } from './application-deployment-targets-list'
import '@/i18n'

const deploymentTarget = {
  applicationId: 'app_1',
  autoDeploy: false,
  availableReplicas: 1,
  clusterId: 'cluster_1',
  cpuRequest: '100m',
  dataVolumes: [],
  deleteStatus: 'active',
  desiredReplicas: 1,
  enabled: false,
  environmentId: 'env_1',
  id: 'target_1',
  imageRef: 'registry.example/app:v1',
  memoryRequest: '128Mi',
  name: 'app-dev',
  projectId: 'prj_1',
  readyReplicas: 1,
  replicas: 1,
  sourceType: 'image',
  stage: 'dev',
} as unknown as DeploymentTarget

const release = {
  applicationId: 'app_1',
  buildRunId: '',
  createdAt: '2026-08-17T00:00:00Z',
  createdBy: 'user_1',
  deploymentTargetId: 'target_1',
  environmentId: 'env_1',
  forceImagePull: false,
  id: 'release_1',
  imageRef: 'registry.example/app:v1',
  message: 'Deployment has minimum availability.',
  projectId: 'prj_1',
  revision: 3,
  rollbackFromId: '',
  status: 'succeeded',
  type: 'deploy',
} satisfies Release

function renderList(items: DeploymentTargetRow[] = []) {
  return render(
    <TooltipProvider>
      <ApplicationDeploymentTargetsList
        applicationId="app_1"
        createReleasePending={false}
        deletePending={false}
        deployableBuildRuns={[]}
        items={items}
        projectId="prj_1"
        pullLatestPending={false}
        restartPending={false}
        rollbackPending={false}
        onCopy={vi.fn()}
        onDeleteTarget={vi.fn()}
        onOpenConsole={vi.fn()}
        onOpenReleaseDialog={vi.fn()}
        onOpenTargetDialog={vi.fn()}
        onPullLatestImageDeploy={vi.fn()}
        onRestart={vi.fn()}
        onRollback={vi.fn()}
        onViewLogs={vi.fn()}
      />
    </TooltipProvider>,
  )
}

describe('application deployment targets layout', () => {
  it('contains intrinsic table width and switches views by content width', () => {
    const { container } = renderList()

    expect(container.querySelector('[data-slot="deployment-targets-list"]')).toHaveClass(
      'w-full',
      'min-w-0',
      'max-w-full',
      '@container/deployment-targets',
    )
    expect(container.querySelector('[data-slot="deployment-targets-table"]')).toHaveClass(
      'hidden',
      'min-w-0',
      'max-w-full',
      '@[68rem]/deployment-targets:block',
    )
    expect(container.querySelector('[data-slot="deployment-targets-cards"]')).toHaveClass(
      'min-w-0',
      'max-w-full',
      '@[68rem]/deployment-targets:hidden',
    )
  })

  it('keeps successful deployment details focused on operational information', () => {
    const { container } = renderList([{
      internalEndpoint: {
        fqdn: 'app-dev.luna-dev.svc.cluster.local',
        namespace: 'luna-dev',
        serviceName: 'app-dev',
      },
      release,
      runtimeStatus: { podCount: 1, summary: '', value: 'ready' },
      target: deploymentTarget,
      webConsoleEnabled: false,
    }])

    expect(screen.queryByText(/自动部署|Auto deploy/i)).not.toBeInTheDocument()
    expect(screen.queryByText('Deployment has minimum availability.')).not.toBeInTheDocument()
    expect(container.querySelectorAll('details dl')).toHaveLength(2)
    expect(screen.getAllByText('#3')).toHaveLength(2)
  })

  it('keeps actionable rollout diagnostics in deployment details', () => {
    renderList([{
      release: { ...release, message: 'ImagePullBackOff', status: 'failed' },
      runtimeStatus: { podCount: 1, summary: 'ImagePullBackOff', value: 'image-pull-back-off' },
      target: deploymentTarget,
      webConsoleEnabled: false,
    }])

    expect(screen.getAllByText('ImagePullBackOff').length).toBeGreaterThan(0)
  })
})
