import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { CliCommandSnippet } from './cli-command-snippet'

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

const spec = {
  command: 'deployment.exec',
  server: 'https://devops.example.com',
  projectId: 'prj_123',
  applicationId: 'app_456',
  targetId: 'dplt_789',
} as const

describe('cli command snippet', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
  })

  it('copies the command, exposes copy state, and links its localized help', async () => {
    const interaction = userEvent.setup()
    const writeText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })

    render(
      <TooltipProvider>
        <CliCommandSnippet spec={spec} />
      </TooltipProvider>,
    )

    expect(screen.getByRole('group', { name: 'CLI command' })).toHaveTextContent('luna deployment exec server=https://devops.example.com projectId=prj_123 applicationId=app_456 targetId=dplt_789')
    const docs = screen.getByRole('link', { name: 'View CLI documentation' })
    expect(docs).toHaveAttribute('href', 'https://luna-devops.liteyuki.org/en/use/cli')
    expect(docs).toHaveAttribute('target', '_blank')
    expect(docs).toHaveAttribute('rel', 'noreferrer')

    await interaction.hover(docs)
    expect(await screen.findByRole('tooltip')).toHaveTextContent('Run in your local terminal; open the Luna CLI documentation')

    await interaction.click(screen.getByRole('button', { name: 'Copy CLI command' }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('luna deployment exec server=https://devops.example.com projectId=prj_123 applicationId=app_456 targetId=dplt_789'))
    expect(screen.getByRole('button', { name: 'CLI command copied' })).toBeInTheDocument()
    expect(toast.success).toHaveBeenCalledWith('Copied')
  })

  it('shows a localized failure without changing copy state', async () => {
    const interaction = userEvent.setup()
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn(() => Promise.reject(new Error('denied'))) },
    })

    render(
      <TooltipProvider>
        <CliCommandSnippet spec={spec} />
      </TooltipProvider>,
    )

    await interaction.click(screen.getByRole('button', { name: 'Copy CLI command' }))
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Copy failed'))
    expect(screen.getByRole('button', { name: 'Copy CLI command' })).toBeInTheDocument()
  })
})
