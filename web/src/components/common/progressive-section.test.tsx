import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ProgressiveSection } from './progressive-section'

describe('progressive section', () => {
  it('keeps guidance in a focusable tooltip and toggles the section body', async () => {
    const { container } = render(
      <TooltipProvider>
        <ProgressiveSection
          description="Configure runtime resources"
          hint="Runtime configuration guidance"
          storageKey="test.progressive.runtime"
          summary="1 replica · 1 CPU"
          title="Runtime configuration"
        >
          <div>Runtime fields</div>
        </ProgressiveSection>
      </TooltipProvider>,
    )

    expect(screen.queryByText('Runtime fields')).not.toBeInTheDocument()
    expect(screen.queryByText('Configure runtime resources')).not.toBeInTheDocument()
    expect(screen.getByText('1 replica · 1 CPU')).toBeVisible()
    expect(screen.queryByText('Runtime configuration guidance')).not.toBeInTheDocument()

    const hintTrigger = container.querySelector<HTMLElement>('[data-slot="tooltip-trigger"]')
    expect(hintTrigger).not.toBeNull()
    expect(screen.getByText('Runtime configuration').parentElement).toContainElement(hintTrigger)
    fireEvent.focus(hintTrigger!)
    const hintContents = await screen.findAllByText('Runtime configuration guidance')
    expect(hintContents[0]).toBeVisible()

    const trigger = screen.getByRole('button', { expanded: false })
    fireEvent.click(trigger)
    expect(screen.getByText('Runtime fields')).toBeVisible()
    expect(screen.getByText('Configure runtime resources')).toBeVisible()
    expect(trigger).toHaveAttribute('aria-controls')
    expect(document.getElementById(trigger.getAttribute('aria-controls')!)).toHaveAttribute('data-slot', 'collapsible-content')
    expect(localStorage.getItem('test.progressive.runtime')).toBe('true')
  })

  it('restores the persisted open state and stores subsequent changes', () => {
    localStorage.setItem('test.progressive.persisted', 'true')

    render(
      <ProgressiveSection storageKey="test.progressive.persisted" title="Advanced settings">
        <div>Advanced fields</div>
      </ProgressiveSection>,
    )

    const trigger = screen.getByRole('button', { expanded: true })
    expect(screen.getByText('Advanced fields')).toBeVisible()

    fireEvent.click(trigger)
    expect(screen.queryByText('Advanced fields')).not.toBeInTheDocument()
    expect(localStorage.getItem('test.progressive.persisted')).toBe('false')
  })
})
