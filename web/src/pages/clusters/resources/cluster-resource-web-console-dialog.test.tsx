import type { ClusterResource, RuntimeCluster } from '@/api'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ClusterResourceWebConsoleDialog } from './cluster-resource-web-console-dialog'

vi.mock('@/components/common/runtime-terminal-panel', () => ({
  RuntimeTerminalPanel: () => <div>Terminal</div>,
}))

const cluster = { id: 'clu_123' } as RuntimeCluster
const pod: ClusterResource = {
  id: 'pod_123',
  kind: 'Pod',
  name: 'api-7f6cb48d8f-j9km2',
  namespace: 'luna-project',
  status: 'Running',
  summary: '',
  projectId: 'prj_123',
  applicationId: 'app_456',
  deploymentTargetId: 'dplt_789',
  releaseId: 'rel_123',
  routeId: '',
  projectName: 'Project',
  applicationName: 'Application',
  deploymentTargetName: 'Production',
  labels: {},
  createdAt: '2026-09-06T00:00:00Z',
  updatedAt: '2026-09-06T00:00:00Z',
}

describe('cluster pod web console CLI command', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('uses the existing pod-terminal command and tracks the selected container', () => {
    render(
      <TooltipProvider>
        <ClusterResourceWebConsoleDialog cluster={cluster} pod={pod} onOpenChange={vi.fn()} />
      </TooltipProvider>,
    )

    const command = screen.getByRole('group', { name: 'CLI command' })
    expect(command).toHaveTextContent(`luna cluster pod-terminal server=${window.location.origin} clusterId=clu_123 namespace=luna-project name=api-7f6cb48d8f-j9km2`)

    fireEvent.change(screen.getByRole('textbox', { name: 'Container' }), { target: { value: 'sidecar' } })
    expect(command).toHaveTextContent(`luna cluster pod-terminal server=${window.location.origin} clusterId=clu_123 namespace=luna-project name=api-7f6cb48d8f-j9km2 container=sidecar`)
  })
})
