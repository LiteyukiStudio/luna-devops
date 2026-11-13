import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import i18next from '@/i18n'
import { ApplicationTopologyPanel } from './application-topology-panel'
import { toneForResourceStatus } from './application-topology-utils'

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return { ...actual, api: { ...actual.api, getApplicationTopology: vi.fn(), getProjectTopology: vi.fn() } }
})

function renderPanel() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <ApplicationTopologyPanel applicationId="app_1" projectId="prj_1" />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const resource = { namespace: 'luna-app', summary: '', clusterId: 'cls_1', clusterName: 'main', deploymentTargetId: 'dplt_1' }

describe('application topology panel', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
    vi.mocked(api.getProjectTopology).mockResolvedValue({ nodes: [], edges: [], warnings: [], generatedAt: '2026-11-06T00:00:00Z' } as unknown as Awaited<ReturnType<typeof api.getProjectTopology>>)
    vi.mocked(api.getApplicationTopology).mockResolvedValue({
      generatedAt: '2026-11-06T00:00:00Z',
      targets: [{ id: 'dplt_1', name: 'web', stage: 'production' }],
      nodes: [
        { ...resource, id: 'svc', kind: 'Service', name: 'web-svc', status: 'ready' },
        { ...resource, id: 'deploy', kind: 'Deployment', name: 'web', status: 'progressing', summary: '1/2 ready' },
        { ...resource, id: 'cm', kind: 'ConfigMap', name: 'web-config', status: '' },
      ],
      edges: [
        { id: 'svc->deploy', source: 'svc', target: 'deploy', type: 'selects' },
        { id: 'cm->deploy', source: 'cm', target: 'deploy', type: 'configures' },
      ],
      warnings: [],
    } as unknown as Awaited<ReturnType<typeof api.getApplicationTopology>>)
  })

  it('renders resources on the shared topology graph and opens resource details', async () => {
    renderPanel()

    const deployment = await screen.findByLabelText('web, Deployment, status Progressing')
    expect(screen.getByLabelText('web-svc, Service, status Ready')).toBeInTheDocument()
    // React Flow 在 jsdom 中无法测量节点尺寸，节点保持 visibility:hidden，故按可访问名称而非 role 查询
    expect(deployment.tagName).toBe('BUTTON')
    // 依赖资源默认隐藏
    expect(screen.queryByLabelText(/web-config/)).not.toBeInTheDocument()
    expect(screen.getByLabelText('Legend')).toHaveTextContent('Traffic and ownership (solid)')

    await userEvent.click(deployment)
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('1/2 ready')
    expect(dialog).toHaveTextContent('luna-app')
  })

  it('shows dependency resources when toggled', async () => {
    renderPanel()

    await userEvent.click(await screen.findByRole('button', { name: 'Dependencies' }))
    expect(await screen.findByLabelText('web-config, ConfigMap, status Unknown')).toBeInTheDocument()
  })

  it('classifies degraded, OOM, and not-ready topology statuses without false green tones', () => {
    expect(toneForResourceStatus('not-ready')).toBe('warning')
    expect(toneForResourceStatus('terminating')).toBe('warning')
    expect(toneForResourceStatus('oom-killed')).toBe('danger')
    expect(toneForResourceStatus('crash-loop-back-off')).toBe('danger')
    expect(toneForResourceStatus('degraded')).toBe('danger')
    expect(toneForResourceStatus('unschedulable')).toBe('danger')
    expect(toneForResourceStatus('ready')).toBe('success')
  })
})
