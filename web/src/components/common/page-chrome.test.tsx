import { render, screen } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { PageChromeHeading, PageChromeTabs, PageChromeTools } from './page-chrome'
import { WorkspaceChromeTargetsProvider } from './workspace-chrome-context'

describe('page chrome', () => {
  it('portals heading, tabs and tools into the page header once', async () => {
    render(<ChromeTargetsFixture />)

    expect(await screen.findByTestId('heading-target')).toContainElement(screen.getByRole('heading', { level: 1, name: 'Edit target' }))
    expect(screen.getByTestId('tabs-target')).toHaveTextContent('Overview')
    expect(screen.getByTestId('tools-target')).toHaveTextContent('Create')
    expect(screen.getAllByText('Create')).toHaveLength(1)
  })

  it('renders in place when no workspace layout provides targets', () => {
    render(
      <>
        <PageChromeHeading title="Standalone" />
        <PageChromeTabs>Overview</PageChromeTabs>
        <PageChromeTools>Create</PageChromeTools>
      </>,
    )

    expect(screen.getByRole('heading', { level: 1, name: 'Standalone' })).toBeInTheDocument()
    expect(screen.getByText('Overview')).toBeInTheDocument()
    expect(screen.getByText('Create')).toBeInTheDocument()
  })

  it('registers page header content for its mounted lifetime', () => {
    const unregisterHeading = vi.fn()
    const unregisterTabs = vi.fn()
    const unregisterTools = vi.fn()
    const registerHeading = vi.fn(() => unregisterHeading)
    const registerTabs = vi.fn(() => unregisterTabs)
    const registerTools = vi.fn(() => unregisterTools)
    const { unmount } = render(
      <WorkspaceChromeTargetsProvider value={{ description: null, heading: null, registerHeading, registerTabs, registerTools, tabs: null, tools: null }}>
        <PageChromeHeading title="Edit target" />
        <PageChromeTabs>Overview</PageChromeTabs>
        <PageChromeTools>Create</PageChromeTools>
      </WorkspaceChromeTargetsProvider>,
    )

    expect(registerHeading).toHaveBeenCalledOnce()
    expect(registerTabs).toHaveBeenCalledOnce()
    expect(registerTools).toHaveBeenCalledOnce()

    unmount()
    expect(unregisterHeading).toHaveBeenCalledOnce()
    expect(unregisterTabs).toHaveBeenCalledOnce()
    expect(unregisterTools).toHaveBeenCalledOnce()
  })
})

function ChromeTargetsFixture() {
  const [heading, setHeading] = useState<HTMLDivElement | null>(null)
  const [tabs, setTabs] = useState<HTMLDivElement | null>(null)
  const [tools, setTools] = useState<HTMLDivElement | null>(null)

  return (
    <WorkspaceChromeTargetsProvider
      value={{
        description: null,
        heading,
        registerHeading: () => () => {},
        registerTabs: () => () => {},
        registerTools: () => () => {},
        tabs,
        tools,
      }}
    >
      <div ref={setHeading} data-testid="heading-target" />
      <div ref={setTabs} data-testid="tabs-target" />
      <div ref={setTools} data-testid="tools-target" />
      <PageChromeHeading title="Edit target" />
      <PageChromeTabs>Overview</PageChromeTabs>
      <PageChromeTools>Create</PageChromeTools>
    </WorkspaceChromeTargetsProvider>
  )
}
