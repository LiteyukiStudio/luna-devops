import { render, screen } from '@testing-library/react'
import { useState } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { PageBackNavigation, PageChromeTabs, PageChromeTools } from './page-chrome'
import { WorkspaceChromeTargetsProvider } from './workspace-chrome-context'

describe('page chrome', () => {
  it('renders back navigation as a regular page-flow link', () => {
    render(
      <MemoryRouter>
        <PageBackNavigation label="Back to project spaces" to="/projects" />
      </MemoryRouter>,
    )

    expect(screen.getByRole('link', { name: 'Back to project spaces' })).toHaveAttribute('href', '/projects')
  })

  it('portals page tabs and desktop tools into workspace targets', async () => {
    render(<ChromeTargetsFixture />)

    expect(await screen.findByTestId('tabs-target')).toHaveTextContent('Overview')
    expect(screen.getByTestId('tools-target')).toHaveTextContent('Create')
    expect(screen.getAllByText('Create')).toHaveLength(2)
  })

  it('registers secondary-row content for its mounted lifetime', () => {
    const unregisterTabs = vi.fn()
    const unregisterTools = vi.fn()
    const registerTabs = vi.fn(() => unregisterTabs)
    const registerTools = vi.fn(() => unregisterTools)
    const { unmount } = render(
      <WorkspaceChromeTargetsProvider value={{ registerTabs, registerTools, tabs: null, tools: null }}>
        <PageChromeTabs>Overview</PageChromeTabs>
        <PageChromeTools>Create</PageChromeTools>
      </WorkspaceChromeTargetsProvider>,
    )

    expect(registerTabs).toHaveBeenCalledOnce()
    expect(registerTools).toHaveBeenCalledOnce()

    unmount()
    expect(unregisterTabs).toHaveBeenCalledOnce()
    expect(unregisterTools).toHaveBeenCalledOnce()
  })
})

function ChromeTargetsFixture() {
  const [tabs, setTabs] = useState<HTMLDivElement | null>(null)
  const [tools, setTools] = useState<HTMLDivElement | null>(null)

  return (
    <WorkspaceChromeTargetsProvider
      value={{
        registerTabs: () => () => {},
        registerTools: () => () => {},
        tabs,
        tools,
      }}
    >
      <div ref={setTabs} data-testid="tabs-target" />
      <div ref={setTools} data-testid="tools-target" />
      <PageChromeTabs>Overview</PageChromeTabs>
      <PageChromeTools>Create</PageChromeTools>
    </WorkspaceChromeTargetsProvider>
  )
}
