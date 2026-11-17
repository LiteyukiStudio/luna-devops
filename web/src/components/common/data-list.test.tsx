import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
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

    expect(screen.getByRole('textbox', { name: 'Search projects' })).toBeVisible()
    fireEvent.change(screen.getByPlaceholderText('Search projects'), { target: { value: 'demo' } })
    expect(onSearch).toHaveBeenCalledWith('demo')
    expect(screen.getByRole('button', { name: 'Sort projects' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Create project' })).toBeVisible()
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

  it('shows the error slot with retry instead of the empty state or stale rows', async () => {
    await i18next.changeLanguage('en-US')
    const retry = vi.fn()
    render(
      <DataList
        columns={columns}
        emptyTitle="Nothing here"
        error={{ description: 'Try again later', onRetry: retry, title: 'Failed to load' }}
        items={[{ id: 'one', name: 'Stale row' }]}
        pagination={{ page: 1, pageInfoLabel: 'Page 1', pageSize: 10, total: 1, totalPages: 1, onPageChange: vi.fn() }}
        rowKey={item => item.id}
      />,
    )

    expect(screen.getByRole('alert')).toHaveTextContent('Failed to load')
    expect(screen.queryByText('Nothing here')).not.toBeInTheDocument()
    expect(screen.queryByText('Stale row')).not.toBeInTheDocument()
    expect(screen.queryByText('Page 1')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(retry).toHaveBeenCalledOnce()
  })

  it('keeps the loading skeleton ahead of the error slot', () => {
    render(
      <DataList
        columns={columns}
        emptyTitle="Nothing here"
        error={{ title: 'Failed to load' }}
        items={[]}
        loading
        rowKey={item => item.id}
      />,
    )

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByText('Nothing here')).not.toBeInTheDocument()
  })

  it('expands naturally without viewport max-height clipping when constrainedHeight is false', () => {
    const { container } = render(
      <DataList
        columns={columns}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        rowKey={item => item.id}
        viewportOffset={24}
      />,
    )

    const root = container.querySelector('[data-slot="data-list"]')
    const tableFrame = container.querySelector('[data-slot="table-frame"]')
    const scrollArea = container.querySelector('[data-slot="scroll-area"]')

    expect(root).not.toBeNull()
    expect(root?.className).not.toMatch(/md:max-h-\[calc\(100d?vh-/)
    expect(root?.className).not.toMatch(/md:h-\[calc\(100d?vh-/)
    expect(tableFrame).not.toHaveClass('flex-1')
    expect(scrollArea).not.toHaveClass('h-full')
    expect(scrollArea).toHaveAttribute('data-scrollbars', 'horizontal')
  })

  it('applies definite viewport height and internal bidirectional scroll area when constrainedHeight is true', () => {
    const { container } = render(
      <DataList
        columns={columns}
        constrainedHeight
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        rowKey={item => item.id}
        viewportOffset={24}
      />,
    )

    const root = container.querySelector('[data-slot="data-list"]')
    const tableFrame = container.querySelector('[data-slot="table-frame"]')
    const scrollArea = container.querySelector('[data-slot="scroll-area"]')

    expect(root).toHaveClass('md:h-[calc(100dvh-24rem)]')
    expect(root?.className).not.toMatch(/md:max-h-\[calc\(100d?vh-/)
    expect(tableFrame).toHaveClass('min-h-0', 'flex-1')
    expect(scrollArea).toHaveClass('h-full')
    expect(scrollArea).toHaveAttribute('data-scrollbars', 'both')
  })

  it('enforces 32px button height in toolbar and collapses empty null-rendering permission toolbar slots', () => {
    function HiddenPermissionControl() {
      return null
    }
    const { container } = render(
      <DataList
        columns={columns}
        emptyTitle="Empty"
        items={[{ id: 'one', name: 'One' }]}
        rowKey={item => item.id}
        toolbar={<HiddenPermissionControl />}
      />,
    )

    const tools = container.querySelector('[data-slot="data-list-tools"]')
    const controls = container.querySelector('[data-slot="data-list-tools-controls"]')
    expect(tools?.className).toContain('[&_[data-slot=button]]:h-8')
    expect(tools?.className).toContain('has-[>[data-slot=data-list-tools-controls]:only-child:empty]:hidden')
    expect(controls).toBeEmptyDOMElement()
    expect(controls).toHaveClass('empty:hidden')
  })
})
