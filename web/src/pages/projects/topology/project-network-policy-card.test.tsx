import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ProjectNetworkPolicyCard } from './project-network-policy-card'

const mocks = vi.hoisted(() => ({ getProjectNetworkPolicy: vi.fn(), updateProjectNetworkPolicy: vi.fn() }))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return { ...actual, api: { ...actual.api, getProjectNetworkPolicy: mocks.getProjectNetworkPolicy, updateProjectNetworkPolicy: mocks.updateProjectNetworkPolicy } }
})

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

function renderCard(canManage: boolean) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><TooltipProvider><ProjectNetworkPolicyCard canManage={canManage} projectId="prj_demo" /></TooltipProvider></QueryClientProvider>)
}

describe('project network policy card', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.getProjectNetworkPolicy.mockResolvedValue({ ingressMode: 'isolated', egressMode: 'internet', egressAllowlist: '' })
  })

  it('saves stricter modes with the allowlist', async () => {
    mocks.updateProjectNetworkPolicy.mockImplementation((_projectId: string, payload: unknown) => Promise.resolve(payload))
    renderCard(true)
    const user = userEvent.setup()
    await user.selectOptions(await screen.findByLabelText('Ingress'), 'dependencies')
    await user.selectOptions(screen.getByLabelText('Egress'), 'allowlist')
    await user.type(screen.getByLabelText('Egress allowlist'), '1.1.1.1/32 443')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(mocks.updateProjectNetworkPolicy).toHaveBeenCalledWith('prj_demo', {
      ingressMode: 'dependencies',
      egressMode: 'allowlist',
      egressAllowlist: '1.1.1.1/32 443',
    }))
  })

  it('rejects malformed or IPv4-mapped allowlist entries before saving', async () => {
    renderCard(true)
    const user = userEvent.setup()
    await user.selectOptions(await screen.findByLabelText('Egress'), 'allowlist')
    await user.type(screen.getByLabelText('Egress allowlist'), '::ffff:0.0.0.0/96')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText(/egress allowlist is invalid/i)).toBeInTheDocument()
    expect(mocks.updateProjectNetworkPolicy).not.toHaveBeenCalled()
  })

  it('is read-only without project management permission', async () => {
    renderCard(false)
    expect(await screen.findByLabelText('Ingress')).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
  })
})
