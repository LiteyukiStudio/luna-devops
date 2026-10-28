import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { FormField } from './form-field'

describe('form field', () => {
  it('labels a single control and describes it with hint and error text', () => {
    render(
      <TooltipProvider>
        <FormField error="Required" hint="Shown in the console" label="Name" required>
          <input />
        </FormField>
      </TooltipProvider>,
    )

    const input = screen.getByRole('textbox', { name: 'Name' })
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(input).toHaveAccessibleDescription('Shown in the console Required')
    expect(screen.getByRole('alert')).toHaveTextContent('Required')
  })

  it('lets composite controls place the association on their real input', () => {
    render(
      <FormField label="Memory">
        {control => (
          <div>
            <input {...control} />
            <select aria-label="Unit" />
          </div>
        )}
      </FormField>,
    )

    expect(screen.getByRole('textbox', { name: 'Memory' })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Unit' })).toBeInTheDocument()
  })
})
