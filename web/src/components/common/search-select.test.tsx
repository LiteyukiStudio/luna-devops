import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { ProjectSpaceSelect } from './project-space-select'
import { SearchMultiSelect, SearchSelect } from './search-select'

describe('search select loading and empty states', () => {
  it('shows loading text on the trigger and inside the popover instead of empty options while loading', async () => {
    const user = userEvent.setup()
    const { rerender } = render(
      <SearchSelect
        ariaLabel="Cluster"
        emptyLabel="No clusters"
        loading
        options={[]}
        placeholder="Select cluster"
        value=""
        onValueChange={vi.fn()}
      />,
    )

    const trigger = screen.getByRole('button', { name: 'Cluster' })
    expect(trigger).toHaveTextContent(i18next.t('common.loading'))
    expect(trigger).not.toHaveTextContent('Select cluster')

    await user.click(trigger)
    expect(screen.getByRole('status')).toHaveTextContent(i18next.t('common.loading'))
    expect(screen.queryByText('No clusters')).not.toBeInTheDocument()

    rerender(
      <SearchSelect
        ariaLabel="Cluster"
        emptyLabel="No clusters"
        loading={false}
        options={[]}
        placeholder="Select cluster"
        value=""
        onValueChange={vi.fn()}
      />,
    )

    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(screen.getByText('No clusters')).toBeInTheDocument()
  })

  it('shows loading text on SearchMultiSelect when selected values are awaiting option hydration', () => {
    render(
      <SearchMultiSelect
        ariaLabel="Projects"
        loading
        options={[]}
        placeholder="Select projects"
        value={['prj_1']}
        onValueChange={vi.fn()}
      />,
    )

    const trigger = screen.getByRole('button', { name: 'Projects' })
    expect(trigger).toHaveTextContent(i18next.t('common.loading'))
    expect(trigger).not.toHaveTextContent('Select projects')
  })

  it('forwards loading to ProjectSpaceSelect so pending project queries do not flash emptyTitle', async () => {
    const user = userEvent.setup()
    render(
      <ProjectSpaceSelect
        loading
        projects={[]}
        value=""
        onChange={vi.fn()}
      />,
    )

    const trigger = screen.getByRole('button', { name: i18next.t('projectSpaces.selectProject') })
    expect(trigger).toHaveTextContent(i18next.t('common.loading'))

    await user.click(trigger)
    expect(screen.getByRole('status')).toHaveTextContent(i18next.t('common.loading'))
    expect(screen.queryByText(i18next.t('projectSpaces.emptyTitle'))).not.toBeInTheDocument()
  })

  it('does not prune selected values outside current remote search results or when error is true', async () => {
    const user = userEvent.setup()
    const onValueChange = vi.fn()
    const { rerender } = render(
      <SearchMultiSelect
        ariaLabel="Targets"
        filterLocally={false}
        options={[{ label: 'Backend API', value: 'dplt_api' }]}
        placeholder="Select targets"
        pruneUnknownValues
        search="api"
        value={['dplt_web']}
        onSearchChange={vi.fn()}
        onValueChange={onValueChange}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'Targets' }))
    await user.click(screen.getByRole('button', { name: 'Backend API' }))
    expect(onValueChange).toHaveBeenCalledWith(['dplt_web', 'dplt_api'])

    rerender(
      <SearchMultiSelect
        ariaLabel="Targets"
        error
        options={[]}
        placeholder="Select targets"
        pruneUnknownValues
        value={['dplt_web']}
        onValueChange={onValueChange}
      />,
    )

    expect(screen.getByRole('button', { name: 'Targets' })).toHaveTextContent(i18next.t('common.unavailable'))
    expect(screen.getByRole('alert')).toHaveTextContent(i18next.t('common.unavailable'))
    expect(screen.queryByRole('button', { name: i18next.t('common.clearSelection') })).not.toBeInTheDocument()
  })
})
