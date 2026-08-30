import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DescriptionListItem } from './description-list-item'

describe('description list item', () => {
  it('renders accessible description-list semantics for text and rich values', () => {
    render(
      <dl>
        <DescriptionListItem label="Status" value={<strong>Ready</strong>} />
      </dl>,
    )

    expect(screen.getByText('Status').tagName).toBe('DT')
    expect(screen.getByText('Ready').closest('dd')).toHaveAttribute('data-slot', 'description-list-value')
  })

  it('preserves layout, mono, danger and explicit empty-value variants', () => {
    render(
      <dl>
        <DescriptionListItem
          danger
          className="sm:col-span-2"
          emptyFallback="—"
          label="Span ID"
          mono
          value=""
        />
      </dl>,
    )

    const value = screen.getByText('—')
    expect(value.tagName).toBe('DD')
    expect(value).toHaveClass('font-mono', 'text-xs', 'text-danger')
    expect(value.parentElement).toHaveClass('sm:col-span-2')
  })

  it('does not treat zero as an empty value', () => {
    render(
      <dl>
        <DescriptionListItem emptyFallback="—" label="Errors" value={0} />
      </dl>,
    )

    expect(screen.getByText('0')).toBeInTheDocument()
    expect(screen.queryByText('—')).not.toBeInTheDocument()
  })
})
