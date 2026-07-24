import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DataList } from './data-list'

describe('data list layout', () => {
  it('keeps intrinsic column width so a narrow container can scroll horizontally', () => {
    render(
      <DataList
        columns={[
          { key: 'name', header: 'Name', minWidth: 320, render: item => item.name },
          { key: 'description', header: 'Description', minWidth: 480, render: item => item.description },
        ]}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One', description: 'Description' }]}
        rowKey={item => item.id}
      />,
    )

    const table = screen.getByRole('table')
    expect(table).toHaveClass('w-max', 'min-w-full', 'bg-transparent')
    expect(screen.getAllByRole('rowgroup')[1]).toHaveClass('bg-card')
    expect(table.closest('[data-scrollbars="both"]')).toHaveAttribute('data-scroll-area-type', 'auto')
    expect(table.closest('[data-slot="table-frame"]')).toHaveClass(
      'mt-group',
      'rounded-container',
      'border',
      'border-border',
      'overflow-hidden',
      'bg-card',
    )
    expect(table.closest('[data-slot="table-frame-clip"]')).not.toBeInTheDocument()
  })

  it('uses content width even when a page supplies a legacy fixed width', () => {
    render(
      <DataList
        columns={[
          { key: 'name', header: 'Name', render: item => item.name },
          {
            key: 'actions',
            header: 'Actions',
            className: 'w-64 min-w-64 px-4 text-right',
            render: () => <button type="button">...</button>,
          },
        ]}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        rowKey={item => item.id}
      />,
    )

    const header = screen.getByRole('columnheader', { name: 'Actions' })
    expect(header).toHaveClass('w-px', 'min-w-0', 'px-2', 'sm:px-4')
    expect(header).not.toHaveClass('w-64', 'min-w-64')
  })

  it('keeps explicitly sticky action columns fixed on the right', () => {
    render(
      <DataList
        columns={[
          { key: 'name', header: 'Name', render: item => item.name },
          {
            key: 'actions',
            header: 'Actions',
            sticky: 'right',
            render: () => <button type="button">...</button>,
          },
        ]}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        rowKey={item => item.id}
      />,
    )

    expect(screen.getByRole('columnheader', { name: 'Actions' })).toHaveClass('sticky', 'right-0')
    expect(screen.getByRole('cell', { name: '...' })).toHaveClass('sticky', 'right-0', 'border-l-0')
    expect(screen.getByRole('cell', { name: '...' })).not.toHaveClass('border-separator-strong')
  })

  it('renders query controls in the list header toolbar', () => {
    render(
      <DataList
        columns={[{
          key: 'name',
          header: 'Name',
          className: 'px-4 py-3 align-middle',
          render: item => item.name,
        }]}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        rowKey={item => item.id}
        title="Projects"
        toolbar={<button type="button">Sort projects</button>}
      />,
    )

    const toolbarButton = screen.getByRole('button', { name: 'Sort projects' })
    expect(screen.getByText('Projects').parentElement?.parentElement).toContainElement(toolbarButton)
  })

  it('uses a clean white header surface and left-aligns query controls without a repeated title', () => {
    render(
      <DataList
        columns={[{ key: 'name', header: 'Name', render: item => item.name }]}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        rowKey={item => item.id}
        search={{ value: '', placeholder: 'Search projects', onChange: () => undefined }}
        toolbar={<button type="button">Sort projects</button>}
      />,
    )

    expect(screen.getAllByRole('rowgroup')[0]).toHaveClass('[background:var(--data-list-header-surface)]')
    expect(screen.getAllByRole('rowgroup')[0]).not.toHaveClass('border-separator-strong')
    expect(screen.getByRole('row', { name: 'One' })).toHaveClass(
      'border-border',
      'border-t',
      'hover:[&>td]:[background:var(--data-list-row-hover)]',
    )
    expect(screen.getByRole('row', { name: 'One' })).not.toHaveClass(
      'hover:border-surface-subtle',
      '[&>td:first-child]:rounded-l-container',
      '[&>td:last-child]:rounded-r-container',
    )
    expect(screen.getByRole('button', { name: 'Sort projects' }).closest('[data-slot="data-list-tools"]')).toHaveClass('pb-4')
    expect(screen.getByRole('button', { name: 'Sort projects' }).closest('[data-slot="data-list-tools"]')?.className).not.toContain('after:')
    expect(screen.getByRole('table').closest('[data-slot="table-frame"]')).toHaveClass(
      'w-full',
      'rounded-container',
      'border',
      'border-border',
    )
    expect(screen.getByRole('table').closest('[data-slot="table-frame"]')).not.toHaveClass('mt-group')
    expect(screen.getByRole('table').closest('[data-slot="scroll-area"]')).not.toHaveClass(
      'rounded-container',
      'border',
      'after:ring-1',
    )
    const search = screen.getByPlaceholderText('Search projects')
    expect(search.parentElement).not.toHaveClass('sm:justify-end')
    expect(search.parentElement).toContainElement(screen.getByRole('button', { name: 'Sort projects' }))
    expect(screen.getByRole('table').closest('[data-slot="card"]')).not.toBeInTheDocument()
    expect(screen.getByRole('table').closest('[data-slot="data-list"]')).toBeInTheDocument()
  })

  it('uses the table frame as the only boundary below the toolbar', () => {
    render(
      <DataList
        columns={[{ key: 'name', header: 'Name', render: item => item.name }]}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        pagination={{
          page: 1,
          pageInfoLabel: '1 item',
          pageSize: 10,
          total: 1,
          totalPages: 1,
          onPageChange: () => undefined,
        }}
        rowKey={item => item.id}
        title="Projects"
      />,
    )

    const titleBar = screen.getByText('Projects').parentElement?.parentElement
    const pagination = screen.getByText('1 item').closest<HTMLElement>('[data-slot="table-frame-footer"]')
    expect(titleBar?.className).not.toContain('after:')
    expect(pagination).toHaveClass('border-t', 'border-border', 'bg-card')
    expect(screen.getByRole('table').closest('[data-slot="table-frame"]')).toHaveClass(
      'rounded-container',
      'border',
      'border-border',
    )
    expect(screen.getByRole('table').closest('[data-slot="table-frame"]')).toContainElement(pagination)
    expect(screen.getByRole('table').closest('[data-slot="card"]')).not.toBeInTheDocument()
    expect(screen.getByRole('row', { name: 'One' })).toHaveClass('border-t', 'border-border')
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('pagination.pageSizeAria')).not.toBeInTheDocument()
  })

  it('uses compact mobile cells and keeps responsive controls for multiple pages', () => {
    render(
      <DataList
        columns={[{ key: 'name', header: 'Name', render: item => item.name }]}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        pagination={{
          page: 1,
          pageInfoLabel: 'Page 1',
          pageSize: 10,
          total: 20,
          totalPages: 2,
          onPageChange: () => undefined,
          onPageSizeChange: () => undefined,
        }}
        rowKey={item => item.id}
      />,
    )

    expect(screen.getByRole('cell', { name: 'One' })).toHaveClass('px-3', 'sm:px-4')
    expect(screen.getByRole('cell', { name: 'One' })).not.toHaveClass('px-4')
    expect(screen.getByRole('navigation').parentElement).toHaveClass(
      'w-full',
      'justify-between',
      'sm:w-auto',
      'sm:justify-center',
    )
  })

  it('uses the same top border for the header-to-row and row-to-row separators', () => {
    render(
      <DataList
        columns={[{ key: 'name', header: 'Name', render: item => item.name }]}
        emptyTitle="Empty"
        items={[
          { id: 'one', name: 'One' },
          { id: 'two', name: 'Two' },
        ]}
        rowKey={item => item.id}
      />,
    )

    const rows = screen.getAllByRole('row').slice(1)
    expect(rows).toHaveLength(2)
    for (const row of rows)
      expect(row).toHaveClass('border-t', 'border-border')
  })

  it('does not render pagination controls for an empty result', () => {
    render(
      <DataList
        columns={[{ key: 'name', header: 'Name', render: item => item.name }]}
        emptyTitle="Empty"
        items={[] as { id: string, name: string }[]}
        pagination={{
          page: 1,
          pageInfoLabel: '0 items',
          pageSize: 10,
          total: 0,
          totalPages: 0,
          onPageChange: () => undefined,
        }}
        rowKey={item => item.id}
      />,
    )

    expect(screen.queryByText('0 items')).not.toBeInTheDocument()
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
    expect(screen.getByText('Empty').closest('[data-slot="table-frame"]')).toHaveClass('border-0', 'bg-transparent')
    expect(screen.getByText('Empty').closest('[data-slot="table-frame"]')).not.toHaveClass(
      'rounded-container',
      'border',
      'border-border',
    )
  })

  it('renders a structured loading state instead of the empty state', () => {
    render(
      <DataList
        columns={[{ key: 'name', header: 'Name', render: item => item.name }]}
        emptyTitle="Empty"
        items={[] as { id: string, name: string }[]}
        loading
        rowKey={item => item.id}
      />,
    )

    expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true')
    expect(screen.getByRole('status').closest('[data-slot="table-frame"]')).toHaveClass(
      'rounded-container',
      'border',
      'border-border',
    )
    expect(screen.queryByText('Empty')).not.toBeInTheDocument()
  })

  it('can hide secondary columns on mobile while keeping action columns intact', () => {
    render(
      <DataList
        columns={[
          { key: 'name', header: 'Name', render: item => item.name },
          { key: 'detail', header: 'Detail', mobile: 'hidden', render: item => item.detail },
          { key: 'actions', header: 'Actions', sticky: 'right', render: () => <button type="button">Open</button> },
        ]}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One', detail: 'Secondary' }]}
        rowKey={item => item.id}
      />,
    )

    expect(screen.getByRole('columnheader', { name: 'Detail' })).toHaveClass('hidden', 'md:table-cell')
    expect(screen.getByRole('columnheader', { name: 'Actions' })).not.toHaveClass('hidden')
  })

  it('collapses action groups into one overflow trigger on mobile', () => {
    const originalMatchMedia = window.matchMedia
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: (query: string) => ({
        matches: query === '(max-width: 47.999rem)',
        media: query,
        onchange: null,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })

    const view = render(
      <DataList
        columns={[
          { key: 'name', header: 'Name', render: item => item.name },
          {
            key: 'actions',
            header: 'Actions',
            sticky: 'right',
            render: () => (
              <div>
                <button type="button">Edit</button>
                <button type="button">Delete</button>
              </div>
            ),
          },
        ]}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        rowKey={item => item.id}
      />,
    )

    expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Actions' }))
    expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument()

    view.unmount()
    Object.defineProperty(window, 'matchMedia', { configurable: true, value: originalMatchMedia })
  })

  it('keeps an existing responsive action menu inline on mobile', () => {
    const originalMatchMedia = window.matchMedia
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: (query: string) => ({
        matches: query === '(max-width: 47.999rem)',
        media: query,
        onchange: null,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })

    const view = render(
      <DataList
        columns={[
          { key: 'name', header: 'Name', render: item => item.name },
          {
            key: 'actions',
            header: 'Actions',
            mobileActions: 'inline',
            render: () => <button type="button">Existing menu</button>,
          },
        ]}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        rowKey={item => item.id}
      />,
    )

    expect(screen.getByRole('button', { name: 'Existing menu' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()

    view.unmount()
    Object.defineProperty(window, 'matchMedia', { configurable: true, value: originalMatchMedia })
  })

  it('renders filtered empty results as a compact centered state with a clear action', () => {
    render(
      <DataList
        columns={[{ key: 'name', header: 'Name', render: item => item.name }]}
        emptyActions={<button type="button">Clear filters</button>}
        emptyMode="filtered"
        emptyTitle="No matching results"
        items={[] as { id: string, name: string }[]}
        rowKey={item => item.id}
      />,
    )

    expect(screen.getByText('No matching results').closest('[data-slot="empty"]')).toHaveClass('min-h-24', 'items-center')
    expect(screen.getByRole('button', { name: 'Clear filters' })).toBeInTheDocument()
  })
})
