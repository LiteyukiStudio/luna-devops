import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AIToolApprovalExemptionsDialog } from './tool-approval-exemptions'

const { listAIToolApprovalExemptions, revokeAIToolApprovalExemption } = vi.hoisted(() => ({
  listAIToolApprovalExemptions: vi.fn(),
  revokeAIToolApprovalExemption: vi.fn(),
}))

vi.mock('@/api', () => ({
  api: {
    listAIToolApprovalExemptions,
    revokeAIToolApprovalExemption,
  },
}))

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <AIToolApprovalExemptionsDialog />
    </QueryClientProvider>,
  )
}

describe('ai tool approval exemptions', () => {
  beforeEach(() => {
    listAIToolApprovalExemptions.mockReset()
    revokeAIToolApprovalExemption.mockReset()
  })

  it('loads the account rules and revokes one by operation id', async () => {
    await i18next.changeLanguage('zh-CN')
    listAIToolApprovalExemptions.mockResolvedValue({
      items: [{ operationId: 'restartDeploymentTarget', createdAt: '2026-08-20T10:00:00Z' }],
    })
    revokeAIToolApprovalExemption.mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: '管理始终允许的工具' }))
    expect(await screen.findByText('restartDeploymentTarget')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '撤销 restartDeploymentTarget 的始终允许规则' }))
    await waitFor(() => expect(revokeAIToolApprovalExemption.mock.calls[0]?.[0]).toBe('restartDeploymentTarget'))
  })
})
