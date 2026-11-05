import { fireEvent, render, screen } from '@testing-library/react'
import { useEffect, useState } from 'react'
import { beforeEach, describe, expect, it } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { RuntimeWebConsoleDialog } from './runtime-web-console-dialog'

function ConsoleHarness() {
  const [open, setOpen] = useState(true)

  return (
    <TooltipProvider>
      <button type="button" onClick={() => setOpen(true)}>Open console</button>
      <RuntimeWebConsoleDialog
        closeLabel="Close console"
        containerLabel="Container"
        containerPlaceholder="Optional container"
        description="Interactive runtime shell"
        exitFullscreenLabel="Exit fullscreen"
        fullscreenLabel="Enter fullscreen"
        loadingLabel="Loading terminal"
        open={open}
        resourceKey="release-1"
        resourceLabel="release-1"
        title="Runtime console"
        onOpenChange={setOpen}
      >
        {({ attempt, container, fullscreen }) => (
          <output data-testid="render-context">
            {container || 'default'}
            |
            {fullscreen ? 'fullscreen' : 'windowed'}
            |
            {attempt}
          </output>
        )}
      </RuntimeWebConsoleDialog>
    </TooltipProvider>
  )
}

function DisconnectedTerminal({ attempt, onConnectionChange }: { attempt: number, onConnectionChange: (state: 'disconnected') => void }) {
  useEffect(() => onConnectionChange('disconnected'), [onConnectionChange])
  return <output data-testid="terminal-attempt">{attempt}</output>
}

describe('runtime web console dialog', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('shows the connection status and reconnects by bumping the attempt', async () => {
    render(
      <TooltipProvider>
        <RuntimeWebConsoleDialog
          closeLabel="Close console"
          containerLabel="Container"
          containerPlaceholder="Optional container"
          description="Interactive runtime shell"
          exitFullscreenLabel="Exit fullscreen"
          fullscreenLabel="Enter fullscreen"
          loadingLabel="Loading terminal"
          open
          resourceKey="release-1"
          resourceLabel="release-1"
          title="Runtime console"
          onOpenChange={() => {}}
        >
          {({ attempt, onConnectionChange }) => <DisconnectedTerminal key={attempt} attempt={attempt} onConnectionChange={onConnectionChange} />}
        </RuntimeWebConsoleDialog>
      </TooltipProvider>,
    )

    expect(await screen.findByText('Disconnected')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Reconnect' }))
    expect(screen.getByTestId('terminal-attempt')).toHaveTextContent('1')
  })

  it('provides container and fullscreen context, then resets both after closing', () => {
    render(<ConsoleHarness />)

    const dialog = screen.getByRole('dialog', { name: 'Runtime console' })
    expect(dialog).toHaveAccessibleDescription('Interactive runtime shell')
    expect(screen.getByTestId('render-context')).toHaveTextContent('default|windowed|0')

    const containerInput = screen.getByRole('textbox', { name: 'Container' })
    expect(containerInput).toHaveAttribute('placeholder', 'Optional container')
    fireEvent.change(containerInput, { target: { value: 'worker' } })
    expect(screen.getByTestId('render-context')).toHaveTextContent('worker|windowed|0')

    fireEvent.click(screen.getByRole('button', { name: 'Enter fullscreen' }))
    expect(screen.getByRole('button', { name: 'Exit fullscreen' })).toBeInTheDocument()
    expect(screen.getByTestId('render-context')).toHaveTextContent('worker|fullscreen|0')

    fireEvent.click(screen.getByRole('button', { name: 'Close console' }))
    expect(screen.queryByRole('dialog', { name: 'Runtime console' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Open console' }))
    expect(screen.getByRole('textbox', { name: 'Container' })).toHaveValue('')
    expect(screen.getByRole('button', { name: 'Enter fullscreen' })).toBeInTheDocument()
    expect(screen.getByTestId('render-context')).toHaveTextContent('default|windowed|0')
  })
})
