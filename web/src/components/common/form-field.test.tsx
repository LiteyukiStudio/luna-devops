import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { FormField } from './form-field'
import { ProjectSpaceMultiSelect, ProjectSpaceSelect } from './project-space-select'
import '@/i18n'

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

  it('forwards Slot id, aria-describedby, and aria-invalid through ProjectSpaceSelect and ProjectSpaceMultiSelect', () => {
    const projects = [{ description: 'demo', id: 'prj_1', identifier: 'demo', name: 'Demo Space' }]
    render(
      <TooltipProvider>
        <FormField error="Select a project" hint="Target space" label="Project" required>
          <ProjectSpaceSelect projects={projects} value="" onChange={vi.fn()} />
        </FormField>
        <FormField error="Select at least one" label="Scoped projects">
          <ProjectSpaceMultiSelect projects={projects} value={[]} onChange={vi.fn()} />
        </FormField>
      </TooltipProvider>,
    )

    const single = screen.getByRole('button', { name: 'Project' })
    expect(single).toHaveAttribute('aria-invalid', 'true')
    expect(single).toHaveAccessibleDescription('Target space Select a project')
    expect(single.className).toContain('w-full')

    const multi = screen.getByRole('button', { name: 'Scoped projects' })
    expect(multi).toHaveAttribute('aria-invalid', 'true')
    expect(multi).toHaveAccessibleDescription('Select at least one')
  })
})
