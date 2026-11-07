import type { PlatformEvent } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { EventsPage } from './EventsPage'

const mocks = vi.hoisted(() => ({
  getPlatformEvent: vi.fn(),
  listPlatformEventCatalog: vi.fn(),
  listPlatformEvents: vi.fn(),
  listProjects: vi.fn(),
}))

vi.mock('@/app/session-context', () => ({
  useSession: () => ({ user: { id: 'usr_1', permissions: [], role: 'user' } }),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      ...mocks,
    },
  }
})

function platformEvent(overrides: Partial<PlatformEvent>): PlatformEvent {
  return {
    id: 'evt_1',
    type: 'build.webhook_rejected',
    category: 'build',
    severity: 'warning',
    status: 'failed',
    projectId: 'prj_1',
    applicationId: 'app_1',
    deploymentTargetId: 'dplt_1',
    resourceType: 'repository_binding',
    resourceId: 'rpb_651266c2a50996ccb2607d54',
    resourceName: 'LiteyukiStudio/neo-blog',
    actorId: 'usr_1',
    resourceOwnerUserId: '',
    summaryKey: 'events.types.build_webhook_rejected',
    message: 'build.webhook_state_unavailable',
    correlationId: '',
    traceId: '',
    occurredAt: '2026-11-07T08:00:00Z',
    createdAt: '2026-11-07T08:00:00Z',
    detail: {},
    links: {},
    deliveryCount: 0,
    ...overrides,
  }
}

const namedEvent = platformEvent({})
const deletedResourceEvent = platformEvent({
  id: 'evt_2',
  type: 'build.failed',
  resourceType: 'build',
  resourceId: 'bldr_deleted',
  resourceName: '',
  detail: {
    application: { id: 'app_1', name: 'Blog frontend', identifier: 'blog-web' },
    deploymentTarget: { id: 'dplt_1', name: 'prod', identifier: 'prod' },
  },
})
const unknownTypeEvent = platformEvent({
  id: 'evt_3',
  resourceType: 'future_resource',
  resourceId: 'fut_1',
  resourceName: '',
})

describe('events page resources', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.listProjects.mockResolvedValue([])
    mocks.listPlatformEventCatalog.mockResolvedValue([])
    mocks.listPlatformEvents.mockResolvedValue({
      items: [namedEvent, deletedResourceEvent, unknownTypeEvent],
      page: 1,
      pageSize: 20,
      sortBy: 'occurredAt',
      sortOrder: 'desc',
      total: 3,
      totalPages: 1,
    })
    mocks.getPlatformEvent.mockResolvedValue(namedEvent)
  })

  it('shows the localized resource type with the server-resolved name and falls back to the raw id', async () => {
    renderPage()

    expect(await screen.findByText('LiteyukiStudio/neo-blog')).toBeInTheDocument()
    expect(screen.getAllByText('Repository binding')[0]).toBeInTheDocument()
    expect(screen.queryByText('rpb_651266c2a50996ccb2607d54')).not.toBeInTheDocument()

    expect(screen.getByText('bldr_deleted')).toBeInTheDocument()
    expect(screen.getByText('Build run · Blog frontend / prod')).toBeInTheDocument()

    expect(screen.getByText('fut_1')).toBeInTheDocument()
    expect(screen.getByText('future_resource')).toBeInTheDocument()
  })

  it('shows the resource name in the detail sheet and keeps the raw id under identifiers', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(await screen.findByText('LiteyukiStudio/neo-blog'))

    const sheet = await screen.findByRole('dialog')
    expect(await within(sheet).findByText('Repository binding · LiteyukiStudio/neo-blog')).toBeInTheDocument()
    expect(within(sheet).getByText('repository_binding / rpb_651266c2a50996ccb2607d54')).toBeInTheDocument()
    expect(mocks.getPlatformEvent).toHaveBeenCalledWith('evt_1')
  })
})

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <MemoryRouter initialEntries={['/events']}>
          <EventsPage />
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  )
}
