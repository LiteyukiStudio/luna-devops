import type { ReactNode } from 'react'
import type { CurrentUser } from '@/api'
import type { SessionContextValue } from '@/app/session-context'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SessionContext } from '@/app/session-context'
import i18next from '@/i18n'
import { AppLayout } from './AppLayout'

vi.mock('@/components/common/ai-assistant/runtime-provider', () => ({
  AIAssistantRuntimeProvider: ({ children }: { children: ReactNode }) => children,
}))

vi.mock('@/components/common/ai-assistant/runtime-context', () => ({
  useAIAssistantRuntime: () => ({ enabled: false, open: false }),
}))

vi.mock('@/components/common/ai-assistant/presentation-mode', () => ({
  useAIAssistantPresentationMode: () => 'window',
}))

// 命令面板与收件箱有独立测试；这里只验证顶栏层级。
vi.mock('@/components/common/command-palette', () => ({
  CommandPalette: () => null,
}))

vi.mock('@/components/common/inbox/inbox-trigger', () => ({
  InboxTrigger: () => null,
}))

const currentUser: CurrentUser = {
  id: 'usr_member',
  email: 'member@example.com',
  name: 'Member',
  avatarUrl: '',
  passwordSet: true,
  role: 'user',
  language: 'zh-CN',
  brandColorPreset: 'blue',
  permissions: [],
}

const sessionValue: SessionContextValue = {
  initialized: true,
  isLoading: false,
  isLoggingIn: false,
  isLoggingOut: false,
  recentLoginUsers: [],
  user: currentUser,
  login: vi.fn(async () => currentUser),
  logout: vi.fn(async () => {}),
  refreshUser: vi.fn(async () => {}),
  updateProfile: vi.fn(async () => currentUser),
  updateLanguage: vi.fn(async () => currentUser),
}

function renderLayout(initialEntry: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } } })
  queryClient.setQueryData(['projects'], [])
  queryClient.setQueryData(['ai', 'capabilities'], { enabled: false, maxInputBytes: 0 })
  queryClient.setQueryData(['project', 'prj_1'], { id: 'prj_1', name: 'Demo' })
  queryClient.setQueryData(['application', 'prj_1', 'app_1'], { id: 'app_1', name: 'Web', projectId: 'prj_1' })
  return render(
    <QueryClientProvider client={queryClient}>
      <SessionContext value={sessionValue}>
        <MemoryRouter initialEntries={[initialEntry]}>
          <Routes>
            <Route element={<AppLayout />}>
              <Route path="/projects/:projectId/apps/:applicationId" element={<div>Application</div>} />
              <Route path="/projects/:projectId/apps/:applicationId/deployments/new" element={<div>Editor</div>} />
              <Route path="/projects/:projectId/apps/:applicationId/deployments/:targetId/edit" element={<div>Editor</div>} />
            </Route>
          </Routes>
        </MemoryRouter>
      </SessionContext>
    </QueryClientProvider>,
  )
}

describe('app layout breadcrumbs', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn(() => ({ addEventListener: vi.fn(), matches: false, removeEventListener: vi.fn() })),
    })
  })

  it('keeps the application as a clickable ancestor on the deploy config editor', async () => {
    renderLayout('/projects/prj_1/apps/app_1/deployments/new')

    const breadcrumbs = await screen.findByRole('navigation', { name: '面包屑导航' })
    expect(within(breadcrumbs).getByRole('link', { name: '项目空间' })).toHaveAttribute('href', '/projects')
    expect(within(breadcrumbs).getByRole('link', { name: 'Demo' })).toHaveAttribute('href', '/projects/prj_1')
    expect(within(breadcrumbs).getByRole('link', { name: 'Web' })).toHaveAttribute('href', '/projects/prj_1/apps/app_1')
    expect(screen.getByRole('heading', { level: 1, name: '新增部署配置' })).toBeInTheDocument()
    await waitFor(() => expect(document.title).toContain('新增部署配置：Demo / Web'))
    // 窄屏隐藏面包屑后，标题左侧的返回按钮指向面包屑上一级。
    expect(screen.getByRole('link', { name: '返回Web' })).toHaveAttribute('href', '/projects/prj_1/apps/app_1')
  })

  it('titles the edit page and returns to the project from the application page', async () => {
    const { unmount } = renderLayout('/projects/prj_1/apps/app_1/deployments/dpt_1/edit')
    await waitFor(() => expect(document.title).toContain('编辑部署配置：Demo / Web'))
    unmount()

    renderLayout('/projects/prj_1/apps/app_1')
    const breadcrumbs = await screen.findByRole('navigation', { name: '面包屑导航' })
    expect(within(breadcrumbs).queryByRole('link', { name: 'Web' })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: '返回Demo' })).toHaveAttribute('href', '/projects/prj_1')
  })
})
