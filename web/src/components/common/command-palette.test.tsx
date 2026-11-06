import type { ComponentProps } from 'react'
import type { Application, CurrentUser, Project } from '@/api'
import type { SessionContextValue } from '@/app/session-context'
import type { ThemeContextValue } from '@/app/theme-context'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Boxes, LayoutDashboard } from 'lucide-react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { SessionContext } from '@/app/session-context'
import { ThemeContext } from '@/app/theme-context'
import i18next from '@/i18n'
import { CommandPalette } from './command-palette'

const mocks = vi.hoisted(() => ({
  listApplications: vi.fn(),
  listProjects: vi.fn(),
  setMode: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listApplications: mocks.listApplications,
      listProjects: mocks.listProjects,
    },
  }
})

describe('command palette', () => {
  beforeAll(() => {
    // cmdk 会把选中项滚动到可见区域，jsdom 未实现该方法。
    Element.prototype.scrollIntoView = vi.fn()
  })

  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.listProjects.mockResolvedValue([projectFixture()])
    mocks.listApplications.mockResolvedValue([applicationFixture()])
  })

  it('opens from the shortcut, closes with Escape, and ignores the shortcut inside editors', async () => {
    const user = userEvent.setup()
    renderPalette()

    fireEvent.keyDown(document.body, { key: 'k', metaKey: true })
    const dialog = await screen.findByRole('dialog', { name: 'Command palette' })
    expect(within(dialog).getByRole('combobox', { name: 'Command palette' })).toHaveFocus()

    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    const editor = document.createElement('div')
    editor.className = 'cm-editor'
    const content = document.createElement('div')
    content.tabIndex = 0
    editor.append(content)
    document.body.append(editor)
    fireEvent.keyDown(content, { ctrlKey: true, key: 'k' })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    editor.remove()
  })

  it('filters commands and navigates to the selected page', async () => {
    const user = userEvent.setup()
    renderPalette()

    await user.click(screen.getByRole('button', { name: 'Search…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Command palette' })
    expect(within(dialog).queryByRole('option', { name: 'Global Settings' })).not.toBeInTheDocument()

    await user.type(within(dialog).getByRole('combobox'), 'regis')
    expect(within(dialog).getByRole('option', { name: 'Registries' })).toBeInTheDocument()
    expect(within(dialog).queryByRole('option', { name: 'Dashboard' })).not.toBeInTheDocument()

    await user.keyboard('{Enter}')
    expect(await screen.findByTestId('location')).toHaveTextContent('/registries')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('lists project spaces and applications of the current project space', async () => {
    const user = userEvent.setup()
    renderPalette(['/projects/prj_1'], { permissions: ['user.manage'] })

    expect(mocks.listProjects).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Search…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Command palette' })
    expect(await within(dialog).findByRole('group', { name: 'Applications in Blog' })).toBeInTheDocument()
    expect(within(dialog).getByRole('option', { name: 'Global Settings' })).toBeInTheDocument()
    expect(mocks.listApplications).toHaveBeenCalledWith('prj_1')

    await user.click(within(dialog).getByRole('option', { name: /^Web/ }))
    expect(await screen.findByTestId('location')).toHaveTextContent('/projects/prj_1/apps/app_web')
  })

  it('lists the sections of the current project space supplied by the layout', async () => {
    const user = userEvent.setup()
    renderPalette(['/projects/prj_1/apps/app_web'], {}, [
      { icon: LayoutDashboard, label: 'Overview', to: '/projects/prj_1' },
      { icon: Boxes, label: 'Shared config', to: '/projects/prj_1?tab=shared-config' },
    ])

    await user.click(screen.getByRole('button', { name: 'Search…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Command palette' })
    const sections = await within(dialog).findByRole('group', { name: 'Sections in Blog' })
    expect(within(sections).getAllByRole('option').map(option => option.textContent)).toEqual(['Overview', 'Shared config'])

    await user.click(within(sections).getByRole('option', { name: 'Shared config' }))
    expect(await screen.findByTestId('location')).toHaveTextContent('/projects/prj_1?tab=shared-config')
  })

  it('runs existing actions', async () => {
    const user = userEvent.setup()
    renderPalette()

    await user.click(screen.getByRole('button', { name: 'Search…' }))
    await user.click(await screen.findByRole('option', { name: 'Toggle light/dark theme' }))
    expect(mocks.setMode).toHaveBeenCalledWith('dark')

    await user.click(screen.getByRole('button', { name: 'Search…' }))
    await user.click(await screen.findByRole('option', { name: 'Create project space' }))
    expect(await screen.findByTestId('location')).toHaveTextContent('/projects?create=1')
  })
})

const currentUser: CurrentUser = {
  id: 'usr_1',
  email: 'user@example.com',
  name: 'User',
  avatarUrl: '',
  passwordSet: true,
  role: 'user',
  language: 'en-US',
  brandColorPreset: 'blue',
  permissions: [],
}

function LocationProbe() {
  const location = useLocation()
  return <div data-testid="location">{`${location.pathname}${location.search}`}</div>
}

function renderPalette(initialEntries = ['/dashboard'], userOverrides: Partial<CurrentUser> = {}, projectSections?: ComponentProps<typeof CommandPalette>['projectSections']) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const user: CurrentUser = { ...currentUser, ...userOverrides }
  const theme: ThemeContextValue = { mode: 'light', setMode: mocks.setMode }
  return render(
    <QueryClientProvider client={queryClient}>
      <SessionContext value={sessionValue(user)}>
        <ThemeContext value={theme}>
          <MemoryRouter initialEntries={initialEntries}>
            <CommandPalette projectSections={projectSections} />
            <Routes>
              <Route element={<LocationProbe />} path="*" />
            </Routes>
          </MemoryRouter>
        </ThemeContext>
      </SessionContext>
    </QueryClientProvider>,
  )
}

function sessionValue(user: CurrentUser): SessionContextValue {
  return {
    initialized: true,
    isLoading: false,
    isLoggingIn: false,
    isLoggingOut: false,
    recentLoginUsers: [],
    user,
    login: vi.fn(async () => user),
    logout: vi.fn(async () => {}),
    refreshUser: vi.fn(async () => {}),
    updateProfile: vi.fn(async () => user),
    updateLanguage: vi.fn(async () => user),
  }
}

function projectFixture(): Project {
  return {
    id: 'prj_1',
    identifier: 'blog',
    kubernetesNamespace: 'blog',
    name: 'Blog',
    description: '',
    maxConcurrentBuilds: 2,
    webConsoleEnabled: true,
    billingOwnerUserId: 'usr_1',
    systemKey: '',
    deleteStatus: 'active',
    deleteMessage: '',
    dashboardOrder: 0,
    useCount: 0,
    createdAt: '2026-11-01T00:00:00Z',
    updatedAt: '2026-11-01T00:00:00Z',
  }
}

function applicationFixture(): Application {
  return {
    id: 'app_web',
    projectId: 'prj_1',
    identifier: 'web',
    name: 'Web',
    icon: '',
    deleteStatus: 'active',
    deleteMessage: '',
    deleteCode: '',
    createdAt: '2026-11-01T00:00:00Z',
    updatedAt: '2026-11-01T00:00:00Z',
  }
}
