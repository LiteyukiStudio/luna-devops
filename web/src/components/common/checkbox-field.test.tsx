import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { Controller, useForm } from 'react-hook-form'
import { describe, expect, it, vi } from 'vitest'
import { ControlledCheckboxField } from './checkbox-field'

function BooleanCheckboxForm({ onSubmit }: { onSubmit: (values: { enabled: boolean }) => void }) {
  const form = useForm({ defaultValues: { enabled: false } })

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      <Controller
        control={form.control}
        name="enabled"
        render={({ field }) => (
          <ControlledCheckboxField description="Controls availability" field={field}>
            Enabled
          </ControlledCheckboxField>
        )}
      />
      <button type="submit">Save</button>
    </form>
  )
}

describe('checkbox field', () => {
  it('binds the UI checkbox to a React Hook Form boolean', async () => {
    const onSubmit = vi.fn()
    render(<BooleanCheckboxForm onSubmit={onSubmit} />)

    const checkbox = screen.getByRole('checkbox', { name: /Enabled/ })
    expect(checkbox).toHaveAttribute('aria-describedby')
    expect(checkbox).toHaveAttribute('aria-checked', 'false')

    fireEvent.click(checkbox)
    expect(checkbox).toHaveAttribute('aria-checked', 'true')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith({ enabled: true }, expect.anything()))
  })
})
