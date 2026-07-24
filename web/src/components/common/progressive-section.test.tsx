import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ProgressiveSection } from './progressive-section'

describe('progressive section', () => {
  it('keeps guidance in a focusable tooltip and toggles the section body', async () => {
    const { container } = render(
      <TooltipProvider>
        <ProgressiveSection
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
    expect(screen.queryByText('Runtime configuration guidance')).not.toBeInTheDocument()

    const hintTrigger = container.querySelector<HTMLElement>('[data-slot="tooltip-trigger"]')
    expect(hintTrigger).not.toBeNull()
    fireEvent.focus(hintTrigger!)
    const hintContents = await screen.findAllByText('Runtime configuration guidance')
    expect(hintContents[0]).toBeVisible()

    fireEvent.click(screen.getByRole('button', { expanded: false }))
    expect(screen.getByText('Runtime fields')).toBeVisible()
    expect(localStorage.getItem('test.progressive.runtime')).toBe('true')
  })
})
