import type { ClusterResource, RuntimeCluster } from '@/api'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ClusterResourceWebConsoleDialog } from './cluster-resource-web-console-dialog'

vi.mock('@/components/common/runtime-terminal-panel', () => ({
  RuntimeTerminalPanel: ({ socketUrl }: { socketUrl?: string }) => <output data-testid="terminal-socket-url">{socketUrl}</output>,
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

describe('cluster pod web console dialog', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('builds the pod terminal WebSocket URL and tracks the selected container', async () => {
    render(
      <TooltipProvider>
        <ClusterResourceWebConsoleDialog cluster={cluster} pod={pod} onOpenChange={vi.fn()} />
      </TooltipProvider>,
    )

    const socketUrl = await screen.findByTestId('terminal-socket-url')
    expect(socketUrl).toHaveTextContent('/runtime/clusters/clu_123/pods/terminal?namespace=luna-project&name=api-7f6cb48d8f-j9km2')

    fireEvent.change(screen.getByRole('textbox', { name: 'Container' }), { target: { value: 'sidecar' } })
    expect(await screen.findByTestId('terminal-socket-url')).toHaveTextContent('/runtime/clusters/clu_123/pods/terminal?namespace=luna-project&name=api-7f6cb48d8f-j9km2&container=sidecar')
  })
})
