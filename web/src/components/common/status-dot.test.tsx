import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { StatusDot } from './status-dot'

describe('status dot', () => {
  it('states the status in text and keeps the dot decorative', () => {
    render(<StatusDot detail="2/2" label="Running" tone="success" />)

    const status = screen.getByText('Running').closest('[data-slot="status-dot"]')
    expect(status).toHaveAttribute('data-tone', 'success')
    expect(screen.getByText('2/2')).toBeVisible()
    expect(status?.querySelector('[data-slot="status-dot-indicator"]')).toHaveAttribute('aria-hidden', 'true')
    expect(status?.querySelector('[data-slot="status-dot-indicator"]')).toHaveClass('bg-success-solid')
  })

  it('maps every tone to a semantic token color', () => {
    const { container, rerender } = render(<StatusDot label="Failed" tone="danger" />)
    const indicator = () => container.querySelector('[data-slot="status-dot-indicator"]')

    expect(indicator()).toHaveClass('bg-danger-solid')
    rerender(<StatusDot label="Pending" tone="warning" />)
    expect(indicator()).toHaveClass('bg-warning-solid')
    rerender(<StatusDot label="Info" tone="info" />)
    expect(indicator()).toHaveClass('bg-info-solid')
    rerender(<StatusDot label="Stopped" tone="neutral" />)
    expect(indicator()).toHaveClass('bg-muted-foreground')
  })

  it('omits empty detail text', () => {
    render(<StatusDot detail="" label="Unknown" tone="neutral" />)

    expect(screen.getByText('Unknown').nextElementSibling).toBeNull()
  })

  it('adds a reduced-motion-aware breathing ring only for in-progress states', () => {
    const { container, rerender } = render(<StatusDot label="Building" pulse tone="warning" />)
    const ring = () => container.querySelector('[data-slot=status-dot-indicator] > span')

    expect(container.querySelector('[data-slot=status-dot]')).toHaveAttribute('data-pulse', 'true')
    expect(ring()).toHaveClass('motion-safe:animate-status-pulse', 'motion-reduce:hidden', 'bg-warning-solid')

    rerender(<StatusDot label="Ready" tone="success" />)
    expect(container.querySelector('[data-slot=status-dot]')).not.toHaveAttribute('data-pulse')
    expect(ring()).toBeNull()
  })
})
