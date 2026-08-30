import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { BillingPriceList } from './billing-price-list'

const mocks = vi.hoisted(() => ({
  listBillingRateRules: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listBillingRateRules: mocks.listBillingRateRules,
    },
  }
})

beforeAll(async () => {
  await i18next.changeLanguage('zh-CN')
})

describe('billing price list', () => {
  it('shows all configured prices including disabled meters', async () => {
    mocks.listBillingRateRules.mockResolvedValue([
      {
        id: 'brte_build_cpu',
        meter: 'build.cpu_vcpu_minute',
        unit: 'vcpu_minute',
        creditsPerUnit: '10',
        enabled: true,
        description: 'Build CPU usage',
        createdAt: '2026-08-14T00:00:00Z',
        updatedAt: '2026-08-14T00:00:00Z',
      },
      {
        id: 'brte_gateway_requests',
        meter: 'gateway.requests_1000',
        unit: '1000_requests',
        creditsPerUnit: '0',
        enabled: false,
        description: 'Gateway request count',
        createdAt: '2026-08-14T00:00:00Z',
        updatedAt: '2026-08-14T00:00:00Z',
      },
      {
        id: 'brte_ai_input',
        meter: 'ai.input_tokens_1m',
        unit: 'million_tokens',
        creditsPerUnit: '3',
        enabled: true,
        description: 'AI input tokens',
        createdAt: '2026-08-14T00:00:00Z',
        updatedAt: '2026-08-14T00:00:00Z',
      },
    ])
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

    render(
      <QueryClientProvider client={queryClient}>
        <BillingPriceList
          billingDisplay={{
            formatAmountWithUnit: value => `${value} Credits`,
            formatFiatAmount: () => '',
          }}
        />
      </QueryClientProvider>,
    )

    expect(await screen.findByText('构建 CPU')).toBeInTheDocument()
    expect(screen.getByText('10 Credits')).toBeInTheDocument()
    expect(screen.getByText('核分')).toBeInTheDocument()
    expect(screen.getByText('访问请求')).toBeInTheDocument()
    expect(screen.getByText('已禁用')).toBeInTheDocument()
    expect(screen.queryByText('AI input tokens')).not.toBeInTheDocument()
  })
})
