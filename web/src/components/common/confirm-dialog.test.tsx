import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { ConfirmDialog } from './confirm-dialog'

describe('confirm dialog', () => {
  it('waits for an uncontrolled async confirmation, closes on success, and restores focus', async () => {
    const interaction = userEvent.setup()
    const onConfirm = vi.fn()
    let resolveConfirmation!: () => void
    onConfirm.mockReturnValue(new Promise<void>((resolve) => {
      resolveConfirmation = resolve
    }))

    render(
      <ConfirmDialog cancelText="Cancel" confirmText="Delete" description="Delete this item permanently." title="Delete item?" onConfirm={onConfirm}>
        <button type="button">Open confirmation</button>
      </ConfirmDialog>,
    )

    const trigger = screen.getByRole('button', { name: 'Open confirmation' })
    await interaction.click(trigger)
    const confirm = screen.getByRole('button', { name: 'Delete' })
    const cancel = screen.getByRole('button', { name: 'Cancel' })

    expect(confirm).toHaveClass('bg-danger')
    expect(cancel).toHaveClass('bg-surface')
    await interaction.click(confirm)
    expect(onConfirm).toHaveBeenCalledOnce()
    expect(confirm).toBeDisabled()
    expect(cancel).toBeDisabled()

    await act(async () => resolveConfirmation())

    await waitFor(() => expect(screen.queryByText('Delete item?')).not.toBeInTheDocument())
    await waitFor(() => expect(trigger).toHaveFocus())
  })

  it('keeps the dialog open and restores its actions when confirmation fails', async () => {
    const interaction = userEvent.setup()
    let rejectConfirmation!: (reason: Error) => void
    const onConfirm = vi.fn(() => new Promise<void>((_, reject) => {
      rejectConfirmation = reject
    }))

    render(
      <ConfirmDialog cancelText="Cancel" confirmText="Delete" description="Delete this item permanently." title="Delete item?" onConfirm={onConfirm}>
        <button type="button">Open confirmation</button>
      </ConfirmDialog>,
    )

    await interaction.click(screen.getByRole('button', { name: 'Open confirmation' }))
    await interaction.click(screen.getByRole('button', { name: 'Delete' }))
    await act(async () => rejectConfirmation(new Error('request failed')))

    expect(screen.getByText('Delete item?')).toBeVisible()
    await waitFor(() => expect(screen.getByRole('button', { name: 'Delete' })).toBeEnabled())
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeEnabled()
  })

  it('keeps the dialog open after a successful confirmation when closeOnConfirm is false', async () => {
    const interaction = userEvent.setup()
    const onConfirm = vi.fn(async () => undefined)

    render(
      <ConfirmDialog closeOnConfirm={false} cancelText="Cancel" confirmText="Save" description="Apply this configuration." title="Apply changes?" onConfirm={onConfirm}>
        <button type="button">Open confirmation</button>
      </ConfirmDialog>,
    )

    await interaction.click(screen.getByRole('button', { name: 'Open confirmation' }))
    await interaction.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce())
    expect(screen.getByText('Apply changes?')).toBeVisible()
    await waitFor(() => expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled())
  })

  it('supports controlled open state and disables both actions while externally pending', async () => {
    const interaction = userEvent.setup()

    function ControlledDialog({ pending }: { pending: boolean }) {
      const [open, setOpen] = useState(true)
      return (
        <ConfirmDialog
          cancelText="Cancel"
          confirmText="Continue"
          description="Continue with the action."
          open={open}
          pending={pending}
          title="Continue?"
          onConfirm={() => undefined}
          onOpenChange={setOpen}
        />
      )
    }

    const view = render(<ControlledDialog pending />)
    expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()

    view.rerender(<ControlledDialog pending={false} />)
    await interaction.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByText('Continue?')).not.toBeInTheDocument())
  })
})
