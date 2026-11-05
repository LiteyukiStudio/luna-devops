import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ProjectsPage } from './ProjectsPage'

const mocks = vi.hoisted(() => ({
  listProjectsPage: vi.fn(),
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
      listProjectsPage: mocks.listProjectsPage,
    },
  }
})

describe('projects page', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.listProjectsPage.mockResolvedValue({ items: [], page: 1, pageSize: 10, sortBy: 'lastUsed', sortOrder: 'desc', total: 0, totalPages: 0 })
  })

  it('opens the create dialog from the create query parameter and removes it on close', async () => {
    const user = userEvent.setup()
    renderPage(['/projects?create=1'])

    expect(await screen.findByRole('dialog', { name: 'Create project space' })).toBeInTheDocument()

    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog', { name: 'Create project space' })).not.toBeInTheDocument()
    expect(screen.getByTestId('location')).toHaveTextContent(/^\/projects$/)
  })
})

function LocationProbe() {
  const location = useLocation()
  return <div data-testid="location">{`${location.pathname}${location.search}`}</div>
}

function renderPage(initialEntries: string[]) {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <MemoryRouter initialEntries={initialEntries}>
          <Routes>
            <Route
              element={(
                <>
                  <ProjectsPage />
                  <LocationProbe />
                </>
              )}
              path="/projects"
            />
          </Routes>
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  )
}
