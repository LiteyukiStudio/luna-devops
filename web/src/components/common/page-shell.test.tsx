import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PageShell } from './page-shell'

describe('page shell layout', () => {
  it('creates a shrinkable single-column content track', () => {
    render(<PageShell>Content</PageShell>)

    expect(screen.getByText('Content')).toHaveClass(
      'grid',
      'w-full',
      'min-w-0',
      'grid-cols-[minmax(0,1fr)]',
    )
  })
})
