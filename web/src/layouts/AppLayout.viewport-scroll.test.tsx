import type { ReactNode } from 'react'
import type { CurrentUser } from '@/api'
import type { SessionContextValue } from '@/app/session-context'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { ESLint } from 'eslint'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PublicConfigContext } from '@/app/public-config-context'
import { SessionContext } from '@/app/session-context'
import { ForbiddenPage } from '@/components/common/forbidden-page'
import i18next from '@/i18n'
import { LoginPage } from '@/pages/login/LoginPage'
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

function createSessionValue(user?: CurrentUser): SessionContextValue {
  return {
    initialized: true,
    isLoading: false,
    isLoggingIn: false,
    isLoggingOut: false,
    recentLoginUsers: [],
    user,
    login: vi.fn(async () => currentUser),
    logout: vi.fn(async () => {}),
    refreshUser: vi.fn(async () => {}),
    updateProfile: vi.fn(async () => currentUser),
    updateLanguage: vi.fn(async () => currentUser),
  }
}

describe('viewport and mobile scroll architecture contracts', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn(() => ({ addEventListener: vi.fn(), matches: false, removeEventListener: vi.fn() })),
    })
  })

  it('keeps global html, body, and #root unlocked in index.css without deprecated overflow: overlay or static vh', () => {
    const indexCssPath = path.resolve(import.meta.dirname, '../index.css')
    const css = readFileSync(indexCssPath, 'utf8')

    // Extract rule blocks for html/body/#root and ensure they never lock overflow: hidden or height: 100%.
    const rootBlockMatches = css.match(/(?:html\s*,\s*body|#root|\bbody)\s*\{[^}]*\}/g) ?? []
    expect(rootBlockMatches.length).toBeGreaterThanOrEqual(3)
    for (const block of rootBlockMatches) {
      expect(block).not.toMatch(/overflow\s*:\s*hidden/)
      expect(block).not.toMatch(/(?:^|[\s;])height\s*:\s*100%/)
      expect(block).not.toMatch(/min-width\s*:/)
    }

    expect(css).not.toContain('overflow: overlay')
    expect(css).not.toContain('overflow-y: overlay')
    expect(css).not.toMatch(/\d+(?:\.\d+)?vh\b/)
  })

  it('confines workspace fixed viewport and overscroll behavior to AppLayout', () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } } })
    queryClient.setQueryData(['projects'], [])
    queryClient.setQueryData(['ai', 'capabilities'], { enabled: false, maxInputBytes: 0 })

    const { container } = render(
      <QueryClientProvider client={queryClient}>
        <PublicConfigContext value={{}}>
          <SessionContext value={createSessionValue(currentUser)}>
            <MemoryRouter initialEntries={['/projects']}>
              <Routes>
                <Route element={<AppLayout />}>
                  <Route path="/projects" element={<div>Projects content</div>} />
                </Route>
              </Routes>
            </MemoryRouter>
          </SessionContext>
        </PublicConfigContext>
      </QueryClientProvider>,
    )

    const shell = container.querySelector('[data-slot="workspace-shell"]')
    const main = container.querySelector('[data-slot="workspace-main"]')
    expect(shell).not.toBeNull()
    expect(shell).toHaveClass('h-dvh', 'overflow-hidden', 'overscroll-none')
    expect(main).not.toBeNull()
    expect(main).toHaveClass('overflow-y-auto', 'overflow-x-hidden', 'overscroll-y-contain')
    expect(screen.getByText('Projects content')).toBeInTheDocument()
  })

  it('renders standalone LoginPage and ForbiddenPage with min-h-dvh and natural document scroll (no overflow-hidden)', () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    queryClient.setQueryData(['auth', 'providers'], [])
    queryClient.setQueryData(['auth', 'registration-status'], { emailRegistrationEnabled: false })

    const { container: loginContainer, unmount } = render(
      <QueryClientProvider client={queryClient}>
        <PublicConfigContext value={{}}>
          <SessionContext value={createSessionValue()}>
            <MemoryRouter initialEntries={['/login']}>
              <LoginPage />
            </MemoryRouter>
          </SessionContext>
        </PublicConfigContext>
      </QueryClientProvider>,
    )

    const loginRoot = loginContainer.firstElementChild
    expect(loginRoot).not.toBeNull()
    expect(loginRoot).toHaveClass('min-h-dvh', 'min-w-0')
    expect(loginRoot).not.toHaveClass('overflow-hidden', 'h-dvh', 'min-h-screen')
    unmount()

    const { container: forbiddenContainer } = render(
      <MemoryRouter initialEntries={['/forbidden']}>
        <ForbiddenPage />
      </MemoryRouter>,
    )
    const forbiddenRoot = forbiddenContainer.firstElementChild
    expect(forbiddenRoot).not.toBeNull()
    expect(forbiddenRoot).toHaveClass('min-h-dvh', 'min-w-0')
    expect(forbiddenRoot).not.toHaveClass('overflow-hidden', 'h-dvh', 'min-h-screen')
  })

  it('enforces ESLint rules blocking static vh/*-screen height classes and scrollIntoView inside effects/IntersectionObserver', async () => {
    const webRoot = path.resolve(import.meta.dirname, '../..')
    const eslint = new ESLint({ cwd: webRoot })
    const virtualFile = path.join(webRoot, 'src/pages/example-check.tsx')

    const [invalidReport] = await eslint.lintText(
      [
        'import { useEffect, useRef } from \'react\'',
        'export function InvalidExample() {',
        '  const ref = useRef<HTMLDivElement>(null)',
        '  useEffect(() => {',
        '    ref.current?.scrollIntoView?.({ block: \'nearest\' })',
        '  }, [])',
        '  return <div ref={ref} className="min-h-screen max-h-[88vh]" />',
        '}',
        '',
      ].join('\n'),
      { filePath: virtualFile },
    )
    const syntaxMessages = invalidReport.messages
      .filter(item => item.ruleId === 'no-restricted-syntax')
      .map(item => item.message)
    expect(syntaxMessages.some(msg => msg.includes('scrollIntoView()'))).toBe(true)
    expect(syntaxMessages.some(msg => msg.includes('dynamic viewport units'))).toBe(true)

    const [validReport] = await eslint.lintText(
      [
        'import { useRef } from \'react\'',
        'export function ValidExample() {',
        '  const ref = useRef<HTMLDivElement>(null)',
        '  return <button className="min-h-dvh max-h-[88dvh]" type="button" onClick={() => ref.current?.scrollIntoView()}>Jump</button>',
        '}',
        '',
      ].join('\n'),
      { filePath: virtualFile },
    )
    expect(validReport.messages.filter(item => item.ruleId === 'no-restricted-syntax')).toEqual([])
  })
})
