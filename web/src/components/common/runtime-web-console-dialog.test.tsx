import { fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import { RuntimeWebConsoleDialog } from './runtime-web-console-dialog'

function ConsoleHarness() {
  const [open, setOpen] = useState(true)

  return (
    <>
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
        {({ container, fullscreen }) => (
          <output data-testid="render-context">
            {container || 'default'}
            |
            {fullscreen ? 'fullscreen' : 'windowed'}
          </output>
        )}
      </RuntimeWebConsoleDialog>
    </>
  )
}

describe('runtime web console dialog', () => {
  it('provides container and fullscreen context, then resets both after closing', () => {
    render(<ConsoleHarness />)

    const dialog = screen.getByRole('dialog', { name: 'Runtime console' })
    expect(dialog).toHaveAccessibleDescription('Interactive runtime shell')
    expect(screen.getByTestId('render-context')).toHaveTextContent('default|windowed')

    const containerInput = screen.getByRole('textbox', { name: 'Container' })
    expect(containerInput).toHaveAttribute('placeholder', 'Optional container')
    fireEvent.change(containerInput, { target: { value: 'worker' } })
    expect(screen.getByTestId('render-context')).toHaveTextContent('worker|windowed')

    fireEvent.click(screen.getByRole('button', { name: 'Enter fullscreen' }))
    expect(screen.getByRole('button', { name: 'Exit fullscreen' })).toBeInTheDocument()
    expect(screen.getByTestId('render-context')).toHaveTextContent('worker|fullscreen')

    fireEvent.click(screen.getByRole('button', { name: 'Close console' }))
    expect(screen.queryByRole('dialog', { name: 'Runtime console' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Open console' }))
    expect(screen.getByRole('textbox', { name: 'Container' })).toHaveValue('')
    expect(screen.getByRole('button', { name: 'Enter fullscreen' })).toBeInTheDocument()
    expect(screen.getByTestId('render-context')).toHaveTextContent('default|windowed')
  })
})
