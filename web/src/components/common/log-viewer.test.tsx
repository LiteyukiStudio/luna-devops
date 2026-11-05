import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { LogViewer } from './log-viewer'

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

function renderViewer(props: Partial<Parameters<typeof LogViewer>[0]> = {}) {
  return render(
    <TooltipProvider>
      <LogViewer downloadName="build.log" emptyLabel="No logs yet" resetKey="job-1" state="ready" {...props} />
    </TooltipProvider>,
  )
}

describe('log viewer', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
  })

  it('renders numbered lines, highlights failures, and copies plain text', async () => {
    const user = userEvent.setup()
    const writeText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    renderViewer({ content: 'step 1\n\u001B[31mERROR: build failed\u001B[0m\n' })

    const log = screen.getByRole('log', { name: 'Log output' })
    expect(log).toHaveTextContent('1step 12ERROR: build failed')
    expect(screen.getByText('ERROR: build failed').closest('div')).toHaveClass('text-terminal-error')

    await user.click(screen.getByRole('button', { name: 'Show line numbers' }))
    expect(log).toHaveTextContent('step 1ERROR: build failed')
    expect(screen.getByRole('button', { name: 'Show line numbers' })).toHaveAttribute('aria-pressed', 'false')

    await user.click(screen.getByRole('button', { name: 'Copy logs' }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('step 1\nERROR: build failed'))
    expect(toast.success).toHaveBeenCalledWith('Copied')
  })

  it('shows loading, empty, and retryable unavailable placeholders instead of a blank panel', async () => {
    const user = userEvent.setup()
    const retry = vi.fn()
    const view = renderViewer({ loadingLabel: 'Waiting for logs', state: 'loading' })
    expect(screen.getByRole('status')).toHaveTextContent('Waiting for logs')
    expect(screen.getByRole('button', { name: 'Copy logs' })).toBeDisabled()

    view.unmount()
    const empty = renderViewer()
    expect(screen.getByRole('status')).toHaveTextContent('No logs yet')

    empty.unmount()
    renderViewer({ content: 'stale output', state: 'error', onRetry: retry })
    expect(screen.getByRole('alert')).toHaveTextContent('Unavailable')
    expect(screen.queryByText('stale output')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(retry).toHaveBeenCalledOnce()
  })

  it('offers a follow toggle only for live logs', async () => {
    const user = userEvent.setup()
    const view = renderViewer({ content: 'done' })
    expect(screen.queryByRole('button', { name: 'Follow latest output' })).not.toBeInTheDocument()

    view.unmount()
    renderViewer({ content: 'streaming', live: true })
    const follow = screen.getByRole('button', { name: 'Follow latest output' })
    expect(follow).toHaveAttribute('aria-pressed', 'true')
    await user.click(follow)
    expect(follow).toHaveAttribute('aria-pressed', 'false')
  })
})
