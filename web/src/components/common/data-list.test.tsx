import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DataList } from './data-list'

const originalMatchMedia = window.matchMedia

afterEach(() => {
  Object.defineProperty(window, 'matchMedia', { configurable: true, value: originalMatchMedia })
})

function setMobileViewport() {
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
}

const columns = [{ key: 'name', header: 'Name', render: (item: { id: string, name: string }) => item.name }]

describe('data list behavior', () => {
  it('opens an actionable row by pointer or keyboard without stealing nested actions', () => {
    const onRowClick = vi.fn()
    const onNestedClick = vi.fn()
    render(
      <DataList
        columns={[
          ...columns,
          { key: 'actions', header: 'Actions', render: () => <button type="button" onClick={onNestedClick}>Inspect</button> },
        ]}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        rowActionLabel={() => 'Open One'}
        rowKey={item => item.id}
        onRowClick={onRowClick}
      />,
    )

    const row = screen.getByLabelText('Open One')
    fireEvent.click(row)
    fireEvent.keyDown(row, { key: 'Enter' })
    expect(onRowClick).toHaveBeenCalledTimes(2)

    fireEvent.click(screen.getByRole('button', { name: 'Inspect' }))
    expect(onNestedClick).toHaveBeenCalledOnce()
    expect(onRowClick).toHaveBeenCalledTimes(2)
  })

  it('renders query controls and reports search input', () => {
    const onSearch = vi.fn()
    render(
      <DataList
        columns={columns}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        rowKey={item => item.id}
        search={{ value: '', placeholder: 'Search projects', onChange: onSearch }}
        toolbar={<button type="button">Sort projects</button>}
        toolbarActions={<button type="button">Create project</button>}
      />,
    )

    fireEvent.change(screen.getByPlaceholderText('Search projects'), { target: { value: 'demo' } })
    expect(onSearch).toHaveBeenCalledWith('demo')
    expect(screen.getByRole('button', { name: 'Sort projects' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Create project' })).toBeVisible()
  })

  it('keeps an explicitly sticky action column available while scrolling', () => {
    render(
      <DataList
        columns={[
          ...columns,
          { key: 'actions', header: 'Actions', sticky: 'right', render: () => <button type="button">Inspect</button> },
        ]}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        rowKey={item => item.id}
      />,
    )

    expect(screen.getByRole('columnheader', { name: 'Actions' })).toHaveClass('sticky', 'right-0')
    expect(screen.getByRole('button', { name: 'Inspect' })).toBeVisible()
  })

  it('reports row and select-all selection changes and exposes partial selection', () => {
    const onSelectionChange = vi.fn()
    const { rerender } = render(
      <DataList
        columns={columns}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }, { id: 'two', name: 'Two' }]}
        rowKey={item => item.id}
        selection={{
          selectedKeys: [],
          selectAllLabel: 'Select all',
          selectRowLabel: item => `Select ${item.name}`,
          selectedLabel: 'Selected',
          onSelectionChange,
        }}
      />,
    )

    fireEvent.click(screen.getByLabelText('Select One'))
    expect(onSelectionChange).toHaveBeenLastCalledWith(['one'])
    fireEvent.click(screen.getByLabelText('Select all'))
    expect(onSelectionChange).toHaveBeenLastCalledWith(['one', 'two'])

    rerender(
      <DataList
        columns={columns}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }, { id: 'two', name: 'Two' }]}
        rowKey={item => item.id}
        selection={{
          selectedKeys: ['one'],
          selectAllLabel: 'Select all',
          selectRowLabel: item => `Select ${item.name}`,
          selectedLabel: 'Selected',
          onSelectionChange,
        }}
      />,
    )
    expect(screen.getByLabelText('Select all')).toHaveAttribute('aria-checked', 'mixed')
  })

  it('shows pagination only for non-empty multi-page results and reports navigation', () => {
    const onPageChange = vi.fn()
    const { rerender } = render(
      <DataList
        columns={columns}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        pagination={{ page: 1, pageInfoLabel: 'Page 1', pageSize: 10, total: 20, totalPages: 2, onPageChange }}
        rowKey={item => item.id}
      />,
    )

    fireEvent.click(screen.getByLabelText('Go to page 2'))
    expect(onPageChange).toHaveBeenCalledWith(2)

    rerender(
      <DataList
        columns={columns}
        emptyTitle="Empty"
        items={[]}
        pagination={{ page: 1, pageInfoLabel: '0 items', pageSize: 10, total: 0, totalPages: 0, onPageChange }}
        rowKey={item => item.id}
      />,
    )
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
    expect(screen.queryByText('0 items')).not.toBeInTheDocument()
  })

  it('renders a structured loading state instead of the empty state', () => {
    render(<DataList columns={columns} emptyTitle="Empty" items={[]} loading rowKey={item => item.id} />)

    expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true')
    expect(screen.queryByText('Empty')).not.toBeInTheDocument()
  })

  it('collapses action groups into one mobile overflow trigger', () => {
    setMobileViewport()
    render(
      <DataList
        columns={[
          ...columns,
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
    expect(screen.getByRole('button', { name: 'Edit' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Delete' })).toBeVisible()
  })

  it('keeps an existing responsive action menu inline on mobile', () => {
    setMobileViewport()
    render(
      <DataList
        columns={[
          ...columns,
          { key: 'actions', header: 'Actions', mobileActions: 'inline', render: () => <button type="button">Existing menu</button> },
        ]}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        rowKey={item => item.id}
      />,
    )

    expect(screen.getByRole('button', { name: 'Existing menu' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
  })

  it('renders filtered empty results with a clear action', () => {
    const clearFilters = vi.fn()
    render(
      <DataList
        columns={columns}
        emptyActions={<button type="button" onClick={clearFilters}>Clear filters</button>}
        emptyMode="filtered"
        emptyTitle="No matching results"
        items={[]}
        rowKey={item => item.id}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(clearFilters).toHaveBeenCalledOnce()
    expect(screen.getByText('No matching results')).toBeVisible()
  })
})
