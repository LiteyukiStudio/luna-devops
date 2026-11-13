import type { ComponentProps } from 'react'
import type { AIAssistantRuntimeValue } from './ai-assistant/runtime-context'
import type { Application, AppTemplate, CurrentUser, Project, SearchResponse } from '@/api'
import type { SessionContextValue } from '@/app/session-context'
import type { ThemeContextValue } from '@/app/theme-context'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Boxes, LayoutDashboard } from 'lucide-react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { SessionContext } from '@/app/session-context'
import { ThemeContext } from '@/app/theme-context'
import i18next from '@/i18n'
import { AIAssistantRuntimeContext } from './ai-assistant/runtime-context'
import { CommandPalette } from './command-palette'
import { readCommandPaletteRecents, recordCommandPaletteRecent } from './command-palette-model'

const mocks = vi.hoisted(() => ({
  listApplications: vi.fn(),
  listAppTemplates: vi.fn(),
  listProjects: vi.fn(),
  searchPlatformResources: vi.fn(),
  setMode: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listApplications: mocks.listApplications,
      listAppTemplates: mocks.listAppTemplates,
      listProjects: mocks.listProjects,
      searchPlatformResources: mocks.searchPlatformResources,
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
    window.localStorage.clear()
    await i18next.changeLanguage('en-US')
    mocks.listProjects.mockResolvedValue([projectFixture()])
    mocks.listApplications.mockResolvedValue([applicationFixture()])
    mocks.listAppTemplates.mockResolvedValue([templateFixture()])
    mocks.searchPlatformResources.mockResolvedValue({
      query: '',
      visibility: 'related',
      items: [],
    } satisfies SearchResponse)
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

  it('filters commands and navigates to pages including keyword aliases like PAT and Feishu', async () => {
    const user = userEvent.setup()
    renderPalette()

    await user.click(screen.getByRole('button', { name: 'Search…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Command palette' })
    expect(within(dialog).queryByRole('option', { name: 'Global Settings' })).not.toBeInTheDocument()

    const combobox = within(dialog).getByRole('combobox')
    await user.type(combobox, 'regis')
    expect(within(dialog).getByRole('option', { name: 'Registries' })).toBeInTheDocument()
    expect(within(dialog).queryByRole('option', { name: 'Dashboard' })).not.toBeInTheDocument()

    await user.keyboard('{Enter}')
    expect(await screen.findByTestId('location')).toHaveTextContent('/registries')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Search…' }))
    const reopened = await screen.findByRole('dialog', { name: 'Command palette' })
    await user.type(within(reopened).getByRole('combobox'), 'feishu')
    await user.click(within(reopened).getByRole('option', { name: 'Notification Channels' }))
    expect(await screen.findByTestId('location')).toHaveTextContent('/notification-channels')
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

    const appGroup = within(dialog).getByRole('group', { name: 'Applications in Blog' })
    await user.click(within(appGroup).getByRole('option', { name: /^Web/ }))
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

  it('searches cross-project resources via unified API, supports scope chips, and deep-links to target tabs', async () => {
    const user = userEvent.setup()
    mocks.searchPlatformResources.mockResolvedValue({
      query: 'pay.alpha',
      visibility: 'related',
      items: [
        {
          type: 'gateway_route',
          id: 'gwr_pay',
          title: 'pay.alpha.example.com/v1',
          subtitle: 'auto-https',
          identifier: '/v1',
          related: true,
          projectId: 'prj_2',
          projectName: 'Commerce',
          applicationId: 'app_checkout',
          applicationName: 'Checkout API',
          updatedAt: '2026-11-13T08:00:00Z',
        },
        {
          type: 'deployment_target',
          id: 'dplt_prod',
          title: 'Production',
          subtitle: 'ghcr.io/liteyuki/checkout:2.4.1',
          identifier: 'production',
          related: true,
          projectId: 'prj_2',
          projectName: 'Commerce',
          applicationId: 'app_checkout',
          applicationName: 'Checkout API',
          updatedAt: '2026-11-13T08:00:00Z',
        },
      ],
    } satisfies SearchResponse)

    renderPalette()
    await user.click(screen.getByRole('button', { name: 'Search…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Command palette' })

    await user.click(within(dialog).getByRole('button', { name: 'Domains' }))
    await user.type(within(dialog).getByRole('combobox'), 'pay.alpha')

    await waitFor(() => {
      expect(mocks.searchPlatformResources).toHaveBeenCalledWith(
        expect.objectContaining({
          q: 'pay.alpha',
          types: ['gateway_route'],
          visibility: undefined,
        }),
        expect.any(AbortSignal),
      )
    })

    const domainGroup = await within(dialog).findByRole('group', { name: 'Domains & gateway routes' })
    await user.click(within(domainGroup).getByRole('option', { name: /pay\.alpha\.example\.com\/v1/ }))
    expect(await screen.findByTestId('location')).toHaveTextContent('/projects/prj_2/apps/app_checkout?tab=gateway')
  })

  it('allows platform administrators to toggle all-spaces visibility and marks global hits', async () => {
    const user = userEvent.setup()
    mocks.searchPlatformResources.mockResolvedValue({
      query: 'shadow',
      visibility: 'all',
      items: [
        {
          type: 'application',
          id: 'app_shadow',
          title: 'Shadow Worker',
          identifier: 'shadow-worker',
          related: false,
          projectId: 'prj_other',
          projectName: 'Other Space',
          applicationId: 'app_shadow',
          applicationName: 'Shadow Worker',
          updatedAt: '2026-11-13T08:00:00Z',
        },
      ],
    } satisfies SearchResponse)

    renderPalette(['/dashboard'], { role: 'platform_admin', permissions: ['user.manage'] })
    await user.click(screen.getByRole('button', { name: 'Search…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Command palette' })

    const allSpacesBtn = within(dialog).getByRole('button', { name: 'All spaces' })
    await user.click(allSpacesBtn)
    await user.type(within(dialog).getByRole('combobox'), 'shadow')

    await waitFor(() => {
      expect(mocks.searchPlatformResources).toHaveBeenCalledWith(
        expect.objectContaining({
          q: 'shadow',
          visibility: 'all',
        }),
        expect.any(AbortSignal),
      )
    })

    const appOption = await within(dialog).findByRole('option', { name: /Shadow Worker/ })
    expect(within(appOption).getByText('Global')).toBeInTheDocument()
  })

  it('isolates recent visits by userId and offers Luna Assistant handoff when enabled', async () => {
    recordCommandPaletteRecent('usr_other', {
      key: 'project:prj_secret',
      kind: 'project',
      id: 'prj_secret',
      label: 'Secret Space',
      to: '/projects/prj_secret',
    })
    recordCommandPaletteRecent('usr_1', {
      key: 'application:app_pay',
      kind: 'application',
      id: 'app_pay',
      projectId: 'prj_2',
      label: 'Checkout Service',
      sublabel: 'Commerce',
      to: '/projects/prj_2/apps/app_pay',
    })
    expect(readCommandPaletteRecents('usr_1').map(item => item.id)).toEqual(['app_pay'])

    const onDraftChange = vi.fn()
    const openAssistant = vi.fn()
    const user = userEvent.setup()

    renderPalette(['/dashboard'], {}, undefined, {
      enabled: true,
      openAssistant,
      onDraftChange,
    })

    await user.click(screen.getByRole('button', { name: 'Search…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Command palette' })

    const recentGroup = within(dialog).getByRole('group', { name: 'Recent' })
    expect(within(recentGroup).getByRole('option', { name: /Checkout Service/ })).toBeInTheDocument()
    expect(within(dialog).queryByText('Secret Space')).not.toBeInTheDocument()

    await user.type(within(dialog).getByRole('combobox'), 'why is pod crashing')
    const askOption = await within(dialog).findByRole('option', { name: /Ask Luna Assistant about "why is pod crashing"/ })
    await user.click(askOption)

    expect(onDraftChange).toHaveBeenCalledWith('why is pod crashing')
    expect(await screen.findByTestId('location')).toHaveTextContent('/ai-assistant')
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
  return <div data-testid="location">{`${location.pathname}${location.search}${location.hash}`}</div>
}

function renderPalette(
  initialEntries = ['/dashboard'],
  userOverrides: Partial<CurrentUser> = {},
  projectSections?: ComponentProps<typeof CommandPalette>['projectSections'],
  aiOverrides?: { enabled: boolean, openAssistant: () => void, onDraftChange: (value: string) => void },
) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const user: CurrentUser = { ...currentUser, ...userOverrides }
  const theme: ThemeContextValue = { mode: 'light', setMode: mocks.setMode }
  const aiContextValue = aiOverrides
    ? ({
        enabled: aiOverrides.enabled,
        open: false,
        openAssistant: aiOverrides.openAssistant,
        closeAssistant: vi.fn(),
        rememberWorkspaceLocation: vi.fn(),
        composerProps: {
          draft: '',
          onDraftChange: aiOverrides.onDraftChange,
          onSend: vi.fn(),
          disabled: false,
          running: false,
          models: [],
          onCancel: vi.fn(),
          onModelChange: vi.fn(),
        },
      } as unknown as AIAssistantRuntimeValue)
    : null

  return render(
    <QueryClientProvider client={queryClient}>
      <SessionContext value={sessionValue(user)}>
        <ThemeContext value={theme}>
          <AIAssistantRuntimeContext value={aiContextValue}>
            <MemoryRouter initialEntries={initialEntries}>
              <CommandPalette projectSections={projectSections} />
              <Routes>
                <Route element={<LocationProbe />} path="*" />
              </Routes>
            </MemoryRouter>
          </AIAssistantRuntimeContext>
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

function templateFixture(): AppTemplate {
  return {
    id: 'tpl_probe',
    slug: 'traffic-probe',
    name: 'Traffic Probe',
    description: 'Cluster network traffic probe component',
    category: 'observability',
    icon: 'activity',
    officialWebsite: '',
    officialRepository: '',
    popularityWeight: 10,
    featured: true,
    image: 'ghcr.io/liteyuki/traffic-probe:latest',
    version: '1.0.0',
    servicePort: 8080,
    defaultReplicas: 1,
    defaultCPU: '100m',
    defaultMemory: '128Mi',
    dataVolumes: [],
    components: [],
    values: [],
  }
}
