import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './table'

describe('table layout', () => {
  it('preserves intrinsic width inside its horizontal scroll container', () => {
    render(
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Description</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell>Example</TableCell>
            <TableCell>Details</TableCell>
          </TableRow>
        </TableBody>
      </Table>,
    )

    const table = screen.getByRole('table')
    expect(table).toHaveClass('w-max', 'min-w-full', 'bg-transparent')
    expect(table.closest('[data-slot="scroll-area"]')).toHaveAttribute('data-scroll-area-type', 'auto')
    expect(table.closest('[data-slot="scroll-area"]')).toHaveAttribute('data-scrollbars', 'horizontal')
    expect(table.closest('[data-slot="table-container"]')).toHaveClass(
      'rounded-container',
      'border',
      'border-border',
      'overflow-hidden',
      'bg-card',
    )
    expect(table.closest('[data-slot="table-frame-clip"]')).not.toBeInTheDocument()
    expect(table.closest('[data-slot="scroll-area"]')).not.toHaveClass(
      'rounded-container',
      'border',
      'after:ring-1',
    )
    expect(screen.getAllByRole('rowgroup')[0]).toHaveClass('[background:var(--data-list-header-surface)]')
    expect(screen.getAllByRole('rowgroup')[1]).toHaveClass('bg-card')
    expect(screen.getAllByRole('row')[0]).toHaveClass('border-b', 'border-border')
    expect(screen.getAllByRole('row')[1]).toHaveClass(
      'border-b',
      'border-border',
      'hover:[background:var(--data-list-row-hover)]',
    )
  })
})
