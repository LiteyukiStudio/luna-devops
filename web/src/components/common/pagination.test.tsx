import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { PaginationController } from './pagination'

describe('pagination controller', () => {
  it('does not render controls when the normalized page total is zero', () => {
    const { container } = render(<PaginationController initialPage={1} total={0} />)

    expect(container).toBeEmptyDOMElement()
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
  })

  it('does not report the current page merely because controls mounted', () => {
    const onPageChange = vi.fn()
    render(<PaginationController initialPage={2} pageSize={10} total={30} onPageChange={onPageChange} />)

    expect(onPageChange).not.toHaveBeenCalled()
  })

  it('reports only an out-of-range controlled page adjustment', () => {
    const onPageChange = vi.fn()
    render(<PaginationController initialPage={9} pageSize={10} total={21} onPageChange={onPageChange} />)

    expect(onPageChange).toHaveBeenCalledOnce()
    expect(onPageChange).toHaveBeenCalledWith(3)
  })
})
