import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { SiteSettingsPage } from './SiteSettingsPage'

const mocks = vi.hoisted(() => ({
  cleanupDataRetention: vi.fn(),
  getConfigs: vi.fn(),
  getDataRetentionCatalog: vi.fn(),
  listConfigDefinitions: vi.fn(),
  previewDataRetention: vi.fn(),
  updateConfigs: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      cleanupDataRetention: mocks.cleanupDataRetention,
      getConfigs: mocks.getConfigs,
      getDataRetentionCatalog: mocks.getDataRetentionCatalog,
      listConfigDefinitions: mocks.listConfigDefinitions,
      previewDataRetention: mocks.previewDataRetention,
      updateConfigs: mocks.updateConfigs,
    },
  }
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <SiteSettingsPage />
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

describe('site settings page', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    window.history.replaceState(null, '', '/')
    await i18next.changeLanguage('en-US')
    mocks.listConfigDefinitions.mockResolvedValue([{
      default: '30',
      key: 'retention.buildLogsDays',
      public: false,
      type: 'number',
    }])
    mocks.getConfigs.mockResolvedValue({ 'retention.buildLogsDays': '30' })
    mocks.getDataRetentionCatalog.mockResolvedValue({
      items: [{ configKey: 'retention.buildLogsDays', defaultDays: 30, key: 'build_logs' }],
    })
    mocks.previewDataRetention.mockResolvedValue({
      items: [{ dataset: 'build_logs', deleted: 0, matched: 12 }],
    })
    mocks.cleanupDataRetention.mockResolvedValue({
      items: [{ dataset: 'build_logs', deleted: 12, matched: 12 }],
    })
  })

  it('invalidates cleanup authorization when the preview inputs change', async () => {
    const user = userEvent.setup()
    window.history.replaceState(null, '', '/#tab=retention')
    renderPage()

    const cleanupButton = await screen.findByRole('button', { name: i18next.t('settings.retentionCleanup') })
    expect(cleanupButton).toBeDisabled()

    await user.click(screen.getByRole('button', { name: i18next.t('settings.retentionDatasets') }))
    await user.click(await screen.findByRole('button', { name: i18next.t('settings.retentionDatasetLabels.build_logs') }))
    await user.keyboard('{Escape}')
    fireEvent.change(screen.getByLabelText(i18next.t('settings.retentionStartAt')), { target: { value: '2026-07-01T08:30' } })
    fireEvent.change(screen.getByLabelText(i18next.t('settings.retentionEndAt')), { target: { value: '2026-07-02T09:45' } })

    await user.click(screen.getByRole('button', { name: i18next.t('settings.retentionPreview') }))
    await waitFor(() => expect(cleanupButton).toBeEnabled())
    expect(mocks.previewDataRetention).toHaveBeenCalledWith({
      datasets: ['build_logs'],
      startAt: new Date('2026-07-01T08:30').toISOString(),
      endAt: new Date('2026-07-02T09:45').toISOString(),
    })

    await user.click(cleanupButton)
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('12')).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: i18next.t('common.cancel') }))

    fireEvent.change(screen.getByLabelText(i18next.t('settings.retentionEndAt')), { target: { value: '2026-07-03T09:45' } })
    expect(cleanupButton).toBeDisabled()
    expect(screen.queryByText(i18next.t('settings.retentionPreviewResults'))).not.toBeInTheDocument()
  })

  it('places the default brand color before the remaining brand settings', async () => {
    mocks.listConfigDefinitions.mockResolvedValue([
      {
        default: 'Luna DevOps',
        key: 'site.title',
        public: true,
        type: 'string',
      },
      {
        default: 'blue',
        key: 'site.brandColorPreset',
        options: ['red', 'blue'],
        public: true,
        type: 'select',
      },
    ])
    mocks.getConfigs.mockResolvedValue({
      'site.brandColorPreset': 'blue',
      'site.title': 'Luna DevOps',
    })

    renderPage()

    const brandColorGroup = await screen.findByRole('radiogroup', { name: i18next.t('settings.configDefinitions.site.brandColorPreset.label') })
    const siteTitleInput = screen.getByRole('textbox')
    expect(brandColorGroup.compareDocumentPosition(siteTitleInput) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('keeps the AI settings form outside other forms', async () => {
    const user = userEvent.setup()
    const { container } = renderPage()
    await user.click(await screen.findByRole('tab', { name: i18next.t('settings.ai.tab') }))
    expect(container.querySelectorAll('form')).toHaveLength(1)
    expect(container.querySelector('form form')).toBeNull()
  })
})
