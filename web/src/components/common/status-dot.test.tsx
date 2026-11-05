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
    expect(status?.querySelector('[data-slot="status-dot-indicator"]')).toHaveClass('bg-success')
  })

  it('maps every tone to a semantic token color', () => {
    const { container, rerender } = render(<StatusDot label="Failed" tone="danger" />)
    const indicator = () => container.querySelector('[data-slot="status-dot-indicator"]')

    expect(indicator()).toHaveClass('bg-danger')
    rerender(<StatusDot label="Pending" tone="warning" />)
    expect(indicator()).toHaveClass('bg-warning')
    rerender(<StatusDot label="Info" tone="info" />)
    expect(indicator()).toHaveClass('bg-info')
    rerender(<StatusDot label="Stopped" tone="neutral" />)
    expect(indicator()).toHaveClass('bg-muted-foreground')
  })

  it('omits empty detail text', () => {
    render(<StatusDot detail="" label="Unknown" tone="neutral" />)

    expect(screen.getByText('Unknown').nextElementSibling).toBeNull()
  })
})
