import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { AIAssistantSettingsPanel } from './ai-assistant-settings-panel'

const mocks = vi.hoisted(() => ({
  getConfigs: vi.fn(),
  listConfigDefinitions: vi.fn(),
  updateConfigs: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getConfigs: mocks.getConfigs,
      listConfigDefinitions: mocks.listConfigDefinitions,
      updateConfigs: mocks.updateConfigs,
    },
  }
})

vi.mock('./ai-model-management', () => ({ AIModelManagement: () => null }))

const runtimeDefaults = {
  'ai.provider.compatibility': 'auto',
  'ai.provider.prompt_cache_key_mode': 'auto',
  'ai.runtime.provider_timeout_seconds': '300',
  'ai.runtime.max_request_retries': '5',
  'ai.runtime.run_timeout_seconds': '3600',
  'ai.runtime.agent_concurrent_runs': '10',
}

function renderPanel() {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <AIAssistantSettingsPanel />
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

describe('aI assistant channel affinity setting', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('zh-CN')
    mocks.getConfigs.mockResolvedValue({ ...runtimeDefaults })
    mocks.listConfigDefinitions.mockResolvedValue(Object.entries(runtimeDefaults).map(([key, value]) => ({
      default: value,
      key,
      public: false,
      type: key.startsWith('ai.provider.') ? 'select' : 'number',
    })))
    mocks.updateConfigs.mockImplementation(async values => values)
  })

  it('defaults on, exposes its tip, and can be disabled', async () => {
    const user = userEvent.setup()
    renderPanel()

    const affinitySwitch = await screen.findByRole('switch', { name: i18next.t('settings.ai.channelAffinity') })
    expect(affinitySwitch).toBeChecked()
    await waitFor(() => expect(screen.getByRole('button', { name: i18next.t('settings.restoreDefaults') })).toBeEnabled())

    const help = screen.getByRole('button', { name: i18next.t('settings.ai.channelAffinityHelp') })
    await user.hover(help)
    expect(await screen.findByRole('tooltip')).toHaveTextContent(i18next.t('settings.ai.channelAffinityTip'))

    await user.click(affinitySwitch)
    expect(affinitySwitch).not.toBeChecked()
  })

  it('defaults capability-sensitive Provider policies to automatic mode', async () => {
    renderPanel()

    expect(await screen.findByRole('combobox', { name: i18next.t('settings.ai.providerCompatibility') })).toHaveValue('auto')
    expect(screen.getByRole('combobox', { name: i18next.t('settings.ai.promptCacheKeyMode') })).toHaveValue('auto')
  })
})
